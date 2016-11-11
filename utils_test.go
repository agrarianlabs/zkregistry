package zkregistry

import (
	"fmt"
	"testing"
)

func TestSanitizePath(t *testing.T) {
	for testPath, expect := range map[string]string{
		"":          "",
		"/":         "",
		"/a":        "a",
		"/a/":       "a",
		"a/":        "a",
		"/a/b":      "a/b",
		"/a/b/":     "a/b",
		"/a/b/c/d/": "a/b/c/d",
		"a/b/c/d/e": "a/b/c/d/e",
	} {
		if got := sanitizePath(testPath); expect != got {
			t.Errorf("Unexpected result.\nExpect:\t%s\nGot:\t%s", expect, got)
		}
	}
}

func TestParseConfigPath(t *testing.T) {
	for _, elem := range []struct {
		path    string // Input.
		offset  uint   // Input.
		name    string // Expected value.
		version string // Expected value.
		addr    string // Expected value.
		err     error  // Expected value.
	}{
		{"/name/version/addr:0", 0, "name", "version", "addr:0", nil},
		{"name/version/addr", 0, "name", "version", "addr", nil},
		{"/name/version/", 0, "name", "version", "", nil},
		{"/name", 0, "name", "", "", nil},
		{"/test/name/version/addr:0", 1, "name", "version", "addr:0", nil},
		{"/company/platform/test/name/version/addr:0", 3, "name", "version", "addr:0", nil},

		{"/", 0, "", "", "", nil},
		{"", 0, "", "", "", nil},
		{"/", 1, "", "", "", nil},

		{"/name/version/addr/extra/extra", 0, "", "", "",
			fmt.Errorf("invalid path received: \"name/version/addr/extra/extra\"")},
		{"/name/version/addr", 42, "", "", "",
			fmt.Errorf("invalid path received: \"name/version/addr\"")},
		{"", 42, "", "", "",
			fmt.Errorf("invalid path received: \"\"")},
	} {
		name, version, addr, err := parseConfigPath(elem.path, elem.offset)
		if expect, got := elem.err, err; expect != nil && got != nil {
			if expect.Error() != got.Error() {
				t.Errorf("[%s|%d] Unexpected error.\nExpect:\t%s\nGot:\t%s", elem.path, elem.offset, expect, got)
			}
		} else if elem.err != err && (elem.err != nil || err != nil) {
			t.Errorf("[%s|%d] Unexpected error.\nExpect:\t%v\nGot:\t%v", elem.path, elem.offset, expect, got)
		}

		if expect, got := elem.name, name; expect != got {
			t.Errorf("[%s|%d] Unexpected name.\nExpect:\t%s\nGot:\t%s", elem.path, elem.offset, expect, got)
		}
		if expect, got := elem.version, version; expect != got {
			t.Errorf("[%s|%d] Unexpected version.\nExpect:\t%s\nGot:\t%s", elem.path, elem.offset, expect, got)
		}
		if expect, got := elem.addr, addr; expect != got {
			t.Errorf("[%s|%d] Unexpected addr.\nExpect:\t%s\nGot:\t%s", elem.path, elem.offset, expect, got)
		}
	}
}
