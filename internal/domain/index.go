package domain

import (
	"sort"
	"time"
)

const IndexVersion = 1

type Index struct {
	Version  int                      `json:"version"`
	Projects map[string]*ProjectIndex `json:"projects"`
}

type ProjectIndex struct {
	Envs map[string]*EnvIndex `json:"envs"`
}

type EnvIndex struct {
	Keys  map[string]*KeyMetadata  `json:"keys"`
	Files map[string]*FileMetadata `json:"files,omitempty"`
}

type KeyMetadata struct {
	LastUpdated time.Time `json:"lastUpdated"`
}

type FileMetadata struct {
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	MIME        string    `json:"mime,omitempty"`
	LastUpdated time.Time `json:"lastUpdated"`
}

func NewIndex() Index {
	return Index{
		Version:  IndexVersion,
		Projects: map[string]*ProjectIndex{},
	}
}

func (idx *Index) ensureProject(project string) *ProjectIndex {
	if idx.Projects == nil {
		idx.Projects = map[string]*ProjectIndex{}
	}
	p, ok := idx.Projects[project]
	if !ok {
		p = &ProjectIndex{Envs: map[string]*EnvIndex{}}
		idx.Projects[project] = p
	}
	return p
}

func (idx *Index) ensureEnv(project, env string) *EnvIndex {
	p := idx.ensureProject(project)
	if p.Envs == nil {
		p.Envs = map[string]*EnvIndex{}
	}
	e, ok := p.Envs[env]
	if !ok {
		e = &EnvIndex{Keys: map[string]*KeyMetadata{}, Files: map[string]*FileMetadata{}}
		p.Envs[env] = e
	}
	return e
}

func (idx *Index) SetKey(project, env, key string, updated time.Time) {
	e := idx.ensureEnv(project, env)
	if e.Keys == nil {
		e.Keys = map[string]*KeyMetadata{}
	}
	e.Keys[key] = &KeyMetadata{LastUpdated: updated.UTC()}
}

func (idx *Index) SetFile(project, env, name string, meta FileMetadata) {
	e := idx.ensureEnv(project, env)
	if e.Files == nil {
		e.Files = map[string]*FileMetadata{}
	}
	meta.LastUpdated = meta.LastUpdated.UTC()
	e.Files[name] = &meta
}

func (idx *Index) RemoveKey(project, env, key string) {
	p, ok := idx.Projects[project]
	if !ok {
		return
	}
	e, ok := p.Envs[env]
	if !ok {
		return
	}
	delete(e.Keys, key)
	if len(e.Keys) > 0 || len(e.Files) > 0 {
		return
	}
	delete(p.Envs, env)
	if len(p.Envs) == 0 {
		delete(idx.Projects, project)
	}
}

func (idx *Index) RemoveFile(project, env, name string) {
	p, ok := idx.Projects[project]
	if !ok {
		return
	}
	e, ok := p.Envs[env]
	if !ok {
		return
	}
	delete(e.Files, name)
	if len(e.Keys) > 0 || len(e.Files) > 0 {
		return
	}
	delete(p.Envs, env)
	if len(p.Envs) == 0 {
		delete(idx.Projects, project)
	}
}

func (idx Index) ListProjects() []string {
	projects := make([]string, 0, len(idx.Projects))
	for name := range idx.Projects {
		projects = append(projects, name)
	}
	sort.Strings(projects)
	return projects
}

func (idx Index) ListEnvs(project string) []string {
	p, ok := idx.Projects[project]
	if !ok {
		return nil
	}
	envs := make([]string, 0, len(p.Envs))
	for name := range p.Envs {
		envs = append(envs, name)
	}
	sort.Strings(envs)
	return envs
}

type KeyInfo struct {
	Name        string
	LastUpdated time.Time
}

func (idx Index) ListKeys(project, env string) []KeyInfo {
	p, ok := idx.Projects[project]
	if !ok {
		return nil
	}
	e, ok := p.Envs[env]
	if !ok {
		return nil
	}
	keys := make([]KeyInfo, 0, len(e.Keys))
	for name, meta := range e.Keys {
		keys = append(keys, KeyInfo{Name: name, LastUpdated: meta.LastUpdated})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name < keys[j].Name
	})
	return keys
}

type FileInfo struct {
	Name        string
	Size        int64
	SHA256      string
	MIME        string
	LastUpdated time.Time
}

func (idx Index) ListFiles(project, env string) []FileInfo {
	p, ok := idx.Projects[project]
	if !ok {
		return nil
	}
	e, ok := p.Envs[env]
	if !ok {
		return nil
	}
	files := make([]FileInfo, 0, len(e.Files))
	for name, meta := range e.Files {
		files = append(files, FileInfo{
			Name:        name,
			Size:        meta.Size,
			SHA256:      meta.SHA256,
			MIME:        meta.MIME,
			LastUpdated: meta.LastUpdated,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})
	return files
}
