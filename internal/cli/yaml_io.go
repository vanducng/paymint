package cli

import (
	"os"
	"path/filepath"

	yaml "github.com/goccy/go-yaml"
)

// writeYAMLFile marshals v and atomically writes to path with mode 0600.
func writeYAMLFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.MarshalWithOptions(v, yaml.Indent(2))
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
