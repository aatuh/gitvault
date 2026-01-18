package domain

import (
	"strings"
	"testing"

	"github.com/aatuh/gitvault/internal/testutil"
)

func TestParseDotenvDocumentPreservesComments(t *testing.T) {
	value := testutil.RandomString(t, 8)
	input := strings.Join([]string{
		"# header",
		"export API_KEY=" + value + " # inline",
		"",
		"OTHER_KEY=ok",
	}, "\n") + "\n"

	doc, issues := ParseDotenvDocument([]byte(input))
	for _, issue := range issues {
		if issue.Severity == IssueError {
			t.Fatalf("unexpected error: %v", issue)
		}
	}
	output := string(doc.Render())
	if !strings.Contains(output, "# header") {
		t.Fatalf("expected header comment preserved")
	}
	if !strings.Contains(output, "export API_KEY="+value) {
		t.Fatalf("expected export line preserved")
	}
	if !strings.Contains(output, "# inline") {
		t.Fatalf("expected inline comment preserved")
	}
	if !strings.Contains(output, "OTHER_KEY=ok") {
		t.Fatalf("expected other key preserved")
	}
}
