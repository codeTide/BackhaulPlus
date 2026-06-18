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
		{name: "zero", in: 0, want: "0MiB"},
		{name: "one", in: mib, want: "1MiB"},
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
		{name: "positive", before: 10 * mib, after: 20 * mib, want: "+10MiB"},
		{name: "negative", before: 20 * mib, after: 10 * mib, want: "-10MiB"},
		{name: "zero", before: 10 * mib, after: 10 * mib, want: "0MiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSignedMiBDelta(tt.before, tt.after); got != tt.want {
				t.Fatalf("formatSignedMiBDelta(%d, %d) = %q, want %q", tt.before, tt.after, got, tt.want)
			}
		})
	}
}

func TestFormatMemoryReleaseSummary(t *testing.T) {
	before := memorySnapshot{HeapAlloc: 10 * mib, HeapIdle: 20 * mib, HeapReleased: 3 * mib, HeapSys: 40 * mib, Sys: 50 * mib}
	after := memorySnapshot{HeapAlloc: 8 * mib, HeapIdle: 25 * mib, HeapReleased: 6 * mib, HeapSys: 40 * mib, Sys: 51 * mib}
	summary := formatMemoryReleaseSummary(before, after)

	for _, want := range []string{"heap_alloc=", "heap_idle=", "heap_released=", "heap_sys=", "sys="} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q does not contain %q", summary, want)
		}
	}
	if strings.Contains(summary, "rss=") {
		t.Fatalf("summary %q contains rss without HasRSS", summary)
	}
}

func TestFormatMemoryReleaseSummaryIncludesRSSWhenAvailable(t *testing.T) {
	before := memorySnapshot{RSS: 20 * mib, HasRSS: true}
	after := memorySnapshot{RSS: 15 * mib, HasRSS: true}
	summary := formatMemoryReleaseSummary(before, after)
	if !strings.Contains(summary, "rss=20MiB->15MiB (-5MiB)") {
		t.Fatalf("summary %q does not include expected rss change", summary)
	}
}
