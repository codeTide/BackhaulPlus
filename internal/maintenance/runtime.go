package maintenance

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/codeTide/BackhaulPlus/internal/config"

	"github.com/sirupsen/logrus"
)

// StartRuntimeMaintenance starts optional process-wide runtime maintenance
// tasks. It returns immediately and only starts goroutines for enabled options.
func StartRuntimeMaintenance(cfg config.RuntimeConfig, logger *logrus.Logger) {
	if cfg.MemoryReleaseIntervalDuration <= 0 && cfg.AutoRestartIntervalDuration <= 0 {
		return
	}

	if cfg.MemoryReleaseIntervalDuration > 0 {
		logger.Infof("runtime maintenance: memory release enabled; interval=%s", cfg.MemoryReleaseIntervalDuration)
		go runMemoryRelease(cfg.MemoryReleaseIntervalDuration, logger)
	}

	if cfg.AutoRestartIntervalDuration > 0 {
		logger.Infof("runtime maintenance: auto restart enabled; interval=%s", cfg.AutoRestartIntervalDuration)
		go runAutoRestart(cfg.AutoRestartIntervalDuration, logger)
	}
}

func runMemoryRelease(interval time.Duration, logger *logrus.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		debug.FreeOSMemory()
		logger.Info("runtime maintenance: released idle memory")
	}
}

func runAutoRestart(interval time.Duration, logger *logrus.Logger) {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	<-timer.C
	logger.Info("runtime maintenance: auto restart interval reached; re-executing process")
	if err := reexecSelf(); err != nil {
		logger.Errorf("runtime maintenance: failed to re-exec process: %v", err)
		os.Exit(1)
	}
}
