package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aatuh/gitvault/internal/ports"
)

type CheckStatus string

const (
	CheckOK   CheckStatus = "ok"
	CheckWarn CheckStatus = "warn"
	CheckFail CheckStatus = "fail"
)

type CheckResult struct {
	Name    string
	Status  CheckStatus
	Message string
}

type DoctorReport struct {
	Checks []CheckResult
}

func (r DoctorReport) HasFailures() bool {
	for _, check := range r.Checks {
		if check.Status == CheckFail {
			return true
		}
	}
	return false
}

type DoctorService struct {
	Store     VaultStore
	Encrypter ports.Encrypter
	FS        ports.FileSystem
}

func (s DoctorService) Run(ctx context.Context, root string) (DoctorReport, error) {
	var report DoctorReport

	cfg, cfgErr := s.Store.LoadConfig(root)
	if cfgErr != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "vault config",
			Status:  CheckFail,
			Message: cfgErr.Error(),
		})
		return report, nil
	}

	report.Checks = append(report.Checks, CheckResult{
		Name:    "vault config",
		Status:  CheckOK,
		Message: "loaded",
	})

	if _, err := s.Store.LoadIndex(root); err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "vault index",
			Status:  CheckFail,
			Message: err.Error(),
		})
	} else {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "vault index",
			Status:  CheckOK,
			Message: "loaded",
		})
	}

	if version, err := s.Encrypter.Version(ctx); err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "sops",
			Status:  CheckFail,
			Message: err.Error(),
		})
	} else {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "sops",
			Status:  CheckOK,
			Message: strings.TrimSpace(version),
		})
	}

	if status, msg := checkAgeIdentity(); status != "" {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "age identity",
			Status:  status,
			Message: msg,
		})
	}

	if status, msg := checkWritable(root); status != "" {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "vault writable",
			Status:  status,
			Message: msg,
		})
	}

	secretFiles, err := s.Store.ListSecretFiles(root)
	if err != nil {
		report.Checks = append(report.Checks, CheckResult{
			Name:    "decrypt test",
			Status:  CheckFail,
			Message: err.Error(),
		})
		return report, nil
	}

	switch {
	case len(secretFiles) > 0:
		data, err := s.FS.ReadFile(secretFiles[0])
		if err != nil {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "decrypt test",
				Status:  CheckFail,
				Message: err.Error(),
			})
			break
		}
		if _, err := s.Encrypter.DecryptDotenv(ctx, data); err != nil {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "decrypt test",
				Status:  CheckFail,
				Message: err.Error(),
			})
		} else {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "decrypt test",
				Status:  CheckOK,
				Message: fmt.Sprintf("decrypted %s", filepath.Base(secretFiles[0])),
			})
		}
	case len(cfg.Recipients) == 0:
		report.Checks = append(report.Checks, CheckResult{
			Name:    "decrypt test",
			Status:  CheckWarn,
			Message: "no recipients configured",
		})
	default:
		payload := []byte("GITVAULT_HEALTHCHECK=ok\n")
		enc, err := s.Encrypter.EncryptDotenv(ctx, payload, cfg.Recipients)
		if err != nil {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "decrypt test",
				Status:  CheckFail,
				Message: err.Error(),
			})
			break
		}
		if _, err := s.Encrypter.DecryptDotenv(ctx, enc); err != nil {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "decrypt test",
				Status:  CheckFail,
				Message: err.Error(),
			})
		} else {
			report.Checks = append(report.Checks, CheckResult{
				Name:    "decrypt test",
				Status:  CheckOK,
				Message: "round-trip encrypt/decrypt succeeded",
			})
		}
	}

	return report, nil
}

func checkAgeIdentity() (CheckStatus, string) {
	envPath := strings.TrimSpace(os.Getenv("SOPS_AGE_KEY_FILE"))
	path := envPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".config", "sops", "age", "keys.txt")
		}
	}
	if path == "" {
		return CheckWarn, "age identity file path not resolved"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if envPath != "" {
				return CheckWarn, fmt.Sprintf("age identity file not found at SOPS_AGE_KEY_FILE=%s", path)
			}
			return CheckWarn, fmt.Sprintf("age identity file not found at %s (set SOPS_AGE_KEY_FILE to override)", path)
		}
		if envPath != "" {
			return CheckWarn, fmt.Sprintf("age identity check failed for SOPS_AGE_KEY_FILE=%s: %v", path, err)
		}
		return CheckWarn, fmt.Sprintf("age identity check failed: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if envPath != "" {
			return CheckOK, fmt.Sprintf("age identities found in %s (SOPS_AGE_KEY_FILE; default ignored)", path)
		}
		return CheckOK, fmt.Sprintf("age identities found in %s", path)
	}
	if envPath != "" {
		return CheckWarn, fmt.Sprintf("age identity file empty at %s (SOPS_AGE_KEY_FILE)", path)
	}
	return CheckWarn, fmt.Sprintf("age identity file empty at %s", path)
}

func checkWritable(root string) (CheckStatus, string) {
	path := filepath.Join(root, metadataDirName)
	info, err := os.Stat(path)
	if err != nil {
		return CheckFail, err.Error()
	}
	if !info.IsDir() {
		return CheckFail, "metadata path is not a directory"
	}
	tmp, err := os.CreateTemp(path, "writecheck")
	if err != nil {
		return CheckFail, err.Error()
	}
	_ = tmp.Close()
	_ = os.Remove(tmp.Name())
	return CheckOK, "ok"
}
