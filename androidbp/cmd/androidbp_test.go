package main

import (
	"bufio"
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
		blueprint: `test = Variable`,
		expected:  `$(Variable)`,
	},
	{
		blueprint: `test = "string"`,
		expected:  `string`,
	},
	{
		blueprint: `test = ["a", "b"]`,
		expected: `\
			   a \
			   b`,
	},
	{
		blueprint: `test = Var + "b"`,
		expected:  `$(Var)b`,
	},
	{
		blueprint: `test = ["a"] + ["b"]`,
		expected: `\
			   a\
			   b`,
	},
}

func TestValueToString(t *testing.T) {
	for _, testCase := range valueTestCases {
		blueprint, errs := bpparser.Parse("", strings.NewReader(testCase.blueprint), nil)
		if len(errs) > 0 {
			t.Errorf("Failed to read blueprint: %q", errs)
		}

		str := valueToString(blueprint.Defs[0].(*bpparser.Assignment).Value)
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
			mapScope:  make(map[string][]*bpparser.Property),
			Writer:    bufio.NewWriter(buf),
		}

		module := blueprint.Defs[0].(*bpparser.Module)
		writer.handleModule(module)
		writer.Flush()

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
