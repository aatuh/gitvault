package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/aatuh/gitvault/internal/domain"
	"github.com/aatuh/gitvault/internal/ports"
)

type FileService struct {
	Store     VaultStore
	Encrypter ports.Encrypter
	Clock     ports.Clock
}

func (s FileService) Put(ctx context.Context, root, project, env, name string, data []byte) (domain.FileMetadata, error) {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return domain.FileMetadata{}, err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return domain.FileMetadata{}, err
	}
	if err := domain.ValidateIdentifier(name, "file name"); err != nil {
		return domain.FileMetadata{}, err
	}

	cfg, err := s.Store.LoadConfig(root)
	if err != nil {
		return domain.FileMetadata{}, err
	}
	if len(cfg.Recipients) == 0 {
		return domain.FileMetadata{}, errors.New("no recipients configured; add with 'gitvault keys add'")
	}

	ciphertext, err := s.Encrypter.EncryptBinary(ctx, data, cfg.Recipients)
	if err != nil {
		return domain.FileMetadata{}, err
	}

	path := s.Store.FilePath(root, project, env, name)
	if err := s.Store.FS.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return domain.FileMetadata{}, err
	}
	if err := writeFileAtomic(s.Store.FS, path, ciphertext, 0600); err != nil {
		return domain.FileMetadata{}, err
	}

	meta := buildFileMetadata(data)
	meta.LastUpdated = s.Clock.Now()
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return domain.FileMetadata{}, err
	}
	idx.SetFile(project, env, name, meta)
	if err := s.Store.SaveIndex(root, idx); err != nil {
		return domain.FileMetadata{}, err
	}
	return meta, nil
}

func (s FileService) Get(ctx context.Context, root, project, env, name string) ([]byte, domain.FileMetadata, error) {
	if err := domain.ValidateIdentifier(project, "project"); err != nil {
		return nil, domain.FileMetadata{}, err
	}
	if err := domain.ValidateIdentifier(env, "env"); err != nil {
		return nil, domain.FileMetadata{}, err
	}
	if err := domain.ValidateIdentifier(name, "file name"); err != nil {
		return nil, domain.FileMetadata{}, err
	}

	path := s.Store.FilePath(root, project, env, name)
	data, err := s.Store.FS.ReadFile(path)
	if err != nil {
		return nil, domain.FileMetadata{}, err
	}
	plaintext, err := s.Encrypter.DecryptBinary(ctx, data)
	if err != nil {
		return nil, domain.FileMetadata{}, err
	}

	meta := buildFileMetadata(plaintext)
	if idx, err := s.Store.LoadIndex(root); err == nil {
		if stored, ok := fileMetadataFromIndex(idx, project, env, name); ok {
			meta.LastUpdated = stored.LastUpdated
			if stored.MIME != "" {
				meta.MIME = stored.MIME
			}
			if stored.SHA256 != "" {
				meta.SHA256 = stored.SHA256
			}
			if stored.Size > 0 {
				meta.Size = stored.Size
			}
		}
	}

	return plaintext, meta, nil
}

func buildFileMetadata(data []byte) domain.FileMetadata {
	hash := sha256.Sum256(data)
	meta := domain.FileMetadata{
		Size:   int64(len(data)),
		SHA256: hex.EncodeToString(hash[:]),
		MIME:   "application/octet-stream",
	}
	if len(data) > 0 {
		sample := data
		if len(sample) > 512 {
			sample = sample[:512]
		}
		meta.MIME = http.DetectContentType(sample)
	}
	return meta
}

func fileMetadataFromIndex(idx domain.Index, project, env, name string) (domain.FileMetadata, bool) {
	projectIndex, ok := idx.Projects[project]
	if !ok {
		return domain.FileMetadata{}, false
	}
	envIndex, ok := projectIndex.Envs[env]
	if !ok {
		return domain.FileMetadata{}, false
	}
	meta, ok := envIndex.Files[name]
	if !ok || meta == nil {
		return domain.FileMetadata{}, false
	}
	return *meta, true
}
