package services

import (
	"sort"

	"github.com/aatuh/gitvault/internal/domain"
)

type ListingService struct {
	Store VaultStore
}

func (s ListingService) ListProjects(root string) ([]string, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	return idx.ListProjects(), nil
}

func (s ListingService) ListEnvs(root, project string) ([]string, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	return idx.ListEnvs(project), nil
}

func (s ListingService) ListKeys(root, project, env string) ([]domain.KeyInfo, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	return idx.ListKeys(project, env), nil
}

func (s ListingService) ListFiles(root, project, env string) ([]domain.FileInfo, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	return idx.ListFiles(project, env), nil
}

func (s ListingService) FindKeys(root, pattern string) ([]string, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	matches := []string{}
	for project, p := range idx.Projects {
		for env, e := range p.Envs {
			for key := range e.Keys {
				ref := project + "/" + env + "/" + key
				if pattern == "" || containsFold(ref, pattern) {
					matches = append(matches, ref)
				}
			}
		}
	}
	sort.Strings(matches)
	return matches, nil
}

func (s ListingService) ListAllKeys(root string) ([]domain.KeyInfo, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	keys := []domain.KeyInfo{}
	for project, p := range idx.Projects {
		for env, e := range p.Envs {
			for key, meta := range e.Keys {
				ref := project + "/" + env + "/" + key
				keys = append(keys, domain.KeyInfo{Name: ref, LastUpdated: meta.LastUpdated})
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name < keys[j].Name
	})
	return keys, nil
}

func (s ListingService) ListAllFiles(root string) ([]domain.FileInfo, error) {
	idx, err := s.Store.LoadIndex(root)
	if err != nil {
		return nil, err
	}
	files := []domain.FileInfo{}
	for project, p := range idx.Projects {
		for env, e := range p.Envs {
			for name, meta := range e.Files {
				ref := project + "/" + env + "/" + name
				files = append(files, domain.FileInfo{
					Name:        ref,
					Size:        meta.Size,
					SHA256:      meta.SHA256,
					MIME:        meta.MIME,
					LastUpdated: meta.LastUpdated,
				})
			}
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files, nil
}

func containsFold(s, substr string) bool {
	if substr == "" {
		return true
	}
	return domain.ContainsFold(s, substr)
}
