package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/aatuh/gitvault/internal/domain"
	"github.com/aatuh/gitvault/internal/ports"
)

type InitService struct {
	Store VaultStore
	Git   ports.Git
	Clock ports.Clock
}

type InitOptions struct {
	Root       string
	Name       string
	Recipients []string
	Force      bool
	InitGit    bool
}

func (s InitService) Init(ctx context.Context, opts InitOptions) error {
	if strings.TrimSpace(opts.Root) == "" {
		return errors.New("root path is required")
	}
	if err := s.Store.EnsureLayout(opts.Root); err != nil {
		return err
	}

	configPath := s.Store.ConfigPath(opts.Root)
	if _, err := s.Store.FS.Stat(configPath); err == nil && !opts.Force {
		return errors.New("vault already initialized (use --force to overwrite)")
	}

	cfg := domain.DefaultConfig(opts.Name, opts.Recipients, s.Clock.Now())
	if err := s.Store.SaveConfig(opts.Root, cfg); err != nil {
		return err
	}
	idx := domain.NewIndex()
	if err := s.Store.SaveIndex(opts.Root, idx); err != nil {
		return err
	}

	if opts.InitGit {
		isRepo, err := s.Git.IsRepo(ctx, opts.Root)
		if err == nil && !isRepo {
			if err := s.Git.InitRepo(ctx, opts.Root); err != nil {
				return err
			}
		}
	}

	readmePath := filepath.Join(opts.Root, "README.md")
	if _, err := s.Store.FS.Stat(readmePath); err != nil && errors.Is(err, os.ErrNotExist) {
		_ = s.Store.FS.WriteFile(readmePath, []byte("# GitVault Vault\n\nThis repository stores encrypted secrets managed by gitvault.\n"), 0644)
	}
	return nil
}
