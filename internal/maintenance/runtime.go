package maintenance

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/codeTide/BackhaulPlus/internal/config"

	"github.com/sirupsen/logrus"
)

const mib = 1024 * 1024

type memorySnapshot struct {
	HeapAlloc    uint64
	HeapIdle     uint64
	HeapReleased uint64
	HeapSys      uint64
	Sys          uint64
	RSS          uint64
	HasRSS       bool
}

// StartRuntimeMaintenance starts optional process-wide runtime maintenance
// tasks. It returns immediately and only starts goroutines for enabled options.
func StartRuntimeMaintenance(cfg config.RuntimeConfig, logger *logrus.Logger) {
	if cfg.MemoryReleaseIntervalDuration <= 0 && cfg.AutoRestartIntervalDuration <= 0 {
		return
	}

	if cfg.MemoryReleaseIntervalDuration > 0 {
		logger.Infof("runtime: memory release enabled; interval=%s", cfg.MemoryReleaseIntervalDuration)
		go runMemoryRelease(cfg.MemoryReleaseIntervalDuration, logger)
	}

	if cfg.AutoRestartIntervalDuration > 0 {
		logger.Infof("runtime: auto restart enabled; interval=%s", cfg.AutoRestartIntervalDuration)
		go runAutoRestart(cfg.AutoRestartIntervalDuration, logger)
	}
}

func runMemoryRelease(interval time.Duration, logger *logrus.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		before := snapshotMemory()
		debug.FreeOSMemory()
		after := snapshotMemory()
		logger.Infof("runtime: memory released; %s", formatMemoryReleaseSummary(before, after))
		logger.Debugf("runtime: memory details; %s", formatMemoryReleaseDetails(before, after))
	}
}

func snapshotMemory() memorySnapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	rss, hasRSS := readRSS()
	return memorySnapshot{
		HeapAlloc:    stats.HeapAlloc,
		HeapIdle:     stats.HeapIdle,
		HeapReleased: stats.HeapReleased,
		HeapSys:      stats.HeapSys,
		Sys:          stats.Sys,
		RSS:          rss,
		HasRSS:       hasRSS,
	}
}

func formatMemoryReleaseSummary(before, after memorySnapshot) string {
	parts := []string{}
	if before.HasRSS && after.HasRSS {
		parts = append(parts, formatMetricChange("RAM", before.RSS, after.RSS))
	}
	parts = append(parts,
		formatMetricChange("active Go memory", before.HeapAlloc, after.HeapAlloc),
		formatMetricChange("returned to system", before.HeapReleased, after.HeapReleased),
	)
	return strings.Join(parts, ", ")
}

func formatMemoryReleaseDetails(before, after memorySnapshot) string {
	return strings.Join([]string{
		formatMetricChange("reusable Go memory", before.HeapIdle, after.HeapIdle),
		formatMetricChange("reserved Go memory", before.HeapSys, after.HeapSys),
		formatMetricChange("total Go runtime memory", before.Sys, after.Sys),
	}, ", ")
}

func formatMetricChange(name string, before, after uint64) string {
	return fmt.Sprintf("%s %d -> %s (%s)", name, before/mib, formatMiB(after), formatSignedMiBDelta(before, after))
}

func formatMiB(bytes uint64) string {
	return fmt.Sprintf("%d MiB", bytes/mib)
}

func formatSignedMiBDelta(before, after uint64) string {
	beforeMiB := int64(before / mib)
	afterMiB := int64(after / mib)
	delta := afterMiB - beforeMiB
	if delta > 0 {
		return fmt.Sprintf("+%d MiB", delta)
	}
	return fmt.Sprintf("%d MiB", delta)
}

func runAutoRestart(interval time.Duration, logger *logrus.Logger) {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	<-timer.C
	logger.Info("runtime: auto restart interval reached; re-executing process")
	if err := reexecSelf(); err != nil {
		logger.Errorf("runtime: failed to re-exec process: %v", err)
		os.Exit(1)
	}
}
