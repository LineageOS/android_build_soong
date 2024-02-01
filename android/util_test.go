// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"cmp"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

var firstUniqueStringsTestCases = []struct {
	in  []string
	out []string
}{
	{
		in:  []string{"a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b", "a"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"b", "a", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"a", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "b", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"liblog", "libdl", "libc++", "libdl", "libc", "libm"},
		out: []string{"liblog", "libdl", "libc++", "libc", "libm"},
	},
}

func TestFirstUniqueStrings(t *testing.T) {
	f := func(t *testing.T, imp func([]string) []string, in, want []string) {
		t.Helper()
		out := imp(in)
		if !reflect.DeepEqual(out, want) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", in)
			t.Errorf("  expected: %#v", want)
			t.Errorf("       got: %#v", out)
		}
	}

	for _, testCase := range firstUniqueStringsTestCases {
		t.Run("list", func(t *testing.T) {
			f(t, firstUniqueList[string], testCase.in, testCase.out)
		})
		t.Run("map", func(t *testing.T) {
			f(t, firstUniqueMap[string], testCase.in, testCase.out)
		})
	}
}

var lastUniqueStringsTestCases = []struct {
	in  []string
	out []string
}{
	{
		in:  []string{"a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "a"},
		out: []string{"a"},
	},
	{
		in:  []string{"a", "b", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"b", "a", "a"},
		out: []string{"b", "a"},
	},
	{
		in:  []string{"a", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"a", "b", "a", "b"},
		out: []string{"a", "b"},
	},
	{
		in:  []string{"liblog", "libdl", "libc++", "libdl", "libc", "libm"},
		out: []string{"liblog", "libc++", "libdl", "libc", "libm"},
	},
}

func TestLastUniqueStrings(t *testing.T) {
	for _, testCase := range lastUniqueStringsTestCases {
		out := LastUniqueStrings(testCase.in)
		if !reflect.DeepEqual(out, testCase.out) {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", out)
		}
	}
}

func TestJoinWithPrefix(t *testing.T) {
	testcases := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "zero_inputs",
			input:    []string{},
			expected: "",
		},
		{
			name:     "one_input",
			input:    []string{"a"},
			expected: "prefix:a",
		},
		{
			name:     "two_inputs",
			input:    []string{"a", "b"},
			expected: "prefix:a prefix:b",
		},
	}

	prefix := "prefix:"

	for _, testCase := range testcases {
		t.Run(testCase.name, func(t *testing.T) {
			out := JoinWithPrefix(testCase.input, prefix)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("     input: %#v", testCase.input)
				t.Errorf("    prefix: %#v", prefix)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestIndexList(t *testing.T) {
	input := []string{"a", "b", "c"}

	testcases := []struct {
		key      string
		expected int
	}{
		{
			key:      "a",
			expected: 0,
		},
		{
			key:      "b",
			expected: 1,
		},
		{
			key:      "c",
			expected: 2,
		},
		{
			key:      "X",
			expected: -1,
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.key, func(t *testing.T) {
			out := IndexList(testCase.key, input)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("       key: %#v", testCase.key)
				t.Errorf("     input: %#v", input)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestInList(t *testing.T) {
	input := []string{"a"}

	testcases := []struct {
		key      string
		expected bool
	}{
		{
			key:      "a",
			expected: true,
		},
		{
			key:      "X",
			expected: false,
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.key, func(t *testing.T) {
			out := InList(testCase.key, input)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("       key: %#v", testCase.key)
				t.Errorf("     input: %#v", input)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestPrefixInList(t *testing.T) {
	prefixes := []string{"a", "b"}

	testcases := []struct {
		str      string
		expected bool
	}{
		{
			str:      "a-example",
			expected: true,
		},
		{
			str:      "b-example",
			expected: true,
		},
		{
			str:      "X-example",
			expected: false,
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.str, func(t *testing.T) {
			out := HasAnyPrefix(testCase.str, prefixes)
			if out != testCase.expected {
				t.Errorf("incorrect output:")
				t.Errorf("       str: %#v", testCase.str)
				t.Errorf("  prefixes: %#v", prefixes)
				t.Errorf("  expected: %#v", testCase.expected)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestFilterList(t *testing.T) {
	input := []string{"a", "b", "c", "c", "b", "d", "a"}
	filter := []string{"a", "c"}
	remainder, filtered := FilterList(input, filter)

	expected := []string{"b", "b", "d"}
	if !reflect.DeepEqual(remainder, expected) {
		t.Errorf("incorrect remainder output:")
		t.Errorf("     input: %#v", input)
		t.Errorf("    filter: %#v", filter)
		t.Errorf("  expected: %#v", expected)
		t.Errorf("       got: %#v", remainder)
	}

	expected = []string{"a", "c", "c", "a"}
	if !reflect.DeepEqual(filtered, expected) {
		t.Errorf("incorrect filtered output:")
		t.Errorf("     input: %#v", input)
		t.Errorf("    filter: %#v", filter)
		t.Errorf("  expected: %#v", expected)
		t.Errorf("       got: %#v", filtered)
	}
}

func TestFilterListPred(t *testing.T) {
	pred := func(s string) bool { return strings.HasPrefix(s, "a/") }
	AssertArrayString(t, "filter", FilterListPred([]string{"a/c", "b/a", "a/b"}, pred), []string{"a/c", "a/b"})
	AssertArrayString(t, "filter", FilterListPred([]string{"b/c", "a/a", "b/b"}, pred), []string{"a/a"})
	AssertArrayString(t, "filter", FilterListPred([]string{"c/c", "b/a", "c/b"}, pred), []string{})
	AssertArrayString(t, "filter", FilterListPred([]string{"a/c", "a/a", "a/b"}, pred), []string{"a/c", "a/a", "a/b"})
}

func TestRemoveListFromList(t *testing.T) {
	input := []string{"a", "b", "c", "d", "a", "c", "d"}
	filter := []string{"a", "c"}
	expected := []string{"b", "d", "d"}
	out := RemoveListFromList(input, filter)
	if !reflect.DeepEqual(out, expected) {
		t.Errorf("incorrect output:")
		t.Errorf("     input: %#v", input)
		t.Errorf("    filter: %#v", filter)
		t.Errorf("  expected: %#v", expected)
		t.Errorf("       got: %#v", out)
	}
}

func TestRemoveFromList(t *testing.T) {
	testcases := []struct {
		name          string
		key           string
		input         []string
		expectedFound bool
		expectedOut   []string
	}{
		{
			name:          "remove_one_match",
			key:           "a",
			input:         []string{"a", "b", "c"},
			expectedFound: true,
			expectedOut:   []string{"b", "c"},
		},
		{
			name:          "remove_three_matches",
			key:           "a",
			input:         []string{"a", "b", "a", "c", "a"},
			expectedFound: true,
			expectedOut:   []string{"b", "c"},
		},
		{
			name:          "remove_zero_matches",
			key:           "X",
			input:         []string{"a", "b", "a", "c", "a"},
			expectedFound: false,
			expectedOut:   []string{"a", "b", "a", "c", "a"},
		},
		{
			name:          "remove_all_matches",
			key:           "a",
			input:         []string{"a", "a", "a", "a"},
			expectedFound: true,
			expectedOut:   []string{},
		},
	}

	for _, testCase := range testcases {
		t.Run(testCase.name, func(t *testing.T) {
			found, out := RemoveFromList(testCase.key, testCase.input)
			if found != testCase.expectedFound {
				t.Errorf("incorrect output:")
				t.Errorf("       key: %#v", testCase.key)
				t.Errorf("     input: %#v", testCase.input)
				t.Errorf("  expected: %#v", testCase.expectedFound)
				t.Errorf("       got: %#v", found)
			}
			if !reflect.DeepEqual(out, testCase.expectedOut) {
				t.Errorf("incorrect output:")
				t.Errorf("       key: %#v", testCase.key)
				t.Errorf("     input: %#v", testCase.input)
				t.Errorf("  expected: %#v", testCase.expectedOut)
				t.Errorf("       got: %#v", out)
			}
		})
	}
}

func TestCopyOfEmptyAndNil(t *testing.T) {
	emptyList := []string{}
	copyOfEmptyList := CopyOf(emptyList)
	AssertBoolEquals(t, "Copy of an empty list should be an empty list and not nil", true, copyOfEmptyList != nil)
	copyOfNilList := CopyOf([]string(nil))
	AssertBoolEquals(t, "Copy of a nil list should be a nil list and not an empty list", true, copyOfNilList == nil)
}

func ExampleCopyOf() {
	a := []string{"1", "2", "3"}
	b := CopyOf(a)
	a[0] = "-1"
	fmt.Printf("a = %q\n", a)
	fmt.Printf("b = %q\n", b)

	// Output:
	// a = ["-1" "2" "3"]
	// b = ["1" "2" "3"]
}

func ExampleCopyOf_append() {
	a := make([]string, 1, 2)
	a[0] = "foo"

	fmt.Println("Without CopyOf:")
	b := append(a, "bar")
	c := append(a, "baz")
	fmt.Printf("a = %q\n", a)
	fmt.Printf("b = %q\n", b)
	fmt.Printf("c = %q\n", c)

	a = make([]string, 1, 2)
	a[0] = "foo"

	fmt.Println("With CopyOf:")
	b = append(CopyOf(a), "bar")
	c = append(CopyOf(a), "baz")
	fmt.Printf("a = %q\n", a)
	fmt.Printf("b = %q\n", b)
	fmt.Printf("c = %q\n", c)

	// Output:
	// Without CopyOf:
	// a = ["foo"]
	// b = ["foo" "baz"]
	// c = ["foo" "baz"]
	// With CopyOf:
	// a = ["foo"]
	// b = ["foo" "bar"]
	// c = ["foo" "baz"]
}

func TestSplitFileExt(t *testing.T) {
	t.Run("soname with version", func(t *testing.T) {
		root, suffix, ext := SplitFileExt("libtest.so.1.0.30")
		expected := "libtest"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".so.1.0.30"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		expected = ".so"
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})

	t.Run("soname with svn version", func(t *testing.T) {
		root, suffix, ext := SplitFileExt("libtest.so.1svn")
		expected := "libtest"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".so.1svn"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		expected = ".so"
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})

	t.Run("version numbers in the middle should be ignored", func(t *testing.T) {
		root, suffix, ext := SplitFileExt("libtest.1.0.30.so")
		expected := "libtest.1.0.30"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".so"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		expected = ".so"
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})

	t.Run("no known file extension", func(t *testing.T) {
		root, suffix, ext := SplitFileExt("test.exe")
		expected := "test"
		if root != expected {
			t.Errorf("root should be %q but got %q", expected, root)
		}
		expected = ".exe"
		if suffix != expected {
			t.Errorf("suffix should be %q but got %q", expected, suffix)
		}
		if ext != expected {
			t.Errorf("ext should be %q but got %q", expected, ext)
		}
	})
}

func Test_Shard(t *testing.T) {
	type args struct {
		strings   []string
		shardSize int
	}
	tests := []struct {
		name string
		args args
		want [][]string
	}{
		{
			name: "empty",
			args: args{
				strings:   nil,
				shardSize: 1,
			},
			want: [][]string(nil),
		},
		{
			name: "single shard",
			args: args{
				strings:   []string{"a", "b"},
				shardSize: 2,
			},
			want: [][]string{{"a", "b"}},
		},
		{
			name: "single short shard",
			args: args{
				strings:   []string{"a", "b"},
				shardSize: 3,
			},
			want: [][]string{{"a", "b"}},
		},
		{
			name: "shard per input",
			args: args{
				strings:   []string{"a", "b", "c"},
				shardSize: 1,
			},
			want: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "balanced shards",
			args: args{
				strings:   []string{"a", "b", "c", "d"},
				shardSize: 2,
			},
			want: [][]string{{"a", "b"}, {"c", "d"}},
		},
		{
			name: "unbalanced shards",
			args: args{
				strings:   []string{"a", "b", "c"},
				shardSize: 2,
			},
			want: [][]string{{"a", "b"}, {"c"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("strings", func(t *testing.T) {
				if got := ShardStrings(tt.args.strings, tt.args.shardSize); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("ShardStrings(%v, %v) = %v, want %v",
						tt.args.strings, tt.args.shardSize, got, tt.want)
				}
			})

			t.Run("paths", func(t *testing.T) {
				stringsToPaths := func(strings []string) Paths {
					if strings == nil {
						return nil
					}
					paths := make(Paths, len(strings))
					for i, s := range strings {
						paths[i] = PathForTesting(s)
					}
					return paths
				}

				paths := stringsToPaths(tt.args.strings)

				var want []Paths
				if sWant := tt.want; sWant != nil {
					want = make([]Paths, len(sWant))
					for i, w := range sWant {
						want[i] = stringsToPaths(w)
					}
				}

				if got := ShardPaths(paths, tt.args.shardSize); !reflect.DeepEqual(got, want) {
					t.Errorf("ShardPaths(%v, %v) = %v, want %v",
						paths, tt.args.shardSize, got, want)
				}
			})
		})
	}
}

func BenchmarkFirstUniqueStrings(b *testing.B) {
	implementations := []struct {
		name string
		f    func([]string) []string
	}{
		{
			name: "list",
			f:    firstUniqueList[string],
		},
		{
			name: "map",
			f:    firstUniqueMap[string],
		},
		{
			name: "optimal",
			f:    FirstUniqueStrings,
		},
	}
	const maxSize = 1024
	uniqueStrings := make([]string, maxSize)
	for i := range uniqueStrings {
		uniqueStrings[i] = strconv.Itoa(i)
	}
	sameString := make([]string, maxSize)
	for i := range sameString {
		sameString[i] = uniqueStrings[0]
	}

	f := func(b *testing.B, imp func([]string) []string, s []string) {
		for i := 0; i < b.N; i++ {
			b.ReportAllocs()
			s = append([]string(nil), s...)
			imp(s)
		}
	}

	for n := 1; n <= maxSize; n <<= 1 {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for _, implementation := range implementations {
				b.Run(implementation.name, func(b *testing.B) {
					b.Run("same", func(b *testing.B) {
						f(b, implementation.f, sameString[:n])
					})
					b.Run("unique", func(b *testing.B) {
						f(b, implementation.f, uniqueStrings[:n])
					})
				})
			}
		})
	}
}

func testSortedKeysHelper[K cmp.Ordered, V any](t *testing.T, name string, input map[K]V, expected []K) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		actual := SortedKeys(input)
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("expected %v, got %v", expected, actual)
		}
	})
}

func TestSortedKeys(t *testing.T) {
	testSortedKeysHelper(t, "simple", map[string]string{
		"b": "bar",
		"a": "foo",
	}, []string{
		"a",
		"b",
	})
	testSortedKeysHelper(t, "ints", map[int]interface{}{
		10: nil,
		5:  nil,
	}, []int{
		5,
		10,
	})

	testSortedKeysHelper(t, "nil", map[string]string(nil), nil)
	testSortedKeysHelper(t, "empty", map[string]string{}, nil)
}

func TestSortedStringValues(t *testing.T) {
	testCases := []struct {
		name     string
		in       interface{}
		expected []string
	}{
		{
			name:     "nil",
			in:       map[string]string(nil),
			expected: nil,
		},
		{
			name:     "empty",
			in:       map[string]string{},
			expected: nil,
		},
		{
			name:     "simple",
			in:       map[string]string{"foo": "a", "bar": "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "duplicates",
			in:       map[string]string{"foo": "a", "bar": "b", "baz": "b"},
			expected: []string{"a", "b", "b"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := SortedStringValues(tt.in)
			if g, w := got, tt.expected; !reflect.DeepEqual(g, w) {
				t.Errorf("wanted %q, got %q", w, g)
			}
		})
	}
}

func TestSortedUniqueStringValues(t *testing.T) {
	testCases := []struct {
		name     string
		in       interface{}
		expected []string
	}{
		{
			name:     "nil",
			in:       map[string]string(nil),
			expected: nil,
		},
		{
			name:     "empty",
			in:       map[string]string{},
			expected: nil,
		},
		{
			name:     "simple",
			in:       map[string]string{"foo": "a", "bar": "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "duplicates",
			in:       map[string]string{"foo": "a", "bar": "b", "baz": "b"},
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := SortedUniqueStringValues(tt.in)
			if g, w := got, tt.expected; !reflect.DeepEqual(g, w) {
				t.Errorf("wanted %q, got %q", w, g)
			}
		})
	}
}

var reverseTestCases = []struct {
	name     string
	in       []string
	expected []string
}{
	{
		name:     "nil",
		in:       nil,
		expected: nil,
	},
	{
		name:     "empty",
		in:       []string{},
		expected: []string{},
	},
	{
		name:     "one",
		in:       []string{"one"},
		expected: []string{"one"},
	},
	{
		name:     "even",
		in:       []string{"one", "two"},
		expected: []string{"two", "one"},
	},
	{
		name:     "odd",
		in:       []string{"one", "two", "three"},
		expected: []string{"three", "two", "one"},
	},
}

func TestReverseSliceInPlace(t *testing.T) {
	for _, testCase := range reverseTestCases {
		t.Run(testCase.name, func(t *testing.T) {
			slice := CopyOf(testCase.in)
			slice2 := slice
			ReverseSliceInPlace(slice)
			if !reflect.DeepEqual(slice, testCase.expected) {
				t.Errorf("expected %#v, got %#v", testCase.expected, slice)
			}
			if unsafe.SliceData(slice) != unsafe.SliceData(slice2) {
				t.Errorf("expected slices to share backing array")
			}
		})
	}
}

func TestReverseSlice(t *testing.T) {
	for _, testCase := range reverseTestCases {
		t.Run(testCase.name, func(t *testing.T) {
			slice := ReverseSlice(testCase.in)
			if !reflect.DeepEqual(slice, testCase.expected) {
				t.Errorf("expected %#v, got %#v", testCase.expected, slice)
			}
			if cap(slice) > 0 && unsafe.SliceData(testCase.in) == unsafe.SliceData(slice) {
				t.Errorf("expected slices to have different backing arrays")
			}
		})
	}
}
