// Copyright 2020 The Android Open Source Project
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

package rust

import (
	"strings"
	"testing"

	"android/soong/android"
)

func TestRustBindgen(t *testing.T) {
	ctx := testRust(t, `
		rust_bindgen {
			name: "libbindgen",
			defaults: ["cc_defaults_flags"],
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			bindgen_flags: ["--bindgen-flag.*"],
			cflags: ["--clang-flag()"],
			shared_libs: ["libfoo_shared"],
		}
		rust_bindgen {
			name: "libbindgen_staticlib",
			wrapper_src: "src/any.h",
			crate_name: "bindgen_staticlib",
			stem: "libbindgen_staticlib",
			source_stem: "bindings",
			static_libs: ["libfoo_static"],
		}
		rust_bindgen {
			name: "libbindgen_headerlib",
			wrapper_src: "src/any.h",
			crate_name: "bindgen_headerlib",
			stem: "libbindgen_headerlib",
			source_stem: "bindings",
			header_libs: ["libfoo_header"],
		}
		cc_library_shared {
			name: "libfoo_shared",
			export_include_dirs: ["shared_include"],
		}
		cc_library_static {
			name: "libfoo_static",
			export_include_dirs: ["static_include"],
		}
		cc_library_headers {
			name: "libfoo_header",
			export_include_dirs: ["header_include"],
		}
		cc_defaults {
			name: "cc_defaults_flags",
			cflags: ["--default-flag"],
		}
	`)
	libbindgen := ctx.ModuleForTests("libbindgen", "android_arm64_armv8-a_source").Output("bindings.rs")
	libbindgenStatic := ctx.ModuleForTests("libbindgen_staticlib", "android_arm64_armv8-a_source").Output("bindings.rs")
	libbindgenHeader := ctx.ModuleForTests("libbindgen_headerlib", "android_arm64_armv8-a_source").Output("bindings.rs")
	libbindgenHeaderModule := ctx.ModuleForTests("libbindgen_headerlib", "android_arm64_armv8-a_source").Module().(*Module)
	// Ensure that the flags are present and escaped
	if !strings.Contains(libbindgen.Args["flags"], "'--bindgen-flag.*'") {
		t.Errorf("missing bindgen flags in rust_bindgen rule: flags %#v", libbindgen.Args["flags"])
	}
	if !strings.Contains(libbindgen.Args["cflags"], "'--clang-flag()'") {
		t.Errorf("missing clang cflags in rust_bindgen rule: cflags %#v", libbindgen.Args["cflags"])
	}
	if !strings.Contains(libbindgen.Args["cflags"], "-Ishared_include") {
		t.Errorf("missing shared_libs exported includes in rust_bindgen rule: cflags %#v", libbindgen.Args["cflags"])
	}
	if !strings.Contains(libbindgenStatic.Args["cflags"], "-Istatic_include") {
		t.Errorf("missing static_libs exported includes in rust_bindgen rule: cflags %#v", libbindgenStatic.Args["cflags"])
	}
	if !strings.Contains(libbindgenHeader.Args["cflags"], "-Iheader_include") {
		t.Errorf("missing header_libs exported includes in rust_bindgen rule: cflags %#v", libbindgenHeader.Args["cflags"])
	}

	if android.InList("libfoo_static", libbindgenHeaderModule.Properties.AndroidMkHeaderLibs) {
		t.Errorf("Static library dependency should not be in HeaderLibs list")
	}

	if !strings.Contains(libbindgen.Args["cflags"], "--default-flag") {
		t.Errorf("rust_bindgen missing cflags defined in cc_defaults: cflags %#v", libbindgen.Args["cflags"])
	}
}

func TestRustBindgenCustomBindgen(t *testing.T) {
	ctx := testRust(t, `
		rust_bindgen {
			name: "libbindgen",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			custom_bindgen: "my_bindgen"
		}
		rust_binary_host {
			name: "my_bindgen",
			srcs: ["foo.rs"],
		}
	`)

	libbindgen := ctx.ModuleForTests("libbindgen", "android_arm64_armv8-a_source").Output("bindings.rs")

	// The rule description should contain the custom binary name rather than bindgen, so checking the description
	// should be sufficient.
	if !strings.Contains(libbindgen.Description, "my_bindgen") {
		t.Errorf("Custom bindgen binary %s not used for libbindgen: rule description %#v", "my_bindgen",
			libbindgen.Description)
	}
}

func TestRustBindgenStdVersions(t *testing.T) {
	testRustError(t, "c_std and cpp_std cannot both be defined at the same time.", `
		rust_bindgen {
			name: "libbindgen",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			c_std: "somevalue",
			cpp_std: "somevalue",
		}
	`)

	ctx := testRust(t, `
		rust_bindgen {
			name: "libbindgen_cstd",
			wrapper_src: "src/any.hpp",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			c_std: "foo"
		}
		rust_bindgen {
			name: "libbindgen_cppstd",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			cpp_std: "foo"
		}
	`)

	libbindgen_cstd := ctx.ModuleForTests("libbindgen_cstd", "android_arm64_armv8-a_source").Output("bindings.rs")
	libbindgen_cppstd := ctx.ModuleForTests("libbindgen_cppstd", "android_arm64_armv8-a_source").Output("bindings.rs")

	if !strings.Contains(libbindgen_cstd.Args["cflags"], "-std=foo") {
		t.Errorf("c_std value not passed in to rust_bindgen as a clang flag")
	}

	if !strings.Contains(libbindgen_cppstd.Args["cflags"], "-std=foo") {
		t.Errorf("cpp_std value not passed in to rust_bindgen as a clang flag")
	}

	// Make sure specifying cpp_std emits the '-x c++' flag
	if !strings.Contains(libbindgen_cppstd.Args["cflags"], "-x c++") {
		t.Errorf("Setting cpp_std should cause the '-x c++' flag to be emitted")
	}

	// Make sure specifying c_std omits the '-x c++' flag
	if strings.Contains(libbindgen_cstd.Args["cflags"], "-x c++") {
		t.Errorf("Setting c_std should not cause the '-x c++' flag to be emitted")
	}
}

func TestBindgenDisallowedFlags(t *testing.T) {
	// Make sure passing '-x c++' to cflags generates an error
	testRustError(t, "cflags: -x c\\+\\+ should not be specified in cflags.*", `
		rust_bindgen {
			name: "libbad_flag",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			cflags: ["-x c++"]
		}
	`)

	// Make sure passing '-std=' to cflags generates an error
	testRustError(t, "cflags: -std should not be specified in cflags.*", `
		rust_bindgen {
			name: "libbad_flag",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			cflags: ["-std=foo"]
		}
	`)
}

func TestBindgenFlagFile(t *testing.T) {
	ctx := testRust(t, `
		rust_bindgen {
			name: "libbindgen",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			bindgen_flag_files: [
				"flag_file.txt",
			],
		}
	`)
	libbindgen := ctx.ModuleForTests("libbindgen", "android_arm64_armv8-a_source").Output("bindings.rs")

	if !strings.Contains(libbindgen.Args["flagfiles"], "/dev/null") {
		t.Errorf("missing /dev/null in rust_bindgen rule: flags %#v", libbindgen.Args["flagfiles"])
	}
	if !strings.Contains(libbindgen.Args["flagfiles"], "flag_file.txt") {
		t.Errorf("missing bindgen flags file in rust_bindgen rule: flags %#v", libbindgen.Args["flagfiles"])
	}
	// TODO: The best we can do right now is check $flagfiles. Once bindgen.go switches to RuleBuilder,
	// we may be able to check libbinder.RuleParams.Command to see if it contains $(cat /dev/null flag_file.txt)
}

func TestBindgenHandleStaticInlining(t *testing.T) {
	ctx := testRust(t, `
		rust_bindgen {
			name: "libbindgen",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			handle_static_inline: true,
			static_inline_library: "libbindgen_staticfns"
		}

		cc_library_static {
			name: "libbindgen_staticfns",
			srcs: [":libbindgen"],
			include_dirs: ["src/"],
		}
	`)
	libbindgen := ctx.ModuleForTests("libbindgen", "android_arm64_armv8-a_source").Output("bindings.rs")
	// Make sure the flag to support `static inline` functions is present
	if !strings.Contains(libbindgen.Args["flags"], "--wrap-static-fns") {
		t.Errorf("missing flag to handle static inlining in rust_bindgen rule: flags %#v", libbindgen.Args["flags"])
	}

	if !strings.Contains(libbindgen.Args["flags"], "--wrap-static-fns-path") {
		t.Errorf("missing flag to define path for static inlining C source from bindgen (--wrap-static-fns-path): flags %#v", libbindgen.Args["flags"])
	}

}

func TestBindgenStaticInlineProperties(t *testing.T) {
	// Make sure handle_static_inline without static_inline_library generates an error
	testRustError(t, "requires declaring static_inline_library to the corresponding cc_library module that includes the generated C source from bindgen", `
		rust_bindgen {
			name: "libbindgen",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			handle_static_inline: true
		}
	`)
	testRustError(t, "requires declaring handle_static_inline", `
		rust_bindgen {
			name: "libbindgen",
			wrapper_src: "src/any.h",
			crate_name: "bindgen",
			stem: "libbindgen",
			source_stem: "bindings",
			static_inline_library: "libbindgen_staticfns"
		}

		cc_library_static {
			name: "libbindgen_staticfns",
			srcs: [":libbindgen"],
			include_dirs: ["src/"],
		}
	`)
}
