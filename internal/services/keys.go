package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aatuh/gitvault/internal/ports"
)

type RotateReport struct {
	Total   int
	Rotated int
	Failed  int
	Errors  []string
}

type KeysService struct {
	Store     VaultStore
	Encrypter ports.Encrypter
}

func (s KeysService) List(root string) ([]string, error) {
	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	return append([]string{}, cfg.Recipients...), nil
}

func (s KeysService) Add(root, recipient string) error {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return errors.New("recipient cannot be empty")
	}
	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return err
	}
	for _, existing := range cfg.Recipients {
		if existing == recipient {
			return nil
		}
	}
	cfg.Recipients = append(cfg.Recipients, recipient)
	return s.Store.SaveConfig(root, cfg)
}

func (s KeysService) Remove(root, recipient string) error {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return errors.New("recipient cannot be empty")
	}
	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return err
	}
	filtered := cfg.Recipients[:0]
	for _, existing := range cfg.Recipients {
		if existing == recipient {
			continue
		}
		filtered = append(filtered, existing)
	}
	cfg.Recipients = filtered
	return s.Store.SaveConfig(root, cfg)
}

func (s KeysService) Rotate(ctx context.Context, root string) (RotateReport, error) {
	report := RotateReport{}
	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return report, err
	}
	if len(cfg.Recipients) == 0 {
		return report, errors.New("no recipients configured")
	}
	files, err := s.Store.ListSecretFiles(root)
	if err != nil {
		return report, err
	}
	for _, path := range files {
		report.Total++
		data, err := s.Store.FS.ReadFile(path)
		if err != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		plaintext, err := s.Encrypter.DecryptDotenv(ctx, data)
		if err != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		ciphertext, err := s.Encrypter.EncryptDotenv(ctx, plaintext, cfg.Recipients)
		if err != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if err := writeFileAtomic(s.Store.FS, path, ciphertext, 0600); err != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		report.Rotated++
	}
	if report.Total == 0 {
		return report, os.ErrNotExist
	}
	return report, nil
}
