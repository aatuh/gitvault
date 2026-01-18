package domain

import (
	"strings"
	"testing"

	"github.com/aatuh/gitvault/internal/testutil"
)

func TestRenderDotenvOrderedPreservesOrder(t *testing.T) {
	valueA := testutil.RandomString(t, 6)
	valueB := testutil.RandomString(t, 6)
	valueC := testutil.RandomString(t, 6)
	values := map[string]string{
		"A_KEY": valueA,
		"B_KEY": valueB,
		"C_KEY": valueC,
	}
	order := []string{"B_KEY", "A_KEY"}

	payload := RenderDotenvOrdered(values, order)
	lines := strings.Split(strings.TrimSpace(string(payload)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "B_KEY="+valueB {
		t.Fatalf("expected B_KEY first, got %s", lines[0])
	}
	if lines[1] != "A_KEY="+valueA {
		t.Fatalf("expected A_KEY second, got %s", lines[1])
	}
	if lines[2] != "C_KEY="+valueC {
		t.Fatalf("expected C_KEY last, got %s", lines[2])
	}
}
