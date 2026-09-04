package db

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Installed struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Method  string `json:"method"` // binary|source
	RepoRev string `json:"repo_rev,omitempty"`
	// Files drives remove/verify
	Files       []string `json:"files"`
	InstalledAt string   `json:"installed_at,omitempty"`
}

func Path(lexRoot, name string) string {
	return filepath.Join(lexRoot, "db", name+".lexdata")
}

// Save writes atomically (tmp+rename).
func Save(lexRoot string, ins *Installed) error {
	if ins.Kind == "" {
		ins.Kind = "installed"
	}
	data, err := json.MarshalIndent(ins, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Join(lexRoot, "db"), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(lexRoot, "db"), "."+ins.Name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, Path(lexRoot, ins.Name))
}

func Load(lexRoot, name string) (*Installed, error) {
	data, err := os.ReadFile(Path(lexRoot, name))
	if err != nil {
		return nil, err
	}
	var ins Installed
	if err := json.Unmarshal(data, &ins); err != nil {
		return nil, fmt.Errorf("invalid db entry %s: %w", name, err)
	}
	if err := ins.Validate(); err != nil {
		return nil, fmt.Errorf("invalid db entry %s: %w", name, err)
	}
	return &ins, nil
}

func (ins *Installed) Validate() error {
	if ins.Kind != "" && ins.Kind != "installed" {
		return fmt.Errorf("kind must be %q", "installed")
	}
	if ins.Name == "" {
		return fmt.Errorf("missing name")
	}
	if ins.Version == "" {
		return fmt.Errorf("missing version")
	}
	if ins.Method != "" && ins.Method != "binary" && ins.Method != "source" {
		return fmt.Errorf("method must be binary|source")
	}
	return nil
}

// sorted package names
func List(lexRoot string) ([]string, error) {
	dir := filepath.Join(lexRoot, "db")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".lexdata" {
			continue
		}
		out = append(out, name[:len(name)-len(".lexdata")])
	}
	return out, nil
}
