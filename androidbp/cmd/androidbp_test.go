package main

import (
	"strings"
	"testing"

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
		if str != testCase.expected {
			t.Errorf("test case: %s", testCase.blueprint)
			t.Errorf("unexpected difference:")
			t.Errorf("  expected: %s", testCase.expected)
			t.Errorf("       got: %s", str)
		}
	}
}
