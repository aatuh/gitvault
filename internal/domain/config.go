package domain

import (
	"errors"
	"strings"
	"time"
)

const ConfigVersion = 1

// Config holds vault-level settings.
type Config struct {
	Version    int       `json:"version"`
	Name       string    `json:"name"`
	Recipients []string  `json:"recipients"`
	CreatedAt  time.Time `json:"createdAt"`
}

func DefaultConfig(name string, recipients []string, now time.Time) Config {
	clean := make([]string, 0, len(recipients))
	seen := map[string]struct{}{}
	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		clean = append(clean, r)
	}

	return Config{
		Version:    ConfigVersion,
		Name:       strings.TrimSpace(name),
		Recipients: clean,
		CreatedAt:  now.UTC(),
	}
}

func (c Config) Validate() error {
	if c.Version <= 0 {
		return errors.New("config version must be positive")
	}
	for _, r := range c.Recipients {
		if strings.TrimSpace(r) == "" {
			return errors.New("recipient cannot be empty")
		}
	}
	return nil
}
