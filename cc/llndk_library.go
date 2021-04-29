// Copyright 2017 Google Inc. All rights reserved.
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
	llndkLibrarySuffix = ".llndk"
	llndkHeadersSuffix = ".llndk"
)

// Holds properties to describe a stub shared library based on the provided version file.
type llndkLibraryProperties struct {
	// Relative path to the symbol map.
	// An example file can be seen here: TODO(danalbert): Make an example.
	Symbol_file *string

	// Whether to export any headers as -isystem instead of -I. Mainly for use by
	// bionic/libc.
	Export_headers_as_system *bool

	// Which headers to process with versioner. This really only handles
	// bionic/libc/include right now.
	Export_preprocessed_headers []string

	// Whether the system library uses symbol versions.
	Unversioned *bool

	// list of llndk headers to re-export include directories from.
	Export_llndk_headers []string

	// list of directories relative to the Blueprints file that willbe added to the include path
	// (using -I) for any module that links against the LLNDK variant of this module, replacing
	// any that were listed outside the llndk clause.
	Override_export_include_dirs []string

	// whether this module can be directly depended upon by libs that are installed
	// to /vendor and /product.
	// When set to true, this module can only be depended on by VNDK libraries, not
	// vendor nor product libraries. This effectively hides this module from
	// non-system modules. Default value is false.
	Private *bool

	// if true, make this module available to provide headers to other modules that set
	// llndk.symbol_file.
	Llndk_headers *bool
}
