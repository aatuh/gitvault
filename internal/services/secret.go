package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	Strategy        MergeStrategy
	Resolver        ConflictResolver
	NoPreserveOrder bool
}

type ImportReport struct {
	Added    int
	Updated  int
	Skipped  int
	Warnings []string
}

type ExportOptions struct {
	NoPreserveOrder bool
}

type ApplyOptions struct {
	OnlyExisting bool
}

type ApplyReport struct {
	Updated int
	Added   int
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

	dotenv, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return err
	}
	if _, ok := dotenv.Values[key]; !ok {
		dotenv.Order = append(dotenv.Order, key)
	}
	dotenv.Values[key] = value
	if err := s.writeEnv(ctx, root, project, env, cfg.Recipients, dotenv, true); err != nil {
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
	dotenv, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return err
	}
	if _, ok := dotenv.Values[key]; !ok {
		return fmt.Errorf("key '%s' not found", key)
	}
	delete(dotenv.Values, key)
	dotenv.Order = removeKeyFromOrder(dotenv.Order, key)

	if len(dotenv.Values) == 0 {
		path := s.Store.SecretFilePath(root, project, env)
		if err := s.Store.FS.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		if err := s.writeEnv(ctx, root, project, env, cfg.Recipients, dotenv, true); err != nil {
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

	existing, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return report, err
	}

	values := existing.Values
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

	preserveOrder := !options.NoPreserveOrder
	order := existing.Order
	if preserveOrder {
		order = mergeOrder(existing.Order, parsed.Order, values)
	}
	dotenv := domain.Dotenv{Values: values, Order: order}
	if err := s.writeEnv(ctx, root, project, env, cfg.Recipients, dotenv, preserveOrder); err != nil {
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
	return s.ExportEnvWithOptions(ctx, root, project, env, ExportOptions{})
}

func (s SecretService) ExportEnvWithOptions(ctx context.Context, root, project, env string, options ExportOptions) ([]byte, error) {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return nil, err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return nil, err
	}
	dotenv, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return nil, err
	}
	if options.NoPreserveOrder {
		return domain.RenderDotenv(dotenv.Values), nil
	}
	return domain.RenderDotenvOrdered(dotenv.Values, dotenv.Order), nil
}

func (s SecretService) readEnv(ctx context.Context, root, project, env string) (domain.Dotenv, error) {
	path := s.Store.SecretFilePath(root, project, env)
	data, err := s.Store.FS.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Dotenv{Values: map[string]string{}, Order: []string{}}, nil
		}
		return domain.Dotenv{}, err
	}
	plaintext, err := s.Encrypter.DecryptDotenv(ctx, data)
	if err != nil {
		return domain.Dotenv{}, err
	}
	parsed, issues := domain.ParseDotenv(plaintext)
	for _, issue := range issues {
		if issue.Severity == domain.IssueError {
			return domain.Dotenv{}, fmt.Errorf("vault dotenv error on line %d: %s", issue.Line, issue.Message)
		}
	}
	return parsed, nil
}

func (s SecretService) writeEnv(ctx context.Context, root, project, env string, recipients []string, dotenv domain.Dotenv, preserveOrder bool) error {
	payload := domain.RenderDotenv(dotenv.Values)
	if preserveOrder {
		payload = domain.RenderDotenvOrdered(dotenv.Values, dotenv.Order)
	}
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

func (s SecretService) ApplyEnvFile(ctx context.Context, root, project, env, path string, options ApplyOptions) (ApplyReport, error) {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return ApplyReport{}, err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return ApplyReport{}, err
	}
	if strings.TrimSpace(path) == "" {
		return ApplyReport{}, errors.New("file path is required")
	}
	if _, err := s.Store.FS.Stat(path); err != nil {
		return ApplyReport{}, err
	}

	dotenv, err := s.readEnv(ctx, root, project, env)
	if err != nil {
		return ApplyReport{}, err
	}

	data, err := s.Store.FS.ReadFile(path)
	if err != nil {
		return ApplyReport{}, err
	}
	doc, issues := domain.ParseDotenvDocument(data)
	for _, issue := range issues {
		if issue.Severity == domain.IssueError {
			return ApplyReport{}, fmt.Errorf("dotenv error on line %d: %s", issue.Line, issue.Message)
		}
	}

	updated, added := applyValuesToDocument(&doc, dotenv.Values, options.OnlyExisting)
	if updated == 0 && added == 0 {
		return ApplyReport{}, nil
	}
	payload := doc.Render()
	if err := writeFileAtomic(s.Store.FS, path, payload, 0600); err != nil {
		return ApplyReport{}, err
	}
	return ApplyReport{Updated: updated, Added: added}, nil
}

func applyValuesToDocument(doc *domain.DotenvDocument, values map[string]string, onlyExisting bool) (int, int) {
	updated := 0
	added := 0
	existing := map[string]struct{}{}
	for i, line := range doc.Lines {
		if line.Kind != domain.DotenvLineKey {
			continue
		}
		existing[line.Key] = struct{}{}
		value, ok := values[line.Key]
		if !ok {
			continue
		}
		if line.Value != value {
			updated++
		}
		line.Value = value
		doc.Lines[i] = line
	}
	if onlyExisting {
		return updated, added
	}
	missing := make([]string, 0, len(values))
	for key := range values {
		if _, ok := existing[key]; ok {
			continue
		}
		missing = append(missing, key)
	}
	sort.Strings(missing)
	for _, key := range missing {
		doc.Lines = append(doc.Lines, domain.DotenvLine{
			Kind:  domain.DotenvLineKey,
			Key:   key,
			Value: values[key],
		})
		added++
	}
	return updated, added
}

func mergeOrder(existing []string, file []string, values map[string]string) []string {
	order := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, key := range file {
		if _, ok := values[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		order = append(order, key)
	}
	for _, key := range existing {
		if _, ok := values[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		order = append(order, key)
	}
	if len(seen) == len(values) {
		return order
	}
	missing := make([]string, 0, len(values)-len(seen))
	for key := range values {
		if _, ok := seen[key]; ok {
			continue
		}
		missing = append(missing, key)
	}
	sort.Strings(missing)
	return append(order, missing...)
}

func removeKeyFromOrder(order []string, key string) []string {
	if len(order) == 0 {
		return order
	}
	filtered := order[:0]
	for _, item := range order {
		if item == key {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
