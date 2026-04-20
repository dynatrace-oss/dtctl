// Package harness loads dqlbench fixtures and evaluates dtctl's DQL
// cost-lint + rewriter behavior against expectations. v1 is snapshot-only
// (no tenant); the tenant mode under build tag `dqlbench` adds
// ScannedBytes measurement.
package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Fixture is one prompt/query test case.
type Fixture struct {
	ID                   string   `yaml:"id"`
	Description          string   `yaml:"description"`
	Query                string   `yaml:"query"`
	ExpectRules          []string `yaml:"expect_rules"`
	AfterRewriteNoRules  []string `yaml:"after_rewrite_no_rules"`
	MustContain          []string `yaml:"must_contain"`
	MustNotContain       []string `yaml:"must_not_contain"`
	Budget               struct {
		MaxScannedBytes int64 `yaml:"max_scanned_bytes"`
	} `yaml:"budget"`
}

// LoadFixtures reads every *.yaml under dir, returning one Fixture per file,
// sorted by ID.
func LoadFixtures(dir string) ([]Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir: %w", err)
	}
	var out []Fixture
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var f Fixture
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if f.ID == "" {
			return nil, fmt.Errorf("fixture %s missing id", path)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
