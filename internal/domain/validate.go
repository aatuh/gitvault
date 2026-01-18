package domain

import (
	"errors"
	"fmt"
	"strings"
)

func ValidateIdentifier(name, field string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '_', '.':
			continue
		default:
			return fmt.Errorf("%s contains invalid character '%c'", field, r)
		}
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return errors.New("identifier must not contain path separators")
	}
	return nil
}
