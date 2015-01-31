package parser

import (
	"strings"
	"testing"
)

var splitNTestCases = []struct {
	in       *MakeString
	expected []*MakeString
	sep      string
	n        int
}{
	{
		in: &MakeString{
			strings: []string{
				"a b c",
				"d e f",
				" h i j",
			},
			variables: []Variable{
				variable{name: SimpleMakeString("var1")},
				variable{name: SimpleMakeString("var2")},
			},
		},
		sep: " ",
		n:   -1,
		expected: []*MakeString{
			SimpleMakeString("a"),
			SimpleMakeString("b"),
			&MakeString{
				strings: []string{"c", "d"},
				variables: []Variable{
					variable{name: SimpleMakeString("var1")},
				},
			},
			SimpleMakeString("e"),
			&MakeString{
				strings: []string{"f", ""},
				variables: []Variable{
					variable{name: SimpleMakeString("var2")},
				},
			},
			SimpleMakeString("h"),
			SimpleMakeString("i"),
			SimpleMakeString("j"),
		},
	},
	{
		in: &MakeString{
			strings: []string{
				"a b c",
				"d e f",
				" h i j",
			},
			variables: []Variable{
				variable{name: SimpleMakeString("var1")},
				variable{name: SimpleMakeString("var2")},
			},
		},
		sep: " ",
		n:   3,
		expected: []*MakeString{
			SimpleMakeString("a"),
			SimpleMakeString("b"),
			&MakeString{
				strings: []string{"c", "d e f", " h i j"},
				variables: []Variable{
					variable{name: SimpleMakeString("var1")},
					variable{name: SimpleMakeString("var2")},
				},
			},
		},
	},
}

func TestMakeStringSplitN(t *testing.T) {
	for _, test := range splitNTestCases {
		got := test.in.SplitN(test.sep, test.n)
		gotString := dumpArray(got)
		expectedString := dumpArray(test.expected)
		if gotString != expectedString {
			t.Errorf("expected:\n%s\ngot:\n%s", expectedString, gotString)
		}
	}
}

func dumpArray(a []*MakeString) string {
	ret := make([]string, len(a))

	for i, s := range a {
		ret[i] = s.Dump()
	}

	return strings.Join(ret, "|||")
}
