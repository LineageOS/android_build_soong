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
	"path/filepath"
	"testing"

	"android/soong/android"

	"github.com/google/blueprint"
)

var prebuiltFixtureFactory = ccFixtureFactory.Extend(
	android.PrepareForTestWithAndroidMk,
)

func testPrebuilt(t *testing.T, bp string, fs android.MockFS, handlers ...android.FixturePreparer) *android.TestContext {
	result := prebuiltFixtureFactory.Extend(
		fs.AddToFixture(),
	).Extend(handlers...).RunTestWithBp(t, bp)

	return result.TestContext
}

type configCustomizer func(config android.Config)

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

		cc_object {
			name: "crtx",
		}

		cc_prebuilt_object {
			name: "crtx",
			srcs: ["crtx.o"],
		}
	`

	ctx := testPrebuilt(t, bp, map[string][]byte{
		"liba.so": nil,
		"libb.a":  nil,
		"libd.so": nil,
		"libe.a":  nil,
		"libf.a":  nil,
		"libf.so": nil,
		"crtx.o":  nil,
	})

	// Verify that all the modules exist and that their dependencies were connected correctly
	liba := ctx.ModuleForTests("liba", "android_arm64_armv8-a_shared").Module()
	libb := ctx.ModuleForTests("libb", "android_arm64_armv8-a_static").Module()
	libd := ctx.ModuleForTests("libd", "android_arm64_armv8-a_shared").Module()
	libe := ctx.ModuleForTests("libe", "android_arm64_armv8-a_static").Module()
	libfStatic := ctx.ModuleForTests("libf", "android_arm64_armv8-a_static").Module()
	libfShared := ctx.ModuleForTests("libf", "android_arm64_armv8-a_shared").Module()
	crtx := ctx.ModuleForTests("crtx", "android_arm64_armv8-a").Module()

	prebuiltLiba := ctx.ModuleForTests("prebuilt_liba", "android_arm64_armv8-a_shared").Module()
	prebuiltLibb := ctx.ModuleForTests("prebuilt_libb", "android_arm64_armv8-a_static").Module()
	prebuiltLibd := ctx.ModuleForTests("prebuilt_libd", "android_arm64_armv8-a_shared").Module()
	prebuiltLibe := ctx.ModuleForTests("prebuilt_libe", "android_arm64_armv8-a_static").Module()
	prebuiltLibfStatic := ctx.ModuleForTests("prebuilt_libf", "android_arm64_armv8-a_static").Module()
	prebuiltLibfShared := ctx.ModuleForTests("prebuilt_libf", "android_arm64_armv8-a_shared").Module()
	prebuiltCrtx := ctx.ModuleForTests("prebuilt_crtx", "android_arm64_armv8-a").Module()

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

	if !hasDep(crtx, prebuiltCrtx) {
		t.Errorf("crtx missing dependency on prebuilt_crtx")
	}
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
	`, map[string][]byte{
		"libf.so": nil,
	})

	shared := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libtest.so")
}

func TestPrebuiltLibraryStatic(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_static {
		name: "libtest",
		srcs: ["libf.a"],
	}
	`, map[string][]byte{
		"libf.a": nil,
	})

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.a")
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
	`, map[string][]byte{
		"libf.a":  nil,
		"libf.so": nil,
	})

	shared := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libtest.so")

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.a")
}

func TestPrebuiltLibraryStem(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library {
		name: "libfoo",
		stem: "libbar",
		static: {
			srcs: ["libfoo.a"],
		},
		shared: {
			srcs: ["libfoo.so"],
		},
		strip: {
			none: true,
		},
	}
	`, map[string][]byte{
		"libfoo.a":  nil,
		"libfoo.so": nil,
	})

	static := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libfoo.a")

	shared := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libbar.so")
}

func TestPrebuiltLibrarySharedStem(t *testing.T) {
	ctx := testPrebuilt(t, `
	cc_prebuilt_library_shared {
		name: "libfoo",
		stem: "libbar",
		srcs: ["libfoo.so"],
		strip: {
			none: true,
		},
	}
	`, map[string][]byte{
		"libfoo.so": nil,
	})

	shared := ctx.ModuleForTests("libfoo", "android_arm64_armv8-a_shared").Module().(*Module)
	assertString(t, shared.OutputFile().Path().Base(), "libbar.so")
}

func TestPrebuiltSymlinkedHostBinary(t *testing.T) {
	if android.BuildOs != android.Linux {
		t.Skipf("Skipping host prebuilt testing that is only supported on %s not %s", android.Linux, android.BuildOs)
	}

	ctx := testPrebuilt(t, `
	cc_prebuilt_library_shared {
		name: "libfoo",
		device_supported: false,
		host_supported: true,
		target: {
			linux_glibc_x86_64: {
				srcs: ["linux_glibc_x86_64/lib64/libfoo.so"],
			},
		},
	}

	cc_prebuilt_binary {
		name: "foo",
		device_supported: false,
		host_supported: true,
		shared_libs: ["libfoo"],
		target: {
			linux_glibc_x86_64: {
				srcs: ["linux_glibc_x86_64/bin/foo"],
			},
		},
	}
	`, map[string][]byte{
		"libfoo.so": nil,
		"foo":       nil,
	})

	fooRule := ctx.ModuleForTests("foo", "linux_glibc_x86_64").Rule("Symlink")
	assertString(t, fooRule.Output.String(),
		filepath.Join(buildDir, ".intermediates/foo/linux_glibc_x86_64/foo"))
	assertString(t, fooRule.Args["fromPath"], "$$PWD/linux_glibc_x86_64/bin/foo")

	var libfooDep android.Path
	for _, dep := range fooRule.Implicits {
		if dep.Base() == "libfoo.so" {
			libfooDep = dep
			break
		}
	}
	assertString(t, libfooDep.String(),
		filepath.Join(buildDir, ".intermediates/libfoo/linux_glibc_x86_64_shared/libfoo.so"))
}

func TestPrebuiltLibrarySanitized(t *testing.T) {
	bp := `cc_prebuilt_library {
	name: "libtest",
		static: {
                        sanitized: { none: { srcs: ["libf.a"], }, hwaddress: { srcs: ["libf.hwasan.a"], }, },
		},
		shared: {
                        sanitized: { none: { srcs: ["libf.so"], }, hwaddress: { srcs: ["hwasan/libf.so"], }, },
		},
	}
	cc_prebuilt_library_static {
		name: "libtest_static",
                sanitized: { none: { srcs: ["libf.a"], }, hwaddress: { srcs: ["libf.hwasan.a"], }, },
	}
	cc_prebuilt_library_shared {
		name: "libtest_shared",
                sanitized: { none: { srcs: ["libf.so"], }, hwaddress: { srcs: ["hwasan/libf.so"], }, },
	}`

	fs := map[string][]byte{
		"libf.a":         nil,
		"libf.hwasan.a":  nil,
		"libf.so":        nil,
		"hwasan/libf.so": nil,
	}

	// Without SANITIZE_TARGET.
	ctx := testPrebuilt(t, bp, fs)

	shared_rule := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared").Rule("android/soong/cc.strip")
	assertString(t, shared_rule.Input.String(), "libf.so")

	static := ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.a")

	shared_rule2 := ctx.ModuleForTests("libtest_shared", "android_arm64_armv8-a_shared").Rule("android/soong/cc.strip")
	assertString(t, shared_rule2.Input.String(), "libf.so")

	static2 := ctx.ModuleForTests("libtest_static", "android_arm64_armv8-a_static").Module().(*Module)
	assertString(t, static2.OutputFile().Path().Base(), "libf.a")

	// With SANITIZE_TARGET=hwaddress
	ctx = testPrebuilt(t, bp, fs,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"hwaddress"}
		}),
	)

	shared_rule = ctx.ModuleForTests("libtest", "android_arm64_armv8-a_shared_hwasan").Rule("android/soong/cc.strip")
	assertString(t, shared_rule.Input.String(), "hwasan/libf.so")

	static = ctx.ModuleForTests("libtest", "android_arm64_armv8-a_static_hwasan").Module().(*Module)
	assertString(t, static.OutputFile().Path().Base(), "libf.hwasan.a")

	shared_rule2 = ctx.ModuleForTests("libtest_shared", "android_arm64_armv8-a_shared_hwasan").Rule("android/soong/cc.strip")
	assertString(t, shared_rule2.Input.String(), "hwasan/libf.so")

	static2 = ctx.ModuleForTests("libtest_static", "android_arm64_armv8-a_static_hwasan").Module().(*Module)
	assertString(t, static2.OutputFile().Path().Base(), "libf.hwasan.a")
}
