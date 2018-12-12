package vfsutil

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
)

func TestGitHubRepoVFS(t *testing.T) {
	// Ensure fetch logic works
	cleanup := useEmptyArchiveCacheDir()
	defer cleanup()

	// Any public repo will work.
	fs, err := NewGitHubRepoVFS(context.Background(), "github.com/gorilla/schema", "0164a00ab4cd01d814d8cd5bf63fd9fcea30e23b")
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	want := map[string]string{
		"/LICENSE":         "...",
		"/README.md":       "schema...",
		"/cache.go":        "// Copyright...",
		"/converter.go":    "// Copyright...",
		"/decoder.go":      "// Copyright...",
		"/decoder_test.go": "// Copyright...",
		"/doc.go":          "// Copyright...",
		"/.travis.yml":     "...",
	}

	testVFS(t, fs, want)
}

func useEmptyArchiveCacheDir() func() {
	d, err := ioutil.TempDir("", "vfsutil_test")
	if err != nil {
		panic(err)
	}
	orig := ArchiveCacheDir
	ArchiveCacheDir = d
	return func() {
		os.RemoveAll(d)
		ArchiveCacheDir = orig
	}
}
