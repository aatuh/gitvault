package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aatuh/gitvault/internal/infra/exec"
	"github.com/aatuh/gitvault/internal/ports"
)

const gitBinary = "git"

type Client struct {
	Runner executil.Runner
}

func (c Client) IsRepo(ctx context.Context, path string) (bool, error) {
	stdout, _, err := c.Runner.Run(ctx, gitBinary, []string{"-C", path, "rev-parse", "--is-inside-work-tree"}, nil, nil, "")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(stdout)) == "true", nil
}

func (c Client) InitRepo(ctx context.Context, path string) error {
	_, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", path, "init"}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("git init failed: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

func (c Client) TopLevel(ctx context.Context, path string) (string, error) {
	stdout, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", path, "rev-parse", "--show-toplevel"}, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (c Client) IsPathTracked(ctx context.Context, repoRoot, path string) (bool, error) {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return false, err
	}
	_, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", repoRoot, "ls-files", "--error-unmatch", rel}, nil, nil, "")
	if err != nil {
		if strings.Contains(strings.ToLower(string(stderr)), "did not match any files") {
			return false, nil
		}
		return false, nil
	}
	return true, nil
}

func (c Client) IsDirty(ctx context.Context, repoRoot string) (bool, error) {
	stdout, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", repoRoot, "status", "--porcelain"}, nil, nil, "")
	if err != nil {
		return false, fmt.Errorf("git status failed: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return strings.TrimSpace(string(stdout)) != "", nil
}

func (c Client) LastCommitInfo(ctx context.Context, repoRoot, path string) (ports.CommitInfo, error) {
	var info ports.CommitInfo
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return info, err
	}
	stdout, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", repoRoot, "log", "-1", "--format=%H|%an|%ai|%s", "--", rel}, nil, nil, "")
	if err != nil {
		return info, fmt.Errorf("git log failed: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	parts := strings.SplitN(strings.TrimSpace(string(stdout)), "|", 4)
	if len(parts) < 4 {
		return info, errors.New("unexpected git log output")
	}
	info.Hash = parts[0]
	info.Author = parts[1]
	info.Time = parts[2]
	info.Message = parts[3]
	return info, nil
}

func (c Client) Pull(ctx context.Context, repoRoot string) error {
	_, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", repoRoot, "pull", "--rebase"}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("git pull failed: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

func (c Client) Push(ctx context.Context, repoRoot string) error {
	_, stderr, err := c.Runner.Run(ctx, gitBinary, []string{"-C", repoRoot, "push"}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("git push failed: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}
