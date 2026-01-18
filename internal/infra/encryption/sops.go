package encryption

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aatuh/gitvault/internal/infra/exec"
)

const defaultSopsBinary = "sops"

type Sops struct {
	Runner executil.Runner
	Path   string
}

func NewSops(runner executil.Runner) Sops {
	path := os.Getenv("GITVAULT_SOPS_PATH")
	if strings.TrimSpace(path) == "" {
		path = defaultSopsBinary
	}
	return Sops{Runner: runner, Path: path}
}

func (s Sops) Version(ctx context.Context) (string, error) {
	stdout, stderr, err := s.Runner.Run(ctx, s.Path, []string{"--version"}, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("sops not available: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return normalizeVersionOutput(string(stdout)), nil
}

func (s Sops) EncryptDotenv(ctx context.Context, plaintext []byte, recipients []string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errors.New("no recipients provided")
	}
	args := []string{"--encrypt", "--input-type", "dotenv", "--output-type", "dotenv", "--age", strings.Join(recipients, ",")}
	file, cleanup, err := s.tempFile(plaintext)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	args = append(args, file)
	stdout, stderr, err := s.Runner.Run(ctx, s.Path, args, nil, nil, "")
	if err != nil {
		return nil, sopsError("encrypt", err, stderr)
	}
	return stdout, nil
}

func (s Sops) DecryptDotenv(ctx context.Context, ciphertext []byte) ([]byte, error) {
	args := []string{"--decrypt", "--input-type", "dotenv", "--output-type", "dotenv"}
	file, cleanup, err := s.tempFile(ciphertext)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	args = append(args, file)
	stdout, stderr, err := s.Runner.Run(ctx, s.Path, args, nil, nil, "")
	if err != nil {
		return nil, sopsError("decrypt", err, stderr)
	}
	return stdout, nil
}

func (s Sops) tempFile(data []byte) (string, func(), error) {
	if runtime.GOOS == "windows" {
		file, err := os.CreateTemp("", "gitvault-plaintext")
		if err != nil {
			return "", nil, err
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			return "", nil, err
		}
		if err := file.Close(); err != nil {
			return "", nil, err
		}
		return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
	}

	tmpDir := os.TempDir()
	file, err := os.CreateTemp(tmpDir, "gitvault-plaintext")
	if err != nil {
		return "", nil, err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return "", nil, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		return "", nil, err
	}
	path := filepath.Clean(file.Name())
	return path, func() { _ = os.Remove(path) }, nil
}

func normalizeVersionOutput(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "warning") {
			continue
		}
		if strings.HasPrefix(lower, "deprecated") {
			continue
		}
		if strings.HasPrefix(lower, "sops") {
			return line
		}
	}
	return strings.TrimSpace(output)
}

func sopsError(op string, err error, stderr []byte) error {
	msg := sanitizeSopsError(op, stderr)
	if msg == "" {
		return fmt.Errorf("sops %s failed: %w", op, err)
	}
	return fmt.Errorf("sops %s failed: %s", op, msg)
}

func sanitizeSopsError(op string, stderr []byte) string {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	if op == "decrypt" {
		identityAvailable := ageIdentityAvailable()
		if strings.Contains(lower, "failed to open") && strings.Contains(lower, "keys.txt") {
			return "age identity not found"
		}
		if strings.Contains(lower, "no identities matched") ||
			strings.Contains(lower, "no identity matched") ||
			strings.Contains(lower, "no identity found") ||
			strings.Contains(lower, "failed to decrypt data key") ||
			strings.Contains(lower, "no matching keys") ||
			strings.Contains(lower, "no matching key") ||
			strings.Contains(lower, "no keys found") {
			if identityAvailable {
				return "age identity does not match recipients"
			}
			return "age identity not found"
		}
		if strings.Contains(lower, "no identity") || strings.Contains(lower, "age identity") {
			if identityAvailable {
				return "age identity does not match recipients"
			}
			return "age identity not found"
		}
	}
	if idx := strings.Index(msg, "\n"); idx >= 0 {
		return strings.TrimSpace(msg[:idx])
	}
	return msg
}

func ageIdentityAvailable() bool {
	path := strings.TrimSpace(os.Getenv("SOPS_AGE_KEY_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".config", "sops", "age", "keys.txt")
		}
	}
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return true
	}
	return false
}
