// Copyright 2021 Google Inc. All rights reserved.
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
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
)

func TestDroidstubs(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		droiddoc_exported_dir {
			name: "droiddoc-templates-sdk",
			path: ".",
		}

		droidstubs {
			name: "bar-stubs",
			srcs: ["bar-doc/a.java"],
			api_levels_annotations_dirs: ["droiddoc-templates-sdk"],
			api_levels_annotations_enabled: true,
		}

		droidstubs {
			name: "bar-stubs-other",
			srcs: ["bar-doc/a.java"],
			high_mem: true,
			api_levels_annotations_dirs: ["droiddoc-templates-sdk"],
			api_levels_annotations_enabled: true,
			api_levels_jar_filename: "android.other.jar",
		}
		`,
		map[string][]byte{
			"bar-doc/a.java": nil,
		})
	testcases := []struct {
		moduleName          string
		expectedJarFilename string
		high_mem            bool
	}{
		{
			moduleName:          "bar-stubs",
			expectedJarFilename: "android.jar",
			high_mem:            false,
		},
		{
			moduleName:          "bar-stubs-other",
			expectedJarFilename: "android.other.jar",
			high_mem:            true,
		},
	}
	for _, c := range testcases {
		m := ctx.ModuleForTests(c.moduleName, "android_common")
		manifest := m.Output("metalava.sbox.textproto")
		sboxProto := android.RuleBuilderSboxProtoForTests(t, manifest)
		expected := "--android-jar-pattern ./%/public/" + c.expectedJarFilename
		if actual := String(sboxProto.Commands[0].Command); !strings.Contains(actual, expected) {
			t.Errorf("For %q, expected metalava argument %q, but was not found %q", c.moduleName, expected, actual)
		}

		metalava := m.Rule("metalava")
		rp := metalava.RuleParams
		if actual := rp.Pool != nil && strings.Contains(rp.Pool.String(), "highmem"); actual != c.high_mem {
			t.Errorf("Expected %q high_mem to be %v, was %v", c.moduleName, c.high_mem, actual)
		}
	}
}

func TestDroidstubsSandbox(t *testing.T) {
	ctx, _ := testJavaWithFS(t, `
		genrule {
			name: "foo",
			out: ["foo.txt"],
			cmd: "touch $(out)",
		}

		droidstubs {
			name: "bar-stubs",
			srcs: ["bar-doc/a.java"],

			args: "--reference $(location :foo)",
			arg_files: [":foo"],
		}
		`,
		map[string][]byte{
			"bar-doc/a.java": nil,
		})

	m := ctx.ModuleForTests("bar-stubs", "android_common")
	metalava := m.Rule("metalava")
	if g, w := metalava.Inputs.Strings(), []string{"bar-doc/a.java"}; !reflect.DeepEqual(w, g) {
		t.Errorf("Expected inputs %q, got %q", w, g)
	}

	manifest := android.RuleBuilderSboxProtoForTests(t, m.Output("metalava.sbox.textproto"))
	if g, w := manifest.Commands[0].GetCommand(), "reference __SBOX_SANDBOX_DIR__/out/.intermediates/foo/gen/foo.txt"; !strings.Contains(g, w) {
		t.Errorf("Expected command to contain %q, got %q", w, g)
	}
}

func TestDroidstubsWithSystemModules(t *testing.T) {
	ctx, _ := testJava(t, `
		droidstubs {
		    name: "stubs-source-system-modules",
		    srcs: [
		        "bar-doc/a.java",
		    ],
				sdk_version: "none",
				system_modules: "source-system-modules",
		}

		java_library {
				name: "source-jar",
		    srcs: [
		        "a.java",
		    ],
		}

		java_system_modules {
				name: "source-system-modules",
				libs: ["source-jar"],
		}

		droidstubs {
		    name: "stubs-prebuilt-system-modules",
		    srcs: [
		        "bar-doc/a.java",
		    ],
				sdk_version: "none",
				system_modules: "prebuilt-system-modules",
		}

		java_import {
				name: "prebuilt-jar",
				jars: ["a.jar"],
		}

		java_system_modules_import {
				name: "prebuilt-system-modules",
				libs: ["prebuilt-jar"],
		}
		`)

	checkSystemModulesUseByDroidstubs(t, ctx, "stubs-source-system-modules", "source-jar.jar")

	checkSystemModulesUseByDroidstubs(t, ctx, "stubs-prebuilt-system-modules", "prebuilt-jar.jar")
}

func checkSystemModulesUseByDroidstubs(t *testing.T, ctx *android.TestContext, moduleName string, systemJar string) {
	metalavaRule := ctx.ModuleForTests(moduleName, "android_common").Rule("metalava")
	var systemJars []string
	for _, i := range metalavaRule.Implicits {
		systemJars = append(systemJars, i.Base())
	}
	if len(systemJars) < 1 || systemJars[0] != systemJar {
		t.Errorf("inputs of %q must be []string{%q}, but was %#v.", moduleName, systemJar, systemJars)
	}
}
