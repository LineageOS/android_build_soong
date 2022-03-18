// Copyright 2016 Google Inc. All rights reserved.
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
	"strings"

	"android/soong/bazel"

	"github.com/google/blueprint"
)

func init() {
	RegisterModuleType("filegroup", FileGroupFactory)
}

var PrepareForTestWithFilegroup = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterModuleType("filegroup", FileGroupFactory)
})

// IsFilegroup checks that a module is a filegroup type
func IsFilegroup(ctx bazel.OtherModuleContext, m blueprint.Module) bool {
	return ctx.OtherModuleType(m) == "filegroup"
}

// https://docs.bazel.build/versions/master/be/general.html#filegroup
type bazelFilegroupAttributes struct {
	Srcs bazel.LabelListAttribute
}

// ConvertWithBp2build performs bp2build conversion of filegroup
func (fg *fileGroup) ConvertWithBp2build(ctx TopDownMutatorContext) {
	srcs := bazel.MakeLabelListAttribute(
		BazelLabelForModuleSrcExcludes(ctx, fg.properties.Srcs, fg.properties.Exclude_srcs))

	// For Bazel compatibility, don't generate the filegroup if there is only 1
	// source file, and that the source file is named the same as the module
	// itself. In Bazel, eponymous filegroups like this would be an error.
	//
	// Instead, dependents on this single-file filegroup can just depend
	// on the file target, instead of rule target, directly.
	//
	// You may ask: what if a filegroup has multiple files, and one of them
	// shares the name? The answer: we haven't seen that in the wild, and
	// should lock Soong itself down to prevent the behavior. For now,
	// we raise an error if bp2build sees this problem.
	for _, f := range srcs.Value.Includes {
		if f.Label == fg.Name() {
			if len(srcs.Value.Includes) > 1 {
				ctx.ModuleErrorf("filegroup '%s' cannot contain a file with the same name", fg.Name())
			}
			return
		}
	}

	attrs := &bazelFilegroupAttributes{
		Srcs: srcs,
	}

	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "filegroup",
		Bzl_load_location: "//build/bazel/rules:filegroup.bzl",
	}

	ctx.CreateBazelTargetModule(props, CommonAttributes{Name: fg.Name()}, attrs)
}

type fileGroupProperties struct {
	// srcs lists files that will be included in this filegroup
	Srcs []string `android:"path"`

	Exclude_srcs []string `android:"path"`

	// The base path to the files.  May be used by other modules to determine which portion
	// of the path to use.  For example, when a filegroup is used as data in a cc_test rule,
	// the base path is stripped off the path and the remaining path is used as the
	// installation directory.
	Path *string

	// Create a make variable with the specified name that contains the list of files in the
	// filegroup, relative to the root of the source tree.
	Export_to_make_var *string
}

type fileGroup struct {
	ModuleBase
	BazelModuleBase
	properties fileGroupProperties
	srcs       Paths
}

var _ SourceFileProducer = (*fileGroup)(nil)

// filegroup contains a list of files that are referenced by other modules
// properties (such as "srcs") using the syntax ":<name>". filegroup are
// also be used to export files across package boundaries.
func FileGroupFactory() Module {
	module := &fileGroup{}
	module.AddProperties(&module.properties)
	InitAndroidModule(module)
	InitBazelModule(module)
	return module
}

func (fg *fileGroup) maybeGenerateBazelBuildActions(ctx ModuleContext) {
	if !fg.MixedBuildsEnabled(ctx) {
		return
	}

	archVariant := ctx.Arch().String()
	osVariant := ctx.Os()
	if len(fg.Srcs()) == 1 && fg.Srcs()[0].Base() == fg.Name() {
		// This will be a regular file target, not filegroup, in Bazel.
		// See FilegroupBp2Build for more information.
		archVariant = Common.String()
		osVariant = CommonOS
	}

	bazelCtx := ctx.Config().BazelContext
	filePaths, ok := bazelCtx.GetOutputFiles(fg.GetBazelLabel(ctx, fg), configKey{archVariant, osVariant})
	if !ok {
		return
	}

	bazelOuts := make(Paths, 0, len(filePaths))
	for _, p := range filePaths {
		src := PathForBazelOut(ctx, p)
		bazelOuts = append(bazelOuts, src)
	}

	fg.srcs = bazelOuts
}

func (fg *fileGroup) GenerateAndroidBuildActions(ctx ModuleContext) {
	fg.srcs = PathsForModuleSrcExcludes(ctx, fg.properties.Srcs, fg.properties.Exclude_srcs)
	if fg.properties.Path != nil {
		fg.srcs = PathsWithModuleSrcSubDir(ctx, fg.srcs, String(fg.properties.Path))
	}

	fg.maybeGenerateBazelBuildActions(ctx)
}

func (fg *fileGroup) Srcs() Paths {
	return append(Paths{}, fg.srcs...)
}

func (fg *fileGroup) MakeVars(ctx MakeVarsModuleContext) {
	if makeVar := String(fg.properties.Export_to_make_var); makeVar != "" {
		ctx.StrictRaw(makeVar, strings.Join(fg.srcs.Strings(), " "))
	}
}
