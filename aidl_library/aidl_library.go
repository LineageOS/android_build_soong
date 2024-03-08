// Copyright 2023 Google Inc. All rights reserved.
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

package aidl_library

import (
	"android/soong/android"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var PrepareForTestWithAidlLibrary = android.FixtureRegisterWithContext(func(ctx android.RegistrationContext) {
	registerAidlLibraryBuildComponents(ctx)
})

func init() {
	registerAidlLibraryBuildComponents(android.InitRegistrationContext)
}

func registerAidlLibraryBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("aidl_library", AidlLibraryFactory)
}

type aidlLibraryProperties struct {
	// srcs lists files that are included in this module for aidl compilation
	Srcs []string `android:"path"`

	// hdrs lists the headers that are imported by srcs but are not compiled by aidl to language binding code
	// hdrs is provided to support Bazel migration. It is a no-op until
	// we enable input sandbox in aidl compilation action
	Hdrs []string `android:"path"`

	// The prefix to strip from the paths of the .aidl files
	// The remaining path is the package path of the aidl interface
	Strip_import_prefix *string

	// List of aidl files or aidl_library depended on by the module
	Deps []string `android:"arch_variant"`
}

type AidlLibrary struct {
	android.ModuleBase
	properties aidlLibraryProperties
}

type AidlLibraryInfo struct {
	// The direct aidl files of the module
	Srcs android.Paths
	// The include dirs to the direct aidl files and those provided from transitive aidl_library deps
	IncludeDirs android.DepSet[android.Path]
	// The direct hdrs and hdrs from transitive deps
	Hdrs android.DepSet[android.Path]
}

// AidlLibraryProvider provides the srcs and the transitive include dirs
var AidlLibraryProvider = blueprint.NewProvider(AidlLibraryInfo{})

func (lib *AidlLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	includeDirsDepSetBuilder := android.NewDepSetBuilder[android.Path](android.PREORDER)
	hdrsDepSetBuilder := android.NewDepSetBuilder[android.Path](android.PREORDER)

	if len(lib.properties.Srcs) == 0 && len(lib.properties.Hdrs) == 0 {
		ctx.ModuleErrorf("at least srcs or hdrs prop must be non-empty")
	}

	srcs := android.PathsForModuleSrc(ctx, lib.properties.Srcs)
	hdrs := android.PathsForModuleSrc(ctx, lib.properties.Hdrs)

	if lib.properties.Strip_import_prefix != nil {
		srcs = android.PathsWithModuleSrcSubDir(
			ctx,
			srcs,
			android.String(lib.properties.Strip_import_prefix),
		)

		hdrs = android.PathsWithModuleSrcSubDir(
			ctx,
			hdrs,
			android.String(lib.properties.Strip_import_prefix),
		)
	}
	hdrsDepSetBuilder.Direct(hdrs...)

	includeDir := android.PathForModuleSrc(
		ctx,
		proptools.StringDefault(lib.properties.Strip_import_prefix, ""),
	)
	includeDirsDepSetBuilder.Direct(includeDir)

	for _, dep := range ctx.GetDirectDepsWithTag(aidlLibraryTag) {
		if ctx.OtherModuleHasProvider(dep, AidlLibraryProvider) {
			info := ctx.OtherModuleProvider(dep, AidlLibraryProvider).(AidlLibraryInfo)
			includeDirsDepSetBuilder.Transitive(&info.IncludeDirs)
			hdrsDepSetBuilder.Transitive(&info.Hdrs)
		}
	}

	ctx.SetProvider(AidlLibraryProvider, AidlLibraryInfo{
		Srcs:        srcs,
		IncludeDirs: *includeDirsDepSetBuilder.Build(),
		Hdrs:        *hdrsDepSetBuilder.Build(),
	})
}

// aidl_library contains a list of .aidl files and the strip_import_prefix to
// to strip from the paths of the .aidl files. The sub-path left-over after stripping
// corresponds to the aidl package path the aidl interfaces are scoped in
func AidlLibraryFactory() android.Module {
	module := &AidlLibrary{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

type aidlDependencyTag struct {
	blueprint.BaseDependencyTag
}

var aidlLibraryTag = aidlDependencyTag{}

func (lib *AidlLibrary) DepsMutator(ctx android.BottomUpMutatorContext) {
	for _, dep := range lib.properties.Deps {
		ctx.AddDependency(lib, aidlLibraryTag, dep)
	}
}
