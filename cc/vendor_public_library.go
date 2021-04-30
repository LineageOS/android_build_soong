// Copyright 2018 Google Inc. All rights reserved.
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

var (
	vendorPublicLibrarySuffix = ".vendorpublic"
)

// Creates a stub shared library for a vendor public library. Vendor public libraries
// are vendor libraries (owned by them and installed to /vendor partition) that are
// exposed to Android apps via JNI. The libraries are made public by being listed in
// /vendor/etc/public.libraries.txt.
//
// This stub library is a build-time only artifact that provides symbols that are
// exposed from a vendor public library.
//
// Example:
//
// vendor_public_library {
//     name: "libfoo",
//     symbol_file: "libfoo.map.txt",
//     export_public_headers: ["libfoo_headers"],
// }
//
// cc_headers {
//     name: "libfoo_headers",
//     export_include_dirs: ["include"],
// }
//
type vendorPublicLibraryProperties struct {
	// Relative path to the symbol map.
	Symbol_file *string

	// Whether the system library uses symbol versions.
	Unversioned *bool

	// list of header libs to re-export include directories from.
	Export_public_headers []string `android:"arch_variant"`

	// list of directories relative to the Blueprints file that willbe added to the include path
	// (using -I) for any module that links against the LLNDK variant of this module, replacing
	// any that were listed outside the llndk clause.
	Override_export_include_dirs []string
}
