package maintenance

import (
	"strings"
	"testing"
)

func TestFormatMiB(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		want string
	}{
		{name: "zero", in: 0, want: "0 MiB"},
		{name: "one", in: mib, want: "1 MiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMiB(tt.in); got != tt.want {
				t.Fatalf("formatMiB(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatSignedMiBDelta(t *testing.T) {
	tests := []struct {
		name          string
		before, after uint64
		want          string
	}{
		{name: "positive", before: 10 * mib, after: 20 * mib, want: "+10 MiB"},
		{name: "negative", before: 20 * mib, after: 10 * mib, want: "-10 MiB"},
		{name: "zero", before: 10 * mib, after: 10 * mib, want: "0 MiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSignedMiBDelta(tt.before, tt.after); got != tt.want {
				t.Fatalf("formatSignedMiBDelta(%d, %d) = %q, want %q", tt.before, tt.after, got, tt.want)
			}
		})
	}
}

func TestFormatMemoryReleaseSummaryWithRSS(t *testing.T) {
	before := memorySnapshot{HeapAlloc: 10 * mib, HeapReleased: 3 * mib, RSS: 20 * mib, HasRSS: true}
	after := memorySnapshot{HeapAlloc: 8 * mib, HeapReleased: 6 * mib, RSS: 15 * mib, HasRSS: true}
	summary := formatMemoryReleaseSummary(before, after)

	for _, want := range []string{"RAM 20 -> 15 MiB (-5 MiB)", "active Go memory 10 -> 8 MiB (-2 MiB)", "returned to system 3 -> 6 MiB (+3 MiB)"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q does not contain %q", summary, want)
		}
	}
	for _, unwanted := range []string{"heap_alloc", "heap_released", "_"} {
		if strings.Contains(summary, unwanted) {
			t.Fatalf("summary %q contains unwanted %q", summary, unwanted)
		}
	}
}

func TestFormatMemoryReleaseSummaryWithoutRSS(t *testing.T) {
	before := memorySnapshot{HeapAlloc: 10 * mib, HeapReleased: 3 * mib}
	after := memorySnapshot{HeapAlloc: 8 * mib, HeapReleased: 6 * mib}
	summary := formatMemoryReleaseSummary(before, after)
	if strings.Contains(summary, "RAM") {
		t.Fatalf("summary %q contains RAM without RSS", summary)
	}
	for _, want := range []string{"active Go memory", "returned to system"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q does not contain %q", summary, want)
		}
	}
}

func TestFormatMemoryReleaseDetails(t *testing.T) {
	before := memorySnapshot{HeapIdle: 20 * mib, HeapSys: 40 * mib, Sys: 50 * mib}
	after := memorySnapshot{HeapIdle: 25 * mib, HeapSys: 40 * mib, Sys: 51 * mib}
	details := formatMemoryReleaseDetails(before, after)

	for _, want := range []string{"reusable Go memory 20 -> 25 MiB (+5 MiB)", "reserved Go memory 40 -> 40 MiB (0 MiB)", "total Go runtime memory 50 -> 51 MiB (+1 MiB)"} {
		if !strings.Contains(details, want) {
			t.Fatalf("details %q does not contain %q", details, want)
		}
	}
	for _, unwanted := range []string{"heap_idle", "heap_sys", "_"} {
		if strings.Contains(details, unwanted) {
			t.Fatalf("details %q contains unwanted %q", details, unwanted)
		}
	}
}
