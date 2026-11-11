// Copyright 2015 Google Inc. All rights reserved.
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

// A genrule module takes a list of source files ("srcs" property), an optional
// list of tools ("tools" property), and a command line ("cmd" property), to
// generate output files ("out" property).

package genrule

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	RegisterGenruleBuildComponents(android.InitRegistrationContext)
}

// Test fixture preparer that will register most genrule build components.
//
// Singletons and mutators should only be added here if they are needed for a majority of genrule
// module types, otherwise they should be added under a separate preparer to allow them to be
// selected only when needed to reduce test execution time.
//
// Module types do not have much of an overhead unless they are used so this should include as many
// module types as possible. The exceptions are those module types that require mutators and/or
// singletons in order to function in which case they should be kept together in a separate
// preparer.
var PrepareForTestWithGenRuleBuildComponents = android.GroupFixturePreparers(
	android.FixtureRegisterWithContext(RegisterGenruleBuildComponents),
)

// Prepare a fixture to use all genrule module types, mutators and singletons fully.
//
// This should only be used by tests that want to run with as much of the build enabled as possible.
var PrepareForIntegrationTestWithGenrule = android.GroupFixturePreparers(
	PrepareForTestWithGenRuleBuildComponents,
)

func RegisterGenruleBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("genrule_defaults", defaultsFactory)

	ctx.RegisterModuleType("gensrcs", GenSrcsFactory)
	ctx.RegisterModuleType("genrule", GenRuleFactory)

	ctx.FinalDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("genrule_tool_deps", toolDepsMutator)
	})
}

var (
	pctx = android.NewPackageContext("android/soong/genrule")

	// Used by gensrcs when there is more than 1 shard to merge the outputs
	// of each shard into a zip file.
	gensrcsMerge = pctx.AndroidStaticRule("gensrcsMerge", blueprint.RuleParams{
		Command:         "${soongZip} -o ${tmpZip} @${tmpZip}.rsp && ${zipSync} -d ${genDir} ${tmpZip}",
		CommandDeps:     []string{"${soongZip}", "${zipSync}"},
		Rspfile:         "${tmpZip}.rsp",
		RspfileContent:  "${zipArgs}",
		SandboxDisabled: true,
	}, "tmpZip", "genDir", "zipArgs")
)

func init() {
	pctx.Import("android/soong/android")

	pctx.HostBinToolVariable("soongZip", "soong_zip")
	pctx.HostBinToolVariable("zipSync", "zipsync")
}

type SourceFileGenerator interface {
	android.SourceFileGenerator
}

type hostToolDependencyTag struct {
	blueprint.BaseDependencyTag
	android.LicenseAnnotationToolchainDependencyTag
	label string
}

func (t hostToolDependencyTag) AllowDisabledModuleDependency(target android.Module) bool {
	// Allow depending on a disabled module if it's replaced by a prebuilt
	// counterpart. We get the prebuilt through android.PrebuiltGetPreferred in
	// GenerateAndroidBuildActions.
	return target.IsReplacedByPrebuilt()
}

func (t hostToolDependencyTag) AllowDisabledModuleDependencyProxy(
	ctx android.OtherModuleProviderContext, target android.ModuleProxy) bool {
	return android.OtherModuleProviderOrDefault(
		ctx, target, android.PrebuiltInfoProvider).ReplacedByPrebuilt
}

var _ android.AllowDisabledModuleDependency = (*hostToolDependencyTag)(nil)

type generatorProperties struct {
	// The command to run on one or more input files. Cmd supports substitution of a few variables.
	//
	// Available variables for substitution:
	//
	//  $(location): the path to the first entry in tools or tool_files.
	//  $(location <label>): the path to the tool, tool_file, input or output with name <label>. Use $(location) if <label> refers to a rule that outputs exactly one file.
	//  $(locations <label>): the paths to the tools, tool_files, inputs or outputs with name <label>. Use $(locations) if <label> refers to a rule that outputs two or more files.
	//  $(in): one or more input files.
	//  $(out): a single output file.
	//  $(genDir): the sandbox directory for this tool; contains $(out).
	//  $$: a literal $
	Cmd proptools.Configurable[string] `android:"replace_instead_of_append"`

	// name of the modules (if any) that produces the host executable.   Leave empty for
	// prebuilts or scripts that do not need a module to build them.
	Tools []string

	// Local files that are used by the tool
	Tool_files []string `android:"path"`

	// List of directories to export generated headers from
	Export_include_dirs []string

	// list of input files without variations such as plain files, or output of another `genrule`.
	// If you want to depend on modules with variations, use `device_first_srcs`,
	// `device_common_srcs`, `common_os_srcs`, `host_first_srcs`, or `host_second_srcs` instead.
	Srcs proptools.Configurable[[]string] `android:"path,arch_variant"`

	// Same as srcs, but will add dependencies on modules via a device os variation and the device's
	// first supported arch's variation. Can be used to add a dependency from a host genrule to
	// a device module.
	Device_first_srcs proptools.Configurable[[]string] `android:"path_device_first"`

	// Same as srcs, but will add dependencies on modules via a device os variation and the common
	// arch variation. Can be used to add a dependency from a host genrule to a device module.
	Device_common_srcs proptools.Configurable[[]string] `android:"path_device_common"`

	// Same as srcs, but will add dependencies on modules via a common_os os variation.
	Common_os_srcs proptools.Configurable[[]string] `android:"path_common_os"`

	// Same as srcs, but will add dependencies on modules via for host os variation.
	Host_first_srcs proptools.Configurable[[]string] `android:"path_host_first"`

	// Same as srcs, but will add dependencies on modules via for host 2nd arch os variation.
	Host_second_srcs proptools.Configurable[[]string] `android:"path_host_second"`

	// input files to exclude
	Exclude_srcs []string `android:"path,arch_variant"`

	// Enable restat to update the output only if the output is changed
	Write_if_changed *bool

	// When set to true, an additional $(build_number_file) label will be available
	// to use in the cmd. This will be the location of a text file containing the
	// build number. The dependency on this file will be "order-only", meaning that
	// the genrule will not rerun when only this file changes, to avoid rerunning
	// the genrule every build, because the build number changes every build.
	// This also means that you should not attempt to consume the build number from
	// the result of this genrule in another build rule. If you do, the build number
	// in the second build rule will be stale when the second build rule rebuilds
	// but this genrule does not. Only certain allowlisted modules are allowed to
	// use this property, usages of the build number should be kept to the absolute
	// minimum. Particularly no modules on the system image may include the build
	// number. Prefer using libbuildversion via the use_version_lib property on
	// cc modules.
	Uses_order_only_build_number_file *bool

	// When set to true, an additional $(build_date_file) label will be available
	// to use in the cmd. This will be the location of a text file containing the
	// build date. The dependency on this file will be "order-only", meaning that
	// the genrule will not rerun when only this file changes, to avoid rerunning
	// the genrule every build, because the build date changes every build.
	// This also means that you should not attempt to consume the build date from
	// the result of this genrule in another build rule. If you do, the build date
	// in the second build rule will be stale when the second build rule rebuilds
	// but this genrule does not. Only certain allowlisted modules are allowed to
	// use this property, usages of the build date should be kept to the absolute
	// minimum.
	Uses_order_only_build_date_file *bool
}

type Module struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase

	// For other packages to make their own genrules with extra
	// properties
	Extra interface{}

	// CmdModifier can be set by wrappers around genrule to modify the command, for example to
	// prefix environment variables to it.
	CmdModifier func(ctx android.ModuleContext, cmd string) string

	android.ImageInterface

	properties generatorProperties

	// For the different tasks that genrule and gensrc generate. genrule will
	// generate 1 task, and gensrc will generate 1 or more tasks based on the
	// number of shards the input files are sharded into.
	taskGenerator taskFunc

	rule        blueprint.Rule
	rawCommands []string

	exportedIncludeDirs android.Paths

	outputFiles android.Paths
	outputDeps  android.Paths

	subName string
	subDir  string
}

type taskFunc func(ctx android.ModuleContext, rawCommand string, srcFiles android.Paths) []generateTask

type generateTask struct {
	in          android.Paths
	out         android.WritablePaths
	copyTo      android.WritablePaths // For gensrcs to set on gensrcsMerge rule.
	genDir      android.ModuleGenPath
	extraInputs map[string][]string

	cmd string
	// For gensrsc sharding.
	shard  int
	shards int

	// For nsjail tasks
	useNsjail  bool
	dirSrcs    android.DirectoryPaths
	keepGendir bool
}

func (g *Module) GeneratedSourceFiles() android.Paths {
	return g.outputFiles
}

func (g *Module) Srcs() android.Paths {
	return append(android.Paths{}, g.outputFiles...)
}

func (g *Module) GeneratedHeaderDirs() android.Paths {
	return g.exportedIncludeDirs
}

func (g *Module) GeneratedDeps() android.Paths {
	return g.outputDeps
}

var _ android.SourceFileProducer = (*Module)(nil)

func toolDepsMutator(ctx android.BottomUpMutatorContext) {
	if g, ok := ctx.Module().(*Module); ok {
		for _, tool := range g.properties.Tools {
			tag := hostToolDependencyTag{label: tool}
			if m := android.SrcIsModule(tool); m != "" {
				tool = m
			}
			ctx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(), tag, tool)
		}
	}
}

var buildNumberAllowlistKey = android.NewOnceKey("genruleBuildNumberAllowlistKey")
var buildDateAllowlistKey = android.NewOnceKey("genruleBuildDateAllowlistKey")

// This allowlist should be kept to the bare minimum, it's
// intended for things that existed before the build number
// was tightly controlled. Prefer using libbuildversion
// via the use_version_lib property of cc modules.
// This is a function instead of a global map so that
// soong plugins cannot add entries to the allowlist
func isModuleInBuildNumberAllowlist(ctx android.ModuleContext) bool {
	allowlist := ctx.Config().Once(buildNumberAllowlistKey, func() interface{} {
		// Define the allowlist as a list and then copy it into a map so that
		// gofmt doesn't change unnecessary lines trying to align the values of the map.
		allowlist := []string{
			// go/keep-sorted start
			"build/soong/tests:gen",
			"hardware/google/camera/common/hal/aidl_service:aidl_camera_build_version",
			"tools/tradefederation/core:tradefed_zip",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_test_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_test_vm_x86_64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_arm64_ext_boot.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_x86_64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_x86_64_ext_boot.bin",
			"trusty/vendor/google/aosp/scripts:trusty_memshare_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_security_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_security_vm_x86_64.elf",
			"trusty/vendor/google/aosp/scripts:trusty_tee_package",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_os_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_os_x86_64.elf",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_x86_64.elf",
			"trusty/vendor/google/proprietary/scripts:trusty_tee_package_goog",
			"trusty/vendor/google/proprietary/scripts:trusty_widevine_vm_arm64.bin",
			"trusty/vendor/google/proprietary/scripts:trusty_widevine_vm_x86_64.elf",
			"vendor/google/services/LyricCameraHAL/src/apex:com.google.pixel.camera.hal.manifest",
			"vendor/google_tradefederation/core:gen_google_tradefed_zip",
			// go/keep-sorted end
		}
		allowlistMap := make(map[string]bool, len(allowlist))
		for _, a := range allowlist {
			allowlistMap[a] = true
		}
		return allowlistMap
	}).(map[string]bool)

	_, ok := allowlist[ctx.ModuleDir()+":"+ctx.ModuleName()]
	return ok
}

func isModuleInBuildDateAllowlist(ctx android.ModuleContext) bool {
	allowlist := ctx.Config().Once(buildDateAllowlistKey, func() interface{} {
		// Define the allowlist as a list and then copy it into a map so that
		// gofmt doesn't change unnecessary lines trying to align the values of the map.
		allowlist := []string{
			// go/keep-sorted start
			"build/soong/tests:gen",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_test_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_test_vm_x86_64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_arm64_ext_boot.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_x86_64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_desktop_vm_x86_64_ext_boot.bin",
			"trusty/vendor/google/aosp/scripts:trusty_memshare_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_security_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_security_vm_x86_64.elf",
			"trusty/vendor/google/aosp/scripts:trusty_tee_package",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_os_arm64.bin",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_os_x86_64.elf",
			"trusty/vendor/google/aosp/scripts:trusty_test_vm_x86_64.elf",
			"trusty/vendor/google/proprietary/scripts:trusty_tee_package_goog",
			"trusty/vendor/google/proprietary/scripts:trusty_widevine_vm_arm64.bin",
			"trusty/vendor/google/proprietary/scripts:trusty_widevine_vm_x86_64.elf",
			// go/keep-sorted end
		}
		allowlistMap := make(map[string]bool, len(allowlist))
		for _, a := range allowlist {
			allowlistMap[a] = true
		}
		return allowlistMap
	}).(map[string]bool)

	_, ok := allowlist[ctx.ModuleDir()+":"+ctx.ModuleName()]
	return ok
}

// generateCommonBuildActions contains build action generation logic
// common to both the mixed build case and the legacy case of genrule processing.
// To fully support genrule in mixed builds, the contents of this function should
// approach zero; there should be no genrule action registration done directly
// by Soong logic in the mixed-build case.
func (g *Module) generateCommonBuildActions(ctx android.ModuleContext) {
	// Add the variant as a suffix to the make modules to create, so that the make modules
	// don't conflict because make doesn't know about variants. However, this causes issues with
	// tracking required dependencies as the required property in soong is passed straight to make
	// without accounting for these suffixes. To make it a little easier to work with, don't use
	// a suffix for android_common variants so that java_genrules look like regular 1-variant
	// genrules to make.
	if ctx.ModuleSubDir() != "android_common" {
		g.subName = ctx.ModuleSubDir()
	}

	if len(g.properties.Export_include_dirs) > 0 {
		for _, dir := range g.properties.Export_include_dirs {
			g.exportedIncludeDirs = append(g.exportedIncludeDirs,
				android.PathForModuleGen(ctx, g.subDir, ctx.ModuleDir(), dir))
			// Also export without ModuleDir for consistency with Export_include_dirs not being set
			g.exportedIncludeDirs = append(g.exportedIncludeDirs,
				android.PathForModuleGen(ctx, g.subDir, dir))
		}
	} else {
		g.exportedIncludeDirs = append(g.exportedIncludeDirs, android.PathForModuleGen(ctx, g.subDir))
	}

	locationLabels := map[string]location{}
	firstLabel := ""

	addLocationLabel := func(label string, loc location) {
		if firstLabel == "" {
			firstLabel = label
		}
		if _, exists := locationLabels[label]; !exists {
			locationLabels[label] = loc
		} else {
			ctx.ModuleErrorf("multiple locations for label %q: %q and %q (do you have duplicate srcs entries?)",
				label, locationLabels[label], loc)
		}
	}

	var tools android.Paths
	var packagedTools []android.PackagingSpec
	if len(g.properties.Tools) > 0 {
		seenTools := make(map[string]bool)
		ctx.VisitDirectDepsProxyAllowDisabled(func(module android.ModuleProxy) {
			switch tag := ctx.OtherModuleDependencyTag(module).(type) {
			case hostToolDependencyTag:
				tool := ctx.OtherModuleName(module)
				if h := android.GetHostToolInfo(ctx, module); h != nil {
					// A HostToolProvider provides the path to a tool, which will be copied
					// into the sandbox.
					path := h.HostToolPath
					if !path.Valid() {
						if ctx.Config().AllowMissingDependencies() {
							ctx.AddMissingDependencies([]string{tool})
						} else {
							ctx.ModuleErrorf("host tool %q missing output file", tool)
						}
						return
					}
					if specs := h.TransitivePackagingSpecs.ToList(); specs != nil {
						// If the HostToolProvider has PackgingSpecs, which are definitions of the
						// required relative locations of the tool and its dependencies, use those
						// instead.  They will be copied to those relative locations in the sbox
						// sandbox.
						// Care must be taken since TransitivePackagingSpec may return device-side
						// paths via the required property. Filter them out.
						for i, ps := range specs {
							if ps.Partition() != "" {
								if i == 0 {
									panic("first PackagingSpec is assumed to be the host-side tool")
								}
								continue
							}
							packagedTools = append(packagedTools, ps)
						}
						// Assume that the first PackagingSpec of the module is the tool.
						addLocationLabel(tag.label, packagedToolLocation{specs[0]})
					} else {
						tools = append(tools, path.Path())
						addLocationLabel(tag.label, toolLocation{android.Paths{path.Path()}})
					}
				} else {
					if ctx.Config().AllowMissingDependencies() {
						ctx.AddMissingDependencies([]string{tool})
					} else {
						ctx.ModuleErrorf("%q is not a host tool provider", tool)
					}
					return
				}

				seenTools[tag.label] = true
			}
		})

		// If AllowMissingDependencies is enabled, the build will not have stopped when
		// AddFarVariationDependencies was called on a missing tool, which will result in nonsensical
		// "cmd: unknown location label ..." errors later.  Add a placeholder file to the local label.
		// The command that uses this placeholder file will never be executed because the rule will be
		// replaced with an android.Error rule reporting the missing dependencies.
		if ctx.Config().AllowMissingDependencies() {
			for _, tool := range g.properties.Tools {
				if !seenTools[tool] {
					addLocationLabel(tool, errorLocation{"***missing tool " + tool + "***"})
				}
			}
		}
	}

	if ctx.Failed() {
		return
	}

	for _, toolFile := range g.properties.Tool_files {
		paths := android.PathsForModuleSrc(ctx, []string{toolFile})
		tools = append(tools, paths...)
		addLocationLabel(toolFile, toolLocation{paths})
	}

	addLabelsForInputs := func(propName string, include, exclude []string) android.Paths {
		includeDirInPaths := ctx.DeviceConfig().BuildBrokenInputDir(g.Name())
		var srcFiles android.Paths
		for _, in := range include {
			paths, missingDeps := android.PathsAndMissingDepsRelativeToModuleSourceDir(android.SourceInput{
				Context: ctx, Paths: []string{in}, ExcludePaths: exclude, IncludeDirs: includeDirInPaths,
			})
			if len(missingDeps) > 0 {
				if !ctx.Config().AllowMissingDependencies() {
					panic(fmt.Errorf("should never get here, the missing dependencies %q should have been reported in DepsMutator",
						missingDeps))
				}

				// If AllowMissingDependencies is enabled, the build will not have stopped when
				// the dependency was added on a missing SourceFileProducer module, which will result in nonsensical
				// "cmd: label ":..." has no files" errors later.  Add a placeholder file to the local label.
				// The command that uses this placeholder file will never be executed because the rule will be
				// replaced with an android.Error rule reporting the missing dependencies.
				ctx.AddMissingDependencies(missingDeps)
				addLocationLabel(in, errorLocation{"***missing " + propName + " " + in + "***"})
			} else {
				srcFiles = append(srcFiles, paths...)
				addLocationLabel(in, inputLocation{paths})
			}
		}
		return srcFiles
	}
	srcs := g.properties.Srcs.GetOrDefault(ctx, nil)
	srcFiles := addLabelsForInputs("srcs", srcs, g.properties.Exclude_srcs)
	srcFiles = append(srcFiles, addLabelsForInputs("device_first_srcs", g.properties.Device_first_srcs.GetOrDefault(ctx, nil), nil)...)
	srcFiles = append(srcFiles, addLabelsForInputs("device_common_srcs", g.properties.Device_common_srcs.GetOrDefault(ctx, nil), nil)...)
	srcFiles = append(srcFiles, addLabelsForInputs("common_os_srcs", g.properties.Common_os_srcs.GetOrDefault(ctx, nil), nil)...)
	srcFiles = append(srcFiles, addLabelsForInputs("host_first_src", g.properties.Host_first_srcs.GetOrDefault(ctx, nil), nil)...)
	if len(ctx.Config().Targets[ctx.Config().BuildOS]) > 1 {
		srcFiles = append(srcFiles, addLabelsForInputs("host_second_src", g.properties.Host_second_srcs.GetOrDefault(ctx, nil), nil)...)
	}

	var copyFrom android.Paths
	var outputFiles android.WritablePaths
	var zipArgs strings.Builder

	cmd := g.properties.Cmd.GetOrDefault(ctx, "")
	if g.CmdModifier != nil {
		cmd = g.CmdModifier(ctx, cmd)
	}

	var extraInputs android.Paths
	// Generate tasks, either from genrule or gensrcs.
	for i, task := range g.taskGenerator(ctx, cmd, srcFiles) {
		if len(task.out) == 0 {
			ctx.ModuleErrorf("must have at least one output file")
			return
		}

		// Only handle extra inputs once as these currently are the same across all tasks
		if i == 0 {
			for name, values := range task.extraInputs {
				extraInputs = append(extraInputs, addLabelsForInputs(name, values, []string{})...)
			}
		}

		// Pick a unique path outside the task.genDir for the sbox manifest textproto,
		// a unique rule name, and the user-visible description.
		var rule *android.RuleBuilder
		desc := "generate"
		name := "generator"
		if task.useNsjail {
			rule = android.NewRuleBuilder(pctx, ctx).SandboxDisabled().
				Nsjail(task.genDir, android.PathForModuleOut(ctx, "nsjail_build_sandbox")).
				DirDepsFile(task.genDir.Join(ctx, "dir_deps.d"))
			if task.keepGendir {
				rule.NsjailKeepGendir()
			}
		} else {
			manifestName := "genrule.sbox.textproto"
			if task.shards > 0 {
				manifestName = "genrule_" + strconv.Itoa(task.shard) + ".sbox.textproto"
				desc += " " + strconv.Itoa(task.shard)
				name += strconv.Itoa(task.shard)
			} else if len(task.out) == 1 {
				desc += " " + task.out[0].Base()
			}

			manifestPath := android.PathForModuleOut(ctx, manifestName)

			// Use a RuleBuilder to create a rule that runs the command inside an sbox sandbox.
			rule = android.NewRuleBuilder(pctx, ctx).SandboxDisabled().Sbox(task.genDir, manifestPath)
			rule.DirDepsFile(task.genDir.Join(ctx, "dir_deps.d"))
		}
		if Bool(g.properties.Write_if_changed) {
			rule.Restat()
		}
		cmd := rule.Command()

		for _, out := range task.out {
			addLocationLabel(out.Rel(), outputLocation{out})
		}

		rawCommand, err := android.Expand(task.cmd, func(name string) (string, error) {
			// report the error directly without returning an error to android.Expand to catch multiple errors in a
			// single run
			reportError := func(fmt string, args ...interface{}) (string, error) {
				ctx.PropertyErrorf("cmd", fmt, args...)
				return "SOONG_ERROR", nil
			}

			// Apply shell escape to each cases to prevent source file paths containing $ from being evaluated in shell
			switch name {
			case "location":
				if len(g.properties.Tools) == 0 && len(g.properties.Tool_files) == 0 {
					return reportError("at least one `tools` or `tool_files` is required if $(location) is used")
				}
				loc := locationLabels[firstLabel]
				paths := loc.Paths(cmd)
				if len(paths) == 0 {
					return reportError("default label %q has no files", firstLabel)
				} else if len(paths) > 1 {
					return reportError("default label %q has multiple files, use $(locations %s) to reference it",
						firstLabel, firstLabel)
				}
				return proptools.ShellEscape(paths[0]), nil
			case "in":
				return strings.Join(proptools.ShellEscapeList(cmd.PathsForInputs(srcFiles)), " "), nil
			case "out":
				var sandboxOuts []string
				for _, out := range task.out {
					sandboxOuts = append(sandboxOuts, cmd.PathForOutput(out))
				}
				return strings.Join(proptools.ShellEscapeList(sandboxOuts), " "), nil
			case "genDir":
				return proptools.ShellEscape(cmd.PathForOutput(task.genDir)), nil
			case "build_number_file":
				if !proptools.Bool(g.properties.Uses_order_only_build_number_file) {
					return reportError("to use the $(build_number_file) label, you must set uses_order_only_build_number_file: true")
				}
				return proptools.ShellEscape(cmd.PathForInput(ctx.Config().BuildNumberFile(ctx))), nil
			case "build_date_file":
				if !proptools.Bool(g.properties.Uses_order_only_build_date_file) {
					return reportError("to use the $(build_date_file) label, you must set uses_order_only_build_date_file: true")
				}
				return proptools.ShellEscape(cmd.PathForInput(ctx.Config().BuildDateFile(ctx))), nil
			default:
				if strings.HasPrefix(name, "location ") {
					label := strings.TrimSpace(strings.TrimPrefix(name, "location "))
					if loc, ok := locationLabels[label]; ok {
						paths := loc.Paths(cmd)
						if len(paths) == 0 {
							return reportError("label %q has no files", label)
						} else if len(paths) > 1 {
							return reportError("label %q has multiple files, use $(locations %s) to reference it",
								label, label)
						}
						return proptools.ShellEscape(paths[0]), nil
					} else {
						return reportError("unknown location label %q is not in srcs, out, tools or tool_files.", label)
					}
				} else if strings.HasPrefix(name, "locations ") {
					label := strings.TrimSpace(strings.TrimPrefix(name, "locations "))
					if loc, ok := locationLabels[label]; ok {
						paths := loc.Paths(cmd)
						if len(paths) == 0 {
							return reportError("label %q has no files", label)
						}
						return strings.Join(proptools.ShellEscapeList(paths), " "), nil
					} else {
						return reportError("unknown locations label %q is not in srcs, out, tools or tool_files.", label)
					}
				} else {
					return reportError("unknown variable '$(%s)'", name)
				}
			}
		})

		if err != nil {
			ctx.PropertyErrorf("cmd", "%s", err.Error())
			return
		}

		g.rawCommands = append(g.rawCommands, rawCommand)

		if ctx.Config().BuildArch == android.Arm64 && android.InList("bison", g.properties.Tools) {
			cmd.Text("BISON_PKGDATADIR=prebuilts/build-tools/common/bison")
			cmd.Implicits(ctx.GlobFilesOutsideModuleDir("prebuilts/build-tools/common/bison/**/*", nil))
		}

		cmd.Text(rawCommand)
		cmd.Implicits(srcFiles) // need to be able to reference other srcs
		cmd.Implicits(extraInputs)
		cmd.ImplicitOutputs(task.out)
		cmd.Implicits(task.in)
		cmd.ImplicitTools(tools)
		cmd.ImplicitPackagedTools(packagedTools)
		if proptools.Bool(g.properties.Uses_order_only_build_number_file) {
			if !isModuleInBuildNumberAllowlist(ctx) {
				ctx.ModuleErrorf("Only allowlisted modules may use uses_order_only_build_number_file: true")
			}
			cmd.OrderOnly(ctx.Config().BuildNumberFile(ctx))
		}
		if proptools.Bool(g.properties.Uses_order_only_build_date_file) {
			if !isModuleInBuildDateAllowlist(ctx) {
				ctx.ModuleErrorf("Only allowlisted modules may use uses_order_only_build_date_file: true")
			}
			cmd.OrderOnly(ctx.Config().BuildDateFile(ctx))
		}

		for _, input := range task.dirSrcs {
			cmd.ImplicitDirectory(input)
		}

		// Create the rule to run the genrule command inside sbox.
		rule.Build(name, desc)

		if len(task.copyTo) > 0 {
			// If copyTo is set, multiple shards need to be copied into a single directory.
			// task.out contains the per-shard paths, and copyTo contains the corresponding
			// final path.  The files need to be copied into the final directory by a
			// single rule so it can remove the directory before it starts to ensure no
			// old files remain.  zipsync already does this, so build up zipArgs that
			// zip all the per-shard directories into a single zip.
			outputFiles = append(outputFiles, task.copyTo...)
			copyFrom = append(copyFrom, task.out.Paths()...)
			zipArgs.WriteString(" -C " + task.genDir.String())
			zipArgs.WriteString(android.JoinWithPrefix(task.out.Strings(), " -f "))
		} else {
			outputFiles = append(outputFiles, task.out...)
		}
	}

	if len(copyFrom) > 0 {
		// Create a rule that zips all the per-shard directories into a single zip and then
		// uses zipsync to unzip it into the final directory.
		ctx.Build(pctx, android.BuildParams{
			Rule:        gensrcsMerge,
			Implicits:   copyFrom,
			Outputs:     outputFiles,
			Description: "merge shards",
			Args: map[string]string{
				"zipArgs": zipArgs.String(),
				"tmpZip":  android.PathForModuleGen(ctx, g.subDir+".zip").String(),
				"genDir":  android.PathForModuleGen(ctx, g.subDir).String(),
			},
		})
	}

	g.outputFiles = outputFiles.Paths()
}

func (g *Module) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	g.generateCommonBuildActions(ctx)

	// For <= 6 outputs, just embed those directly in the users. Right now, that covers >90% of
	// the genrules on AOSP. That will make things simpler to look at the graph in the common
	// case. For larger sets of outputs, inject a phony target in between to limit ninja file
	// growth.
	if len(g.outputFiles) <= 6 {
		g.outputDeps = g.outputFiles
	} else {
		phonyFile := android.PathForModuleGen(ctx, "genrule-phony")
		ctx.Build(pctx, android.BuildParams{
			Rule:   blueprint.Phony,
			Output: phonyFile,
			Inputs: g.outputFiles,
		})
		g.outputDeps = android.Paths{phonyFile}
	}

	g.setOutputFiles(ctx)

	if ctx.Os() == android.Windows {
		// Make doesn't support windows:
		// https://cs.android.com/android/platform/superproject/main/+/main:build/make/core/module_arch_supported.mk;l=66;drc=f264690860bb6ee7762784d6b7201aae057ba6f2
		g.HideFromMake()
	}
}

func (g *Module) setOutputFiles(ctx android.ModuleContext) {
	if len(g.outputFiles) == 0 {
		return
	}
	ctx.SetOutputFiles(g.outputFiles, "")
	// non-empty-string-tag should match one of the outputs
	for _, files := range g.outputFiles {
		ctx.SetOutputFiles(android.Paths{files}, files.Rel())
	}

	ctx.CheckbuildFile(g.outputFiles...)
}

// Collect information for opening IDE project files in java/jdeps.go.
func (g *Module) IDEInfo(ctx android.BaseModuleContext, dpInfo *android.IdeInfo) {
	dpInfo.Srcs = append(dpInfo.Srcs, g.Srcs().Strings()...)
	for _, src := range g.properties.Srcs.GetOrDefault(ctx, nil) {
		if strings.HasPrefix(src, ":") {
			src = strings.Trim(src, ":")
			dpInfo.Deps = append(dpInfo.Deps, src)
		}
	}
}

func (g *Module) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(g.outputFiles[0]),
		SubName:    g.subName,
		Extra: []android.AndroidMkExtraFunc{
			func(w io.Writer, outputFile android.Path) {
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE := true")
			},
		},
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			android.WriteAndroidMkData(w, data)
			if data.SubName != "" {
				fmt.Fprintln(w, ".PHONY:", name)
				fmt.Fprintln(w, name, ":", name+g.subName)
			}
		},
	}
}

var _ android.ApexModule = (*Module)(nil)

// Implements android.ApexModule
func (m *Module) MinSdkVersionSupported(ctx android.BaseModuleContext) android.ApiLevel {
	return android.MinApiLevel
}

func generatorFactory(taskGenerator taskFunc, props ...interface{}) *Module {
	module := &Module{
		taskGenerator: taskGenerator,
	}

	module.AddProperties(props...)
	module.AddProperties(&module.properties)

	module.ImageInterface = noopImageInterface{}

	return module
}

type noopImageInterface struct{}

func (x noopImageInterface) ImageMutatorBegin(android.ImageInterfaceContext)         {}
func (x noopImageInterface) ImageMutatorSupported() bool                             { return false }
func (x noopImageInterface) VendorVariantNeeded(android.ImageInterfaceContext) bool  { return false }
func (x noopImageInterface) ProductVariantNeeded(android.ImageInterfaceContext) bool { return false }
func (x noopImageInterface) CoreVariantNeeded(android.ImageInterfaceContext) bool    { return false }
func (x noopImageInterface) RamdiskVariantNeeded(android.ImageInterfaceContext) bool { return false }
func (x noopImageInterface) VendorRamdiskVariantNeeded(android.ImageInterfaceContext) bool {
	return false
}
func (x noopImageInterface) DebugRamdiskVariantNeeded(android.ImageInterfaceContext) bool {
	return false
}
func (x noopImageInterface) RecoveryVariantNeeded(android.ImageInterfaceContext) bool { return false }
func (x noopImageInterface) ExtraImageVariations(ctx android.ImageInterfaceContext) []string {
	return nil
}
func (x noopImageInterface) SetImageVariation(ctx android.ImageInterfaceContext, variation string) {
}

func NewGenSrcs() *Module {
	properties := &genSrcsProperties{}

	// finalSubDir is the name of the subdirectory that output files will be generated into.
	// It is used so that per-shard directories can be placed alongside it an then finally
	// merged into it.
	const finalSubDir = "gensrcs"

	taskGenerator := func(ctx android.ModuleContext, rawCommand string, srcFiles android.Paths) []generateTask {
		shardSize := defaultShardSize
		if s := properties.Shard_size; s != nil {
			shardSize = int(*s)
		}

		// gensrcs rules can easily hit command line limits by repeating the command for
		// every input file.  Shard the input files into groups.
		shards := android.ShardPaths(srcFiles, shardSize)
		var generateTasks []generateTask

		// Expands the output path pattern to form the output path for the given
		// input path.
		expandOutputPath := func(properties *genSrcsProperties, in android.Path) (string, error) {
			template := proptools.String(properties.Output)
			outputExtension := proptools.String(properties.Output_extension)
			if template == "" {
				if outputExtension != "" {
					template = "$(in:fullpath/base)." + outputExtension
				} else {
					template = "$(in)"
				}
			} else if outputExtension != "" {
				return "", fmt.Errorf("cannot specify both output and output_extension")
			}

			return android.Expand(template, func(name string) (string, error) {
				switch name {
				case "in":
					return in.String(), nil
				case "in:base":
					base := in.Base()
					return strings.TrimSuffix(base, filepath.Ext(base)), nil
				case "in:relpath/base":
					base := in.Rel()
					return strings.TrimSuffix(base, filepath.Ext(base)), nil
				case "in:fullpath/base":
					base := in.String()
					return strings.TrimSuffix(base, filepath.Ext(base)), nil
				default:
					return "", fmt.Errorf("unknown variable '$(%s)'", name)
				}
			})
		}

		for i, shard := range shards {
			var commands []string
			var outFiles android.WritablePaths
			var copyTo android.WritablePaths

			// When sharding is enabled (i.e. len(shards) > 1), the sbox rules for each
			// shard will be write to their own directories and then be merged together
			// into finalSubDir.  If sharding is not enabled (i.e. len(shards) == 1),
			// the sbox rule will write directly to finalSubDir.
			genSubDir := finalSubDir
			if len(shards) > 1 {
				genSubDir = strconv.Itoa(i)
			}

			genDir := android.PathForModuleGen(ctx, genSubDir)
			// TODO(ccross): this RuleBuilder is a hack to be able to call
			// rule.Command().PathForOutput.  Replace this with passing the rule into the
			// generator.
			rule := android.NewRuleBuilder(pctx, ctx).SandboxDisabled().Sbox(genDir, nil)

			for _, in := range shard {
				outFileRaw, err := expandOutputPath(properties, in)
				if err != nil {
					ctx.ModuleErrorf("%s", err)
					// exit early to avoid more unrelated errors later
					return nil
				}

				outFile := android.PathForModuleGen(ctx, finalSubDir, outFileRaw)

				// If sharding is enabled, then outFile is the path to the output file in
				// the shard directory, and copyTo is the path to the output file in the
				// final directory.
				if len(shards) > 1 {
					shardFile := android.PathForModuleGen(ctx, genSubDir, outFileRaw)
					copyTo = append(copyTo, outFile)
					outFile = shardFile
				}

				outFiles = append(outFiles, outFile)

				// pre-expand the command line to replace $in and $out with references to
				// a single input and output file.
				command, err := android.Expand(rawCommand, func(name string) (string, error) {
					switch name {
					case "in":
						return in.String(), nil
					case "out":
						return rule.Command().PathForOutput(outFile), nil
					default:
						return "$(" + name + ")", nil
					}
				})
				if err != nil {
					ctx.PropertyErrorf("cmd", err.Error())
				}

				// escape the command in case for example it contains '#', an odd number of '"', etc
				command = fmt.Sprintf("bash -c %v", proptools.ShellEscape(command))
				commands = append(commands, command)
			}
			fullCommand := strings.Join(commands, " && ")

			generateTasks = append(generateTasks, generateTask{
				in:     shard,
				out:    outFiles,
				copyTo: copyTo,
				genDir: genDir,
				cmd:    fullCommand,
				shard:  i,
				shards: len(shards),
				extraInputs: map[string][]string{
					"data": properties.Data,
				},
			})
		}

		return generateTasks
	}

	g := generatorFactory(taskGenerator, properties)
	g.subDir = finalSubDir
	return g
}

func GenSrcsFactory() android.Module {
	m := NewGenSrcs()
	android.InitAndroidModule(m)
	android.InitDefaultableModule(m)
	return m
}

type genSrcsProperties struct {
	// The output filename template.
	//
	// This template string allows the output file name to be generated for
	// each source file, using some limited properties of the source file.
	//
	//	$(in:base): The base filename, no path or extension
	//	$(in:relpath/base): The filename with a relative path but no extension
	//	$(in:fullpath/base): The filename with the full path but no extension
	//	$(in): The full origional input path. Equivalent to $(in:fullpath/base), but with the file
	//         extension as well.
	//
	// The "relative path" as mentioned above is the path of the file relative to the
	// Android.bp file that references it. In most cases this is preferable in order to not
	// expose the location of the code within the tree, which is probably an implementation detail.
	//
	// The template allows arbitrary prefixes and suffixes to be added to the
	// output filename. For example, "a_$(in:base).d" would take an source filename
	// of "b.c" and turn it into "a_b.d".
	//
	// It is an error if the same output file is generated by more than one source file.
	Output *string

	// extension that will be substituted for each output file
	Output_extension *string

	// maximum number of files that will be passed on a single command line.
	Shard_size *int64

	// Additional files needed for build that are not tooling related.
	Data []string `android:"path"`
}

const defaultShardSize = 50

func NewGenRule() *Module {
	properties := &genRuleProperties{}

	taskGenerator := func(ctx android.ModuleContext, rawCommand string, srcFiles android.Paths) []generateTask {
		useNsjail := Bool(properties.Use_nsjail)

		dirSrcs := android.DirectoryPathsForModuleSrc(ctx, properties.Dir_srcs)

		keepGendir := Bool(properties.Keep_gendir)
		if keepGendir && !useNsjail {
			ctx.PropertyErrorf("keep_gendir", "can't use keep_gendir if use_nsjail is false")
			return nil
		}

		outs := make(android.WritablePaths, len(properties.Out))
		for i, out := range properties.Out {
			outs[i] = android.PathForModuleGen(ctx, out)
		}
		return []generateTask{{
			in:         srcFiles,
			out:        outs,
			genDir:     android.PathForModuleGen(ctx),
			cmd:        rawCommand,
			useNsjail:  useNsjail,
			dirSrcs:    dirSrcs,
			keepGendir: keepGendir,
		}}
	}

	return generatorFactory(taskGenerator, properties)
}

func GenRuleFactory() android.Module {
	m := NewGenRule()
	android.InitAndroidModule(m)
	android.InitDefaultableModule(m)
	return m
}

type genRuleProperties struct {
	Use_nsjail *bool

	// List of input directories. Can be set only when use_nsjail is true. Currently, usage of
	// dir_srcs is limited only to Trusty build.
	Dir_srcs []string `android:"path"`

	// If set to true, $(genDir) is not truncated. Useful when this genrule can be incrementally
	// built. Can be set only when use_nsjail is true.
	Keep_gendir *bool

	// names of the output files that will be generated
	Out []string `android:"arch_variant"`
}

var Bool = proptools.Bool
var String = proptools.String

// Defaults
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func defaultsFactory() android.Module {
	return DefaultsFactory()
}

func DefaultsFactory(props ...interface{}) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&generatorProperties{},
		&genRuleProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}
