package constants

import "testing"

func TestIsInternalChannel(t *testing.T) {
	tests := []struct {
		channel  string
		expected bool
	}{
		{"cli", true},
		{"system", true},
		{"subagent", true},
		{"slack", false},
		{"discord", false},
		{"telegram", false},
		{"wecom", false},
		{"", false},
		{"CLI", false},    // case sensitive
		{"System", false}, // case sensitive
	}

	for _, tt := range tests {
		got := IsInternalChannel(tt.channel)
		if got != tt.expected {
			t.Errorf("IsInternalChannel(%q) = %v, want %v", tt.channel, got, tt.expected)
		}
	}
}
