// Copyright 2019 Google Inc. All rights reserved.
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

package java

import (
	"strings"

	"android/soong/android"
)

func init() {
	RegisterHiddenApiSingletonComponents(android.InitRegistrationContext)
}

func RegisterHiddenApiSingletonComponents(ctx android.RegistrationContext) {
	ctx.RegisterParallelSingletonType("hiddenapi", hiddenAPISingletonFactory)
}

var PrepareForTestWithHiddenApiBuildComponents = android.FixtureRegisterWithContext(RegisterHiddenApiSingletonComponents)

type hiddenAPISingletonPathsStruct struct {
	// The path to the CSV file that contains the flags that will be encoded into the dex boot jars.
	//
	// It is created by the generate_hiddenapi_lists.py tool that is passed the stubFlags along with
	// a number of additional files that are used to augment the information in the stubFlags with
	// manually curated data.
	flags android.OutputPath

	// The path to the CSV index file that contains mappings from Java signature to source location
	// information for all Java elements annotated with the UnsupportedAppUsage annotation in the
	// source of all the boot jars.
	//
	// It is created by the merge_csv tool which merges all the hiddenAPI.indexCSVPath files that have
	// been created by the rest of the build. That includes the index files generated for
	// <x>-hiddenapi modules.
	index android.OutputPath

	// The path to the CSV metadata file that contains mappings from Java signature to the value of
	// properties specified on UnsupportedAppUsage annotations in the source of all the boot jars.
	//
	// It is created by the merge_csv tool which merges all the hiddenAPI.metadataCSVPath files that
	// have been created by the rest of the build. That includes the metadata files generated for
	// <x>-hiddenapi modules.
	metadata android.OutputPath

	// The path to the CSV metadata file that contains mappings from Java signature to flags obtained
	// from the public, system and test API stubs.
	//
	// This is created by the hiddenapi tool which is given dex files for the public, system and test
	// API stubs (including product specific stubs) along with dex boot jars, so does not include
	// <x>-hiddenapi modules. For each API surface (i.e. public, system, test) it records which
	// members in the dex boot jars match a member in the dex stub jars for that API surface and then
	// outputs a file containing the signatures of all members in the dex boot jars along with the
	// flags that indicate which API surface it belongs, if any.
	//
	// e.g. a dex member that matches a member in the public dex stubs would have flags
	// "public-api,system-api,test-api" set (as system and test are both supersets of public). A dex
	// member that didn't match a member in any of the dex stubs is still output it just has an empty
	// set of flags.
	//
	// The notion of matching is quite complex, it is not restricted to just exact matching but also
	// follows the Java inheritance rules. e.g. if a method is public then all overriding/implementing
	// methods are also public. If an interface method is public and a class inherits an
	// implementation of that method from a super class then that super class method is also public.
	// That ensures that any method that can be called directly by an App through a public method is
	// visible to that App.
	//
	// Propagating the visibility of members across the inheritance hierarchy at build time will cause
	// problems when modularizing and unbundling as it that propagation can cross module boundaries.
	// e.g. Say that a private framework class implements a public interface and inherits an
	// implementation of one of its methods from a core platform ART class. In that case the ART
	// implementation method needs to be marked as public which requires the build to have access to
	// the framework implementation classes at build time. The work to rectify this is being tracked
	// at http://b/178693149.
	//
	// This file (or at least those items marked as being in the public-api) is used by hiddenapi when
	// creating the metadata and flags for the individual modules in order to perform consistency
	// checks and filter out bridge methods that are part of the public API. The latter relies on the
	// propagation of visibility across the inheritance hierarchy.
	stubFlags android.OutputPath
}

var hiddenAPISingletonPathsKey = android.NewOnceKey("hiddenAPISingletonPathsKey")

// hiddenAPISingletonPaths creates all the paths for singleton files the first time it is called, which may be
// from a ModuleContext that needs to reference a file that will be created by a singleton rule that hasn't
// yet been created.
func hiddenAPISingletonPaths(ctx android.PathContext) hiddenAPISingletonPathsStruct {
	return ctx.Config().Once(hiddenAPISingletonPathsKey, func() interface{} {
		// Make the paths relative to the out/soong/hiddenapi directory instead of to the out/soong/
		// directory. This ensures that if they are used as java_resources they do not end up in a
		// hiddenapi directory in the resulting APK.
		hiddenapiDir := android.PathForOutput(ctx, "hiddenapi")
		return hiddenAPISingletonPathsStruct{
			flags:     hiddenapiDir.Join(ctx, "hiddenapi-flags.csv"),
			index:     hiddenapiDir.Join(ctx, "hiddenapi-index.csv"),
			metadata:  hiddenapiDir.Join(ctx, "hiddenapi-unsupported.csv"),
			stubFlags: hiddenapiDir.Join(ctx, "hiddenapi-stub-flags.txt"),
		}
	}).(hiddenAPISingletonPathsStruct)
}

func hiddenAPISingletonFactory() android.Singleton {
	return &hiddenAPISingleton{}
}

type hiddenAPISingleton struct {
}

// hiddenAPI singleton rules
func (h *hiddenAPISingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// Don't run any hiddenapi rules if hiddenapi checks are disabled
	if ctx.Config().DisableHiddenApiChecks() {
		return
	}

	// If there is a prebuilt hiddenapi dir, generate rules to use the
	// files within. Generally, we build the hiddenapi files from source
	// during the build, ensuring consistency. It's possible, in a split
	// build (framework and vendor) scenario, for the vendor build to use
	// prebuilt hiddenapi files from the framework build. In this scenario,
	// the framework and vendor builds must use the same source to ensure
	// consistency.

	if ctx.Config().PrebuiltHiddenApiDir(ctx) != "" {
		prebuiltFlagsRule(ctx)
		prebuiltIndexRule(ctx)
		return
	}
}

// Checks to see whether the supplied module variant is in the list of boot jars.
//
// TODO(b/179354495): Avoid having to perform this type of check.
func isModuleInConfiguredList(ctx android.BaseModuleContext, module android.Module, configuredBootJars android.ConfiguredJarList) bool {
	name := ctx.OtherModuleName(module)

	// Strip a prebuilt_ prefix so that this can match a prebuilt module that has not been renamed.
	name = android.RemoveOptionalPrebuiltPrefix(name)

	// Ignore any module that is not listed in the boot image configuration.
	index := configuredBootJars.IndexOfJar(name)
	if index == -1 {
		return false
	}

	// It is an error if the module is not an ApexModule.
	if _, ok := module.(android.ApexModule); !ok {
		ctx.ModuleErrorf("%s is configured in boot jars but does not support being added to an apex", ctx.OtherModuleName(module))
		return false
	}

	apexInfo := ctx.OtherModuleProvider(module, android.ApexInfoProvider).(android.ApexInfo)

	// Now match the apex part of the boot image configuration.
	requiredApex := configuredBootJars.Apex(index)
	if android.IsConfiguredJarForPlatform(requiredApex) {
		if len(apexInfo.InApexVariants) != 0 {
			// A platform variant is required but this is for an apex so ignore it.
			return false
		}
	} else if !apexInfo.InApexVariant(requiredApex) {
		// An apex variant for a specific apex is required but this is the wrong apex.
		return false
	}

	return true
}

func prebuiltFlagsRule(ctx android.SingletonContext) {
	outputPath := hiddenAPISingletonPaths(ctx).flags
	inputPath := android.PathForSource(ctx, ctx.Config().PrebuiltHiddenApiDir(ctx), "hiddenapi-flags.csv")

	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Output: outputPath,
		Input:  inputPath,
	})
}

func prebuiltIndexRule(ctx android.SingletonContext) {
	outputPath := hiddenAPISingletonPaths(ctx).index
	inputPath := android.PathForSource(ctx, ctx.Config().PrebuiltHiddenApiDir(ctx), "hiddenapi-index.csv")

	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Output: outputPath,
		Input:  inputPath,
	})
}

// tempPathForRestat creates a path of the same type as the supplied type but with a name of
// <path>.tmp.
//
// e.g. If path is an OutputPath for out/soong/hiddenapi/hiddenapi-flags.csv then this will return
// an OutputPath for out/soong/hiddenapi/hiddenapi-flags.csv.tmp
func tempPathForRestat(ctx android.PathContext, path android.WritablePath) android.WritablePath {
	extWithoutLeadingDot := strings.TrimPrefix(path.Ext(), ".")
	return path.ReplaceExtension(ctx, extWithoutLeadingDot+".tmp")
}

// commitChangeForRestat adds a command to a rule that updates outputPath from tempPath if they are different.  It
// also marks the rule as restat and marks the tempPath as a temporary file that should not be considered an output of
// the rule.
func commitChangeForRestat(rule *android.RuleBuilder, tempPath, outputPath android.WritablePath) {
	rule.Restat()
	rule.Temporary(tempPath)
	rule.Command().
		Text("(").
		Text("if").
		Text("cmp -s").Input(tempPath).Output(outputPath).Text(";").
		Text("then").
		Text("rm").Input(tempPath).Text(";").
		Text("else").
		Text("mv").Input(tempPath).Output(outputPath).Text(";").
		Text("fi").
		Text(")")
}
