package services

import (
	"context"
	"errors"

	"github.com/aatuh/gitvault/internal/ports"
)

type SyncService struct {
	Git ports.Git
}

func (s SyncService) Pull(ctx context.Context, root string, allowDirty bool) error {
	if !allowDirty {
		dirty, err := s.Git.IsDirty(ctx, root)
		if err != nil {
			return err
		}
		if dirty {
			return errors.New("working tree is dirty; commit or use --allow-dirty (e.g., gitvault sync pull --allow-dirty)")
		}
	}
	return s.Git.Pull(ctx, root)
}

func (s SyncService) Push(ctx context.Context, root string, allowDirty bool) error {
	if !allowDirty {
		dirty, err := s.Git.IsDirty(ctx, root)
		if err != nil {
			return err
		}
		if dirty {
			return errors.New("working tree is dirty; commit or use --allow-dirty (e.g., gitvault sync push --allow-dirty)")
		}
	}
	return s.Git.Push(ctx, root)
}
