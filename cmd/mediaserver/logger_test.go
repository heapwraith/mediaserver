package main

import (
	"bytes"
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		level     string
		wantJSON  bool
		wantDebug bool
	}{
		{"prod is JSON", "prod", "info", true, false},
		{"dev is text", "dev", "info", false, false},
		{"debug level", "dev", "debug", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := newLogger(&buf, tt.env, tt.level)
			log.Debug("dbg")
			log.Info("hello", "k", "v")

			out := buf.String()
			if got := strings.Contains(out, "dbg"); got != tt.wantDebug {
				t.Errorf("debug line present = %v, want %v", got, tt.wantDebug)
			}
			last := strings.TrimSpace(out[strings.LastIndex(strings.TrimSpace(out), "\n")+1:])
			var m map[string]any
			isJSON := json.Unmarshal([]byte(last), &m) == nil
			if isJSON != tt.wantJSON {
				t.Errorf("JSON output = %v, want %v (line %q)", isJSON, tt.wantJSON, last)
			}
		})
	}
}
