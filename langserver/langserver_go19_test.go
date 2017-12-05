// +build go1.9

package langserver

func init() {
	serverTestCases["go1.9 type alias"] = serverTestCase{
		rootURI: "file:///src/test/pkg",
		fs: map[string]string{
			"a.go": "package p; type A struct{ a int }",
			"b.go": "package p; type B = A",
		},
		cases: lspTestCases{
			overrideGodefHover: map[string]string{
				"a.go:1:17": "type A struct; struct{ a int }",
				"b.go:1:17": "type B A",
				"b.go:1:20": "",
				//"b.go:1:21": "/src/test/pkg/a.go:1:17-1:18",
			},
			wantHover: map[string]string{
				"a.go:1:17": "type A struct; struct {\n    a int\n}",
				"b.go:1:17": "type B struct; struct {\n    a int\n}",
				"b.go:1:20": "",
				"b.go:1:21": "type A struct; struct {\n    a int\n}",
			},
			wantDefinition: map[string]string{
				"a.go:1:17": "/src/test/pkg/a.go:1:17-1:18",
				"b.go:1:17": "/src/test/pkg/b.go:1:17-1:18",
				"b.go:1:20": "",
				//"b.go:1:21": "/src/test/pkg/a.go:1:17-1:18", // Currently fails with no identifier found
			},
		},
	}
}
