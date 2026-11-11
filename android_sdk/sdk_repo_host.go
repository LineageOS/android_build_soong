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

package android_sdk

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/cc/config"
)

var pctx = android.NewPackageContext("android/soong/android_sdk")

func init() {
	registerBuildComponents(android.InitRegistrationContext)
}

func registerBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_sdk_repo_host", SdkRepoHostFactory)
}

type sdkRepoHost struct {
	android.ModuleBase
	android.PackagingBase
	blueprint.ModuleUsesIncrementalWalkDeps

	properties sdkRepoHostProperties
}

type remapProperties struct {
	From string
	To   string
}

type sdkRepoHostProperties struct {
	// The top level directory to use for the SDK repo.
	Base_dir proptools.Configurable[string] `android:"replace_instead_of_append"`

	// List of src:dst mappings to rename files from `deps`.
	Deps_remap []remapProperties `android:"arch_variant"`

	// List of zip files to merge into the SDK repo.
	Merge_zips []string `android:"arch_variant,path"`

	// List of sources to include into the SDK repo. These are usually raw files, filegroups,
	// or genrules, as most built modules should be referenced via `deps`.
	Srcs []string `android:"arch_variant,path"`

	// List of files to strip. This should be a list of files, not modules. This happens after
	// `deps_remap` and `merge_zips` are applied, but before the `base_dir` is added.
	Strip_files []string `android:"arch_variant"`

	// Add $(OUT)/soong/framework.aidl into the SDK repo if set to true.
	Pack_framework_aidl *bool
}

// android_sdk_repo_host defines an Android SDK repo containing host tools.
//
// This implementation is trying to be a faithful reproduction of how these sdk-repos were produced
// in the Make system, which may explain some of the oddities (like `strip_files` not being
// automatic)
func SdkRepoHostFactory() android.Module {
	return newSdkRepoHostModule()
}

func newSdkRepoHostModule() *sdkRepoHost {
	s := &sdkRepoHost{}
	s.AddProperties(&s.properties)
	android.InitPackageModule(s)
	android.InitAndroidMultiTargetsArchModule(s, android.HostSupported, android.MultilibCommon)
	return s
}

// We need to implement IsNativeCoverageNeeded so that in coverage builds we don't get packaging
// conflicts with required deps that always use the coverage variant.
func (p *sdkRepoHost) IsNativeCoverageNeeded(ctx cc.IsNativeCoverageNeededContext) bool {
	return ctx.DeviceConfig().NativeCoverageEnabled()
}

var _ cc.UseCoverage = (*sdkRepoHost)(nil)

type dependencyTag struct {
	blueprint.BaseDependencyTag
	android.PackagingItemAlwaysDepTag
}

// TODO(b/201696252): Evaluate whether licenses should be propagated through this dependency.
func (d dependencyTag) PropagateLicenses() bool {
	return false
}

var depTag = dependencyTag{}

func (s *sdkRepoHost) DepsMutator(ctx android.BottomUpMutatorContext) {
	s.AddDeps(ctx, depTag)
}

func (s *sdkRepoHost) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	dir := android.PathForModuleOut(ctx, "zip")
	outputZipFile := dir.Join(ctx, "output.zip")
	builder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled().
		Sbox(dir, android.PathForModuleOut(ctx, "out.sbox.textproto"))

	// Always create the dir not rely on BuildNoticeTextOutputFromLicenseMetadata because the notice text may not be generated if there's not other deps.
	builder.Command().Textf("mkdir -p %s", dir)

	// Handle `merge_zips` by extracting their contents into our tmpdir.  Do this first in case their
	// files (like NOTICE.txt) need to be overwritten.
	if zips := android.PathsForModuleSrc(ctx, s.properties.Merge_zips); len(zips) > 0 {
		builder.Command().
			BuiltTool("zipsync").
			FlagWithArg("-d ", dir.String()).
			Inputs(zips)
	}

	// Get files from modules listed in `deps`
	packageSpecs := s.GatherPackagingSpecs(ctx)

	// Handle `deps_remap` renames
	err := remapPackageSpecs(packageSpecs, s.properties.Deps_remap)
	if err != nil {
		ctx.PropertyErrorf("deps_remap", "%s", err.Error())
	}

	s.CopySpecsToDir(ctx, builder, packageSpecs, dir)

	// NOTICES can only be generated if there is any deps defined.
	if len(packageSpecs) > 0 {
		noticeFile := android.PathForModuleOut(ctx, "NOTICES.txt")
		android.BuildNoticeTextOutputFromLicenseMetadata(
			ctx, noticeFile, "", "",
			android.BuildNoticeFromLicenseDataArgs{
				StripPrefix: []string{
					android.PathForModuleInstall(ctx, "sdk-repo").String() + "/",
					outputZipFile.String(),
				},
			}, ctx.ModuleProxy())
		builder.Command().Text("cp").
			Input(noticeFile).
			Text(filepath.Join(dir.String(), "NOTICE.txt"))
	}

	// Copy files from `srcs` into our tmpdir
	for _, src := range android.PathsForModuleSrc(ctx, s.properties.Srcs) {
		builder.Command().
			Text("cp").Input(src).Flag(dir.Join(ctx, src.Rel()).String())
	}

	// Add framework.aidl into our tmpdir.
	if proptools.BoolDefault(s.properties.Pack_framework_aidl, false) {
		frameworkAidlPath := android.PathForOutput(ctx, "framework.aidl")
		builder.Command().
			Text("cp").Input(frameworkAidlPath).Flag(dir.String())
	}

	// Handle `strip_files` by calling the necessary strip commands
	//
	// Note: this stripping logic was copied over from the old Make implementation
	// It's not using the same flags as the regular stripping support, nor does it
	// support the array of per-module stripping options. It would be nice if we
	// pulled the stripped versions from the CC modules, but that doesn't exist
	// for host tools today. (And not all the things we strip are CC modules today)
	if ctx.Darwin() {
		macStrip := config.MacStripPath(ctx)
		for _, strip := range s.properties.Strip_files {
			builder.Command().
				Text(macStrip).Flag("-x").
				Flag(dir.Join(ctx, strip).String())
		}
	} else {
		llvmObjCopy := config.ClangPath(ctx, "bin/llvm-objcopy")
		llvmStrip := config.ClangPath(ctx, "bin/llvm-strip")
		llvmLib := config.ClangToolsRuntimeLib(ctx)
		for _, strip := range s.properties.Strip_files {
			cmd := builder.Command().Tool(llvmStrip).ImplicitTool(llvmLib).ImplicitTool(llvmObjCopy)
			if !ctx.Windows() {
				cmd.Flag("-x")
			}
			cmd.Flag(dir.Join(ctx, strip).String())
		}
	}

	// Fix up the line endings of all text files. This also removes executable permissions.
	builder.Command().
		Text("find").
		Flag(dir.String()).
		Flag("-name '*.aidl' -o -name '*.css' -o -name '*.html' -o -name '*.java'").
		Flag("-o -name '*.js' -o -name '*.prop' -o -name '*.template'").
		Flag("-o -name '*.txt' -o -name '*.windows' -o -name '*.xml' -print0").
		// Using -n 500 for xargs to limit the max number of arguments per call to line_endings
		// to 500. This avoids line_endings failing with "arguments too long".
		Text("| xargs -0 -n 500 dos2unix")

	// Exclude some file types (roughly matching sdk.exclude.atree)
	builder.Command().
		Text("find").
		Flag(dir.String()).
		Flag("'('").
		Flag("-name '.*' -o -name '*~' -o -name 'Makefile' -o -name 'Android.mk' -o").
		Flag("-name '.*.swp' -o -name '.DS_Store' -o -name '*.pyc' -o -name 'OWNERS' -o").
		Flag("-name 'MODULE_LICENSE_*' -o -name '*.ezt' -o -name 'Android.bp'").
		Flag("')' -print0").
		Text("| xargs -0 -r rm -rf")
	builder.Command().
		Text("find").
		Flag(dir.String()).
		Flag("-name '_*' ! -name '__*' -print0").
		Text("| xargs -0 -r rm -rf")

	if ctx.Windows() {
		// Fix EOL chars to make window users happy
		builder.Command().
			Text("find").
			Flag(dir.String()).
			Flag("-maxdepth 2 -name '*.bat' -type f -print0").
			Text("| xargs -0 -r unix2dos")
	}

	// Zip up our temporary directory as the sdk-repo
	builder.Command().
		BuiltTool("soong_zip").
		FlagWithOutput("-o ", outputZipFile).
		FlagWithArg("-P ", s.properties.Base_dir.GetOrDefault(ctx, ".")).
		FlagWithArg("-C ", dir.String()).
		FlagWithArg("-D ", dir.String())
	builder.Command().Text("rm").Flag("-rf").Text(dir.String())

	builder.Build("build_sdk_repo", "Creating sdk-repo-"+s.BaseModuleName())

	ctx.SetOutputFiles(android.Paths{outputZipFile}, "")

	osName := ctx.Os().String()
	if osName == "linux_glibc" {
		osName = "linux"
	}

	// Keep the names of legacy SDK targets, but add an architecture suffix for new
	// cross-compiled arm64 tools.
	if arch := ctx.MultiTargets()[0].Arch.ArchType; arch != android.X86_64 && ctx.Os() != android.Darwin {
		osName += "-" + arch.Name
	}

	name := fmt.Sprintf("sdk-repo-%s-%s", osName, s.BaseModuleName())

	installPath := android.PathForModuleInstall(ctx, "sdk-repo")
	name = name + ".zip"
	installFile := ctx.InstallFile(installPath, name, outputZipFile)

	shortName := "sdk-repo-" + s.BaseModuleName()
	ctx.Phony("sdk_repo", installFile)
	ctx.Phony(shortName, installFile)
	ctx.DistForGoalWithFilenameTag("sdk_repo", outputZipFile, name)
	ctx.DistForGoalWithFilenameTag(shortName, outputZipFile, name)
}

func remapPackageSpecs(specs map[string]android.PackagingSpec, remaps []remapProperties) error {
	remapped := make(map[string]bool)
	for _, remap := range remaps {
		paths := android.SortedKeys(specs)
		for _, path := range paths {
			if _, alreadyRemapped := remapped[path]; alreadyRemapped {
				continue
			}
			if match, err := pathtools.Match(remap.From, path); err != nil {
				return fmt.Errorf("Error parsing %q: %v", remap.From, err)
			} else if match {
				newPath := remap.To
				if pathtools.IsGlob(remap.From) {
					rel, err := filepath.Rel(constantPartOfPattern(remap.From), path)
					if err != nil {
						return fmt.Errorf("Error handling %q", path)
					}
					newPath = filepath.Join(remap.To, rel)
				}
				spec := specs[path]
				delete(specs, path)
				spec.SetRelPathInPackage(newPath)
				specs[newPath] = spec
				remapped[newPath] = true
			}
		}
	}
	return nil
}

func constantPartOfPattern(pattern string) string {
	ret := ""
	for pattern != "" {
		var first string
		first, pattern = splitFirst(pattern)
		if pathtools.IsGlob(first) {
			return ret
		}
		ret = filepath.Join(ret, first)
	}
	return ret
}

func splitFirst(path string) (string, string) {
	i := strings.IndexRune(path, filepath.Separator)
	if i < 0 {
		return path, ""
	}
	return path[:i], path[i+1:]
}
