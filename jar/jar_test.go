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

package jar

import (
	"bytes"
	"io"
	"testing"
)

func TestGetJavaPackage(t *testing.T) {
	type args struct {
		r   io.Reader
		src string
	}
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "simple",
			in:   "package foo.bar;",
			want: "foo.bar",
		},
		{
			name: "comment",
			in:   "/* test */\npackage foo.bar;",
			want: "foo.bar",
		},
		{
			name: "no package",
			in:   "import foo.bar;",
			want: "",
		},
		{
			name:    "missing semicolon error",
			in:      "package foo.bar",
			wantErr: true,
		},
		{
			name:    "parser error",
			in:      "/*",
			wantErr: true,
		},
		{
			name:    "parser ident error",
			in:      "package 0foo.bar;",
			wantErr: true,
		},
		{
			name: "annotations",
			in:   "@NonNullApi\n@X\npackage foo.bar;",
			want: "foo.bar",
		},
		{
			name:    "complex annotation",
			in:      "@Foo(x=y)\n@package foo.bar;",
			wantErr: true, // Complex annotation not supported yet.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBufferString(tt.in)
			got, err := JavaPackage(buf, "<test>")
			if (err != nil) != tt.wantErr {
				t.Errorf("JavaPackage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("JavaPackage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_javaIdentRune(t *testing.T) {
	// runes that should be valid anywhere in an identifier
	validAnywhere := []rune{
		// letters, $, _
		'a',
		'A',
		'$',
		'_',

		// assorted unicode
		'𐐀',
		'𐐨',
		'ǅ',
		'ῼ',
		'ʰ',
		'ﾟ',
		'ƻ',
		'㡢',
		'￦',
		'＿',
		'Ⅰ',
		'𐍊',
	}

	// runes that should be invalid as the first rune in an identifier, but valid anywhere else
	validAfterFirst := []rune{
		// digits
		'0',

		// assorted unicode
		'᥍',
		'𝟎',
		'ྂ',
		'𝆀',

		// control characters
		'\x00',
		'\b',
		'\u000e',
		'\u001b',
		'\u007f',
		'\u009f',
		'\u00ad',
		0xE007F,

		// zero width space
		'\u200b',
	}

	// runes that should never be valid in an identifier
	invalid := []rune{
		';',
		0x110000,
	}

	validFirst := validAnywhere
	invalidFirst := append(validAfterFirst, invalid...)
	validPart := append(validAnywhere, validAfterFirst...)
	invalidPart := invalid

	check := func(t *testing.T, ch rune, i int, want bool) {
		t.Helper()
		if got := javaIdentRune(ch, i); got != want {
			t.Errorf("javaIdentRune() = %v, want %v", got, want)
		}
	}

	t.Run("first", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, ch := range validFirst {
				t.Run(string(ch), func(t *testing.T) {
					check(t, ch, 0, true)
				})
			}
		})

		t.Run("invalid", func(t *testing.T) {
			for _, ch := range invalidFirst {
				t.Run(string(ch), func(t *testing.T) {
					check(t, ch, 0, false)
				})
			}
		})
	})

	t.Run("part", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, ch := range validPart {
				t.Run(string(ch), func(t *testing.T) {
					check(t, ch, 1, true)
				})
			}
		})

		t.Run("invalid", func(t *testing.T) {
			for _, ch := range invalidPart {
				t.Run(string(ch), func(t *testing.T) {
					check(t, ch, 1, false)
				})
			}
		})
	})
}
