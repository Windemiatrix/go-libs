package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Synthetic.photoslibrary")
	tests := []struct {
		name     string
		args     []string
		code     int
		stderr   string // part of stderr; empty: stderr must be empty
		database bool   // database/Photos.sqlite exists afterwards
	}{
		{"help", []string{"-h"}, 0, "-out", false},
		{"no -out", []string{"-n", "30"}, 2, "-out", false},
		{"extra argument", []string{"-out", dir, "extra"}, 2, "extra", false},
		{"unknown flag", []string{"-out", dir, "-schema", "x"}, 2, "-schema", false},
		{"invalid config", []string{"-out", dir, "-n", "-1"}, 1, "gen: invalid config", false},
		{"success", []string{"-out", dir, "-n", "30", "-seed", "7"}, 0, "", true},
		{"output exists", []string{"-out", dir, "-n", "30"}, 1, "gen: output path exists", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(t.Context(), tt.args, &stderr); code != tt.code {
				t.Errorf("exit code %d, want %d; stderr:\n%s", code, tt.code, stderr.String())
			}
			switch got := stderr.String(); {
			case tt.stderr == "" && got != "":
				t.Errorf("stderr is not empty:\n%s", got)
			case !strings.Contains(got, tt.stderr):
				t.Errorf("stderr does not contain %q:\n%s", tt.stderr, got)
			case strings.Contains(got, "gen: gen:"):
				t.Errorf("stderr repeats the prefix:\n%s", got)
			}
			_, err := os.Stat(filepath.Join(dir, "database", "Photos.sqlite"))
			if exists := err == nil; exists != tt.database {
				t.Errorf("database exists: %v, want %v", exists, tt.database)
			}
		})
	}
}
