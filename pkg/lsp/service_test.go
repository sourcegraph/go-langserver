package lsp

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestTextDocumentSyncOptionsOrKind_MarshalUnmarshalJSON(t *testing.T) {
	kindPtr := func(kind TextDocumentSyncKind) *TextDocumentSyncKind {
		return &kind
	}

	tests := []struct {
		data []byte
		want TextDocumentSyncOptionsOrKind
	}{
		{
			data: []byte(`null`),
			want: TextDocumentSyncOptionsOrKind{},
		},
		{
			data: []byte(`2`),
			want: TextDocumentSyncOptionsOrKind{
				Options: &TextDocumentSyncOptions{
					OpenClose: true,
					Change:    TDSKIncremental,
				},
				Kind: kindPtr(2),
			},
		},
		{
			data: []byte(`{"openClose":true,"change":1,"save":{"includeText":true}}`),
			want: TextDocumentSyncOptionsOrKind{
				Options: &TextDocumentSyncOptions{
					OpenClose: true,
					Change:    TDSKFull,
					Save:      &SaveOptions{IncludeText: true},
				},
			},
		},
	}
	for _, test := range tests {
		var got TextDocumentSyncOptionsOrKind
		if err := json.Unmarshal(test.data, &got); err != nil {
			t.Error(err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("got %+v, want %+v", got, test.want)
			continue
		}
		data, err := json.Marshal(got)
		if err != nil {
			t.Error(err)
			continue
		}
		if !bytes.Equal(data, test.data) {
			t.Errorf("got JSON %q, want %q", data, test.data)
		}
	}
}

func TestMarkedString_MarshalUnmarshalJSON(t *testing.T) {
	tests := []struct {
		data []byte
		want MarkedString
	}{{
		data: []byte(`{"language":"go","value":"foo"}`),
		want: MarkedString{Language: "go", Value: "foo", isRawString: false},
	}, {
		data: []byte(`{"language":"","value":"foo"}`),
		want: MarkedString{Language: "", Value: "foo", isRawString: false},
	}, {
		data: []byte(`"foo"`),
		want: MarkedString{Language: "", Value: "foo", isRawString: true},
	}}

	for _, test := range tests {
		var m MarkedString
		if err := json.Unmarshal(test.data, &m); err != nil {
			t.Errorf("json.Unmarshal error: %s", err)
			continue
		}
		if !reflect.DeepEqual(test.want, m) {
			t.Errorf("Unmarshaled %q, expected %+v, but got %+v", string(test.data), test.want, m)
			continue
		}

		marshaled, err := json.Marshal(m)
		if err != nil {
			t.Errorf("json.Marshal error: %s", err)
			continue
		}
		if string(marshaled) != string(test.data) {
			t.Errorf("Marshaled result expected %s, but got %s", string(test.data), string(marshaled))
		}
	}
}

func TestMarkdownString_MarshalUnmarshalJSON(t *testing.T) {
	tests := []struct {
		data []byte
		want MarkdownString
	}{{
		data: []byte(`{"value":"## h2 heading"}`),
		want: MarkdownString{Value: "## h2 heading", IsTrusted: false},
	}, {
		data: []byte(`"# h1 heading"`),
		want: MarkdownString{Value: "# h1 heading", IsTrusted: true},
	},
	}

	for _, test := range tests {
		var m MarkdownString
		if err := json.Unmarshal(test.data, &m); err != nil {
			t.Errorf("json.Unmarshal error: %s", err)
			continue
		}
		if !reflect.DeepEqual(test.want, m) {
			t.Errorf("Unmarshaled %q, expected %+v, but got %+v", string(test.data), test.want, m)
			continue
		}

	}
}
