package recipes

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Store is the content-addressed home of synced recipe content: a recipe tree
// per git or archive pin (tree/<digest>/) and a blob per app bundle version
// (blob/<digest>). Content never changes under a digest, so switching
// contexts, projects or pins back and forth never fetches anything twice.
type Store struct{ Dir string }

// Limits on what a fetched archive may contribute: recipes are small text.
const (
	maxTreeFiles = 5000
	maxTreeBytes = 32 << 20
	maxFileBytes = 1 << 20
)

// TreeDigest is the digest of a recipe tree: SHA-256 over its sorted paths
// and their contents' digests. It depends on content only, never on how the
// archive happened to be compressed or ordered.
func TreeDigest(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		sum := sha256.Sum256(files[p])
		fmt.Fprintf(h, "%s\x00%x\n", p, sum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BlobDigest is the digest of one bundle document's content.
func BlobDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s Store) treeDir(digest string) string { return filepath.Join(s.Dir, "tree", digest) }
func (s Store) blobPath(digest string) string {
	return filepath.Join(s.Dir, "blob", digest+".yaml")
}

// HasTree reports whether a tree is stored.
func (s Store) HasTree(digest string) bool {
	if !sha256Re.MatchString(digest) {
		return false
	}
	_, err := os.Stat(s.treeDir(digest))
	return err == nil
}

// Tree opens a stored tree.
func (s Store) Tree(digest string) (fs.FS, bool) {
	if !s.HasTree(digest) {
		return nil, false
	}
	return os.DirFS(s.treeDir(digest)), true
}

// PutTree stores a recipe tree and returns its digest. The write goes to a
// temporary directory renamed into place, so a reader never sees half a tree.
func (s Store) PutTree(files map[string][]byte) (string, error) {
	digest := TreeDigest(files)
	if s.HasTree(digest) {
		return digest, nil
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, "tree"), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(filepath.Join(s.Dir, "tree"), ".tmp-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for p, data := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, s.treeDir(digest)); err != nil && !s.HasTree(digest) {
		return "", err
	}
	return digest, nil
}

// Blob reads a stored bundle document.
func (s Store) Blob(digest string) ([]byte, bool) {
	if !sha256Re.MatchString(digest) {
		return nil, false
	}
	data, err := os.ReadFile(s.blobPath(digest))
	if err != nil || BlobDigest(data) != digest {
		return nil, false
	}
	return data, true
}

// PutBlob stores a bundle document and returns its digest.
func (s Store) PutBlob(data []byte) (string, error) {
	digest := BlobDigest(data)
	if _, ok := s.Blob(digest); ok {
		return digest, nil
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, "blob"), 0o700); err != nil {
		return "", err
	}
	tmp := s.blobPath(digest) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	return digest, os.Rename(tmp, s.blobPath(digest))
}

// ExtractRecipeTree reads a .tar.gz and returns the recipe content under
// root: .yaml/.yml/.tmpl regular files, re-rooted at root. stripTop drops the
// single top-level directory GitHub archives wrap everything in. Links,
// absolute paths and ".." are refused rather than skipped: an archive that
// carries them is not a recipe tree.
func ExtractRecipeTree(archive io.Reader, root string, stripTop bool) (map[string][]byte, error) {
	root, err := cleanRelPath(root)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		name := strings.TrimPrefix(h.Name, "./")
		if stripTop {
			if i := strings.IndexByte(name, '/'); i >= 0 {
				name = name[i+1:]
			} else {
				continue
			}
		}
		if name == "" || h.Typeflag == tar.TypeDir || h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if h.Typeflag != tar.TypeReg {
			if within(name, root) {
				return nil, fmt.Errorf("archive entry %q is not a regular file", name)
			}
			continue
		}
		clean, err := cleanRelPath(name)
		if err != nil {
			return nil, fmt.Errorf("archive entry %q: %v", h.Name, err)
		}
		if !within(clean, root) {
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(clean, root), "/")
		if !isRecipeContent(rel) {
			continue
		}
		if h.Size > maxFileBytes {
			return nil, fmt.Errorf("archive entry %q is larger than %d bytes", name, maxFileBytes)
		}
		total += h.Size
		if total > maxTreeBytes || len(files) >= maxTreeFiles {
			return nil, fmt.Errorf("archive recipe tree exceeds %d files or %d bytes", maxTreeFiles, maxTreeBytes)
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxFileBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}
		files[rel] = data
	}
	if len(files) == 0 {
		where := "the archive"
		if root != "" {
			where = root
		}
		return nil, fmt.Errorf("no recipe files (.yaml, _fragments/*.tmpl) under %s", where)
	}
	return files, nil
}

func isRecipeContent(rel string) bool {
	base := path.Base(rel)
	if strings.HasPrefix(base, ".") {
		return false
	}
	for _, part := range strings.Split(path.Dir(rel), "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return false
		}
	}
	return strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml") ||
		(path.Dir(rel) == "_fragments" && strings.HasSuffix(rel, ".tmpl"))
}

func within(p, root string) bool {
	return root == "" || p == root || strings.HasPrefix(p, root+"/")
}

// cleanRelPath normalizes a slash path that must stay inside its root.
func cleanRelPath(p string) (string, error) {
	p = strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" || p == "." {
		return "", nil
	}
	c := path.Clean(p)
	if c == ".." || strings.HasPrefix(c, "../") || path.IsAbs(p) {
		return "", fmt.Errorf("%q leaves its root", p)
	}
	return c, nil
}
