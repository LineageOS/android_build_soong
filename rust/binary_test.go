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

// Test that rustlibs default linkage is correct for binaries.
func TestBinaryLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_binary {
			name: "fizz-buzz",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
			host_supported: true,
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
			host_supported: true,
		}`)

	fizzBuzzHost := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Module().(*Module)
	fizzBuzzDevice := ctx.ModuleForTests("fizz-buzz", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libfoo", fizzBuzzHost.Properties.AndroidMkRlibs) {
		t.Errorf("rustlibs dependency libfoo should be an rlib dep for host modules")
	}

	if !android.InList("libfoo", fizzBuzzDevice.Properties.AndroidMkDylibs) {
		t.Errorf("rustlibs dependency libfoo should be an dylib dep for device modules")
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

	fizzBuzz := ctx.ModuleForTests("fizz-buzz", "linux_glibc_x86_64").Output("fizz-buzz")

	flags := fizzBuzz.Args["rustcFlags"]
	if strings.Contains(flags, "--test") {
		t.Errorf("extra --test flag, rustcFlags: %#v", flags)
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

	fizzBuzz := ctx.ModuleForTests("fizz-buzz", "android_arm64_armv8-a").Output("fizz-buzz")
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
	foo.Output("stripped/foo")
	// Check that the `cp` rules is using the stripped version as input.
	cp := foo.Rule("android.Cp")
	if !strings.HasSuffix(cp.Input.String(), "stripped/foo") {
		t.Errorf("installed binary not based on stripped version: %v", cp.Input)
	}

	fizzBar := ctx.ModuleForTests("bar", "android_arm64_armv8-a").MaybeOutput("stripped/bar")
	if fizzBar.Rule != nil {
		t.Errorf("stripped version of bar has been generated")
	}
}
