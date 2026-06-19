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
# performs a source build from GitHub by default (or from a mirror when
# BHP_REPO_URL is set). It never touches tunnel runtime behavior.

set -Eeuo pipefail

# --------------------------------------------------------------------------- #
# Constants / paths
# --------------------------------------------------------------------------- #
# Repository / branch can be overridden via environment variables. This is
# useful for installing from a mirror when GitHub access is blocked or slow:
#   BHP_REPO_URL="https://mirror.example.com/codeTide/BackhaulPlus.git" \
#   BHP_REPO_BRANCH="main" bash install.sh
REPO_URL="${BHP_REPO_URL:-https://github.com/codeTide/BackhaulPlus.git}"
REPO_BRANCH="${BHP_REPO_BRANCH:-main}"

# Offline source mode: install/update from a local source directory or archive
# instead of contacting GitHub. Set exactly one of:
#   BHP_SOURCE_DIR=/path/to/extracted/source-dir
#   BHP_SOURCE_ARCHIVE=/path/to/source.tar.gz|.tgz|.zip
# In this mode the installer never runs git clone/fetch/pull and git is not
# required; Go is still required because the daemon is built from source.
BHP_SOURCE_DIR="${BHP_SOURCE_DIR:-}"
BHP_SOURCE_ARCHIVE="${BHP_SOURCE_ARCHIVE:-}"

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

# --------------------------------------------------------------------------- #
# Git authentication helpers (private repository support)
# --------------------------------------------------------------------------- #
# Allow git network operations (clone/fetch/pull) to authenticate against a
# private GitHub repository using a token, without printing the token, writing
# it to logs, or persisting it in the remote URL / .git/config.
#
# Token sources, in order:
#   1. BHP_GITHUB_TOKEN environment variable
#   2. BHP_GIT_TOKEN environment variable (alias)
#   3. Interactive (no-echo) prompt, only when a git operation fails on a TTY.
#
# Note: fetching install.sh itself from a private repo still needs an
# authenticated download (e.g. a tokenized curl) before running this script.

# get_git_token_from_env: print the token from the environment on stdout, or
# nothing. Does not prompt.
get_git_token_from_env() {
	if [[ -n "${BHP_GITHUB_TOKEN:-}" ]]; then
		printf '%s' "$BHP_GITHUB_TOKEN"
		return
	fi
	if [[ -n "${BHP_GIT_TOKEN:-}" ]]; then
		printf '%s' "$BHP_GIT_TOKEN"
		return
	fi
	printf ''
}

# get_git_token_interactive: prompt the user for a token without echoing input.
# Prints the token on stdout (or nothing if declined / not a TTY).
get_git_token_interactive() {
	local token=""

	if [[ -t 0 ]]; then
		if ask_yes_no "Git authentication may be required. Enter a GitHub token now?" "n"; then
			read -r -s -p "GitHub token: " token
			printf '\n' >&2
			printf '%s' "$token"
		fi
	fi
}

# run_git_with_token: run a git command authenticating with the given token via
# a temporary GIT_ASKPASS helper. The token is passed only through the
# environment for the single invocation; the askpass script is always removed
# afterwards, even on failure.
run_git_with_token() {
	local token="$1"
	shift

	local askpass rc
	askpass="$(mktemp)"
	chmod 700 "$askpass"
	cat > "$askpass" <<'EOF'
#!/usr/bin/env sh
case "$1" in
  *Username*) echo "x-access-token" ;;
  *Password*) echo "$BHP_EFFECTIVE_GIT_TOKEN" ;;
  *) echo "" ;;
esac
EOF

	if BHP_EFFECTIVE_GIT_TOKEN="$token" \
	   GIT_ASKPASS="$askpass" \
	   GIT_TERMINAL_PROMPT=0 \
	   "$@"; then
		rc=0
	else
		rc=$?
	fi

	rm -f "$askpass"
	return "$rc"
}

# git_run: run a git network command. Use ONLY for operations that may hit the
# network (clone/fetch/pull); never for local-only commands.
#
# Order of attempts:
#   1. If a token is set in the environment, use it on the first attempt.
#   2. Otherwise run once unauthenticated with GIT_TERMINAL_PROMPT=0 so git
#      never opens its own credential prompt.
#   3. If that fails, offer our controlled no-echo token prompt and retry.
git_run() {
	local token
	token="$(get_git_token_from_env)"

	# If a token was provided through the environment, use it on the first try.
	if [[ -n "$token" ]]; then
		run_git_with_token "$token" "$@"
		return $?
	fi

	# No token provided. Prevent git from opening its own credential prompt.
	if GIT_TERMINAL_PROMPT=0 "$@"; then
		return 0
	fi

	local rc=$?
	warn "Git command failed. Authentication or network access may be required."

	token="$(get_git_token_interactive)"
	if [[ -n "$token" ]]; then
		info "Retrying git command with token authentication..."
		run_git_with_token "$token" "$@"
		return $?
	fi

	return "$rc"
}

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

	# In offline source mode we never touch GitHub, so git/curl are not needed.
	# Only the extraction tool for the chosen archive type is required (plus Go,
	# checked below for all modes).
	if offline_source_requested; then
		command -v go >/dev/null 2>&1 || die "go is required to build from source. See https://go.dev/dl/"
		if [[ -n "${BHP_SOURCE_ARCHIVE:-}" ]]; then
			case "$(offline_archive_kind "$BHP_SOURCE_ARCHIVE")" in
				tar) command -v tar   >/dev/null 2>&1 || die "tar is required to extract ${BHP_SOURCE_ARCHIVE}." ;;
				zip) command -v unzip >/dev/null 2>&1 || die "unzip is required to extract ${BHP_SOURCE_ARCHIVE}." ;;
				*)   die "Unsupported archive type: ${BHP_SOURCE_ARCHIVE} (expected .tar.gz, .tgz, or .zip)" ;;
			esac
		fi
		ok "Offline source mode: required tools are present (GitHub access not needed)."
		return
	fi

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

# Track old services detected/disabled during migration so we can optionally
# offer to remove their unit files later, only after the new service is healthy.
LEGACY_SERVICES_FOUND=()
LEGACY_SERVICES_DISABLED=()
# Set to 1 when a legacy install was detected and migration ran.
LEGACY_MIGRATED=0

migrate_legacy() {
	# Migrate a legacy manual install at /root/BackhaulPlus if present.
	if [[ -d "$LEGACY_DIR" ]]; then
		LEGACY_MIGRATED=1
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
		info "Legacy folder was left untouched for now. After the new service is confirmed running, the installer may offer optional cleanup with backup."
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
			LEGACY_SERVICES_FOUND+=("$svc")
			warn "Found old service: ${svc}"
			if ask_yes_no "Disable old service ${svc}?" "y"; then
				systemctl disable --now "$svc" >/dev/null 2>&1 || warn "Could not disable ${svc} (it may already be inactive)."
				LEGACY_SERVICES_DISABLED+=("$svc")
				ok "Disabled ${svc}. Its unit file was left in place."
			fi
		fi
	done
}

# --------------------------------------------------------------------------- #
# Optional legacy cleanup (only after the new service is confirmed running)
# --------------------------------------------------------------------------- #
# All cleanup here is opt-in, defaults to No, always backs up before deleting,
# and never removes config backups.

backup_legacy_folder() {
	install -d -m 0750 "$BACKUP_DIR"
	local dest="${BACKUP_DIR}/legacy-root-BackhaulPlus-$(timestamp).tar.gz"
	if tar -czf "$dest" -C /root BackhaulPlus; then
		ok "Legacy folder backed up to ${dest}"
		return 0
	fi
	err "Failed to back up ${LEGACY_DIR}; not removing it."
	return 1
}

cleanup_legacy_folder() {
	if [[ -d "$LEGACY_DIR" ]]; then
		warn "Legacy folder ${LEGACY_DIR} is no longer used by the new service."
		if ask_yes_no "Back up and remove ${LEGACY_DIR} now?" "n"; then
			if backup_legacy_folder; then
				rm -rf "$LEGACY_DIR"
				ok "Removed ${LEGACY_DIR}"
			fi
		else
			info "Left ${LEGACY_DIR} in place."
		fi
	fi
}

cleanup_legacy_service_units() {
	# Offer to remove the unit files of old services we disabled. Each removal is
	# confirmed individually, backs up the unit first, and never touches the new
	# service unit.
	local svc
	for svc in "${LEGACY_SERVICES_DISABLED[@]}"; do
		[[ "$svc" == "$SERVICE_NAME" ]] && continue
		local unit_path="/etc/systemd/system/${svc}"
		[[ -f "$unit_path" ]] || continue
		warn "Old service unit ${unit_path} is no longer used."
		if ask_yes_no "Back up and remove ${unit_path} now?" "n"; then
			install -d -m 0750 "$BACKUP_DIR"
			local dest="${BACKUP_DIR}/legacy-unit-${svc}-$(timestamp)"
			if cp -a "$unit_path" "$dest"; then
				ok "Backed up ${unit_path} to ${dest}"
				rm -f "$unit_path"
				systemctl daemon-reload
				ok "Removed ${unit_path}"
			else
				err "Failed to back up ${unit_path}; not removing it."
			fi
		else
			info "Left ${unit_path} in place."
		fi
	done
}

# offer_legacy_cleanup: entry point called from main() after the new service
# has been confirmed running. Defaults are always No.
offer_legacy_cleanup() {
	if [[ "$LEGACY_MIGRATED" -ne 1 && ${#LEGACY_SERVICES_DISABLED[@]} -eq 0 ]]; then
		return
	fi
	if ! systemctl is-active --quiet "$SERVICE_NAME"; then
		info "New service is not active yet; skipping optional legacy cleanup."
		info "Re-run the installer after the service is running to clean up legacy files."
		return
	fi
	printf '\n'
	info "The new service is running. Optional cleanup of legacy files is available."
	cleanup_legacy_folder
	cleanup_legacy_service_units
}

# --------------------------------------------------------------------------- #
# Offline source mode
# --------------------------------------------------------------------------- #
# offline_source_requested: true when the user asked to install/update from a
# local source directory or archive instead of GitHub.
offline_source_requested() {
	[[ -n "${BHP_SOURCE_DIR:-}" || -n "${BHP_SOURCE_ARCHIVE:-}" ]]
}

# offline_archive_kind: classify the archive path passed via BHP_SOURCE_ARCHIVE.
# Echoes "tar" for .tar.gz/.tgz, "zip" for .zip, or "unknown".
offline_archive_kind() {
	local f="$1"
	case "$f" in
		*.tar.gz|*.tgz) printf 'tar' ;;
		*.zip)          printf 'zip' ;;
		*)              printf 'unknown' ;;
	esac
}

# validate_local_source: ensure a directory looks like a BackhaulPlus checkout.
validate_local_source() {
	local dir="$1"

	[[ -d "$dir" ]] || die "Offline source directory not found: $dir"
	[[ -f "$dir/go.mod" ]] || die "Invalid offline source: missing go.mod"
	[[ -f "$dir/scripts/bhp" ]] || die "Invalid offline source: missing scripts/bhp"

	if [[ ! -f "$dir/packaging/backhaulplus.service" ]]; then
		# Not fatal: the installer has a built-in fallback unit template.
		warn "Offline source is missing packaging/backhaulplus.service; using built-in unit template."
	fi

	if ! grep -q 'module github.com/codeTide/BackhaulPlus' "$dir/go.mod"; then
		warn "go.mod module does not look like github.com/codeTide/BackhaulPlus"
		if ! ask_yes_no "Continue anyway?" "n"; then
			die "Offline source rejected."
		fi
	fi
}

# find_source_root: given a directory that an archive was extracted into, locate
# the actual source root (the directory that contains go.mod). Echoes the path.
find_source_root() {
	local base="$1"

	if [[ -f "$base/go.mod" ]]; then
		printf '%s' "$base"
		return 0
	fi

	# Exactly one child directory that contains go.mod (common for GitHub
	# tarballs that wrap everything in <repo>-<ref>/).
	local -a children=()
	local c
	for c in "$base"/*/; do
		[[ -d "$c" ]] && children+=("${c%/}")
	done
	if [[ ${#children[@]} -eq 1 && -f "${children[0]}/go.mod" ]]; then
		printf '%s' "${children[0]}"
		return 0
	fi

	# Fall back to searching one level deep for a go.mod.
	local d
	for d in "$base"/*/; do
		[[ -f "${d}go.mod" ]] && { printf '%s' "${d%/}"; return 0; }
	done

	return 1
}

# prepare_offline_source: resolve the source directory to install from. Echoes
# the prepared source directory path on stdout. For archives, extracts into a
# temporary directory (left for the OS to reclaim) and returns the source root.
prepare_offline_source() {
	if [[ -n "${BHP_SOURCE_DIR:-}" ]]; then
		validate_local_source "$BHP_SOURCE_DIR" >&2
		printf '%s' "$BHP_SOURCE_DIR"
		return 0
	fi

	local archive="$BHP_SOURCE_ARCHIVE"
	[[ -f "$archive" ]] || die "Offline source archive not found: $archive"

	local kind; kind="$(offline_archive_kind "$archive")"
	local tmp; tmp="$(mktemp -d /tmp/backhaulplus-offline.XXXXXX)"

	case "$kind" in
		tar)
			command -v tar >/dev/null 2>&1 || die "tar is required to extract ${archive}."
			info "Extracting ${archive}..." >&2
			tar -xzf "$archive" -C "$tmp" || die "Failed to extract ${archive}."
			;;
		zip)
			command -v unzip >/dev/null 2>&1 || die "unzip is required to extract ${archive}."
			info "Extracting ${archive}..." >&2
			unzip -q "$archive" -d "$tmp" || die "Failed to extract ${archive}."
			;;
		*)
			die "Unsupported archive type: ${archive} (expected .tar.gz, .tgz, or .zip)"
			;;
	esac

	local root
	if ! root="$(find_source_root "$tmp")"; then
		die "Could not find a source root containing go.mod inside ${archive}."
	fi
	validate_local_source "$root" >&2
	printf '%s' "$root"
}

# backup_existing_source: archive the current source checkout (if any) before it
# is replaced by an offline source.
backup_existing_source() {
	if [[ -d "$SRC_DIR" && -n "$(ls -A "$SRC_DIR" 2>/dev/null)" ]]; then
		install -d -m 0750 "$BACKUP_DIR"
		local dest="${BACKUP_DIR}/source-$(timestamp).tar.gz"
		tar -czf "$dest" -C "$STATE_DIR" "$(basename "$SRC_DIR")"
		ok "Backed up current source checkout to ${dest}"
	fi
}

# install_offline_source_to_src_dir: copy a validated local source into SRC_DIR
# atomically. Builds the replacement in SRC_DIR.new and only swaps after a
# successful copy, so a partial failure never destroys the current source.
install_offline_source_to_src_dir() {
	local source_root="$1"

	validate_local_source "$source_root"
	backup_existing_source

	rm -rf "${SRC_DIR}.new"
	mkdir -p "${SRC_DIR}.new"

	# Copy all contents, including dotfiles if present.
	if ! ( cd "$source_root" && tar -cf - . ) | ( cd "${SRC_DIR}.new" && tar -xf - ); then
		rm -rf "${SRC_DIR}.new"
		die "Failed to copy offline source into ${SRC_DIR}.new"
	fi

	rm -rf "$SRC_DIR"
	mv "${SRC_DIR}.new" "$SRC_DIR"

	# Metadata for version display when .git is unavailable.
	{
		printf 'offline_source: %s\n' "$source_root"
		printf 'installed_at: %s\n' "$(date -Is)"
	} > "${SRC_DIR}/.bhp-source"

	ok "Offline source installed to ${SRC_DIR}"
}

# source_tree_valid: true when SRC_DIR looks like a usable BackhaulPlus source
# tree, regardless of whether it is a git checkout. Used to safely migrate a
# non-git offline source tree to a git checkout on online installer re-runs.
source_tree_valid() {
	[[ -d "$SRC_DIR" ]] || return 1
	[[ -f "${SRC_DIR}/go.mod" ]] || return 1
	[[ -f "${SRC_DIR}/scripts/bhp" ]] || return 1
	return 0
}

# --------------------------------------------------------------------------- #
# Offline source cleanup (optional, opt-in, defaults to No)
# --------------------------------------------------------------------------- #
# After a successful offline install the installed source lives under SRC_DIR,
# so the user-provided archive/extracted directory is no longer required. These
# helpers optionally remove that temporary input with strict safety checks so
# they can never delete the installed source, system paths, or the current
# working directory.

# canonical_path: resolve a path to its absolute, symlink-free form. Echoes the
# resolved path on stdout, or returns non-zero if it cannot be resolved.
canonical_path() {
	local p="$1"
	if command -v readlink >/dev/null 2>&1; then
		readlink -f "$p" 2>/dev/null || return 1
	else
		# fallback: best effort
		(cd "$(dirname "$p")" 2>/dev/null && printf '%s/%s\n' "$(pwd -P)" "$(basename "$p")") || return 1
	fi
}

# is_current_or_parent_of_cwd: true when target IS the current working directory
# or a parent of it.
is_current_or_parent_of_cwd() {
	local target="$1"
	local cwd
	cwd="$(pwd -P)"

	[[ "$cwd" == "$target" ]] && return 0
	case "$cwd" in
		"$target"/*) return 0 ;;
	esac
	return 1
}

# is_dangerous_cleanup_path: true when target must never be removed (system
# roots, the managed state/backup/config dirs, or the installed source tree).
is_dangerous_cleanup_path() {
	local target="$1"
	local src_canon state_canon backup_canon config_canon

	src_canon="$(canonical_path "$SRC_DIR" 2>/dev/null || true)"
	state_canon="$(canonical_path "$STATE_DIR" 2>/dev/null || true)"
	backup_canon="$(canonical_path "$BACKUP_DIR" 2>/dev/null || true)"
	config_canon="$(canonical_path "$CONFIG_DIR" 2>/dev/null || true)"

	case "$target" in
		""|"/"|"/root"|"/home"|"/tmp"|"/var"|"/var/lib"|"/etc") return 0 ;;
	esac

	[[ -n "$src_canon" && "$target" == "$src_canon" ]] && return 0
	[[ -n "$src_canon" && "$target" == "$src_canon"/* ]] && return 0
	[[ -n "$state_canon" && "$target" == "$state_canon" ]] && return 0
	[[ -n "$backup_canon" && "$target" == "$backup_canon" ]] && return 0
	[[ -n "$config_canon" && "$target" == "$config_canon" ]] && return 0

	return 1
}

# maybe_cleanup_offline_input: offer to remove the user-provided offline input
# (archive or extracted source directory). Default is always No. Refuses unsafe
# paths and only removes recognized archive types / validated source trees.
maybe_cleanup_offline_input() {
	local input="$1"
	local kind="$2" # archive or dir

	[[ -n "$input" ]] || return 0

	local target
	if ! target="$(canonical_path "$input")"; then
		warn "Could not resolve offline source path for cleanup: $input"
		return 0
	fi

	if is_dangerous_cleanup_path "$target"; then
		warn "Not removing unsafe cleanup path: $target"
		return 0
	fi

	case "$kind" in
		archive)
			[[ -f "$target" ]] || { warn "Offline archive not found for cleanup: $target"; return 0; }
			case "$target" in
				*.tar.gz|*.tgz|*.zip) ;;
				*) warn "Not removing unsupported archive type: $target"; return 0 ;;
			esac

			if ask_yes_no "Remove offline source archive ${target}?" "n"; then
				rm -f "$target"
				ok "Removed offline source archive: ${target}"
			else
				info "Kept offline source archive: ${target}"
			fi
			;;
		dir)
			[[ -d "$target" ]] || { warn "Offline source directory not found for cleanup: $target"; return 0; }

			# Validate again before even offering rm -rf.
			if [[ ! -f "$target/go.mod" || ! -f "$target/scripts/bhp" ]]; then
				warn "Not removing directory that does not look like BackhaulPlus source: $target"
				return 0
			fi

			if is_current_or_parent_of_cwd "$target"; then
				warn "Not removing source directory because the current shell is inside it: $target"
				info "To remove it later, run from another directory:"
				info "cd /root && sudo rm -rf '$target'"
				return 0
			fi

			if ask_yes_no "Remove offline source directory ${target}?" "n"; then
				rm -rf "$target"
				ok "Removed offline source directory: ${target}"
			else
				info "Kept offline source directory: ${target}"
			fi
			;;
	esac
}

# maybe_cleanup_offline_input_main: dispatch cleanup for the user-provided
# offline input after a successful offline install. Only runs for offline mode.
maybe_cleanup_offline_input_main() {
	offline_source_requested || return 0
	if [[ -n "${BHP_SOURCE_ARCHIVE:-}" ]]; then
		maybe_cleanup_offline_input "$BHP_SOURCE_ARCHIVE" "archive"
	elif [[ -n "${BHP_SOURCE_DIR:-}" ]]; then
		maybe_cleanup_offline_input "$BHP_SOURCE_DIR" "dir"
	fi
}

# --------------------------------------------------------------------------- #
# Source checkout + build
# --------------------------------------------------------------------------- #
# ensure_origin_url: point an existing checkout's origin at the configured
# REPO_URL so BHP_REPO_URL overrides apply to existing clones (mirror support),
# not just fresh clones.
ensure_origin_url() {
	if [[ -d "${SRC_DIR}/.git" ]]; then
		if git -C "$SRC_DIR" remote get-url origin >/dev/null 2>&1; then
			git -C "$SRC_DIR" remote set-url origin "$REPO_URL"
		else
			git -C "$SRC_DIR" remote add origin "$REPO_URL"
		fi
	fi
}

# clone_online_source_to_new_dir: clone the configured repo into a fresh
# temporary directory next to SRC_DIR. Echoes the new directory on success; the
# current SRC_DIR is never touched. Removes the temp dir on failure.
clone_online_source_to_new_dir() {
	local new="${SRC_DIR}.git-new"

	rm -rf "$new"
	info "Cloning ${REPO_URL} (${REPO_BRANCH}) into ${new}..." >&2
	if git_run git clone --branch "$REPO_BRANCH" "$REPO_URL" "$new" >&2; then
		printf '%s' "$new"
		return 0
	fi

	rm -rf "$new"
	return 1
}

# replace_src_dir_with_prepared_checkout: swap a freshly prepared git checkout
# into SRC_DIR. Backs up the current source first and only removes the old tree
# after the swap succeeds; restores it if the final move fails.
replace_src_dir_with_prepared_checkout() {
	local new="$1"

	[[ -d "$new/.git" ]] || { err "Prepared checkout is missing .git: $new"; return 1; }

	backup_existing_source

	local old="${SRC_DIR}.old.$(timestamp)"
	rm -rf "$old"

	if [[ -d "$SRC_DIR" ]]; then
		mv "$SRC_DIR" "$old" || return 1
	fi

	if mv "$new" "$SRC_DIR"; then
		rm -rf "$old"
		ok "Installed online source checkout at ${SRC_DIR}"
		return 0
	fi

	# Roll back source dir if the final move failed.
	if [[ -d "$old" ]]; then
		mv "$old" "$SRC_DIR" || true
	fi

	err "Failed to install online source checkout; restored previous source tree."
	return 1
}

update_source() {
	if [[ -d "${SRC_DIR}/.git" ]]; then
		info "Updating source checkout in ${SRC_DIR}..."
		ensure_origin_url
		# Explicit refspec so origin/<branch> is reliably refreshed.
		git_run git -C "$SRC_DIR" fetch --prune origin \
			"+refs/heads/${REPO_BRANCH}:refs/remotes/origin/${REPO_BRANCH}"
		git -C "$SRC_DIR" checkout "$REPO_BRANCH"
		git_run git -C "$SRC_DIR" pull --ff-only origin "$REPO_BRANCH"
		ok "Source checkout ready."
		return 0
	fi

	# A valid non-git source tree (typical of a previous offline install) cannot
	# be updated with git. Migrate it to a real git checkout, cloning first and
	# only swapping after the clone succeeds so the offline source is never lost.
	if source_tree_valid; then
		info "Current installed source is a valid non-git offline source."
		info "Online install needs to replace it with a git checkout (a backup is created first)."
		if ! ask_yes_no "Switch ${SRC_DIR} to an online git checkout now?" "y"; then
			die "Online install cancelled; offline source left untouched."
		fi
		local new
		if ! new="$(clone_online_source_to_new_dir)"; then
			die "Clone failed; offline source left untouched."
		fi
		replace_src_dir_with_prepared_checkout "$new" || die "Could not install online source checkout."
		ensure_origin_url
		ok "Source checkout ready."
		return 0
	fi

	# Missing or empty source dir: safe to clone fresh.
	if [[ ! -d "$SRC_DIR" || -z "$(ls -A "$SRC_DIR" 2>/dev/null)" ]]; then
		info "Cloning ${REPO_URL} into ${SRC_DIR}..."
		rmdir "$SRC_DIR" 2>/dev/null || true
		git_run git clone --branch "$REPO_BRANCH" "$REPO_URL" "$SRC_DIR"
		ok "Source checkout ready."
		return 0
	fi

	# Non-empty but unrecognized source tree: it may hold unknown user files, so
	# default to No before replacing it.
	warn "Source directory ${SRC_DIR} is neither a git checkout nor a valid BackhaulPlus source tree."
	info "Online install would replace it with a git checkout (a backup is created first)."
	if ! ask_yes_no "Replace ${SRC_DIR} with an online git checkout now?" "n"; then
		die "Online install cancelled; ${SRC_DIR} left untouched."
	fi
	local new
	if ! new="$(clone_online_source_to_new_dir)"; then
		die "Clone failed; ${SRC_DIR} left untouched."
	fi
	replace_src_dir_with_prepared_checkout "$new" || die "Could not install online source checkout."
	ensure_origin_url
	ok "Source checkout ready."
}

# build_binary_from: build the daemon from a given source directory into a temp
# file and echo its path on success (non-zero on failure). Lets offline mode
# build from the prepared source BEFORE replacing the installed checkout.
build_binary_from() {
	local dir="$1"
	local tmpdir
	tmpdir="$(mktemp -d /tmp/backhaulplus-build.XXXXXX)"
	local out="${tmpdir}/backhaulplus"

	info "Building daemon (this can take a moment)..." >&2
	# Repository root is the main package.
	( cd "$dir" && go build -o "$out" . ) || {
		rm -rf "$tmpdir"
		return 1
	}
	printf '%s\n' "$out"
}

build_binary() {
	# Build from the installed source checkout.
	build_binary_from "$SRC_DIR"
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

	if [[ -n "${BHP_SOURCE_DIR:-}" && -n "${BHP_SOURCE_ARCHIVE:-}" ]]; then
		die "Set only one of BHP_SOURCE_DIR or BHP_SOURCE_ARCHIVE, not both."
	fi

	if offline_source_requested; then
		info "Mode:       offline source install (GitHub not contacted)"
		[[ -n "${BHP_SOURCE_DIR:-}" ]]     && info "Source dir: ${BHP_SOURCE_DIR}"
		[[ -n "${BHP_SOURCE_ARCHIVE:-}" ]] && info "Archive:    ${BHP_SOURCE_ARCHIVE}"
	else
		info "Repository: ${REPO_URL}"
		info "Branch:     ${REPO_BRANCH}"
	fi

	require_root
	detect_os
	check_requirements
	create_dirs
	migrate_legacy

	local built=""
	if offline_source_requested; then
		# Offline mode: build from the prepared source BEFORE replacing the
		# installed checkout, so a failed build leaves source/binary/service
		# untouched. Only swap SRC_DIR in after a successful build.
		local source_root
		source_root="$(prepare_offline_source)"
		if ! built="$(build_binary_from "$source_root")"; then
			die "Build failed. Existing source, binary, and service were left untouched."
		fi
		install_offline_source_to_src_dir "$source_root"
	else
		update_source
		if ! built="$(build_binary)"; then
			die "Build failed. The existing installation (if any) was left untouched."
		fi
	fi

	backup_existing_binary
	install_binary "$built"
	rm -rf "$(dirname "$built")"

	install_manager
	install_service_unit
	finalize_config_and_service

	# Optional, opt-in cleanup of legacy files, only after the new service is up.
	offer_legacy_cleanup

	# Optional, opt-in cleanup of the user-provided offline source input. The
	# installed source now lives under SRC_DIR, so the temporary archive /
	# extracted directory is no longer required. Default is No. Only reached
	# after a successful offline install (failures exit earlier via die).
	maybe_cleanup_offline_input_main

	printf '\n'
	ok "Done. Manage BackhaulPlus with: ${C_BOLD}sudo bhp${C_RESET}"
}

main "$@"
