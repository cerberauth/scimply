package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// runAndCount executes args on a fresh root whose commands are stubbed, and
// returns what serveConfigCount reports while the command is running, which is
// when argvio asks for it.
func runAndCount(t *testing.T, args ...string) (n int, ok bool) {
	t.Helper()
	root := NewRootCmd("test", "none", "unknown")
	var called bool
	for _, c := range root.Commands() {
		c.Run = func(*cobra.Command, []string) { n, ok = serveConfigCount(root); called = true }
		c.RunE = nil
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !called {
		t.Fatal("expected the command to run")
	}
	return n, ok
}

func TestServeConfigCount_ForServeReadsTheConfigFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scimply.yaml")
	if err := os.WriteFile(path, []byte("server:\n  addr: \":9090\"\nlog:\n  level: debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, ok := runAndCount(t, "serve", "--config", path); !ok || n != 2 {
		t.Errorf("serveConfigCount() = (%d, %v), want (2, true)", n, ok)
	}
}

func TestServeConfigCount_UnavailableForOtherCommands(t *testing.T) {
	// A config file next to the binary must not be read for `version`.
	t.Chdir(t.TempDir())
	if err := os.WriteFile("scimply.yaml", []byte("log:\n  level: debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, ok := runAndCount(t, "version"); ok || n != 0 {
		t.Errorf("serveConfigCount() = (%d, %v), want (0, false)", n, ok)
	}
}

func TestServeConfigCount_UnavailableWhenTheFileIsMissing(t *testing.T) {
	if n, ok := runAndCount(t, "serve", "--config", filepath.Join(t.TempDir(), "nope.yaml")); ok || n != 0 {
		t.Errorf("serveConfigCount() = (%d, %v), want (0, false)", n, ok)
	}
}
