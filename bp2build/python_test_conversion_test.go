// Copyright 2023 Google Inc. All rights reserved.
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

package bp2build

import (
	"testing"

	"android/soong/python"
)

func TestPythonTestHostSimple(t *testing.T) {
	runBp2BuildTestCaseWithPythonLibraries(t, Bp2buildTestCase{
		Description:                "simple python_test_host converts to a native py_test",
		ModuleTypeUnderTest:        "python_test_host",
		ModuleTypeUnderTestFactory: python.PythonTestHostFactory,
		Filesystem: map[string]string{
			"a.py":           "",
			"b/c.py":         "",
			"b/d.py":         "",
			"b/e.py":         "",
			"files/data.txt": "",
		},
		StubbedBuildDefinitions: []string{"bar"},
		Blueprint: `python_test_host {
    name: "foo",
    main: "a.py",
    srcs: ["**/*.py"],
    exclude_srcs: ["b/e.py"],
    data: ["files/data.txt",],
    libs: ["bar"],
    bazel_module: { bp2build_available: true },
}
    python_library_host {
      name: "bar",
      srcs: ["b/e.py"],
    }`,
		ExpectedBazelTargets: []string{
			MakeBazelTarget("py_test", "foo", AttrNameToString{
				"data":    `["files/data.txt"]`,
				"deps":    `[":bar"]`,
				"main":    `"a.py"`,
				"imports": `["."]`,
				"srcs": `[
        "a.py",
        "b/c.py",
        "b/d.py",
    ]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
