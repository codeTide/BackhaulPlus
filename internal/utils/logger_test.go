package utils

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewPrefixHookFormatsReadablePrefix(t *testing.T) {
	entry := &logrus.Entry{Message: "message"}
	if err := NewPrefixHook("client IR1").Fire(entry); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if entry.Message != "client IR1: message" {
		t.Fatalf("message = %q, want %q", entry.Message, "client IR1: message")
	}
}

func TestNewPrefixHookTrimsAndAvoidsDoubleColon(t *testing.T) {
	entry := &logrus.Entry{Message: "message"}
	if err := NewPrefixHook(" server IR1: ").Fire(entry); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if entry.Message != "server IR1: message" {
		t.Fatalf("message = %q, want %q", entry.Message, "server IR1: message")
	}
}

func TestNewPrefixHookEmptyPrefix(t *testing.T) {
	entry := &logrus.Entry{Message: "message"}
	if err := NewPrefixHook("  ").Fire(entry); err != nil {
		t.Fatalf("Fire returned error: %v", err)
	}
	if entry.Message != "message" {
		t.Fatalf("message = %q, want %q", entry.Message, "message")
	}
}

func TestComponentLogPrefix(t *testing.T) {
	tests := []struct {
		kind string
		name string
		want string
	}{
		{kind: "client", name: "IR1", want: "client IR1"},
		{kind: "server", name: " IR1 ", want: "server IR1"},
		{kind: "client", name: "", want: "client"},
	}
	for _, tt := range tests {
		if got := ComponentLogPrefix(tt.kind, tt.name); got != tt.want {
			t.Fatalf("ComponentLogPrefix(%q, %q) = %q, want %q", tt.kind, tt.name, got, tt.want)
		}
	}
}
