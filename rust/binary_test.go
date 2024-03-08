// Copyright 2019 The Android Open Source Project
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

// Test that rustlibs default linkage is always rlib for host binaries.
func TestBinaryHostLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_binary_host {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
			host_supported: true,
		}
	`)
	fizzBuzz := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Module().(*Module)
	if !android.InList("libfoo.rlib-std", fizzBuzz.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs dependency libfoo should be an rlib dep for host binaries")
	}
}

// Test that rustlibs default linkage is correct for binaries.
func TestBinaryLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
			host_supported: true,
		}
		rust_binary {
			name: "rlib_linked",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
			host_supported: true,
			prefer_rlib: true,
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
			host_supported: true,
		}`)

	fizzBuzzHost := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Module().(*Module)
	fizzBuzzDevice := ctx.ModuleForTests("fizz-buzz", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libfoo.rlib-std", fizzBuzzHost.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs dependency libfoo should be an rlib dep for host modules")
	}

	if !android.InList("libfoo", fizzBuzzDevice.Properties.AndroidMkDylibs) {
		t.Errorf("rustlibs dependency libfoo should be an dylib dep for device modules")
	}

	rlibLinkDevice := ctx.ModuleForTests("rlib_linked", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libfoo.rlib-std", rlibLinkDevice.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs dependency libfoo should be an rlib dep for device modules when prefer_rlib is set")
	}
}

// Test that prefer_rlib links in libstd statically as well as rustlibs.
func TestBinaryPreferRlib(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "rlib_linked",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
			host_supported: true,
			prefer_rlib: true,
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
			host_supported: true,
		}`)

	mod := ctx.ModuleForTests("rlib_linked", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libfoo.rlib-std", mod.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs dependency libfoo should be an rlib dep when prefer_rlib is defined")
	}

	if !android.InList("libstd", mod.Properties.AndroidMkRlibs) {
		t.Errorf("libstd dependency should be an rlib dep when prefer_rlib is defined")
	}
}

// Test that the path returned by HostToolPath is correct
func TestHostToolPath(t *testing.T) {
	ctx := testRust(t, `
		rust_binary_host {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
		}`)

	path := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Module().(*Module).HostToolPath()
	if g, w := path.String(), "/host/linux-x86/bin/fizz-buzz"; !strings.Contains(g, w) {
		t.Errorf("wrong host tool path, expected %q got %q", w, g)
	}
}

// Test that the flags being passed to rust_binary modules are as expected
func TestBinaryFlags(t *testing.T) {
	ctx := testRust(t, `
		rust_binary_host {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
		}`)

	fizzBuzz := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Rule("rustc")

	flags := fizzBuzz.Args["rustcFlags"]
	if strings.Contains(flags, "--test") {
		t.Errorf("extra --test flag, rustcFlags: %#v", flags)
	}
}

// Test that the bootstrap property sets the appropriate linker
func TestBootstrap(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "foo",
			srcs: ["foo.rs"],
			bootstrap: true,
		}`)

	foo := ctx.ModuleForTests("foo", "android_arm64_armv8-a").Rule("rustc")

	flag := "-Wl,-dynamic-linker,/system/bin/bootstrap/linker64"
	if !strings.Contains(foo.Args["linkFlags"], flag) {
		t.Errorf("missing link flag to use bootstrap linker, expecting %#v, linkFlags: %#v", flag, foo.Args["linkFlags"])
	}
}

func TestStaticBinaryFlags(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz",
			srcs: ["foo.rs"],
			static_executable: true,
		}`)

	fizzOut := ctx.ModuleForTests("fizz", "android_arm64_armv8-a").Rule("rustc")
	fizzMod := ctx.ModuleForTests("fizz", "android_arm64_armv8-a").Module().(*Module)

	flags := fizzOut.Args["rustcFlags"]
	linkFlags := fizzOut.Args["linkFlags"]
	if !strings.Contains(flags, "-C relocation-model=static") {
		t.Errorf("static binary missing '-C relocation-model=static' in rustcFlags, found: %#v", flags)
	}
	if !strings.Contains(flags, "-C panic=abort") {
		t.Errorf("static binary missing '-C panic=abort' in rustcFlags, found: %#v", flags)
	}
	if !strings.Contains(linkFlags, "-static") {
		t.Errorf("static binary missing '-static' in linkFlags, found: %#v", flags)
	}

	if !android.InList("libc", fizzMod.Properties.AndroidMkStaticLibs) {
		t.Errorf("static binary not linking against libc as a static library")
	}
	if len(fizzMod.transitiveAndroidMkSharedLibs.ToList()) > 0 {
		t.Errorf("static binary incorrectly linking against shared libraries")
	}
}

func TestLinkObjects(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
			shared_libs: ["libfoo"],
		}
		cc_library {
			name: "libfoo",
		}`)

	fizzBuzz := ctx.ModuleForTests("fizz-buzz", "android_arm64_armv8-a").Rule("rustc")
	linkFlags := fizzBuzz.Args["linkFlags"]
	if !strings.Contains(linkFlags, "/libfoo.so") {
		t.Errorf("missing shared dependency 'libfoo.so' in linkFlags: %#v", linkFlags)
	}
}

// Test that stripped versions are correctly generated and used.
func TestStrippedBinary(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "foo",
			srcs: ["foo.rs"],
		}
		rust_binary {
			name: "bar",
			srcs: ["foo.rs"],
			strip: {
				none: true
			}
		}
	`)

	foo := ctx.ModuleForTests("foo", "android_arm64_armv8-a")
	foo.Output("unstripped/foo")
	foo.Output("foo")

	// Check that the `cp` rules is using the stripped version as input.
	cp := foo.Rule("android.Cp")
	if strings.HasSuffix(cp.Input.String(), "unstripped/foo") {
		t.Errorf("installed binary not based on stripped version: %v", cp.Input)
	}

	fizzBar := ctx.ModuleForTests("bar", "android_arm64_armv8-a").MaybeOutput("unstripped/bar")
	if fizzBar.Rule != nil {
		t.Errorf("unstripped binary exists, so stripped binary has incorrectly been generated")
	}
}
