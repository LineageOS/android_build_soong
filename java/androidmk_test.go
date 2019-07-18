// Copyright 2019 Google Inc. All rights reserved.
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

package java

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"android/soong/android"
)

type testAndroidMk struct {
	*testing.T
	body []byte
}

type testAndroidMkModule struct {
	*testing.T
	props map[string]string
}

func newTestAndroidMk(t *testing.T, r io.Reader) *testAndroidMk {
	t.Helper()
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatal("failed to open read Android.mk.", err)
	}
	return &testAndroidMk{
		T:    t,
		body: buf,
	}
}

func parseAndroidMkProps(lines []string) map[string]string {
	props := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimLeft(line, " ")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tokens := strings.Split(line, " ")
		if tokens[1] == "+=" {
			props[tokens[0]] += " " + strings.Join(tokens[2:], " ")
		} else {
			props[tokens[0]] = strings.Join(tokens[2:], " ")
		}
	}
	return props
}

func (t *testAndroidMk) moduleFor(moduleName string) *testAndroidMkModule {
	t.Helper()
	lines := strings.Split(string(t.body), "\n")
	index := android.IndexList("LOCAL_MODULE := "+moduleName, lines)
	if index == -1 {
		t.Fatalf("%q is not found.", moduleName)
	}
	lines = lines[index:]
	includeIndex := android.IndexListPred(func(line string) bool {
		return strings.HasPrefix(line, "include")
	}, lines)
	if includeIndex == -1 {
		t.Fatalf("%q is not properly defined. (\"include\" not found).", moduleName)
	}
	props := parseAndroidMkProps(lines[:includeIndex])
	return &testAndroidMkModule{
		T:     t.T,
		props: props,
	}
}

func (t *testAndroidMkModule) hasRequired(dep string) {
	t.Helper()
	required, ok := t.props["LOCAL_REQUIRED_MODULES"]
	if !ok {
		t.Error("LOCAL_REQUIRED_MODULES is not found.")
		return
	}
	if !android.InList(dep, strings.Split(required, " ")) {
		t.Errorf("%q is expected in LOCAL_REQUIRED_MODULES, but not found in %q.", dep, required)
	}
}

func (t *testAndroidMkModule) hasNoRequired(dep string) {
	t.Helper()
	required, ok := t.props["LOCAL_REQUIRED_MODULES"]
	if !ok {
		return
	}
	if android.InList(dep, strings.Split(required, " ")) {
		t.Errorf("%q is not expected in LOCAL_REQUIRED_MODULES, but found.", dep)
	}
}

func getAndroidMk(t *testing.T, ctx *android.TestContext, config android.Config, name string) *testAndroidMk {
	t.Helper()
	lib, _ := ctx.ModuleForTests(name, "android_common").Module().(*Library)
	data := android.AndroidMkDataForTest(t, config, "", lib)
	w := &bytes.Buffer{}
	data.Custom(w, name, "", "", data)
	return newTestAndroidMk(t, w)
}

func TestRequired(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			required: ["libfoo"],
		}
	`)

	mk := getAndroidMk(t, ctx, config, "foo")
	mk.moduleFor("foo").hasRequired("libfoo")
}

func TestHostdex(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			hostdex: true,
		}
	`)

	mk := getAndroidMk(t, ctx, config, "foo")
	mk.moduleFor("foo")
	mk.moduleFor("foo-hostdex")
}

func TestHostdexRequired(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			hostdex: true,
			required: ["libfoo"],
		}
	`)

	mk := getAndroidMk(t, ctx, config, "foo")
	mk.moduleFor("foo").hasRequired("libfoo")
	mk.moduleFor("foo-hostdex").hasRequired("libfoo")
}

func TestHostdexSpecificRequired(t *testing.T) {
	ctx, config := testJava(t, `
		java_library {
			name: "foo",
			srcs: ["a.java"],
			hostdex: true,
			target: {
				hostdex: {
					required: ["libfoo"],
				},
			},
		}
	`)

	mk := getAndroidMk(t, ctx, config, "foo")
	mk.moduleFor("foo").hasNoRequired("libfoo")
	mk.moduleFor("foo-hostdex").hasRequired("libfoo")
}
