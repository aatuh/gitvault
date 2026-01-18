package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aatuh/gitvault/internal/domain"
	"github.com/aatuh/gitvault/internal/ports"
)

const (
	metadataDirName = ".gitvault"
	configFileName  = "config.json"
	indexFileName   = "index.json"
	secretsDirName  = "secrets"
)

var ErrVaultNotFound = errors.New("vault config not found")

type VaultStore struct {
	FS ports.FileSystem
}

func (s VaultStore) ConfigPath(root string) string {
	return filepath.Join(root, metadataDirName, configFileName)
}

func (s VaultStore) IndexPath(root string) string {
	return filepath.Join(root, metadataDirName, indexFileName)
}

func (s VaultStore) SecretsDir(root string) string {
	return filepath.Join(root, secretsDirName)
}

func (s VaultStore) SecretFilePath(root, project, env string) string {
	return filepath.Join(root, secretsDirName, project, env+".env")
}

func (s VaultStore) EnsureLayout(root string) error {
	if err := s.FS.MkdirAll(filepath.Join(root, metadataDirName), 0755); err != nil {
		return err
	}
	return s.FS.MkdirAll(s.SecretsDir(root), 0755)
}

func (s VaultStore) LoadConfig(root string) (domain.Config, error) {
	path := s.ConfigPath(root)
	data, err := s.FS.ReadFile(path)
	if err != nil {
		return domain.Config{}, err
	}
	var cfg domain.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return domain.Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return domain.Config{}, err
	}
	return cfg, nil
}

func (s VaultStore) SaveConfig(root string, cfg domain.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := s.ConfigPath(root)
	return writeFileAtomic(s.FS, path, data, 0644)
}

func (s VaultStore) LoadIndex(root string) (domain.Index, error) {
	path := s.IndexPath(root)
	data, err := s.FS.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.NewIndex(), nil
		}
		return domain.Index{}, err
	}
	var idx domain.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return domain.Index{}, err
	}
	if idx.Version <= 0 {
		idx.Version = domain.IndexVersion
	}
	if idx.Projects == nil {
		idx.Projects = map[string]*domain.ProjectIndex{}
	}
	return idx, nil
}

func (s VaultStore) SaveIndex(root string, idx domain.Index) error {
	if idx.Version <= 0 {
		idx.Version = domain.IndexVersion
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	path := s.IndexPath(root)
	return writeFileAtomic(s.FS, path, data, 0644)
}

func (s VaultStore) ListSecretFiles(root string) ([]string, error) {
	var files []string
	rootDir := s.SecretsDir(root)
	_, err := s.FS.Stat(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return files, nil
		}
		return nil, err
	}

	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := s.FS.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			if strings.HasSuffix(entry.Name(), ".env") {
				files = append(files, path)
			}
		}
		return nil
	}
	if err := walk(rootDir); err != nil {
		return nil, err
	}
	return files, nil
}

func FindVaultRoot(start string, fs ports.FileSystem) (string, error) {
	current := start
	for {
		configPath := filepath.Join(current, metadataDirName, configFileName)
		if _, err := fs.Stat(configPath); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%w from %s", ErrVaultNotFound, start)
		}
		current = parent
	}
}

func writeFileAtomic(fs ports.FileSystem, path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return fs.Rename(tmp.Name(), path)
}
