// Copyright 2022 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package bp2build

import (
	"testing"

	"android/soong/android"
	"android/soong/cc"
)

func runYasmTestCase(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, registerYasmModuleTypes, tc)
}

func registerYasmModuleTypes(ctx android.RegistrationContext) {
	cc.RegisterCCBuildComponents(ctx)
	ctx.RegisterModuleType("filegroup", android.FileGroupFactory)
	ctx.RegisterModuleType("cc_library_static", cc.LibraryStaticFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_static", cc.PrebuiltStaticLibraryFactory)
	ctx.RegisterModuleType("cc_library_headers", cc.LibraryHeaderFactory)
}

func TestYasmSimple(t *testing.T) {
	runYasmTestCase(t, Bp2buildTestCase{
		Description:                "Simple yasm test",
		ModuleTypeUnderTest:        "cc_library",
		ModuleTypeUnderTestFactory: cc.LibraryFactory,
		Filesystem: map[string]string{
			"main.cpp":   "",
			"myfile.asm": "",
		},
		Blueprint: `
cc_library {
  name: "foo",
  srcs: ["main.cpp", "myfile.asm"],
}`,
		ExpectedBazelTargets: append([]string{
			makeBazelTarget("yasm", "foo_yasm", map[string]string{
				"include_dirs": `["."]`,
				"srcs":         `["myfile.asm"]`,
			}),
		}, makeCcLibraryTargets("foo", map[string]string{
			"local_includes": `["."]`,
			"srcs": `[
        "main.cpp",
        ":foo_yasm",
    ]`,
		})...),
	})
}

func TestYasmWithIncludeDirs(t *testing.T) {
	runYasmTestCase(t, Bp2buildTestCase{
		Description:                "Simple yasm test",
		ModuleTypeUnderTest:        "cc_library",
		ModuleTypeUnderTestFactory: cc.LibraryFactory,
		Filesystem: map[string]string{
			"main.cpp":                    "",
			"myfile.asm":                  "",
			"include1/foo/myinclude.inc":  "",
			"include2/foo/myinclude2.inc": "",
		},
		Blueprint: `
cc_library {
  name: "foo",
  local_include_dirs: ["include1/foo"],
  export_include_dirs: ["include2/foo"],
  srcs: ["main.cpp", "myfile.asm"],
}`,
		ExpectedBazelTargets: append([]string{
			makeBazelTarget("yasm", "foo_yasm", map[string]string{
				"include_dirs": `[
        "include1/foo",
        ".",
        "include2/foo",
    ]`,
				"srcs": `["myfile.asm"]`,
			}),
		}, makeCcLibraryTargets("foo", map[string]string{
			"local_includes": `[
        "include1/foo",
        ".",
    ]`,
			"export_includes": `["include2/foo"]`,
			"srcs": `[
        "main.cpp",
        ":foo_yasm",
    ]`,
		})...),
	})
}

func TestYasmConditionalBasedOnArch(t *testing.T) {
	runYasmTestCase(t, Bp2buildTestCase{
		Description:                "Simple yasm test",
		ModuleTypeUnderTest:        "cc_library",
		ModuleTypeUnderTestFactory: cc.LibraryFactory,
		Filesystem: map[string]string{
			"main.cpp":   "",
			"myfile.asm": "",
		},
		Blueprint: `
cc_library {
  name: "foo",
  srcs: ["main.cpp"],
  arch: {
    x86: {
      srcs: ["myfile.asm"],
    },
  },
}`,
		ExpectedBazelTargets: append([]string{
			makeBazelTarget("yasm", "foo_yasm", map[string]string{
				"include_dirs": `["."]`,
				"srcs": `select({
        "//build/bazel/platforms/arch:x86": ["myfile.asm"],
        "//conditions:default": [],
    })`,
			}),
		}, makeCcLibraryTargets("foo", map[string]string{
			"local_includes": `["."]`,
			"srcs": `["main.cpp"] + select({
        "//build/bazel/platforms/arch:x86": [":foo_yasm"],
        "//conditions:default": [],
    })`,
		})...),
	})
}

func TestYasmPartiallyConditional(t *testing.T) {
	runYasmTestCase(t, Bp2buildTestCase{
		Description:                "Simple yasm test",
		ModuleTypeUnderTest:        "cc_library",
		ModuleTypeUnderTestFactory: cc.LibraryFactory,
		Filesystem: map[string]string{
			"main.cpp":         "",
			"myfile.asm":       "",
			"mysecondfile.asm": "",
		},
		Blueprint: `
cc_library {
  name: "foo",
  srcs: ["main.cpp", "myfile.asm"],
  arch: {
    x86: {
      srcs: ["mysecondfile.asm"],
    },
  },
}`,
		ExpectedBazelTargets: append([]string{
			makeBazelTarget("yasm", "foo_yasm", map[string]string{
				"include_dirs": `["."]`,
				"srcs": `["myfile.asm"] + select({
        "//build/bazel/platforms/arch:x86": ["mysecondfile.asm"],
        "//conditions:default": [],
    })`,
			}),
		}, makeCcLibraryTargets("foo", map[string]string{
			"local_includes": `["."]`,
			"srcs": `[
        "main.cpp",
        ":foo_yasm",
    ]`,
		})...),
	})
}
