package cmd

import (
	"strings"
	"testing"
)

// A server using only the legacy fields must stay valid: the new mux tuning
// knobs are optional and default to backward-compatible values.
func TestMuxTuning_LegacyServerConfigValid(t *testing.T) {
	content := baseServer + `
ports = ["20000-20100"]
mux_con = 8
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("legacy server config should be valid: %v", err)
	}
}

func TestMuxTuning_ServerKnobsValid(t *testing.T) {
	content := baseServer + `
ports = ["20000-20100"]
mux_con = 8
max_mux_sessions = 2048
mux_spare_sessions = 8
new_conn_request_timeout = 5
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("valid server tuning rejected: %v", err)
	}
}

func TestMuxTuning_NegativeServerKnobsRejected(t *testing.T) {
	cases := map[string]string{
		"max_mux_sessions":         "max_mux_sessions = -1",
		"mux_spare_sessions":       "mux_spare_sessions = -1",
		"new_conn_request_timeout": "new_conn_request_timeout = -1",
	}
	for field, line := range cases {
		t.Run(field, func(t *testing.T) {
			content := baseServer + "\nports = [\"20000\"]\n" + line + "\n"
			err := loadAndValidate(t, content)
			if err == nil {
				t.Fatalf("expected error for negative %s", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Fatalf("error should mention %s, got: %v", field, err)
			}
		})
	}
}

const baseClient = `
[[client]]
name = "C1"
remote_addr = "127.0.0.1:30000"
transport = "tcpmux"
token = "QTRfs754a7"
`

func TestMuxTuning_LegacyClientConfigValid(t *testing.T) {
	content := baseServer + `
ports = ["20000"]
` + baseClient + `
connection_pool = 8
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("legacy client config should be valid: %v", err)
	}
}

func TestMuxTuning_ClientMaxPoolValid(t *testing.T) {
	content := baseServer + `
ports = ["20000"]
` + baseClient + `
connection_pool = 8
max_connection_pool = 2048
`
	if err := loadAndValidate(t, content); err != nil {
		t.Fatalf("valid client max_connection_pool rejected: %v", err)
	}
}

func TestMuxTuning_ClientMaxPoolBelowConnectionPoolRejected(t *testing.T) {
	content := baseServer + `
ports = ["20000"]
` + baseClient + `
connection_pool = 16
max_connection_pool = 8
`
	err := loadAndValidate(t, content)
	if err == nil {
		t.Fatal("expected error when max_connection_pool < connection_pool")
	}
	if !strings.Contains(err.Error(), "max_connection_pool must be greater than or equal to connection_pool") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMuxTuning_NegativeClientMaxPoolRejected(t *testing.T) {
	content := baseServer + `
ports = ["20000"]
` + baseClient + `
connection_pool = 8
max_connection_pool = -1
`
	err := loadAndValidate(t, content)
	if err == nil {
		t.Fatal("expected error for negative max_connection_pool")
	}
	if !strings.Contains(err.Error(), "max_connection_pool") {
		t.Fatalf("unexpected error: %v", err)
	}
}
