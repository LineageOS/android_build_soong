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

package cc

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc/config"
)

type TidyProperties struct {
	// whether to run clang-tidy over C-like sources.
	Tidy *bool

	// Extra flags to pass to clang-tidy
	Tidy_flags []string

	// Extra checks to enable or disable in clang-tidy
	Tidy_checks []string

	// Checks that should be treated as errors.
	Tidy_checks_as_errors []string
}

type tidyFeature struct {
	Properties TidyProperties
}

var quotedFlagRegexp, _ = regexp.Compile(`^-?-[^=]+=('|").*('|")$`)

// When passing flag -name=value, if user add quotes around 'value',
// the quotation marks will be preserved by NinjaAndShellEscapeList
// and the 'value' string with quotes won't work like the intended value.
// So here we report an error if -*='*' is found.
func checkNinjaAndShellEscapeList(ctx ModuleContext, prop string, slice []string) []string {
	for _, s := range slice {
		if quotedFlagRegexp.MatchString(s) {
			ctx.PropertyErrorf(prop, "Extra quotes in: %s", s)
		}
	}
	return proptools.NinjaAndShellEscapeList(slice)
}

func (tidy *tidyFeature) props() []interface{} {
	return []interface{}{&tidy.Properties}
}

// Set this const to true when all -warnings-as-errors in tidy_flags
// are replaced with tidy_checks_as_errors.
// Then, that old style usage will be obsolete and an error.
const NoWarningsAsErrorsInTidyFlags = true

func (tidy *tidyFeature) flags(ctx ModuleContext, flags Flags) Flags {
	CheckBadTidyFlags(ctx, "tidy_flags", tidy.Properties.Tidy_flags)
	CheckBadTidyChecks(ctx, "tidy_checks", tidy.Properties.Tidy_checks)

	// Check if tidy is explicitly disabled for this module
	if tidy.Properties.Tidy != nil && !*tidy.Properties.Tidy {
		return flags
	}
	// Some projects like external/* and vendor/* have clang-tidy disabled by default,
	// unless they are enabled explicitly with the "tidy:true" property or
	// when TIDY_EXTERNAL_VENDOR is set to true.
	if !proptools.Bool(tidy.Properties.Tidy) &&
		config.NoClangTidyForDir(
			ctx.Config().IsEnvTrue("TIDY_EXTERNAL_VENDOR"),
			ctx.ModuleDir()) {
		return flags
	}
	// If not explicitly disabled, set flags.Tidy to generate .tidy rules.
	// Note that libraries and binaries will depend on .tidy files ONLY if
	// the global WITH_TIDY or module 'tidy' property is true.
	flags.Tidy = true

	// If explicitly enabled, by global WITH_TIDY or local tidy:true property,
	// set flags.NeedTidyFiles to make this module depend on .tidy files.
	// Note that locally set tidy:true is ignored if ALLOW_LOCAL_TIDY_TRUE is not set to true.
	if ctx.Config().IsEnvTrue("WITH_TIDY") || (ctx.Config().IsEnvTrue("ALLOW_LOCAL_TIDY_TRUE") && Bool(tidy.Properties.Tidy)) {
		flags.NeedTidyFiles = true
	}

	// Add global WITH_TIDY_FLAGS and local tidy_flags.
	withTidyFlags := ctx.Config().Getenv("WITH_TIDY_FLAGS")
	if len(withTidyFlags) > 0 {
		flags.TidyFlags = append(flags.TidyFlags, withTidyFlags)
	}
	esc := checkNinjaAndShellEscapeList
	flags.TidyFlags = append(flags.TidyFlags, esc(ctx, "tidy_flags", tidy.Properties.Tidy_flags)...)
	// If TidyFlags does not contain -header-filter, add default header filter.
	// Find the substring because the flag could also appear as --header-filter=...
	// and with or without single or double quotes.
	if !android.SubstringInList(flags.TidyFlags, "-header-filter=") {
		defaultDirs := ctx.Config().Getenv("DEFAULT_TIDY_HEADER_DIRS")
		headerFilter := "-header-filter="
		// Default header filter should include only the module directory,
		// not the out/soong/.../ModuleDir/...
		// Otherwise, there will be too many warnings from generated files in out/...
		// If a module wants to see warnings in the generated source files,
		// it should specify its own -header-filter flag.
		if defaultDirs == "" {
			headerFilter += "^" + ctx.ModuleDir() + "/"
		} else {
			headerFilter += "\"(^" + ctx.ModuleDir() + "/|" + defaultDirs + ")\""
		}
		flags.TidyFlags = append(flags.TidyFlags, headerFilter)
	}
	// Work around RBE bug in parsing clang-tidy flags, replace "--flag" with "-flag".
	// Some C/C++ modules added local tidy flags like --header-filter= and --extra-arg-before=.
	doubleDash := regexp.MustCompile("^('?)--(.*)$")
	for i, s := range flags.TidyFlags {
		flags.TidyFlags[i] = doubleDash.ReplaceAllString(s, "$1-$2")
	}

	// If clang-tidy is not enabled globally, add the -quiet flag.
	if !ctx.Config().ClangTidy() {
		flags.TidyFlags = append(flags.TidyFlags, "-quiet")
		flags.TidyFlags = append(flags.TidyFlags, "-extra-arg-before=-fno-caret-diagnostics")
	}

	for _, f := range config.TidyExtraArgFlags() {
		flags.TidyFlags = append(flags.TidyFlags, "-extra-arg-before="+f)
	}

	tidyChecks := "-checks="
	if checks := ctx.Config().TidyChecks(); len(checks) > 0 {
		tidyChecks += checks
	} else {
		tidyChecks += config.TidyChecksForDir(ctx.ModuleDir())
	}
	if len(tidy.Properties.Tidy_checks) > 0 {
		// If Tidy_checks contains "-*", ignore all checks before "-*".
		localChecks := tidy.Properties.Tidy_checks
		ignoreGlobalChecks := false
		for n, check := range tidy.Properties.Tidy_checks {
			if check == "-*" {
				ignoreGlobalChecks = true
				localChecks = tidy.Properties.Tidy_checks[n:]
			}
		}
		if ignoreGlobalChecks {
			tidyChecks = "-checks=" + strings.Join(esc(ctx, "tidy_checks",
				config.ClangRewriteTidyChecks(localChecks)), ",")
		} else {
			tidyChecks = tidyChecks + "," + strings.Join(esc(ctx, "tidy_checks",
				config.ClangRewriteTidyChecks(localChecks)), ",")
		}
	}
	tidyChecks = tidyChecks + config.TidyGlobalNoChecks()
	if ctx.Windows() {
		// https://b.corp.google.com/issues/120614316
		// mingw32 has cert-dcl16-c warning in NO_ERROR,
		// which is used in many Android files.
		tidyChecks += ",-cert-dcl16-c"
	}

	flags.TidyFlags = append(flags.TidyFlags, tidyChecks)

	// Embedding -warnings-as-errors in tidy_flags is error-prone.
	// It should be replaced with the tidy_checks_as_errors list.
	for i, s := range flags.TidyFlags {
		if strings.Contains(s, "-warnings-as-errors=") {
			if NoWarningsAsErrorsInTidyFlags {
				ctx.PropertyErrorf("tidy_flags", "should not contain "+s+"; use tidy_checks_as_errors instead.")
			} else {
				fmt.Printf("%s: warning: module %s's tidy_flags should not contain %s, which is replaced with -warnings-as-errors=-*; use tidy_checks_as_errors for your own as-error warnings instead.\n",
					ctx.BlueprintsFile(), ctx.ModuleName(), s)
				flags.TidyFlags[i] = "-warnings-as-errors=-*"
			}
			break // there is at most one -warnings-as-errors
		}
	}
	// Default clang-tidy flags does not contain -warning-as-errors.
	// If a module has tidy_checks_as_errors, add the list to -warnings-as-errors
	// and then append the TidyGlobalNoErrorChecks.
	if len(tidy.Properties.Tidy_checks_as_errors) > 0 {
		tidyChecksAsErrors := "-warnings-as-errors=" +
			strings.Join(esc(ctx, "tidy_checks_as_errors", tidy.Properties.Tidy_checks_as_errors), ",") +
			config.TidyGlobalNoErrorChecks()
		flags.TidyFlags = append(flags.TidyFlags, tidyChecksAsErrors)
	}
	return flags
}

func init() {
	android.RegisterParallelSingletonType("tidy_phony_targets", TidyPhonySingleton)
}

// This TidyPhonySingleton generates both tidy-* and obj-* phony targets for C/C++ files.
func TidyPhonySingleton() android.Singleton {
	return &tidyPhonySingleton{}
}

type tidyPhonySingleton struct{}

// Given a final module, add its tidy/obj phony targets to tidy/objModulesInDirGroup.
func collectTidyObjModuleTargets(ctx android.SingletonContext, module android.Module,
	tidyModulesInDirGroup, objModulesInDirGroup map[string]map[string]android.Paths) {
	allObjFileGroups := make(map[string]android.Paths)     // variant group name => obj file Paths
	allTidyFileGroups := make(map[string]android.Paths)    // variant group name => tidy file Paths
	subsetObjFileGroups := make(map[string]android.Paths)  // subset group name => obj file Paths
	subsetTidyFileGroups := make(map[string]android.Paths) // subset group name => tidy file Paths

	// (1) Collect all obj/tidy files into OS-specific groups.
	ctx.VisitAllModuleVariants(module, func(variant android.Module) {
		if ctx.Config().KatiEnabled() && android.ShouldSkipAndroidMkProcessing(variant) {
			return
		}
		if m, ok := variant.(*Module); ok {
			osName := variant.Target().Os.Name
			addToOSGroup(osName, m.objFiles, allObjFileGroups, subsetObjFileGroups)
			addToOSGroup(osName, m.tidyFiles, allTidyFileGroups, subsetTidyFileGroups)
		}
	})

	// (2) Add an all-OS group, with "" or "subset" name, to include all os-specific phony targets.
	addAllOSGroup(ctx, module, allObjFileGroups, "", "obj")
	addAllOSGroup(ctx, module, allTidyFileGroups, "", "tidy")
	addAllOSGroup(ctx, module, subsetObjFileGroups, "subset", "obj")
	addAllOSGroup(ctx, module, subsetTidyFileGroups, "subset", "tidy")

	tidyTargetGroups := make(map[string]android.Path)
	objTargetGroups := make(map[string]android.Path)
	genObjTidyPhonyTargets(ctx, module, "obj", allObjFileGroups, objTargetGroups)
	genObjTidyPhonyTargets(ctx, module, "obj", subsetObjFileGroups, objTargetGroups)
	genObjTidyPhonyTargets(ctx, module, "tidy", allTidyFileGroups, tidyTargetGroups)
	genObjTidyPhonyTargets(ctx, module, "tidy", subsetTidyFileGroups, tidyTargetGroups)

	moduleDir := ctx.ModuleDir(module)
	appendToModulesInDirGroup(tidyTargetGroups, moduleDir, tidyModulesInDirGroup)
	appendToModulesInDirGroup(objTargetGroups, moduleDir, objModulesInDirGroup)
}

func (m *tidyPhonySingleton) GenerateBuildActions(ctx android.SingletonContext) {
	// For tidy-* directory phony targets, there are different variant groups.
	// tidyModulesInDirGroup[G][D] is for group G, directory D, with Paths
	// of all phony targets to be included into direct dependents of tidy-D_G.
	tidyModulesInDirGroup := make(map[string]map[string]android.Paths)
	// Also for obj-* directory phony targets.
	objModulesInDirGroup := make(map[string]map[string]android.Paths)

	// Collect tidy/obj targets from the 'final' modules.
	ctx.VisitAllModules(func(module android.Module) {
		if module == ctx.FinalModule(module) {
			collectTidyObjModuleTargets(ctx, module, tidyModulesInDirGroup, objModulesInDirGroup)
		}
	})

	suffix := ""
	if ctx.Config().KatiEnabled() {
		suffix = "-soong"
	}
	generateObjTidyPhonyTargets(ctx, suffix, "obj", objModulesInDirGroup)
	generateObjTidyPhonyTargets(ctx, suffix, "tidy", tidyModulesInDirGroup)
}

// The name for an obj/tidy module variant group phony target is Name_group-obj/tidy,
func objTidyModuleGroupName(module android.Module, group string, suffix string) string {
	if group == "" {
		return module.Name() + "-" + suffix
	}
	return module.Name() + "_" + group + "-" + suffix
}

// Generate obj-* or tidy-* phony targets.
func generateObjTidyPhonyTargets(ctx android.SingletonContext, suffix string, prefix string, objTidyModulesInDirGroup map[string]map[string]android.Paths) {
	// For each variant group, create a <prefix>-<directory>_group target that
	// depends on all subdirectories and modules in the directory.
	for group, modulesInDir := range objTidyModulesInDirGroup {
		groupSuffix := ""
		if group != "" {
			groupSuffix = "_" + group
		}
		mmTarget := func(dir string) string {
			return prefix + "-" + strings.Replace(filepath.Clean(dir), "/", "-", -1) + groupSuffix
		}
		dirs, topDirs := android.AddAncestors(ctx, modulesInDir, mmTarget)
		// Create a <prefix>-soong_group target that depends on all <prefix>-dir_group of top level dirs.
		var topDirPaths android.Paths
		for _, dir := range topDirs {
			topDirPaths = append(topDirPaths, android.PathForPhony(ctx, mmTarget(dir)))
		}
		ctx.Phony(prefix+suffix+groupSuffix, topDirPaths...)
		// Create a <prefix>-dir_group target that depends on all targets in modulesInDir[dir]
		for _, dir := range dirs {
			if dir != "." && dir != "" {
				ctx.Phony(mmTarget(dir), modulesInDir[dir]...)
			}
		}
	}
}

// Append (obj|tidy)TargetGroups[group] into (obj|tidy)ModulesInDirGroups[group][moduleDir].
func appendToModulesInDirGroup(targetGroups map[string]android.Path, moduleDir string, modulesInDirGroup map[string]map[string]android.Paths) {
	for group, phonyPath := range targetGroups {
		if _, found := modulesInDirGroup[group]; !found {
			modulesInDirGroup[group] = make(map[string]android.Paths)
		}
		modulesInDirGroup[group][moduleDir] = append(modulesInDirGroup[group][moduleDir], phonyPath)
	}
}

// Add given files to the OS group and subset group.
func addToOSGroup(osName string, files android.Paths, allGroups, subsetGroups map[string]android.Paths) {
	if len(files) > 0 {
		subsetName := osName + "_subset"
		allGroups[osName] = append(allGroups[osName], files...)
		// Now include only the first variant in the subsetGroups.
		// If clang and clang-tidy get faster, we might include more variants.
		if _, found := subsetGroups[subsetName]; !found {
			subsetGroups[subsetName] = files
		}
	}
}

// Add an all-OS group, with groupName, to include all os-specific phony targets.
func addAllOSGroup(ctx android.SingletonContext, module android.Module, phonyTargetGroups map[string]android.Paths, groupName string, objTidyName string) {
	if len(phonyTargetGroups) > 0 {
		var targets android.Paths
		for group, _ := range phonyTargetGroups {
			targets = append(targets, android.PathForPhony(ctx, objTidyModuleGroupName(module, group, objTidyName)))
		}
		phonyTargetGroups[groupName] = targets
	}
}

// Create one phony targets for each group and add them to the targetGroups.
func genObjTidyPhonyTargets(ctx android.SingletonContext, module android.Module, objTidyName string, fileGroups map[string]android.Paths, targetGroups map[string]android.Path) {
	for group, files := range fileGroups {
		groupName := objTidyModuleGroupName(module, group, objTidyName)
		ctx.Phony(groupName, files...)
		targetGroups[group] = android.PathForPhony(ctx, groupName)
	}
}
