package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aatuh/gitvault/internal/testutil"
)

func TestCheckAgeIdentity_UsesEnvPath(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	identity := "AGE-SECRET-KEY-1" + testutil.RandomString(t, 12)
	if err := os.WriteFile(path, []byte(identity+"\n"), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	t.Setenv("SOPS_AGE_KEY_FILE", path)

	status, message := checkAgeIdentity()

	if status != CheckOK {
		t.Fatalf("expected ok status, got %s (%s)", status, message)
	}
	if !strings.Contains(message, "SOPS_AGE_KEY_FILE") {
		t.Fatalf("expected env var mention, got %s", message)
	}
	if !strings.Contains(message, path) {
		t.Fatalf("expected path mention, got %s", message)
	}
}
