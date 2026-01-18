package domain

import (
	"crypto/rand"
	"testing"

	"github.com/aatuh/gitvault/internal/testutil"
)

func TestParseAndRenderDotenv(t *testing.T) {
	values := map[string]string{
		randomKey(t): testutil.RandomString(t, 12),
		randomKey(t): testutil.RandomString(t, 8),
	}
	payload := RenderDotenv(values)
	parsed, issues := ParseDotenv(payload)
	for _, issue := range issues {
		if issue.Severity == IssueError {
			t.Fatalf("unexpected parse error: %v", issue)
		}
	}
	if len(parsed.Values) != len(values) {
		t.Fatalf("expected %d values, got %d", len(values), len(parsed.Values))
	}
	for key, value := range values {
		if parsed.Values[key] != value {
			t.Fatalf("value mismatch for %s", key)
		}
	}
}

func TestParseDotenvWarnings(t *testing.T) {
	input := []byte("export FOO=bar\n")
	_, issues := ParseDotenv(input)
	if len(issues) == 0 {
		t.Fatalf("expected warning")
	}
	if issues[0].Severity != IssueWarning {
		t.Fatalf("expected warning, got %v", issues[0].Severity)
	}
}

func randomKey(t *testing.T) string {
	t.Helper()
	letters := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ_")
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand read: %v", err)
	}
	buf[0] = letters[int(buf[0])%len(letters)]
	for i := 1; i < len(buf); i++ {
		buf[i] = letters[int(buf[i])%len(letters)]
	}
	return string(buf)
}
