package diffreview

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const maxManifestBytes = 2 << 20

// LoadPath reads a YAML/JSON file or a directory of them.
// Files may contain multiple documents. Empty documents are skipped.
func LoadPath(path string) ([]Object, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("manifest path %s: %w", path, err)
	}
	var files []string
	if !info.IsDir() {
		files = []string{path}
	} else {
		err = filepath.WalkDir(path, func(name string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name != path && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(d.Name())) {
			case ".yaml", ".yml", ".json":
				files = append(files, name)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no .yaml, .yml, or .json files in %s", path)
		}
	}
	var out []Object
	seen := map[string]string{}
	for _, file := range files {
		objs, err := loadFile(file)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			if prev, ok := seen[obj.Key()]; ok {
				return nil, fmt.Errorf("duplicate object %s in %s and %s", obj.Ref(), prev, file)
			}
			seen[obj.Key()] = file
			out = append(out, obj)
		}
	}
	return out, nil
}

func loadFile(path string) ([]Object, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, maxManifestBytes)
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseYAML(path, buf)
}

// ParseYAML decodes one buffer of YAML or JSON documents.
func ParseYAML(origin string, buf []byte) ([]Object, error) {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(buf), 4096)
	var out []Object
	doc := 0
	for {
		u := &unstructured.Unstructured{}
		err := dec.Decode(u)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: document %d: %w", origin, doc+1, err)
		}
		doc++
		if len(u.Object) == 0 || (u.GetKind() == "" && u.GetName() == "") {
			continue
		}
		if u.GetKind() == "" || u.GetName() == "" {
			return nil, fmt.Errorf("%s: document %d is missing kind or metadata.name", origin, doc)
		}
		out = append(out, Object{
			APIVersion: u.GetAPIVersion(),
			Kind:       u.GetKind(),
			Namespace:  u.GetNamespace(),
			Name:       u.GetName(),
			Raw:        u.Object,
		})
	}
	return out, nil
}
