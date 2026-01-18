package domain

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

type DotenvLineKind string

const (
	DotenvLineKey     DotenvLineKind = "key"
	DotenvLineComment DotenvLineKind = "comment"
	DotenvLineBlank   DotenvLineKind = "blank"
	DotenvLineOther   DotenvLineKind = "other"
)

type DotenvLine struct {
	Kind      DotenvLineKind
	Raw       string
	Key       string
	Value     string
	Comment   string
	HasExport bool
}

type DotenvDocument struct {
	Lines []DotenvLine
	Order []string
}

func ParseDotenvDocument(data []byte) (DotenvDocument, []DotenvIssue) {
	doc := DotenvDocument{Lines: []DotenvLine{}, Order: []string{}}
	issues := []DotenvIssue{}
	seen := map[string]struct{}{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			doc.Lines = append(doc.Lines, DotenvLine{Kind: DotenvLineBlank, Raw: raw})
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			doc.Lines = append(doc.Lines, DotenvLine{Kind: DotenvLineComment, Raw: raw})
			continue
		}

		hasExport := false
		payload := trimmed
		if strings.HasPrefix(payload, "export ") {
			hasExport = true
			payload = strings.TrimSpace(strings.TrimPrefix(payload, "export "))
		}
		eq := strings.Index(payload, "=")
		if eq < 0 {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  "missing '=' separator",
			})
			doc.Lines = append(doc.Lines, DotenvLine{Kind: DotenvLineOther, Raw: raw})
			continue
		}
		key := strings.TrimSpace(payload[:eq])
		if key == "" {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  "empty key",
			})
			doc.Lines = append(doc.Lines, DotenvLine{Kind: DotenvLineOther, Raw: raw})
			continue
		}
		if !isValidEnvKey(key) {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  fmt.Sprintf("invalid key '%s'", key),
			})
			doc.Lines = append(doc.Lines, DotenvLine{Kind: DotenvLineOther, Raw: raw})
			continue
		}

		valuePart := strings.TrimSpace(payload[eq+1:])
		comment := ""
		valueForParse := valuePart
		if valuePart != "" && !strings.HasPrefix(valuePart, "\"") && !strings.HasPrefix(valuePart, "'") {
			valueForParse, comment = splitInlineComment(valuePart)
		}
		value, err := parseDotenvValue(strings.TrimSpace(valueForParse))
		if err != nil {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueError,
				Message:  err.Error(),
			})
			doc.Lines = append(doc.Lines, DotenvLine{Kind: DotenvLineOther, Raw: raw})
			continue
		}
		if _, ok := seen[key]; ok {
			issues = append(issues, DotenvIssue{
				Line:     lineNum,
				Severity: IssueWarning,
				Message:  fmt.Sprintf("duplicate key '%s', last value wins", key),
			})
		} else {
			doc.Order = append(doc.Order, key)
		}
		seen[key] = struct{}{}

		doc.Lines = append(doc.Lines, DotenvLine{
			Kind:      DotenvLineKey,
			Key:       key,
			Value:     value,
			Comment:   comment,
			HasExport: hasExport,
		})
	}

	if err := scanner.Err(); err != nil {
		issues = append(issues, DotenvIssue{
			Line:     lineNum,
			Severity: IssueError,
			Message:  err.Error(),
		})
	}

	return doc, issues
}

func (doc DotenvDocument) Render() []byte {
	var b strings.Builder
	for _, line := range doc.Lines {
		switch line.Kind {
		case DotenvLineBlank, DotenvLineComment, DotenvLineOther:
			b.WriteString(line.Raw)
		case DotenvLineKey:
			if line.HasExport {
				b.WriteString("export ")
			}
			b.WriteString(line.Key)
			b.WriteString("=")
			b.WriteString(formatDotenvValue(line.Value))
			if line.Comment != "" {
				b.WriteString(" ")
				b.WriteString(line.Comment)
			}
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func splitInlineComment(value string) (string, string) {
	for i := 0; i < len(value); i++ {
		if value[i] != '#' {
			continue
		}
		if i == 0 {
			return "", strings.TrimSpace(value)
		}
		if value[i-1] == ' ' || value[i-1] == '\t' {
			return strings.TrimSpace(value[:i]), strings.TrimSpace(value[i:])
		}
	}
	return value, ""
}
