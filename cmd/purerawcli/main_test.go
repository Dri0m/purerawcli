package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestFlagRanges(t *testing.T) {
	out := t.TempDir()
	for _, args := range [][]string{
		{"-luminance", "101"},
		{"-force-details", "-101"},
		{"-lens-sharpness-intensity", "201"},
		{"-distortion-crop", "3"},
		{"-dust-sensitivity", "-1"},
		{"-dng-compression", "2"},
		{"-jpeg-quality", "9"},
		{"-smart-lighting-intensity", "101"},
		{"-timeout", "-1s"},
	} {
		os.Args = append(append([]string{"purerawcli", "-o", out}, args...), "x.ARW")
		if code := run(); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestCollectInputs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.ARW", "a.nef", "._a.ARW", "notes.txt", "a.ARW.xmp"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := collectInputs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.nef"), filepath.Join(dir, "b.ARW")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := collectInputs([]string{filepath.Join(dir, "notes.txt")}); err == nil {
		t.Error("explicit non-raw file accepted")
	}
}

func TestCheckCollisionsKeepsInputs(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.dng")
	if err := os.WriteFile(in, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	dng := map[string]bool{"dng": true}
	if _, err := checkCollisions([]string{in}, dir, dng, true, false); err == nil {
		t.Error("output replacing its input accepted with -overwrite")
	}
	if got, err := checkCollisions([]string{in}, dir, map[string]bool{"tiff": true}, false, false); err != nil || len(got) != 1 {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestCollectInputsWildcards(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the shell expands wildcards outside Windows")
	}
	dir := t.TempDir()
	for _, name := range []string{"b.ARW", "a.ARW", "a.nef", "._a.ARW"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := collectInputs([]string{filepath.Join(dir, "*.arw")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.ARW"), filepath.Join(dir, "b.ARW")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := collectInputs([]string{filepath.Join(dir, "*.CR3")}); err == nil {
		t.Error("wildcard without matches accepted")
	}
}
