package domain

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type IssueSeverity string

const (
	IssueError   IssueSeverity = "error"
	IssueWarning IssueSeverity = "warning"
)

type DotenvIssue struct {
	Line     int
	Severity IssueSeverity
	Message  string
}

type Dotenv struct {
	Values map[string]string
	Order  []string
}

func ParseDotenv(data []byte) (Dotenv, []DotenvIssue) {
	result := Dotenv{Values: map[string]string{}, Order: []string{}}
	issues := []DotenvIssue{}
	seen := map[string]struct{}{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueWarning,
				Message:  "line uses export; removed prefix",
			})
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  "missing '=' separator",
			})
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  "empty key",
			})
			continue
		}
		if !isValidEnvKey(key) {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  fmt.Sprintf("invalid key '%s'", key),
			})
			continue
		}
		valuePart := strings.TrimSpace(line[eq+1:])
		value, err := parseDotenvValue(valuePart)
		if err != nil {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  err.Error(),
			})
			continue
		}
		if _, ok := seen[key]; ok {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueWarning,
				Message:  fmt.Sprintf("duplicate key '%s', last value wins", key),
			})
		} else {
			result.Order = append(result.Order, key)
		}
		seen[key] = struct{}{}
		result.Values[key] = value
	}

	if err := scanner.Err(); err != nil {
		issues = append(issues, DotenvIssue{
			Line:     lineNum,
			Severity: IssueError,
			Message:  err.Error(),
		})
	}

	return result, issues
}

func RenderDotenv(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(formatDotenvValue(values[key]))
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func isValidEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func IsValidEnvKey(key string) bool {
	return isValidEnvKey(key)
}

func parseDotenvValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "\"") {
		return parseQuoted(value, '"')
	}
	if strings.HasPrefix(value, "'") {
		return parseQuoted(value, '\'')
	}
	return strings.TrimSpace(stripInlineComment(value)), nil
}

func parseQuoted(value string, quote byte) (string, error) {
	if len(value) < 2 || value[len(value)-1] != quote {
		return "", fmt.Errorf("unterminated quoted value")
	}
	inner := value[1 : len(value)-1]
	if quote == '\'' {
		return inner, nil
	}
	// double-quoted: support basic escapes
	var b strings.Builder
	b.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if ch != '\\' || i == len(inner)-1 {
			b.WriteByte(ch)
			continue
		}
		next := inner[i+1]
		switch next {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '"':
			b.WriteByte(next)
		default:
			b.WriteByte(next)
		}
		i++
	}
	return b.String(), nil
}

func stripInlineComment(value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] == '#' {
			if i == 0 {
				return ""
			}
			if value[i-1] == ' ' || value[i-1] == '\t' {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return value
}

func formatDotenvValue(value string) string {
	if value == "" {
		return ""
	}
	if !strings.ContainsAny(value, " \t\n\r#\"'\\") {
		return value
	}
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	return "\"" + escaped + "\""
}
