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

package cc

import (
	"strings"
	"testing"
)

func TestVendorPublicLibraries(t *testing.T) {
	ctx := testCc(t, `
	cc_library_headers {
		name: "libvendorpublic_headers",
		product_available: true,
		export_include_dirs: ["my_include"],
	}
	cc_library {
		name: "libvendorpublic",
		srcs: ["foo.c"],
		vendor: true,
		no_libcrt: true,
		nocrt: true,
		vendor_public_library: {
			symbol_file: "libvendorpublic.map.txt",
			export_public_headers: ["libvendorpublic_headers"],
		},
	}

	cc_library {
		name: "libsystem",
		shared_libs: ["libvendorpublic"],
		vendor: false,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	cc_library {
		name: "libproduct",
		shared_libs: ["libvendorpublic"],
		product_specific: true,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	cc_library {
		name: "libvendor",
		shared_libs: ["libvendorpublic"],
		vendor: true,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	`)

	coreVariant := "android_arm64_armv8-a_shared"
	vendorVariant := "android_vendor.29_arm64_armv8-a_shared"
	productVariant := "android_product.29_arm64_armv8-a_shared"

	// test if header search paths are correctly added
	// _static variant is used since _shared reuses *.o from the static variant
	cc := ctx.ModuleForTests("libsystem", strings.Replace(coreVariant, "_shared", "_static", 1)).Rule("cc")
	cflags := cc.Args["cFlags"]
	if !strings.Contains(cflags, "-Imy_include") {
		t.Errorf("cflags for libsystem must contain -Imy_include, but was %#v.", cflags)
	}

	// test if libsystem is linked to the stub
	ld := ctx.ModuleForTests("libsystem", coreVariant).Rule("ld")
	libflags := ld.Args["libFlags"]
	stubPaths := getOutputPaths(ctx, coreVariant, []string{"libvendorpublic"})
	if !strings.Contains(libflags, stubPaths[0].String()) {
		t.Errorf("libflags for libsystem must contain %#v, but was %#v", stubPaths[0], libflags)
	}

	// test if libsystem is linked to the stub
	ld = ctx.ModuleForTests("libproduct", productVariant).Rule("ld")
	libflags = ld.Args["libFlags"]
	stubPaths = getOutputPaths(ctx, productVariant, []string{"libvendorpublic"})
	if !strings.Contains(libflags, stubPaths[0].String()) {
		t.Errorf("libflags for libproduct must contain %#v, but was %#v", stubPaths[0], libflags)
	}

	// test if libvendor is linked to the real shared lib
	ld = ctx.ModuleForTests("libvendor", vendorVariant).Rule("ld")
	libflags = ld.Args["libFlags"]
	stubPaths = getOutputPaths(ctx, vendorVariant, []string{"libvendorpublic"})
	if !strings.Contains(libflags, stubPaths[0].String()) {
		t.Errorf("libflags for libvendor must contain %#v, but was %#v", stubPaths[0], libflags)
	}
}
