package env

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsValuesFromDotEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "old")
	path := filepath.Join(t.TempDir(), ".env")
	content := []byte(`
# comment
OPENROUTER_API_KEY=test-key
OPENROUTER_MODEL="openrouter/auto"
export PRIM_API_KEY='prim-key'
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("OPENROUTER_API_KEY"); got != "test-key" {
		t.Fatalf("OPENROUTER_API_KEY = %q", got)
	}
	if got := os.Getenv("OPENROUTER_MODEL"); got != "openrouter/auto" {
		t.Fatalf("OPENROUTER_MODEL = %q", got)
	}
	if got := os.Getenv("PRIM_API_KEY"); got != "prim-key" {
		t.Fatalf("PRIM_API_KEY = %q", got)
	}
}

func TestLoadRejectsInvalidLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("OPENROUTER_API_KEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}
