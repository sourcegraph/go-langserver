package suggest_test

import (
	"bytes"
	"encoding/json"
	"go/importer"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sourcegraph/go-langserver/langserver/internal/gocode/suggest"
)

func TestRegress(t *testing.T) {
	testDirs, err := filepath.Glob("testdata/test.*")
	if err != nil {
		t.Fatal(err)
	}

	for _, testDir := range testDirs {
		testDir := testDir // capture
		name := strings.TrimPrefix(testDir, "testdata/")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testRegress(t, testDir)
		})
	}
}

func testRegress(t *testing.T, testDir string) {
	testDir, err := filepath.Abs(testDir)
	if err != nil {
		t.Errorf("Abs failed: %v", err)
		return
	}

	filename := filepath.Join(testDir, "test.go.in")
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Errorf("ReadFile failed: %v", err)
		return
	}

	cursor := bytes.IndexByte(data, '@')
	if cursor < 0 {
		t.Errorf("Missing @")
		return
	}
	data = append(data[:cursor], data[cursor+1:]...)

	cfg := suggest.Config{
		Importer: importer.Default(),
	}
	if testing.Verbose() {
		cfg.Logf = t.Logf
	}
	if cfgJSON, err := os.Open(filepath.Join(testDir, "config.json")); err == nil {
		if err := json.NewDecoder(cfgJSON).Decode(&cfg); err != nil {
			t.Errorf("Decode failed: %v", err)
			return
		}
	} else if !os.IsNotExist(err) {
		t.Errorf("Open failed: %v", err)
		return
	}
	candidates, prefixLen, err := cfg.Suggest(filename, data, cursor)
	if err != nil {
		t.Fatalf("could not get suggestions: %v", err)
	}

	var out bytes.Buffer
	suggest.NiceFormat(&out, candidates, prefixLen)

	want, _ := ioutil.ReadFile(filepath.Join(testDir, "out.expected"))
	if got := out.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("%s:\nGot:\n%s\nWant:\n%s\n", testDir, got, want)
		return
	}
}
