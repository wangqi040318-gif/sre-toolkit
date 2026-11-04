package daily

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// MergeContexts combines repeated --context values with a contexts file.
// Blank lines and lines starting with # are ignored. Order is preserved and
// duplicates are dropped. An empty result means "use the current context".
func MergeContexts(flags []string, file string) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || strings.HasPrefix(name, "#") {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range flags {
		add(name)
	}
	if strings.TrimSpace(file) == "" {
		return out, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("contexts file %s: %w", file, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		add(sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("contexts file %s: %w", file, err)
	}
	return out, nil
}
