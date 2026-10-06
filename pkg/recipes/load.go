package recipes

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/dynatrace-oss/dtctl/sdk/inventory"
)

// Book is the merged set of recipes visible to one invocation.
type Book struct {
	Recipes map[string]*Recipe
	Domains map[string]*Domain
	Scopes  map[string]*ScopeDim
	// Capabilities are inventory capability definitions added by app bundles.
	Capabilities      map[string]*inventory.CapabilityDef
	CapabilitySources map[string]Source
	// Problems are content that was skipped; loading never fails as a whole.
	Problems []Problem

	fragments []fragment

	// The recipes' query signatures, built on the first MatchQuery.
	sigsOnce sync.Once
	sigs     *querySigs
}

type fragment struct {
	text   string
	source Source
}

// Problem is one piece of content the loader skipped, and why.
type Problem struct {
	Source  Source `json:"source"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	loc := p.Source.Location
	if loc == "" {
		loc = p.Source.String()
	}
	return loc + ": " + p.Message
}

// Sorted returns the recipes ordered by name.
func (b *Book) Sorted() []*Recipe {
	out := make([]*Recipe, 0, len(b.Recipes))
	for _, r := range b.Recipes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Get returns the named recipe, or nil.
func (b *Book) Get(name string) *Recipe { return b.Recipes[name] }

// FileLayer is a directory of recipe content: <domain>/*.yaml (any depth),
// _domains.yaml, _scopes.yaml and _fragments/*.tmpl.
type FileLayer struct {
	Layer Layer
	FS    fs.FS
	// Root is how Location paths are reported ("~/.config/dtctl/recipes").
	Root string
	// Name and Pin identify a declared recipe source (see Source).
	Name, Pin string
}

// BundleDoc is one app-shipped bundle document, already fetched.
type BundleDoc struct {
	Content []byte
	Source  Source // LayerEnvironment, with AppID, BundleVersion and Location
}

// Loader assembles a Book from the layers in precedence order.
type Loader struct {
	Builtin FileLayer
	// NoBuiltinRecipes omits built-in recipes (`builtin: false`); built-in domains,
	// scopes and fragments still load as shared vocabulary.
	NoBuiltinRecipes bool
	// Files are org then user layers, weakest first; a later layer wins.
	Files   []FileLayer
	Bundles []BundleDoc
	// DtctlVersion gates bundles by minDtctlVersion ("" skips it, for dev builds).
	DtctlVersion string
}

// layerContent is what one layer contributed before merging.
type layerContent struct {
	recipes   []*Recipe
	domains   []*Domain
	scopes    []*ScopeDim
	fragments []fragment
}

// Load reads every layer and merges them. It never fails: unreadable or
// invalid content becomes a Problem and the rest still loads.
func (l Loader) Load() *Book {
	b := &Book{
		Recipes:           map[string]*Recipe{},
		Domains:           map[string]*Domain{},
		Scopes:            map[string]*ScopeDim{},
		Capabilities:      map[string]*inventory.CapabilityDef{},
		CapabilitySources: map[string]Source{},
	}
	builtin := b.readFileLayer(l.Builtin)
	b.mergeRegistries(builtin, true)
	files := make([]layerContent, len(l.Files))
	for i, fl := range l.Files {
		files[i] = b.readFileLayer(fl)
		b.mergeRegistries(files[i], false)
	}
	bundles := b.readBundles(l.Bundles, l.DtctlVersion)

	// Merge recipes after all registries are known, so user and app domains can cross-reference.
	if !l.NoBuiltinRecipes {
		b.addRecipes(builtin.recipes)
	}
	b.addRecipes(bundles)
	for _, fc := range files {
		b.addRecipes(fc.recipes)
	}
	return b
}

func (b *Book) problem(src Source, format string, args ...any) {
	b.Problems = append(b.Problems, Problem{Source: src, Message: fmt.Sprintf(format, args...)})
}

// readFileLayer parses one directory layer, silently skipping files of other kinds.
func (b *Book) readFileLayer(fl FileLayer) layerContent {
	var lc layerContent
	if fl.FS == nil {
		return lc
	}
	var paths []string
	_ = fs.WalkDir(fl.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return fs.SkipAll // the layer's directory does not exist
			}
			b.problem(Source{Layer: fl.Layer, Location: path.Join(fl.Root, p), Name: fl.Name, Pin: fl.Pin}, "%v", err)
			return nil
		}
		if d.IsDir() {
			if p != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	sort.Strings(paths)
	seen := map[string]string{}
	for _, p := range paths {
		src := Source{Layer: fl.Layer, Location: path.Join(fl.Root, p), Name: fl.Name, Pin: fl.Pin}
		data, err := fs.ReadFile(fl.FS, p)
		if err != nil {
			b.problem(src, "%v", err)
			continue
		}
		switch {
		case path.Dir(p) == "_fragments" && strings.HasSuffix(p, ".tmpl"):
			lc.fragments = append(lc.fragments, fragment{text: string(data), source: src})
		case strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml"):
			b.readFile(&lc, data, src, seen)
		}
	}
	return lc
}

func (b *Book) readFile(lc *layerContent, data []byte, src Source, seen map[string]string) {
	var head struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		b.problem(src, "invalid YAML: %v", err)
		return
	}
	switch head.Kind {
	case KindRecipe, KindDomains, KindScopes:
	default:
		return
	}
	if head.APIVersion != APIVersion {
		b.problem(src, "apiVersion %q is not supported (want %s)", head.APIVersion, APIVersion)
		return
	}
	switch head.Kind {
	case KindDomains:
		var doc domainsDoc
		if err := decodeStrict(data, &doc); err != nil {
			b.problem(src, "%v", err)
			return
		}
		for name, desc := range doc.Domains {
			lc.domains = append(lc.domains, &Domain{Name: name, Description: desc, Source: src})
		}
	case KindScopes:
		var doc scopesDoc
		if err := decodeStrict(data, &doc); err != nil {
			b.problem(src, "%v", err)
			return
		}
		for name, dim := range doc.Scopes {
			if dim == nil {
				continue
			}
			dim.Name, dim.Source = name, src
			lc.scopes = append(lc.scopes, dim)
		}
	case KindRecipe:
		r, err := ParseRecipe(data)
		if err != nil {
			b.problem(src, "%v", err)
			return
		}
		r.Source = src
		if prev, dup := seen[r.Name()]; dup {
			b.problem(src, "recipe %q is already defined in %s; skipped", r.Name(), prev)
			return
		}
		seen[r.Name()] = src.Location
		lc.recipes = append(lc.recipes, r)
	}
}

// ParseRecipe decodes one Recipe document. It checks the envelope only;
// Book.Validate checks the content against the registries.
func ParseRecipe(data []byte) (*Recipe, error) {
	var r Recipe
	if err := decodeStrict(data, &r); err != nil {
		return nil, err
	}
	if r.APIVersion != APIVersion {
		return nil, fmt.Errorf("apiVersion %q is not supported (want %s)", r.APIVersion, APIVersion)
	}
	if r.Kind != KindRecipe {
		return nil, fmt.Errorf("kind is %q, want %s", r.Kind, KindRecipe)
	}
	return &r, nil
}

// ParseBundle decodes one RecipeBundle document.
func ParseBundle(data []byte) (*Bundle, error) {
	var bd Bundle
	if err := decodeStrict(data, &bd); err != nil {
		return nil, err
	}
	if bd.APIVersion != APIVersion {
		return nil, fmt.Errorf("apiVersion %q is not supported (want %s)", bd.APIVersion, APIVersion)
	}
	if bd.Kind != KindBundle {
		return nil, fmt.Errorf("kind is %q, want %s", bd.Kind, KindBundle)
	}
	if bd.Metadata.Name == "" {
		return nil, fmt.Errorf("metadata.name is required")
	}
	return &bd, nil
}

// decodeStrict rejects unknown fields so a typo fails loudly.
func decodeStrict(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid content: %w", err)
	}
	return nil
}

// mergeRegistries adds a layer's domains, scopes and fragments. Only the built-in
// layer may redefine a name: a change would silently alter recipes it never overrode.
func (b *Book) mergeRegistries(lc layerContent, builtin bool) {
	for _, d := range lc.domains {
		if !domainRe.MatchString(d.Name) {
			b.problem(d.Source, "domain %q: a domain name is lowercase letters and digits (it is the part of a recipe name before the first '-')", d.Name)
			continue
		}
		if prev, ok := b.Domains[d.Name]; ok && !builtin {
			b.problem(d.Source, "domain %q is already defined by %s; a layer may add domains, not redefine them", d.Name, prev.Source.String())
			continue
		}
		b.Domains[d.Name] = d
	}
	for _, s := range lc.scopes {
		if prev, ok := b.Scopes[s.Name]; ok && !builtin {
			b.problem(s.Source, "scope dimension %q is already defined by %s", s.Name, prev.Source.String())
			continue
		}
		if err := validateScope(s); err != nil {
			b.problem(s.Source, "scope dimension %q: %v", s.Name, err)
			continue
		}
		b.Scopes[s.Name] = s
	}
	for _, f := range lc.fragments {
		if dup := b.fragmentConflict(f.text); dup != "" {
			b.problem(f.source, "fragment %q is already defined; a layer may add fragments, not redefine them", dup)
			continue
		}
		b.fragments = append(b.fragments, f)
	}
}

// fragmentConflict reports a {{define}} name in text that an existing fragment defines.
func (b *Book) fragmentConflict(text string) string {
	names, err := defineNames(text)
	if err != nil {
		return ""
	}
	existing := map[string]bool{}
	for _, f := range b.fragments {
		ns, _ := defineNames(f.text)
		for _, n := range ns {
			existing[n] = true
		}
	}
	for _, n := range names {
		if existing[n] {
			return n
		}
	}
	return ""
}

func validateScope(s *ScopeDim) error {
	if s.KeyValue() {
		if !strings.Contains(s.FieldPattern, "{key}") {
			return fmt.Errorf("a key=value dimension needs a fieldPattern with {key}")
		}
		return nil
	}
	if s.Field == "" {
		return fmt.Errorf("field is required")
	}
	return nil
}

// readBundles parses the environment layer. A name two bundles both claim is
// loaded from neither, so meaning cannot depend on install order.
func (b *Book) readBundles(docs []BundleDoc, dtctlVersion string) []*Recipe {
	type parsed struct {
		bundle *Bundle
		src    Source
	}
	var ok []parsed
	for _, d := range docs {
		bd, err := ParseBundle(d.Content)
		if err != nil {
			b.problem(d.Source, "%v", err)
			continue
		}
		src := d.Source
		if src.BundleVersion == 0 {
			src.BundleVersion = bd.Metadata.Version
		}
		if min := bd.Metadata.MinDtctlVersion; min != "" && dtctlVersion != "" && versionLess(dtctlVersion, min) {
			b.problem(src, "bundle %q needs dtctl %s or newer (this is %s); skipped", bd.Metadata.Name, min, dtctlVersion)
			continue
		}
		ok = append(ok, parsed{bd, src})
	}

	claims := func(get func(*Bundle) []string) map[string][]Source {
		m := map[string][]Source{}
		for _, p := range ok {
			for _, n := range get(p.bundle) {
				m[n] = append(m[n], p.src)
			}
		}
		return m
	}
	recipeClaims := claims(func(bd *Bundle) []string {
		var ns []string
		for _, r := range bd.Spec.Recipes {
			ns = append(ns, r.Metadata.Name)
		}
		return ns
	})
	domainClaims := claims(func(bd *Bundle) []string { return sortedKeys(bd.Spec.Domains) })
	capClaims := claims(func(bd *Bundle) []string { return sortedKeys(bd.Spec.Capabilities) })
	conflict := func(kind, name string, srcs []Source) bool {
		if len(srcs) < 2 {
			return false
		}
		names := make([]string, len(srcs))
		for i, s := range srcs {
			names[i] = s.String()
		}
		b.problem(srcs[0], "%s %q is defined by %s; loaded from neither", kind, name, strings.Join(names, " and "))
		return true
	}

	builtinCaps := inventory.BuiltinDefinitions()
	var recipes []*Recipe
	reported := map[string]bool{}
	for _, p := range ok {
		for _, name := range sortedKeys(p.bundle.Spec.Domains) {
			if len(domainClaims[name]) > 1 {
				if !reported["d:"+name] {
					reported["d:"+name] = conflict("domain", name, domainClaims[name])
				}
				continue
			}
			if !domainRe.MatchString(name) {
				b.problem(p.src, "domain %q: a domain name is lowercase letters and digits (it is the part of a recipe name before the first '-')", name)
				continue
			}
			if prev, exists := b.Domains[name]; exists {
				b.problem(p.src, "domain %q is already defined by %s; a bundle may add domains, not redefine them", name, prev.Source.String())
				continue
			}
			b.Domains[name] = &Domain{Name: name, Description: p.bundle.Spec.Domains[name], Source: p.src}
		}
		for _, name := range sortedKeys(p.bundle.Spec.Capabilities) {
			if len(capClaims[name]) > 1 {
				if !reported["c:"+name] {
					reported["c:"+name] = conflict("capability", name, capClaims[name])
				}
				continue
			}
			if _, exists := builtinCaps[name]; exists {
				b.problem(p.src, "capability %q is built in; a bundle may add capabilities, not redefine them", name)
				continue
			}
			def := p.bundle.Spec.Capabilities[name]
			if err := inventory.ValidateDefinitions(map[string]*inventory.CapabilityDef{name: def}); err != nil {
				b.problem(p.src, "%v", err)
				continue
			}
			b.Capabilities[name] = def
			b.CapabilitySources[name] = p.src
		}
		var frags []string
		if strings.TrimSpace(p.bundle.Spec.Fragments) != "" {
			frags = []string{p.bundle.Spec.Fragments}
		}
		for i := range p.bundle.Spec.Recipes {
			br := p.bundle.Spec.Recipes[i]
			if len(recipeClaims[br.Metadata.Name]) > 1 {
				if !reported["r:"+br.Metadata.Name] {
					reported["r:"+br.Metadata.Name] = conflict("recipe", br.Metadata.Name, recipeClaims[br.Metadata.Name])
				}
				continue
			}
			recipes = append(recipes, &Recipe{
				APIVersion: APIVersion, Kind: KindRecipe,
				Metadata: br.Metadata, Spec: br.Spec,
				Source: p.src, fragments: frags,
			})
		}
	}
	return recipes
}

// addRecipes validates and adds one layer's recipes, replacing weaker same-name ones.
func (b *Book) addRecipes(rs []*Recipe) {
	for _, r := range rs {
		if err := b.Validate(r); err != nil {
			b.problem(r.Source, "recipe %q: %v", r.Name(), err)
			continue
		}
		if prev, ok := b.Recipes[r.Name()]; ok {
			if layerRank(prev.Source.Layer) > layerRank(r.Source.Layer) {
				continue
			}
			r.Shadows = append(append([]Source{}, prev.Shadows...), prev.Source)
		}
		b.Recipes[r.Name()] = r
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// versionLess compares dotted numeric versions, ignoring a leading "v" and pre-release suffix.
func versionLess(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		n := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out
}
