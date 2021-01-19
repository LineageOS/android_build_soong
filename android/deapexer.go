// Copyright (C) 2021 The Android Open Source Project
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

package android

import (
	"fmt"
	"strings"

	"github.com/google/blueprint"
)

// Provides support for interacting with the `deapexer` module to which a `prebuilt_apex` module
// will delegate the work to export files from a prebuilt '.apex` file.

// The information exported by the `deapexer` module, access it using `DeapxerInfoProvider`.
type DeapexerInfo struct {
	// map from the name of an exported file from a prebuilt_apex to the path to that file. The
	// exported file name is of the form <module>{<tag>} where <tag> is currently only allowed to be
	// ".dexjar".
	//
	// See Prebuilt.ApexInfoMutator for more information.
	exports map[string]Path
}

// The set of supported prebuilt export tags. Used to verify the tag parameter for
// `PrebuiltExportPath`.
var supportedPrebuiltExportTags = map[string]struct{}{
	".dexjar": {},
}

// PrebuiltExportPath provides the path, or nil if not available, of a file exported from the
// prebuilt_apex that created this ApexInfo.
//
// The exported file is identified by the module name and the tag:
// * The module name is the name of the module that contributed the file when the .apex file
//   referenced by the prebuilt_apex was built. It must be specified in one of the exported_...
//   properties on the prebuilt_apex module.
// * The tag identifies the type of file and is dependent on the module type.
//
// See apex/deapexer.go for more information.
func (i DeapexerInfo) PrebuiltExportPath(name, tag string) Path {

	if _, ok := supportedPrebuiltExportTags[tag]; !ok {
		panic(fmt.Errorf("unsupported prebuilt export tag %q, expected one of %s",
			tag, strings.Join(SortedStringKeys(supportedPrebuiltExportTags), ", ")))
	}

	path := i.exports[name+"{"+tag+"}"]
	return path
}

// Provider that can be used from within the `GenerateAndroidBuildActions` of a module that depends
// on a `deapexer` module to retrieve its `DeapexerInfo`.
var DeapexerProvider = blueprint.NewProvider(DeapexerInfo{})

// NewDeapexerInfo creates and initializes a DeapexerInfo that is suitable
// for use with a prebuilt_apex module.
//
// See apex/deapexer.go for more information.
func NewDeapexerInfo(exports map[string]Path) DeapexerInfo {
	return DeapexerInfo{
		exports: exports,
	}
}

type deapexerTagStruct struct {
	blueprint.BaseDependencyTag
}

// A tag that is used for dependencies on the `deapexer` module.
var DeapexerTag = deapexerTagStruct{}
