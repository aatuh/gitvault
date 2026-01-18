package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aatuh/gitvault/internal/domain"
	"github.com/aatuh/gitvault/internal/ports"
)

type MergeStrategy string

const (
	MergePreferVault MergeStrategy = "prefer-vault"
	MergePreferFile  MergeStrategy = "prefer-file"
	MergeInteractive MergeStrategy = "interactive"
)

type ConflictResolver func(key, vaultValue, fileValue string) (string, error)

type ImportOptions struct {
	Strategy MergeStrategy
	Resolver ConflictResolver
}

type ImportReport struct {
	Added    int
	Updated  int
	Skipped  int
	Warnings []string
}

type SecretService struct {
	Store     VaultStore
	Encrypter ports.Encrypter
	Clock     ports.Clock
}

func (s SecretService) Set(ctx context.Context, root, project, env, key, value string) error {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return err
	}
	if !domain.IsValidEnvKey(key) {
		return fmt.Errorf("invalid key '%s'", key)
	}

	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return err
	}
	if len(cfg.Recipients) == 0 {
		return errors.New("no recipients configured; add with 'gitvault keys add'")
	}

	values, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return err
	}
	values[key] = value
	if err := s.writeEnv(ctx, root, project, env, cfg.Recipients, values); err != nil {
		return err
	}

	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return err
	}
	idx.SetKey(project, env, key, s.Clock.Now())
	return s.Store.SaveIndex(root, idx)
}

func (s SecretService) Unset(ctx context.Context, root, project, env, key string) error {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return err
	}
	if !domain.IsValidEnvKey(key) {
		return fmt.Errorf("invalid key '%s'", key)
	}

	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return err
	}
	values, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return err
	}
	if _, ok := values[key]; !ok {
		return fmt.Errorf("key '%s' not found", key)
	}
	delete(values, key)

	if len(values) == 0 {
		path := s.Store.SecretFilePath(root, project, env)
		if err := s.Store.FS.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		if err := s.writeEnv(ctx, root, project, env, cfg.Recipients, values); err != nil {
			return err
		}
	}

	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return err
	}
	idx.RemoveKey(project, env, key)
	return s.Store.SaveIndex(root, idx)
}

func (s SecretService) ImportEnv(ctx context.Context, root, project, env string, input []byte, options ImportOptions) (ImportReport, error) {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return ImportReport{}, err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return ImportReport{}, err
	}

	parsed, issues := domain.ParseDotenv(input)
	report := ImportReport{}
	for _, issue := range issues {
		if issue.Severity == domain.IssueError {
			return report, fmt.Errorf("dotenv error on line %d: %s", issue.Line, issue.Message)
		}
		report.Warnings = append(report.Warnings, fmt.Sprintf("line %d: %s", issue.Line, issue.Message))
	}

	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return report, err
	}
	if len(cfg.Recipients) == 0 {
		return report, errors.New("no recipients configured; add with 'gitvault keys add'")
	}

	values, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return report, err
	}

	resolver := options.Resolver
	strategy := options.Strategy
	if strategy == "" {
		strategy = MergePreferVault
	}
	changed := map[string]struct{}{}

	for key, fileValue := range parsed.Values {
		vaultValue, exists := values[key]
		if !exists {
			values[key] = fileValue
			report.Added++
			changed[key] = struct{}{}
			continue
		}

		switch strategy {
		case MergePreferFile:
			values[key] = fileValue
			report.Updated++
			changed[key] = struct{}{}
		case MergeInteractive:
			if resolver == nil {
				return report, errors.New("interactive merge requires a resolver")
			}
			resolved, err := resolver(key, vaultValue, fileValue)
			if err != nil {
				return report, err
			}
			if resolved == vaultValue {
				report.Skipped++
			} else {
				values[key] = resolved
				report.Updated++
				changed[key] = struct{}{}
			}
		default:
			report.Skipped++
		}
	}

	if err := s.writeEnv(ctx, root, project, env, cfg.Recipients, values); err != nil {
		return report, err
	}

	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return report, err
	}
	now := s.Clock.Now()
	for key := range changed {
		idx.SetKey(project, env, key, now)
	}
	if err := s.Store.SaveIndex(root, idx); err != nil {
		return report, err
	}
	return report, nil
}

func (s SecretService) ExportEnv(ctx context.Context, root, project, env string) ([]byte, error) {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return nil, err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return nil, err
	}
	values, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return nil, err
	}
	return domain.RenderDotenv(values), nil
}

func (s SecretService) readEnv(ctx context.Context, root, project, env string) (map[string]string, error) {
	path := s.Store.SecretFilePath(root, project, env)
	data, err := s.Store.FS.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	plaintext, err := s.Encrypter.DecryptDotenv(ctx, data)
	if err != nil {
		return nil, err
	}
	parsed, issues := domain.ParseDotenv(plaintext)
	for _, issue := range issues {
		if issue.Severity == domain.IssueError {
			return nil, fmt.Errorf("vault dotenv error on line %d: %s", issue.Line, issue.Message)
		}
	}
	return parsed.Values, nil
}

func (s SecretService) writeEnv(ctx context.Context, root, project, env string, recipients []string, values map[string]string) error {
	payload := domain.RenderDotenv(values)
	ciphertext, err := s.Encrypter.EncryptDotenv(ctx, payload, recipients)
	if err != nil {
		return err
	}
	path := s.Store.SecretFilePath(root, project, env)
	if err := s.Store.FS.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeFileAtomic(s.Store.FS, path, ciphertext, 0600)
}
