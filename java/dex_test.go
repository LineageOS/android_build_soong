// Copyright 2022 Google Inc. All rights reserved.
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
	"testing"

	"android/soong/android"
)

func TestR8(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd.RunTestWithBp(t, `
		android_app {
			name: "app",
			srcs: ["foo.java"],
			libs: ["lib"],
			static_libs: ["static_lib"],
			platform_apis: true,
		}

		android_app {
			name: "stable_app",
			srcs: ["foo.java"],
			sdk_version: "current",
			min_sdk_version: "31",
		}

		android_app {
			name: "core_platform_app",
			srcs: ["foo.java"],
			sdk_version: "core_platform",
		}

		java_library {
			name: "lib",
			srcs: ["foo.java"],
		}

		java_library {
			name: "static_lib",
			srcs: ["foo.java"],
		}
	`)

	app := result.ModuleForTests("app", "android_common")
	stableApp := result.ModuleForTests("stable_app", "android_common")
	corePlatformApp := result.ModuleForTests("core_platform_app", "android_common")
	lib := result.ModuleForTests("lib", "android_common")
	staticLib := result.ModuleForTests("static_lib", "android_common")

	appJavac := app.Rule("javac")
	appR8 := app.Rule("r8")
	stableAppR8 := stableApp.Rule("r8")
	corePlatformAppR8 := corePlatformApp.Rule("r8")
	libHeader := lib.Output("turbine-combined/lib.jar").Output
	staticLibHeader := staticLib.Output("turbine-combined/static_lib.jar").Output

	android.AssertStringDoesContain(t, "expected lib header jar in app javac classpath",
		appJavac.Args["classpath"], libHeader.String())
	android.AssertStringDoesContain(t, "expected static_lib header jar in app javac classpath",
		appJavac.Args["classpath"], staticLibHeader.String())

	android.AssertStringDoesContain(t, "expected lib header jar in app r8 classpath",
		appR8.Args["r8Flags"], libHeader.String())
	android.AssertStringDoesNotContain(t, "expected no  static_lib header jar in app javac classpath",
		appR8.Args["r8Flags"], staticLibHeader.String())
	android.AssertStringDoesContain(t, "expected -ignorewarnings in app r8 flags",
		appR8.Args["r8Flags"], "-ignorewarnings")
	android.AssertStringDoesContain(t, "expected --android-platform-build in app r8 flags",
		appR8.Args["r8Flags"], "--android-platform-build")
	android.AssertStringDoesNotContain(t, "expected no --android-platform-build in stable_app r8 flags",
		stableAppR8.Args["r8Flags"], "--android-platform-build")
	android.AssertStringDoesContain(t, "expected --android-platform-build in core_platform_app r8 flags",
		corePlatformAppR8.Args["r8Flags"], "--android-platform-build")
}

func TestR8Flags(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd.RunTestWithBp(t, `
		android_app {
			name: "app",
			srcs: ["foo.java"],
			platform_apis: true,
			optimize: {
				shrink: false,
				optimize: false,
				obfuscate: false,
				ignore_warnings: false,
			},
		}
	`)

	app := result.ModuleForTests("app", "android_common")
	appR8 := app.Rule("r8")
	android.AssertStringDoesContain(t, "expected -dontshrink in app r8 flags",
		appR8.Args["r8Flags"], "-dontshrink")
	android.AssertStringDoesContain(t, "expected -dontoptimize in app r8 flags",
		appR8.Args["r8Flags"], "-dontoptimize")
	android.AssertStringDoesContain(t, "expected -dontobfuscate in app r8 flags",
		appR8.Args["r8Flags"], "-dontobfuscate")
	android.AssertStringDoesNotContain(t, "expected no -ignorewarnings in app r8 flags",
		appR8.Args["r8Flags"], "-ignorewarnings")
	android.AssertStringDoesContain(t, "expected --android-platform-build in app r8 flags",
		appR8.Args["r8Flags"], "--android-platform-build")
}

func TestD8(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd.RunTestWithBp(t, `
		java_library {
			name: "foo",
			srcs: ["foo.java"],
			libs: ["lib"],
			static_libs: ["static_lib"],
			installable: true,
		}

		java_library {
			name: "lib",
			srcs: ["foo.java"],
		}

		java_library {
			name: "static_lib",
			srcs: ["foo.java"],
		}
	`)

	foo := result.ModuleForTests("foo", "android_common")
	lib := result.ModuleForTests("lib", "android_common")
	staticLib := result.ModuleForTests("static_lib", "android_common")

	fooJavac := foo.Rule("javac")
	fooD8 := foo.Rule("d8")
	libHeader := lib.Output("turbine-combined/lib.jar").Output
	staticLibHeader := staticLib.Output("turbine-combined/static_lib.jar").Output

	android.AssertStringDoesContain(t, "expected lib header jar in foo javac classpath",
		fooJavac.Args["classpath"], libHeader.String())
	android.AssertStringDoesContain(t, "expected static_lib header jar in foo javac classpath",
		fooJavac.Args["classpath"], staticLibHeader.String())

	android.AssertStringDoesContain(t, "expected lib header jar in foo d8 classpath",
		fooD8.Args["d8Flags"], libHeader.String())
	android.AssertStringDoesNotContain(t, "expected no  static_lib header jar in foo javac classpath",
		fooD8.Args["d8Flags"], staticLibHeader.String())
}

func TestProguardFlagsInheritance(t *testing.T) {
	result := PrepareForTestWithJavaDefaultModulesWithoutFakeDex2oatd.RunTestWithBp(t, `
		android_app {
			name: "app",
			static_libs: [
				"primary_android_lib",
				"primary_lib",
			],
			platform_apis: true,
		}

		java_library {
			name: "primary_lib",
			optimize: {
				proguard_flags_files: ["primary.flags"],
			},
		}

		android_library {
			name: "primary_android_lib",
			static_libs: ["secondary_lib"],
			optimize: {
				proguard_flags_files: ["primary_android.flags"],
			},
		}

		java_library {
			name: "secondary_lib",
			static_libs: ["tertiary_lib"],
			optimize: {
				proguard_flags_files: ["secondary.flags"],
			},
		}

		java_library {
			name: "tertiary_lib",
			optimize: {
				proguard_flags_files: ["tertiary.flags"],
			},
		}
	`)

	app := result.ModuleForTests("app", "android_common")
	appR8 := app.Rule("r8")
	android.AssertStringDoesContain(t, "expected primary_lib's proguard flags from direct dep",
		appR8.Args["r8Flags"], "primary.flags")
	android.AssertStringDoesContain(t, "expected primary_android_lib's proguard flags from direct dep",
		appR8.Args["r8Flags"], "primary_android.flags")
	android.AssertStringDoesContain(t, "expected secondary_lib's proguard flags from inherited dep",
		appR8.Args["r8Flags"], "secondary.flags")
	android.AssertStringDoesContain(t, "expected tertiary_lib's proguard flags from inherited dep",
		appR8.Args["r8Flags"], "tertiary.flags")
}
