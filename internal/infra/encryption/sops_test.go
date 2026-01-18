package encryption

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aatuh/gitvault/internal/testutil"
)

func TestSanitizeSopsErrorRecipientMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	identity := "AGE-SECRET-KEY-1" + testutil.RandomString(t, 12)
	if err := os.WriteFile(path, []byte(identity+"\n"), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	t.Setenv("SOPS_AGE_KEY_FILE", path)

	msg := sanitizeSopsError("decrypt", []byte("failed to decrypt data key"))

	if msg != "age identity does not match recipients" {
		t.Fatalf("expected recipient mismatch, got %q", msg)
	}
}

func TestSanitizeSopsErrorMissingIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing-"+testutil.RandomString(t, 6)+".txt")
	t.Setenv("SOPS_AGE_KEY_FILE", path)

	msg := sanitizeSopsError("decrypt", []byte("failed to decrypt data key"))

	if msg != "age identity not found" {
		t.Fatalf("expected missing identity, got %q", msg)
	}
}
