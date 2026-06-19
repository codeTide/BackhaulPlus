#!/usr/bin/env bash
#
# BackhaulPlus bootstrap installer.
#
# Usage:
#   bash <(curl -Ls https://raw.githubusercontent.com/codeTide/BackhaulPlus/main/scripts/install.sh)
#
# or:
#   curl -Ls https://raw.githubusercontent.com/codeTide/BackhaulPlus/main/scripts/install.sh -o install.sh
#   sudo bash install.sh
#
# This script is idempotent: re-running it repairs/updates the installation.
# It installs the daemon, the interactive manager (bhp), a systemd unit, and
# performs a source build from GitHub. It never touches tunnel runtime behavior.

set -Eeuo pipefail

# --------------------------------------------------------------------------- #
# Constants / paths
# --------------------------------------------------------------------------- #
REPO_URL="https://github.com/codeTide/BackhaulPlus.git"
REPO_BRANCH="main"

BIN_DAEMON="/usr/local/bin/backhaulplus"
BIN_COMPAT="/usr/local/bin/BackhaulPlus"
BIN_MANAGER="/usr/local/bin/bhp"

CONFIG_DIR="/etc/backhaulplus"
CONFIG_FILE="${CONFIG_DIR}/config.toml"

STATE_DIR="/var/lib/backhaulplus"
SRC_DIR="${STATE_DIR}/src"

BACKUP_DIR="/var/backups/backhaulplus"

SERVICE_NAME="backhaulplus.service"
SERVICE_UNIT="/etc/systemd/system/${SERVICE_NAME}"

# --------------------------------------------------------------------------- #
# Colors / output helpers
# --------------------------------------------------------------------------- #
if [[ -t 1 ]] && command -v tput >/dev/null 2>&1 && [[ -n "$(tput colors 2>/dev/null || echo 0)" ]] && [[ "$(tput colors 2>/dev/null || echo 0)" -ge 8 ]]; then
	C_RESET="$(tput sgr0)"
	C_BOLD="$(tput bold)"
	C_RED="$(tput setaf 1)"
	C_GREEN="$(tput setaf 2)"
	C_YELLOW="$(tput setaf 3)"
	C_BLUE="$(tput setaf 4)"
	C_CYAN="$(tput setaf 6)"
else
	C_RESET=""; C_BOLD=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""
fi

info()  { printf '%s==>%s %s\n' "${C_BLUE}${C_BOLD}" "${C_RESET}" "$*"; }
ok()    { printf '%s[ ok ]%s %s\n' "${C_GREEN}" "${C_RESET}" "$*"; }
warn()  { printf '%s[warn]%s %s\n' "${C_YELLOW}" "${C_RESET}" "$*"; }
err()   { printf '%s[fail]%s %s\n' "${C_RED}${C_BOLD}" "${C_RESET}" "$*" >&2; }

die() { err "$*"; exit 1; }

# Prompt yes/no. Default yes unless second arg is "n".
# Honors non-interactive runs by using the default.
ask_yes_no() {
	local prompt="$1" default="${2:-y}" reply
	local hint="[Y/n]"
	[[ "$default" == "n" ]] && hint="[y/N]"
	if [[ ! -t 0 ]]; then
		# Non-interactive: fall back to default.
		[[ "$default" == "y" ]]
		return
	fi
	read -r -p "$(printf '%s %s ' "$prompt" "$hint")" reply || reply=""
	reply="${reply:-$default}"
	[[ "$reply" =~ ^[Yy]$ ]]
}

timestamp() { date +%Y%m%d-%H%M%S; }

trap 'err "Installation aborted (line $LINENO)."' ERR

# --------------------------------------------------------------------------- #
# Pre-flight checks
# --------------------------------------------------------------------------- #
require_root() {
	if [[ "$(id -u)" -ne 0 ]]; then
		die "Please run as root: sudo bash install.sh"
	fi
}

detect_os() {
	if [[ ! -r /etc/os-release ]]; then
		warn "Could not read /etc/os-release; assuming a generic systemd Linux."
		OS_ID="unknown"; OS_LIKE=""
		return
	fi
	# shellcheck disable=SC1091
	. /etc/os-release
	OS_ID="${ID:-unknown}"
	OS_LIKE="${ID_LIKE:-}"
	info "Detected OS: ${PRETTY_NAME:-$OS_ID}"
}

is_debian_like() {
	[[ "$OS_ID" == "debian" || "$OS_ID" == "ubuntu" ]] && return 0
	[[ "$OS_LIKE" == *debian* ]] && return 0
	return 1
}

apt_install() {
	local pkgs=("$@")
	if ! is_debian_like; then
		err "Automatic package install is only supported on Debian/Ubuntu."
		return 1
	fi
	if ask_yes_no "Install missing packages via apt-get: ${pkgs[*]}?" "y"; then
		info "Running apt-get update..."
		apt-get update -y
		info "Installing: ${pkgs[*]}"
		DEBIAN_FRONTEND=noninteractive apt-get install -y "${pkgs[@]}"
	else
		return 1
	fi
}

check_requirements() {
	command -v systemctl >/dev/null 2>&1 || die "systemctl not found. This installer requires a systemd-based system."

	local missing=()
	command -v git >/dev/null 2>&1 || missing+=("git")
	command -v go  >/dev/null 2>&1 || missing+=("golang-go")

	if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
		missing+=("curl")
	fi

	if [[ ${#missing[@]} -gt 0 ]]; then
		warn "Missing required tools: ${missing[*]}"
		if is_debian_like; then
			if ! apt_install "${missing[@]}"; then
				cat <<EOF
${C_RED}Required tools are missing.${C_RESET}

Please install them manually, for example:
    sudo apt-get update
    sudo apt-get install -y git golang-go curl

For an up-to-date Go toolchain you may prefer the official tarball:
    https://go.dev/dl/
EOF
				die "Cannot continue without required tools."
			fi
		else
			cat <<EOF
${C_RED}Required tools are missing: ${missing[*]}${C_RESET}

Please install git, the Go toolchain (https://go.dev/dl/), and curl or wget,
then re-run this installer.
EOF
			die "Cannot continue without required tools."
		fi
	fi

	# Re-verify the essentials after any install attempt.
	command -v git >/dev/null 2>&1 || die "git is still not available after install."
	command -v go  >/dev/null 2>&1 || die "go is still not available after install. See https://go.dev/dl/"

	ok "All required tools are present."
}

# --------------------------------------------------------------------------- #
# Directories
# --------------------------------------------------------------------------- #
create_dirs() {
	info "Creating directories..."
	install -d -m 0755 "$CONFIG_DIR"
	install -d -m 0755 "$STATE_DIR"
	install -d -m 0755 "$SRC_DIR"
	install -d -m 0750 "$BACKUP_DIR"
	ok "Directories ready."
}

# --------------------------------------------------------------------------- #
# Legacy migration
# --------------------------------------------------------------------------- #
LEGACY_DIR="/root/BackhaulPlus"
LEGACY_CONFIG="${LEGACY_DIR}/config.toml"

migrate_legacy() {
	# Migrate a legacy manual install at /root/BackhaulPlus if present.
	if [[ -d "$LEGACY_DIR" ]]; then
		info "Existing legacy BackhaulPlus install found at ${LEGACY_DIR}."
		if [[ -f "$LEGACY_CONFIG" ]]; then
			if ask_yes_no "Migrate config to ${CONFIG_FILE}?" "y"; then
				local legacy_backup="${BACKUP_DIR}/legacy-config-$(timestamp).toml"
				cp -a "$LEGACY_CONFIG" "$legacy_backup"
				ok "Backed up legacy config to ${legacy_backup}"

				if [[ ! -f "$CONFIG_FILE" ]]; then
					cp -a "$LEGACY_CONFIG" "$CONFIG_FILE"
					ok "Migrated legacy config to ${CONFIG_FILE}"
				else
					warn "${CONFIG_FILE} already exists."
					if ask_yes_no "Replace it with the legacy config?" "n"; then
						cp -a "$CONFIG_FILE" "${BACKUP_DIR}/config-$(timestamp).toml"
						cp -a "$LEGACY_CONFIG" "$CONFIG_FILE"
						ok "Replaced ${CONFIG_FILE} with legacy config (previous version backed up)."
					else
						info "Kept existing ${CONFIG_FILE}."
					fi
				fi
			fi
		else
			warn "No config.toml found inside ${LEGACY_DIR}."
		fi
		info "Legacy folder was left untouched. After verifying the new service, you may archive or remove it manually."
	fi

	migrate_legacy_services
}

migrate_legacy_services() {
	# Detect old systemd services that may point at the legacy install.
	local candidates=("backhaul.service" "BackhaulPlus.service" "backhaulplus.service")
	local svc
	for svc in "${candidates[@]}"; do
		[[ "$svc" == "$SERVICE_NAME" ]] && continue
		local unit_path="/etc/systemd/system/${svc}"
		local is_legacy=0
		if [[ -f "$unit_path" ]]; then
			if grep -qE '/root/BackhaulPlus/(BackhaulPlus|config\.toml)' "$unit_path" 2>/dev/null; then
				is_legacy=1
			else
				# Still a different service we did not create; flag it.
				is_legacy=1
			fi
		elif systemctl list-unit-files "$svc" >/dev/null 2>&1 && systemctl cat "$svc" >/dev/null 2>&1; then
			is_legacy=1
		fi

		if [[ "$is_legacy" -eq 1 ]]; then
			warn "Found old service: ${svc}"
			if ask_yes_no "Disable old service ${svc}?" "y"; then
				systemctl disable --now "$svc" >/dev/null 2>&1 || warn "Could not disable ${svc} (it may already be inactive)."
				ok "Disabled ${svc}. Its unit file was left in place."
			fi
		fi
	done
}

# --------------------------------------------------------------------------- #
# Source checkout + build
# --------------------------------------------------------------------------- #
update_source() {
	if [[ -d "${SRC_DIR}/.git" ]]; then
		info "Updating source checkout in ${SRC_DIR}..."
		git -C "$SRC_DIR" fetch --prune origin
		git -C "$SRC_DIR" checkout "$REPO_BRANCH"
		git -C "$SRC_DIR" pull --ff-only origin "$REPO_BRANCH"
	else
		info "Cloning ${REPO_URL} into ${SRC_DIR}..."
		# Clone into the (possibly empty) source dir.
		rmdir "$SRC_DIR" 2>/dev/null || true
		git clone --branch "$REPO_BRANCH" "$REPO_URL" "$SRC_DIR"
	fi
	ok "Source checkout ready."
}

build_binary() {
	# Builds the daemon into a temp file and echoes its path on success.
	local tmpdir
	tmpdir="$(mktemp -d /tmp/backhaulplus-build.XXXXXX)"
	local out="${tmpdir}/backhaulplus"

	info "Building daemon (this can take a moment)..." >&2
	# Repository root is the main package.
	( cd "$SRC_DIR" && go build -o "$out" . ) || {
		rm -rf "$tmpdir"
		return 1
	}
	printf '%s\n' "$out"
}

install_binary() {
	# Atomically install the freshly built binary and refresh the symlink.
	local built="$1"
	install -m 0755 "$built" "${BIN_DAEMON}.new"
	mv -f "${BIN_DAEMON}.new" "$BIN_DAEMON"
	ln -sf "$BIN_DAEMON" "$BIN_COMPAT"
	ok "Installed daemon to ${BIN_DAEMON} (symlink ${BIN_COMPAT})."
}

backup_existing_binary() {
	if [[ -f "$BIN_DAEMON" ]]; then
		local dest="${BACKUP_DIR}/backhaulplus-$(timestamp)"
		cp -a "$BIN_DAEMON" "$dest"
		ok "Backed up current binary to ${dest}"
	fi
}

# --------------------------------------------------------------------------- #
# Manager + systemd unit
# --------------------------------------------------------------------------- #
install_manager() {
	local src_manager="${SRC_DIR}/scripts/bhp"
	if [[ -f "$src_manager" ]]; then
		install -m 0755 "$src_manager" "$BIN_MANAGER"
		ok "Installed manager to ${BIN_MANAGER}"
	else
		warn "Manager script not found at ${src_manager}; skipping bhp install."
	fi
}

install_service_unit() {
	local src_unit="${SRC_DIR}/packaging/backhaulplus.service"
	if [[ -f "$src_unit" ]]; then
		install -m 0644 "$src_unit" "$SERVICE_UNIT"
	else
		warn "Packaged unit not found; writing built-in template."
		cat > "$SERVICE_UNIT" <<EOF
[Unit]
Description=BackhaulPlus tunnel service
Documentation=https://github.com/codeTide/BackhaulPlus
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_DAEMON} -c ${CONFIG_FILE}
WorkingDirectory=${CONFIG_DIR}
Restart=always
RestartSec=3
LimitNOFILE=1048576
KillSignal=SIGTERM
TimeoutStopSec=20

[Install]
WantedBy=multi-user.target
EOF
	fi
	systemctl daemon-reload
	ok "Installed systemd unit ${SERVICE_UNIT} and reloaded systemd."
}

# --------------------------------------------------------------------------- #
# Config / service enablement
# --------------------------------------------------------------------------- #
finalize_config_and_service() {
	if [[ ! -f "$CONFIG_FILE" ]]; then
		# Provide an example if the repo ships one.
		local example=""
		if [[ -f "${SRC_DIR}/config.toml.example" ]]; then
			example="${SRC_DIR}/config.toml.example"
		elif [[ -f "${SRC_DIR}/example.toml" ]]; then
			example="${SRC_DIR}/example.toml"
		fi
		if [[ -n "$example" ]]; then
			cp -a "$example" "${CONFIG_FILE}.example"
			info "Wrote example config to ${CONFIG_FILE}.example"
		fi
		warn "No config.toml found. Please create ${CONFIG_FILE} before starting the service."
		info "Installation finished. The service was NOT started (no config)."
		return
	fi

	if ask_yes_no "Enable and start ${SERVICE_NAME} now?" "y"; then
		systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
		systemctl restart "$SERVICE_NAME"
		sleep 1
		if systemctl is-active --quiet "$SERVICE_NAME"; then
			ok "${SERVICE_NAME} is running."
		else
			err "${SERVICE_NAME} failed to start. Check: journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
		fi
	else
		info "Service not started. Start it later with: sudo bhp"
	fi
}

# --------------------------------------------------------------------------- #
# Main
# --------------------------------------------------------------------------- #
main() {
	printf '%s\n' "${C_CYAN}${C_BOLD}BackhaulPlus installer${C_RESET}"
	require_root
	detect_os
	check_requirements
	create_dirs
	migrate_legacy
	update_source
	backup_existing_binary

	local built=""
	if built="$(build_binary)"; then
		install_binary "$built"
		rm -rf "$(dirname "$built")"
	else
		die "Build failed. The existing installation (if any) was left untouched."
	fi

	install_manager
	install_service_unit
	finalize_config_and_service

	printf '\n'
	ok "Done. Manage BackhaulPlus with: ${C_BOLD}sudo bhp${C_RESET}"
}

main "$@"
