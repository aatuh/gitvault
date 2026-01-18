package testutil

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func RandomString(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand read: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
