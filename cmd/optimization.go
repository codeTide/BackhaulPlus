package cmd

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

type sysctlSetting struct {
	key   string
	value string
}

func (s sysctlSetting) argument() string {
	return s.key + "=" + s.value
}

var tcpTuningSettings = []sysctlSetting{
	{key: "net.ipv4.ip_local_port_range", value: "1024 65535"}, // Increase ephemeral ports
	{key: "net.ipv4.tcp_tw_reuse", value: "1"},                 // Reuse TIME_WAIT sockets
	{key: "net.ipv4.tcp_fin_timeout", value: "15"},             // Reduce TCP FIN timeout
	{key: "net.core.somaxconn", value: "4096"},                 // Increase max queue length of incoming connections
	{key: "net.ipv4.tcp_max_syn_backlog", value: "8192"},       // Increase SYN request backlog
	{key: "net.ipv4.tcp_window_scaling", value: "1"},           // Enable TCP window scaling
	{key: "net.ipv4.tcp_fastopen", value: "3"},                 // Enable TCP Fast Open
	{key: "net.ipv4.tcp_rmem", value: "16384 262144 1048576"},  // Maximum of 1MB of TCP read buffer memory
	{key: "net.ipv4.tcp_wmem", value: "16384 262144 1048576"},  // Maximum of 1MB TCP write buffer memory
	{key: "net.ipv4.tcp_notsent_lowat", value: "4096"},         // Limit unsent TCP data buffered in the kernel
	{key: "net.core.rmem_default", value: "262144"},            // Set default receive memory
	{key: "net.core.wmem_default", value: "262144"},
	{key: "net.core.wmem_max", value: "67108864"}, // 64MB: Maximum send buffer size allowed for user sockets
	{key: "net.core.rmem_max", value: "67108864"}, // 64MB: Maximum receive buffer size allowed for user sockets
}

// applyTCPTuning applies temporary TCP optimizations for Linux to handle massive connections
func ApplyTCPTuning() {
	if runtime.GOOS == "linux" {
		logger.Info("tcp tuning: applying Linux optimizations")

		for _, setting := range tcpTuningSettings {
			applySysctlSetting(setting)
		}

		// Set file descriptor limit programmatically
		var rLimit syscall.Rlimit
		err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		if err != nil {
			logger.Errorf("Error getting Rlimit: %v", err)
		} else {
			logger.Debugf("Current file descriptor limit: %d", rLimit.Cur)

			// Set the maximum and current file descriptor limits to 1048576
			rLimit.Max = 1048576
			rLimit.Cur = 1048576
			err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
			if err != nil {
				logger.Errorf("Error setting Rlimit: %v", err)
			} else {
				logger.Debugf("Successfully set file descriptor limit to: %d", rLimit.Cur)
			}
		}
	} else {
		logger.Info("tcp tuning: non-Linux system detected, skipping optimizations")
	}
}

func applySysctlSetting(setting sysctlSetting) {
	arg := setting.argument()
	if os.Geteuid() != 0 {
		logger.Warnf("tcp tuning: skipped %s; root privileges required", arg)
		return
	}

	output, err := exec.Command("sysctl", "-w", arg).CombinedOutput()
	trimmedOutput := strings.TrimSpace(string(output))
	if err != nil {
		if trimmedOutput != "" {
			logger.Warnf("tcp tuning: failed to set %s: %v: %s", arg, err, trimmedOutput)
			return
		}
		logger.Warnf("tcp tuning: failed to set %s: %v", arg, err)
		return
	}
	logger.Debugf("tcp tuning: applied %s", arg)
}
