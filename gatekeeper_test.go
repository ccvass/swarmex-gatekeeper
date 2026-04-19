package gatekeeper

import (
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   Config
	}{
		{
			"defaults",
			map[string]string{},
			Config{Path: "/health/ready", Interval: 5 * time.Second, Timeout: 3 * time.Second, Threshold: 3},
		},
		{
			"custom",
			map[string]string{
				labelPath:      "/ready",
				labelInterval:  "10s",
				labelTimeout:   "2s",
				labelThreshold: "5",
			},
			Config{Path: "/ready", Interval: 10 * time.Second, Timeout: 2 * time.Second, Threshold: 5},
		},
		{
			"invalid values use defaults",
			map[string]string{
				labelInterval:  "bad",
				labelThreshold: "nope",
			},
			Config{Path: "/health/ready", Interval: 5 * time.Second, Timeout: 3 * time.Second, Threshold: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseConfig(tt.labels)
			if got != tt.want {
				t.Errorf("parseConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStripCIDR(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"10.0.0.5/24", "10.0.0.5"},
		{"10.0.0.5", "10.0.0.5"},
		{"fd00::1/64", "fd00::1"},
	}
	for _, tt := range tests {
		if got := stripCIDR(tt.input); got != tt.want {
			t.Errorf("stripCIDR(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
