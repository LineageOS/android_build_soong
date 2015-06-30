package main

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

	bpparser "github.com/google/blueprint/parser"
)

var valueTestCases = []struct {
	blueprint string
	expected  string
}{
	{
		blueprint: `test = false`,
		expected:  `false`,
	},
	{
		blueprint: `test = "string"`,
		expected:  `string`,
	},
	{
		blueprint: `test = ["a", "b"]`,
		expected: `\
		           a \
		           b
		           `,
	},
}

func TestValueToString(t *testing.T) {
	for _, testCase := range valueTestCases {
		blueprint, errs := bpparser.Parse("", strings.NewReader(testCase.blueprint), nil)
		if len(errs) > 0 {
			t.Errorf("Failed to read blueprint: %q", errs)
		}

		str, err := valueToString(blueprint.Defs[0].(*bpparser.Assignment).Value)
		if err != nil {
			t.Error(err.Error())
		}
		expect(t, testCase.blueprint, testCase.expected, str)
	}
}

var moduleTestCases = []struct {
	blueprint string
	androidmk string
}{
	// Target-only
	{
		blueprint: `cc_library_shared { name: "test", }`,
		androidmk: `include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_SHARED_LIBRARY)`,
	},
	// Host-only
	{
		blueprint: `cc_library_host_shared { name: "test", }`,
		androidmk: `include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_HOST_SHARED_LIBRARY)`,
	},
	// Target and Host
	{
		blueprint: `cc_library_shared { name: "test", host_supported: true, }`,
		androidmk: `include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_SHARED_LIBRARY)

			    include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_HOST_SHARED_LIBRARY)`,
	},
	// Static and Shared
	{
		blueprint: `cc_library { name: "test", }`,
		androidmk: `include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_SHARED_LIBRARY)

			    include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_STATIC_LIBRARY)`,
	},
	// Static and Shared / Target and Host
	{
		blueprint: `cc_library { name: "test", host_supported: true, }`,
		androidmk: `include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_SHARED_LIBRARY)

			    include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_STATIC_LIBRARY)

			    include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_HOST_SHARED_LIBRARY)

			    include $(CLEAR_VARS)
			    LOCAL_MODULE := test
			    include $(BUILD_HOST_STATIC_LIBRARY)`,
	},
	// Manual translation
	{
		blueprint: `/* Android.mk:start
					# Manual translation
					Android.mk:end */
					cc_library { name: "test", host_supported: true, }`,
		androidmk: `# Manual translation`,
	},
	// Ignored translation
	{
		blueprint: `/* Android.mk:ignore */
					cc_library { name: "test", host_supported: true, }`,
		androidmk: ``,
	},
}

func TestModules(t *testing.T) {
	for _, testCase := range moduleTestCases {
		blueprint, errs := bpparser.Parse("", strings.NewReader(testCase.blueprint), nil)
		if len(errs) > 0 {
			t.Errorf("Failed to read blueprint: %q", errs)
		}

		buf := &bytes.Buffer{}
		writer := &androidMkWriter{
			blueprint: blueprint,
			path:      "",
			Writer:    buf,
		}

		module := blueprint.Defs[0].(*bpparser.Module)
		err := writer.handleModule(module)
		if err != nil {
			t.Errorf("Unexpected error %s", err.Error())
		}

		expect(t, testCase.blueprint, testCase.androidmk, buf.String())
	}
}

// Trim left whitespace, and any trailing newlines. Leave inner blank lines and
// right whitespace so that we can still check line continuations are correct
func trim(str string) string {
	var list []string
	for _, s := range strings.Split(str, "\n") {
		list = append(list, strings.TrimLeftFunc(s, unicode.IsSpace))
	}
	return strings.TrimRight(strings.Join(list, "\n"), "\n")
}

func expect(t *testing.T, testCase string, expected string, out string) {
	expected = trim(expected)
	out = trim(out)
	if expected != out {
		sep := " "
		if strings.Index(expected, "\n") != -1 || strings.Index(out, "\n") != -1 {
			sep = "\n"
		}

		t.Errorf("test case: %s", testCase)
		t.Errorf("unexpected difference:")
		t.Errorf("  expected:%s%s", sep, expected)
		t.Errorf("       got:%s%s", sep, out)
	}
}
