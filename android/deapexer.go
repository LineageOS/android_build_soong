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
//
// The actual processing that is done is quite convoluted but it is all about combining information
// from multiple different sources in order to allow a prebuilt module to use a file extracted from
// an apex file. As follows:
//
// 1. A prebuilt module, e.g. prebuilt_bootclasspath_fragment or java_import needs to use a file
//    from a prebuilt_apex/apex_set. It knows the path of the file within the apex but does not know
//    where the apex file is or what apex to use.
//
// 2. The connection between the prebuilt module and the prebuilt_apex/apex_set is created through
//    use of an exported_... property on the latter. That causes four things to occur:
//    a. A `deapexer` mopdule is created by the prebuilt_apex/apex_set to extract files from the
//       apex file.
//    b. A dependency is added from the prebuilt_apex/apex_set modules onto the prebuilt modules
//       listed in those properties.
//    c. An APEX variant is created for each of those prebuilt modules.
//    d. A dependency is added from the prebuilt modules to the `deapexer` module.
//
// 3. The prebuilt_apex/apex_set modules do not know which files are available in the apex file.
//    That information could be specified on the prebuilt_apex/apex_set modules but without
//    automated generation of those modules it would be expensive to maintain. So, instead they
//    obtain that information from the prebuilt modules. They do not know what files are actually in
//    the apex file either but they know what files they need from it. So, the
//    prebuilt_apex/apex_set modules obtain the files that should be in the apex file from those
//    modules and then pass those onto the `deapexer` module.
//
// 4. The `deapexer` module's ninja rule extracts all the files from the apex file into an output
//    directory and checks that all the expected files are there. The expected files are declared as
//    the outputs of the ninja rule so they are available to other modules.
//
// 5. The prebuilt modules then retrieve the paths to the files that they needed from the `deapexer`
//    module.
//
// The files that are passed to `deapexer` and those that are passed back have a unique identifier
// that links them together. e.g. If the `deapexer` is passed something like this:
//     javalib/core-libart.jar -> javalib/core-libart.jar
// it will return something like this:
//     javalib/core-libart.jar -> out/soong/.....deapexer.../javalib/core-libart.jar
//
// The reason why the `deapexer` module is separate from the prebuilt_apex/apex_set is to avoid
// cycles. e.g.
//   prebuilt_apex "com.android.art" depends upon java_import "core-libart":
//       This is so it can create an APEX variant of the latter and obtain information about the
//       files that it needs from the apex file.
//   java_import "core-libart" depends upon `deapexer` module:
//       This is so it can retrieve the paths to the files it needs.

// The information exported by the `deapexer` module, access it using `DeapxerInfoProvider`.
type DeapexerInfo struct {
	apexModuleName string

	// map from the name of an exported file from a prebuilt_apex to the path to that file. The
	// exported file name is the apex relative path, e.g. javalib/core-libart.jar.
	//
	// See Prebuilt.ApexInfoMutator for more information.
	exports map[string]WritablePath

	// name of the java libraries exported from the apex
	// e.g. core-libart
	exportedModuleNames []string

	// name of the java libraries exported from the apex that should be dexpreopt'd with the .prof
	// file embedded in the apex
	dexpreoptProfileGuidedExportedModuleNames []string
}

// ApexModuleName returns the name of the APEX module that provided the info.
func (i DeapexerInfo) ApexModuleName() string {
	return i.apexModuleName
}

// PrebuiltExportPath provides the path, or nil if not available, of a file exported from the
// prebuilt_apex that created this ApexInfo.
//
// The exported file is identified by the apex relative path, e.g. "javalib/core-libart.jar".
//
// See apex/deapexer.go for more information.
func (i DeapexerInfo) PrebuiltExportPath(apexRelativePath string) WritablePath {
	path := i.exports[apexRelativePath]
	return path
}

func (i DeapexerInfo) GetExportedModuleNames() []string {
	return i.exportedModuleNames
}

// Provider that can be used from within the `GenerateAndroidBuildActions` of a module that depends
// on a `deapexer` module to retrieve its `DeapexerInfo`.
var DeapexerProvider = blueprint.NewProvider[DeapexerInfo]()

// NewDeapexerInfo creates and initializes a DeapexerInfo that is suitable
// for use with a prebuilt_apex module.
//
// See apex/deapexer.go for more information.
func NewDeapexerInfo(apexModuleName string, exports map[string]WritablePath, moduleNames []string) DeapexerInfo {
	return DeapexerInfo{
		apexModuleName:      apexModuleName,
		exports:             exports,
		exportedModuleNames: moduleNames,
	}
}

func (i *DeapexerInfo) GetDexpreoptProfileGuidedExportedModuleNames() []string {
	return i.dexpreoptProfileGuidedExportedModuleNames
}

func (i *DeapexerInfo) AddDexpreoptProfileGuidedExportedModuleNames(names ...string) {
	i.dexpreoptProfileGuidedExportedModuleNames = append(i.dexpreoptProfileGuidedExportedModuleNames, names...)
}

type deapexerTagStruct struct {
	blueprint.BaseDependencyTag
}

// Mark this tag so dependencies that use it are excluded from APEX contents.
func (t deapexerTagStruct) ExcludeFromApexContents() {}

var _ ExcludeFromApexContentsTag = DeapexerTag

// A tag that is used for dependencies on the `deapexer` module.
var DeapexerTag = deapexerTagStruct{}

// RequiredFilesFromPrebuiltApex must be implemented by modules that require files to be exported
// from a prebuilt_apex/apex_set.
type RequiredFilesFromPrebuiltApex interface {
	// RequiredFilesFromPrebuiltApex returns a list of the file paths (relative to the root of the
	// APEX's contents) that the implementing module requires from within a prebuilt .apex file.
	//
	// For each file path this will cause the file to be extracted out of the prebuilt .apex file, and
	// the path to the extracted file will be stored in the DeapexerInfo using the APEX relative file
	// path as the key, The path can then be retrieved using the PrebuiltExportPath(key) method.
	RequiredFilesFromPrebuiltApex(ctx BaseModuleContext) []string

	// Returns true if a transitive dependency of an apex should use a .prof file to guide dexpreopt
	UseProfileGuidedDexpreopt() bool
}

// Marker interface that identifies dependencies on modules that may require files from a prebuilt
// apex.
type RequiresFilesFromPrebuiltApexTag interface {
	blueprint.DependencyTag

	// Method that differentiates this interface from others.
	RequiresFilesFromPrebuiltApex()
}

// FindDeapexerProviderForModule searches through the direct dependencies of the current context
// module for a DeapexerTag dependency and returns its DeapexerInfo. If a single nonambiguous
// deapexer module isn't found then it returns it an error
// clients should check the value of error and call ctx.ModuleErrof if a non nil error is received
func FindDeapexerProviderForModule(ctx ModuleContext) (*DeapexerInfo, error) {
	var di *DeapexerInfo
	var err error
	ctx.VisitDirectDepsWithTag(DeapexerTag, func(m Module) {
		if err != nil {
			// An err has been found. Do not visit further.
			return
		}
		c, _ := OtherModuleProvider(ctx, m, DeapexerProvider)
		p := &c
		if di != nil {
			// If two DeapexerInfo providers have been found then check if they are
			// equivalent. If they are then use the selected one, otherwise fail.
			if selected := equivalentDeapexerInfoProviders(di, p); selected != nil {
				di = selected
				return
			}
			err = fmt.Errorf("Multiple installable prebuilt APEXes provide ambiguous deapexers: %s and %s", di.ApexModuleName(), p.ApexModuleName())
		}
		di = p
	})
	if err != nil {
		return nil, err
	}
	if di != nil {
		return di, nil
	}
	ai, _ := ModuleProvider(ctx, ApexInfoProvider)
	return nil, fmt.Errorf("No prebuilt APEX provides a deapexer module for APEX variant %s", ai.ApexVariationName)
}

// removeCompressedApexSuffix removes the _compressed suffix from the name if present.
func removeCompressedApexSuffix(name string) string {
	return strings.TrimSuffix(name, "_compressed")
}

// equivalentDeapexerInfoProviders checks to make sure that the two DeapexerInfo structures are
// equivalent.
//
// At the moment <x> and <x>_compressed APEXes are treated as being equivalent.
//
// If they are not equivalent then this returns nil, otherwise, this returns the DeapexerInfo that
// should be used by the build, which is always the uncompressed one. That ensures that the behavior
// of the build is not dependent on which prebuilt APEX is visited first.
func equivalentDeapexerInfoProviders(p1 *DeapexerInfo, p2 *DeapexerInfo) *DeapexerInfo {
	n1 := removeCompressedApexSuffix(p1.ApexModuleName())
	n2 := removeCompressedApexSuffix(p2.ApexModuleName())

	// If the names don't match then they are not equivalent.
	if n1 != n2 {
		return nil
	}

	// Select the uncompressed APEX.
	if n1 == removeCompressedApexSuffix(n1) {
		return p1
	} else {
		return p2
	}
}
