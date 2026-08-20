// Package state persists snapshots in one YAML file.
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sig9org/update-radar/internal/model"
	"gopkg.in/yaml.v3"
)

// File stores snapshots keyed by their site URL.
type File struct {
	Sites map[string]model.Snapshot `yaml:"sites"`
}

// Load returns an empty state when path does not exist.
func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return File{Sites: map[string]model.Snapshot{}}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("read state file %q: %w", path, err)
	}
	var result File
	if err := yaml.Unmarshal(data, &result); err != nil {
		return File{}, fmt.Errorf("parse state file %q: %w", path, err)
	}
	if result.Sites == nil {
		result.Sites = map[string]model.Snapshot{}
	}
	return result, nil
}

// Save atomically replaces path with the supplied state.
func Save(path string, value File) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".cisco-rader-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("protect temporary state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state file %q: %w", path, err)
	}
	ok = true
	return nil
}
