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

package main

import (
	"bytes"
	"io/ioutil"
	"strconv"
	"testing"
)

var testCases = []struct {
	in, out string
}{
	{
		in:  "File.java:40: error: cannot find symbol\n",
		out: "\x1b[1mFile.java:40: \x1b[31merror:\x1b[0m\x1b[1m cannot find symbol\x1b[0m\n",
	},
	{
		in:  "import static com.blah.SYMBOL;\n",
		out: "import static com.blah.SYMBOL;\n",
	},
	{
		in:  "          ^           \n",
		out: "\x1b[1m          \x1b[32m^\x1b[0m\x1b[1m           \x1b[0m\n",
	},
	{
		in:  "File.java:398: warning: [RectIntersectReturnValueIgnored] Return value of com.blah.function() must be checked\n",
		out: "\x1b[1mFile.java:398: \x1b[35mwarning:\x1b[0m\x1b[1m [RectIntersectReturnValueIgnored] Return value of com.blah.function() must be checked\x1b[0m\n",
	},
	{
		in:  "warning: [options] blah\n",
		out: "\x1b[1m\x1b[35mwarning:\x1b[0m\x1b[1m [options] blah\x1b[0m\n",
	},
	{
		in:  "    (see http://go/errorprone/bugpattern/RectIntersectReturnValueIgnored.md)\n",
		out: "    (see http://go/errorprone/bugpattern/RectIntersectReturnValueIgnored.md)\n",
	},
	{
		in: `
Note: Some input files use or override a deprecated API.
Note: Recompile with -Xlint:deprecation for details.
Note: Some input files use unchecked or unsafe operations.
Note: Recompile with -Xlint:unchecked for details.
Note: dir/file.java uses or overrides a deprecated API.
Note: dir/file.java uses unchecked or unsafe operations.
warning: [options] bootstrap class path not set in conjunction with -source 1.7
`,
		out: "\n",
	},
	{
		in:  "\n",
		out: "\n",
	},
	{
		in: `
javadoc: warning - The old Doclet and Taglet APIs in the packages
com.sun.javadoc, com.sun.tools.doclets and their implementations
are planned to be removed in a future JDK release. These
components have been superseded by the new APIs in jdk.javadoc.doclet.
Users are strongly recommended to migrate to the new APIs.
javadoc: option --boot-class-path not allowed with target 1.9
`,
		out: "\n",
	},
	{
		in: `
warning: [options] bootstrap class path not set in conjunction with -source 1.9\n
1 warning
`,
		out: "\n",
	},
	{
		in: `
warning: foo
warning: [options] bootstrap class path not set in conjunction with -source 1.9\n
2 warnings
`,
		out: "\n\x1b[1m\x1b[35mwarning:\x1b[0m\x1b[1m foo\x1b[0m\n1 warning\n",
	},
}

func TestJavacColorize(t *testing.T) {
	for i, test := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			buf := new(bytes.Buffer)
			proc := processor{}
			err := proc.process(bytes.NewReader([]byte(test.in)), buf)
			if err != nil {
				t.Errorf("error: %q", err)
			}
			got := string(buf.Bytes())
			if got != test.out {
				t.Errorf("expected %q got %q", test.out, got)
			}
		})
	}
}

func TestSubprocess(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		exitCode, err := Main(ioutil.Discard, "test", []string{"sh", "-c", "exit 9"})
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		if exitCode != 9 {
			t.Fatal("expected exit code 9, got", exitCode)
		}
	})

	t.Run("signal", func(t *testing.T) {
		exitCode, err := Main(ioutil.Discard, "test", []string{"sh", "-c", "kill -9 $$"})
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		if exitCode != 137 {
			t.Fatal("expected exit code 137, got", exitCode)
		}
	})

	t.Run("success", func(t *testing.T) {
		exitCode, err := Main(ioutil.Discard, "test", []string{"echo"})
		if err != nil {
			t.Fatal("unexpected error", err)
		}
		if exitCode != 0 {
			t.Fatal("expected exit code 0, got", exitCode)
		}
	})

}
