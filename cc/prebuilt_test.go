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

package cc

import (
	"testing"

	"android/soong/android"

	"github.com/google/blueprint"
)

func TestPrebuilt(t *testing.T) {
	bp := `
		cc_library {
			name: "liba",
		}

		cc_prebuilt_library_shared {
			name: "liba",
			srcs: ["liba.so"],
		}

		cc_library {
			name: "libb",
		}

		cc_prebuilt_library_static {
			name: "libb",
			srcs: ["libb.a"],
		}

		cc_library_shared {
			name: "libd",
		}

		cc_prebuilt_library_shared {
			name: "libd",
			srcs: ["libd.so"],
		}

		cc_library_static {
			name: "libe",
		}

		cc_prebuilt_library_static {
			name: "libe",
			srcs: ["libe.a"],
		}

		cc_library {
			name: "libf",
		}

		cc_prebuilt_library {
			name: "libf",
			static: {
				srcs: ["libf.a"],
			},
			shared: {
				srcs: ["libf.so"],
			},
		}
	`

	ctx := testPrebuilt(t, bp)

	// Verify that all the modules exist and that their dependencies were connected correctly
	liba := ctx.ModuleForTests("liba", "android_arm64_armv8-a_shared").Module()
	libb := ctx.ModuleForTests("libb", "android_arm64_armv8-a_static").Module()
	libd := ctx.ModuleForTests("libd", "android_arm64_armv8-a_shared").Module()
	libe := ctx.ModuleForTests("libe", "android_arm64_armv8-a_static").Module()
	libfStatic := ctx.ModuleForTests("libf", "android_arm64_armv8-a_static").Module()
	libfShared := ctx.ModuleForTests("libf", "android_arm64_armv8-a_shared").Module()

	prebuiltLiba := ctx.ModuleForTests("prebuilt_liba", "android_arm64_armv8-a_shared").Module()
	prebuiltLibb := ctx.ModuleForTests("prebuilt_libb", "android_arm64_armv8-a_static").Module()
	prebuiltLibd := ctx.ModuleForTests("prebuilt_libd", "android_arm64_armv8-a_shared").Module()
	prebuiltLibe := ctx.ModuleForTests("prebuilt_libe", "android_arm64_armv8-a_static").Module()
	prebuiltLibfStatic := ctx.ModuleForTests("prebuilt_libf", "android_arm64_armv8-a_static").Module()
	prebuiltLibfShared := ctx.ModuleForTests("prebuilt_libf", "android_arm64_armv8-a_shared").Module()

	hasDep := func(m android.Module, wantDep android.Module) bool {
		t.Helper()
		var found bool
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	if !hasDep(liba, prebuiltLiba) {
		t.Errorf("liba missing dependency on prebuilt_liba")
	}

	if !hasDep(libb, prebuiltLibb) {
		t.Errorf("libb missing dependency on prebuilt_libb")
	}

	if !hasDep(libd, prebuiltLibd) {
		t.Errorf("libd missing dependency on prebuilt_libd")
	}

	if !hasDep(libe, prebuiltLibe) {
		t.Errorf("libe missing dependency on prebuilt_libe")
	}

	if !hasDep(libfStatic, prebuiltLibfStatic) {
		t.Errorf("libf static missing dependency on prebuilt_libf")
	}

	if !hasDep(libfShared, prebuiltLibfShared) {
		t.Errorf("libf shared missing dependency on prebuilt_libf")
	}
}

func testPrebuilt(t *testing.T, bp string) *android.TestContext {
	fs := map[string][]byte{
		"liba.so": nil,
		"libb.a":  nil,
		"libd.so": nil,
		"libe.a":  nil,
		"libf.a":  nil,
		"libf.so": nil,
	}
	config := TestConfig(buildDir, android.Android, nil, bp, fs)
	ctx := CreateTestContext()

	// Enable androidmk support.
	// * Register the singleton
	// * Configure that we are inside make
	// * Add CommonOS to ensure that androidmk processing works.
	android.RegisterAndroidMkBuildComponents(ctx)
	android.SetInMakeForTests(config)

	ctx.Register(config)
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	android.FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	android.FailIfErrored(t, errs)
	return ctx
}

func TestPrebuiltLibraryShared(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_shared {
		name: "libtest",
		srcs: ["libf.so"],
    strip: {
        none: true,
    },
	}
	`)

	shared := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().String(), "libf.so")
}

func TestPrebuiltLibraryStatic(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_static {
		name: "libtest",
		srcs: ["libf.a"],
	}
	`)

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().String(), "libf.a")
}

func TestPrebuiltLibrary(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library {
		name: "libtest",
		static: {
			srcs: ["libf.a"],
		},
		shared: {
			srcs: ["libf.so"],
		},
    strip: {
        none: true,
    },
	}
	`)

	shared := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().String(), "libf.so")

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().String(), "libf.a")
}
