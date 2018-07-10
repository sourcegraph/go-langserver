package langserver

import (
	"reflect"
	"testing"
)

func TestParseFuncArgs(t *testing.T) {
	got := parseFuncArgs("func(a int, b bool, c interface{}, d string...) ([]string, error)")
	want := []string{"a int", "b bool", "c interface{}", "d string..."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Wrong function args parsed. got: %s want: %s", got, want)
	}
}

func TestGenSnippetArgs(t *testing.T) {
	got := genSnippetArgs([]string{"a int", "b bool", "c interface{}", "d string..."})
	want := []string{"${1:a int}", "${2:b bool}", "${3:c interface{\\}}", "${4:d string...}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Wrong snippet args. got: %s want: %s", got, want)
	}
}
