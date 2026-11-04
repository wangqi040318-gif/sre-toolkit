package certs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxPEMBytes = 1 << 20

// LoadPaths reads --file arguments and PEM files under --dir arguments.
// Directory walks include .pem, .crt, .cer, and .cert, and skip hidden directories.
func LoadPaths(files, dirs []string) ([]Material, []string, error) {
	var out []Material
	var notes []string
	for _, path := range files {
		m, err := loadFile(path)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, m)
	}
	for _, dir := range dirs {
		found, err := loadDir(dir)
		if err != nil {
			return nil, nil, err
		}
		if len(found) == 0 {
			notes = append(notes, fmt.Sprintf("No .pem, .crt, .cer, or .cert files in %s.", dir))
			continue
		}
		out = append(out, found...)
	}
	return out, notes, nil
}

func loadFile(path string) (Material, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Material{}, fmt.Errorf("pem file %s: %w", path, err)
	}
	if info.IsDir() {
		return Material{}, fmt.Errorf("pem file %s is a directory; use --dir", path)
	}
	if info.Size() > maxPEMBytes {
		return Material{}, fmt.Errorf("pem file %s is larger than %d bytes", path, maxPEMBytes)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return Material{}, fmt.Errorf("read %s: %w", path, err)
	}
	return Material{
		Source:   SourceFile,
		Name:     filepath.Base(path),
		Resource: "file " + path,
		PEM:      buf,
	}, nil
}

func loadDir(dir string) ([]Material, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("pem dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("pem dir %s is not a directory", dir)
	}
	var out []Material
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !pemExt(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxPEMBytes {
			return fmt.Errorf("pem file %s is larger than %d bytes", path, maxPEMBytes)
		}
		buf, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		out = append(out, Material{
			Source:   SourceFile,
			Name:     filepath.Base(path),
			Resource: "file " + path,
			PEM:      buf,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func pemExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pem", ".crt", ".cer", ".cert":
		return true
	default:
		return false
	}
}
