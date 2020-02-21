// Copyright 2020 Google Inc. All rights reserved.
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
	"strings"
	"testing"
)

func TestLibraryHeaders(t *testing.T) {
	ctx := testCc(t, `
	cc_library_headers {
		name: "headers",
		export_include_dirs: ["my_include"],
	}
	cc_library_static {
		name: "lib",
		srcs: ["foo.c"],
		header_libs: ["headers"],
	}
	`)

	// test if header search paths are correctly added
	cc := ctx.ModuleForTests("lib", "android_arm64_armv8-a_static").Rule("cc")
	cflags := cc.Args["cFlags"]
	if !strings.Contains(cflags, " -Imy_include ") {
		t.Errorf("cflags for libsystem must contain -Imy_include, but was %#v.", cflags)
	}
}

func TestPrebuiltLibraryHeaders(t *testing.T) {
	ctx := testCc(t, `
	cc_prebuilt_library_headers {
		name: "headers",
		export_include_dirs: ["my_include"],
	}
	cc_library_static {
		name: "lib",
		srcs: ["foo.c"],
		header_libs: ["headers"],
	}
	`)

	// test if header search paths are correctly added
	cc := ctx.ModuleForTests("lib", "android_arm64_armv8-a_static").Rule("cc")
	cflags := cc.Args["cFlags"]
	if !strings.Contains(cflags, " -Imy_include ") {
		t.Errorf("cflags for libsystem must contain -Imy_include, but was %#v.", cflags)
	}
}
