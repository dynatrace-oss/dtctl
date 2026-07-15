package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeLookPath resolves only the given names, mimicking exec.LookPath.
func fakeLookPath(available ...string) func(string) (string, error) {
	set := make(map[string]bool, len(available))
	for _, a := range available {
		set[a] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/fake/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}

func TestResolve_LongestMatchWins(t *testing.T) {
	look := fakeLookPath("dtctl-foo", "dtctl-foo-bar")

	inv, ok := Resolve([]string{"foo", "bar", "baz"}, nil, look)
	if !ok {
		t.Fatal("expected a match")
	}
	if inv.Path != "/fake/bin/dtctl-foo-bar" {
		t.Errorf("Path = %q, want dtctl-foo-bar (longest dash-joined match)", inv.Path)
	}
	if len(inv.Args) != 1 || inv.Args[0] != "baz" {
		t.Errorf("Args = %v, want [baz]", inv.Args)
	}
}

func TestResolve_FallsBackToShorterMatch(t *testing.T) {
	look := fakeLookPath("dtctl-foo")

	inv, ok := Resolve([]string{"foo", "bar", "baz"}, []string{"--flag"}, look)
	if !ok {
		t.Fatal("expected a match")
	}
	if inv.Path != "/fake/bin/dtctl-foo" {
		t.Errorf("Path = %q, want dtctl-foo", inv.Path)
	}
	want := []string{"bar", "baz", "--flag"}
	if len(inv.Args) != len(want) {
		t.Fatalf("Args = %v, want %v", inv.Args, want)
	}
	for i := range want {
		if inv.Args[i] != want[i] {
			t.Fatalf("Args = %v, want %v", inv.Args, want)
		}
	}
}

func TestResolve_NoMatch(t *testing.T) {
	if _, ok := Resolve([]string{"nosuch"}, nil, fakeLookPath()); ok {
		t.Error("expected no match")
	}
	if _, ok := Resolve(nil, nil, fakeLookPath("dtctl-foo")); ok {
		t.Error("expected no match for empty words")
	}
}

// writePlugin creates an executable plugin file in dir.
func writePlugin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix executable-bit semantics")
	}
	dir1, dir2 := t.TempDir(), t.TempDir()
	writePlugin(t, dir1, "dtctl-foo")
	writePlugin(t, dir1, "dtctl-foo-bar")
	writePlugin(t, dir2, "dtctl-foo") // shadowed by dir1's dtctl-foo
	writePlugin(t, dir2, "dtctl-get") // collides with a built-in
	writePlugin(t, dir2, "not-a-plugin")
	// Non-executable files are not plugins.
	if err := os.WriteFile(filepath.Join(dir1, "dtctl-noexec"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	pathEnv := strings.Join([]string{dir1, dir2}, string(os.PathListSeparator))
	plugins := Discover(pathEnv, map[string]bool{"get": true})

	byName := map[string]Plugin{}
	for _, p := range plugins {
		byName[p.Name] = p
	}
	if len(plugins) != 3 {
		t.Fatalf("found %d plugins (%v), want 3", len(plugins), byName)
	}
	if p := byName["foo"]; !strings.HasPrefix(p.Path, dir1) {
		t.Errorf("dtctl-foo should resolve to the first PATH entry, got %s", p.Path)
	}
	if p := byName["foo bar"]; p.Binary != "dtctl-foo-bar" {
		t.Errorf("multi-word plugin missing: %+v", byName)
	}
	if p := byName["get"]; !strings.Contains(p.Warning, "built-in") {
		t.Errorf("expected shadow warning on dtctl-get, got %+v", p)
	}
	// Deterministic order.
	for i := 1; i < len(plugins); i++ {
		if plugins[i-1].Name > plugins[i].Name {
			t.Errorf("plugins not sorted: %v", plugins)
		}
	}
}
