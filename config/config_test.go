package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scimply.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCustomizedCount(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want int
	}{
		{"empty file", "", 0},
		{"one setting", "server:\n  addr: \":9090\"\n", 1},
		{"settings in several sections", "server:\n  addr: \":9090\"\nlog:\n  level: debug\nstore:\n  type: postgres\n", 3},
		{"nested settings count once each", "server:\n  tls:\n    cert_file: a.pem\n    key_file: a.key\n", 2},
		{"a duration", "server:\n  read_timeout: 1m\n", 1},
		{"a list counts once", "auth:\n  type: bearer_token\n  tokens: [a, b, c]\n", 2},
		{"a map counts once", "telemetry:\n  headers:\n    x-a: b\n    x-c: d\n", 1},
		{"a value equal to its default", "server:\n  addr: \":8080\"\nlog:\n  level: info\n", 0},
		{"a default and a change", "server:\n  addr: \":8080\"\n  max_page_size: 5000\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CustomizedCount(writeConfig(t, tt.yaml))
			if !ok {
				t.Fatal("expected the count to be available")
			}
			if got != tt.want {
				t.Errorf("CustomizedCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCustomizedCount_UnavailableWhenTheFileCannotBeLoaded(t *testing.T) {
	for name, path := range map[string]string{
		"missing file": filepath.Join(t.TempDir(), "nope.yaml"),
		"invalid yaml": writeConfig(t, "server: [unclosed"),
		"bad duration": writeConfig(t, "server:\n  read_timeout: soon\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if n, ok := CustomizedCount(path); ok || n != 0 {
				t.Errorf("CustomizedCount() = (%d, %v), want (0, false)", n, ok)
			}
		})
	}
}
