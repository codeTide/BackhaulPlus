package cmd

import "testing"

func TestSysctlSettingArgumentUsesKeyValueSingleArgument(t *testing.T) {
	setting := sysctlSetting{key: "net.core.wmem_max", value: "67108864"}
	got := setting.argument()
	want := "net.core.wmem_max=67108864"
	if got != want {
		t.Fatalf("argument() = %q, want %q", got, want)
	}
}
