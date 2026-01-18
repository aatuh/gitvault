package ports

import "context"

type CommitInfo struct {
	Hash    string
	Author  string
	Message string
	Time    string
}

type Git interface {
	IsRepo(ctx context.Context, path string) (bool, error)
	InitRepo(ctx context.Context, path string) error
	TopLevel(ctx context.Context, path string) (string, error)
	IsPathTracked(ctx context.Context, repoRoot, path string) (bool, error)
	IsDirty(ctx context.Context, repoRoot string) (bool, error)
	LastCommitInfo(ctx context.Context, repoRoot, path string) (CommitInfo, error)
	Pull(ctx context.Context, repoRoot string) error
	Push(ctx context.Context, repoRoot string) error
}
