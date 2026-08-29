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

package java

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/remoteexec"
)

//go:generate go run ../../blueprint/gobtools/codegen

func init() {
	pctx.HostBinToolVariable("symbols_map", "symbols_map")
}

type DexProperties struct {
	// If set to true, compile dex regardless of installable.  Defaults to false.
	Compile_dex *bool

	// list of module-specific flags that will be used for dex compiles
	Dxflags proptools.Configurable[[]string] `android:"arch_variant"`

	// A list of files containing rules that specify the classes to keep in the main dex file.
	Main_dex_rules []string `android:"path"`

	Optimize struct {
		// If false, disable all optimization and use D8 to compile dex, otherwise
		// use R8. Defaults to true for android_app and android_test_helper_app
		// modules, false for android_test, java_library, and java_test modules.
		Enabled proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// True if the module containing this has it set by default.
		EnabledByDefault bool `blueprint:"mutated"`

		// If true, then `d8` will be used on eng builds instead of `r8`, even though
		// optimize.enabled is true.
		D8_on_eng *bool

		// If true, disable the use of R8 and hence all optimizations if the build
		// is using source stubs. Overrides the `enabled` property.
		//
		// Using R8 implicitly adds dependencies on the SDK in the boot classpath
		// (more precisely `LegacyCorePlatformBootclasspathLibraries` and/or
		// `FrameworkLibraries` in build/soong/java/config/config.go). If source
		// stubs are enabled (through e.g. PRODUCT_BUILD_FROM_SOURCE_STUB) then
		// implementation libraries in the SDK would get cyclic dependencies on each
		// other with R8. This flag needs to be true for those libraries to resort
		// to D8 in those builds.
		//
		// The R8 dependencies are added statically before `select()` expressions
		// are evaluated. Hence this flag is applicable even if `enabled` is false,
		// and it cannot itself support `select()`.
		Force_disabled_with_source_stubs *bool

		// Whether to allow that library classes inherit from program classes.
		// Defaults to false.
		Ignore_library_extends_program *bool

		// Whether to continue building even if warnings are emitted.  Defaults to true.
		Ignore_warnings *bool

		// Whether runtime invisible annotations should be kept by R8. Defaults to false.
		// This is equivalent to:
		//   -keepattributes RuntimeInvisibleAnnotations,
		//                   RuntimeInvisibleParameterAnnotations,
		//                   RuntimeInvisibleTypeAnnotations
		// This is only applicable when RELEASE_R8_ONLY_RUNTIME_VISIBLE_ANNOTATIONS is
		// enabled and will be used to migrate away from keeping runtime invisible
		// annotations (b/387958004).
		Keep_runtime_invisible_annotations *bool

		// If true, runs R8 in Proguard compatibility mode, otherwise runs R8 in full mode.
		// Defaults to false.
		//
		// Deprecated: This will soon be removed and disabled universally. See b/215530220.
		Proguard_compatibility proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// If true, R8 will not add public or protected members (fields or methods) to
		// the API surface of the compilation unit, i.e., classes that are kept or
		// have kept subclasses will not expose any members added by R8 for internal
		// use. That includes renamed members if obfuscation is enabled.
		// This should only be used for building targets that go on the bootclasspath.
		// Defaults to false.
		Protect_api_surface *bool

		// If true, optimize for size by removing unused code.  Defaults to true for apps,
		// false for libraries and tests.
		Shrink proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// True if the module containing this has it set by default.
		ShrinkByDefault bool `blueprint:"mutated"`

		// If true, optimize bytecode.  Defaults to true for apps when
		// `RELEASE_R8_OPTIMIZE_BY_DEFAULT` is set, otherwise false.
		Optimize proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// True if the module containing this has it set by default.
		OptimizeByDefault bool `blueprint:"mutated"`

		// If true, obfuscate bytecode by renaming packages, classes, and members.
		// Defaults to false.
		Obfuscate proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// If true, do not use the flag files generated by aapt that automatically keep
		// classes referenced by the app manifest.  Defaults to false.
		No_aapt_flags *bool

		// If true, optimize for size by removing unused resources. Ignored for the
		// "eng" build variant. Superseded by the optimized_shrink_resources setting
		// if that one is set explicitly to either true or false. Defaults to false.
		Shrink_resources proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// If true, use optimized resource shrinking in R8. Always false if
		// shrink_resources is false. Defaults to true if
		// RELEASE_USE_OPTIMIZED_RESOURCE_SHRINKING_BY_DEFAULT is true, false
		// otherwise.
		//
		// Optimized shrinking means that R8 will trace and treeshake resources together with code
		// and apply additional optimizations. This implies non final fields in the R classes.
		Optimized_shrink_resources proptools.Configurable[bool] `android:"replace_instead_of_append"`

		// Flags to pass to proguard.
		Proguard_flags proptools.Configurable[[]string]

		// Specifies the locations of files containing proguard flags.
		Proguard_flags_files proptools.Configurable[[]string] `android:"path"`

		// If a proguard file from proguard_flags_files uses -include to include another file,
		// that included file must be listed here so that the build system knows about the
		// dependency.
		Included_proguard_flags_files proptools.Configurable[[]string] `android:"path"`

		// If true, transitive reverse dependencies of this module will have this
		// module's proguard spec appended to their optimization action
		Export_proguard_flags_files *bool

		// Path to a file containing a list of class names that should not be compiled using R8.
		// These classes will be compiled by D8 similar to when Optimize.Enabled is false.
		//
		// Example:
		//
		//   r8.exclude:
		//   com.example.Foo
		//   com.example.Bar
		//   com.example.Bar$Baz
		//
		// By default all classes are compiled using R8 when Optimize.Enabled is set.
		Exclude *string `android:"path"`

		// Path to a file containing a list of class names that should be compiled using R8.
		// A class is only compiled through R8 if and only if it's part of the includes and not
		// part of the excludes. Otherwise D8 will be used.
		//
		// If this attribute is not specified then '**' is used instead, which includes every single
		// class that is not excluded.
		//
		// The file is expected to contain fully-qualified classes name and package names ending
		// in `.*` or `.**`, separated by newline.
		Include *string `android:"path"`

		// Optional list of downstream (Java) libraries from which to trace and preserve references
		// when optimizing. Note that this requires that the source reference does *not* have
		// a strict lib dependency on this target; dependencies should be on intermediate targets
		// statically linked into this target, e.g., if A references B, and we want to trace and
		// keep references from A when optimizing B, you would create an intermediate B.impl (
		// containing all static code), have A depend on `B.impl` via libs, and set
		// `trace_references_from: ["A"]` on B.
		//
		// Also note that these are *not* inherited across targets, they must be specified at the
		// top-level target that is optimized.
		//
		// TODO(b/212737576): Handle this implicitly using bottom-up deps mutation and implicit
		// creation of a proxy `.impl` library.
		Trace_references_from proptools.Configurable[[]string] `android:"arch_variant"`

		// Add libraries using `--classpath` instead of `--libs`, to allow overrides. This
		// should normally not be used, but can be necessary when running R8 on the boot
		// classpath itself.
		Import_libraries_as_classpath *bool
	}

	// Keep the data uncompressed. We always need uncompressed dex for execution,
	// so this might actually save space by avoiding storing the same data twice.
	// This defaults to reasonable value based on module and should not be set.
	// It exists only to support ART tests.
	Uncompress_dex *bool

	// Exclude kotlinc generate files: *.kotlin_module, *.kotlin_builtins. Defaults to false.
	Exclude_kotlinc_generated_files *bool

	// Disable dex container (also known as "multi-dex").
	// This may be necessary as a temporary workaround to mask toolchain bugs (see b/341652226).
	No_dex_container *bool
}

type dexer struct {
	dexProperties DexProperties

	// list of extra proguard flag files
	extraProguardFlagsFiles android.Paths
	// list of files included (with a -include directive) from the extraProguardFlagsFiles
	extraIncludedProguardFlagsFiles android.Paths
	proguardDictionary              android.OptionalPath
	proguardConfiguration           android.OptionalPath
	proguardUsageZip                android.OptionalPath
	resourcesInput                  android.OptionalPath
	resourcesOutput                 android.OptionalPath

	providesTransitiveHeaderJarsForR8

	// Force disables R8 if true, overriding all flags in dexProperties. Set in
	// the deps stage (before any R8 dependencies may be added), because it needs
	// to take the is_stubs_module property into account.
	optimizeForceDisabled bool
}

func (d *dexer) setOptimizeForceDisabled(isStub bool) {
	if isStub {
		// Always force disable R8 on stubs to avoid cyclic dependencies between
		// them, since we add the SDK libraries to all R8 invocations. It's not
		// useful on stubs anyway.
		d.optimizeForceDisabled = true
	}
}

func (d *dexer) isOptimizeForceDisabled(ctx android.EarlyModuleContext) bool {
	if d.optimizeForceDisabled {
		return true
	}
	return proptools.Bool(d.dexProperties.Optimize.Force_disabled_with_source_stubs) && !ctx.Config().BuildFromTextStub()
}

func (d *dexer) effectiveOptimizeEnabled(ctx android.ModuleContext) bool {
	// For eng builds, if Optimize.D8_on_eng is true, then disable optimization.
	if ctx.Config().Eng() && proptools.Bool(d.dexProperties.Optimize.D8_on_eng) {
		return false
	}
	if d.isOptimizeForceDisabled(ctx) {
		return false
	}
	// Otherwise, use the legacy logic of a default value which can be explicitly overridden by the module.
	return d.dexProperties.Optimize.Enabled.GetOrDefault(ctx, d.dexProperties.Optimize.EnabledByDefault)
}

func (d *dexer) resourceShrinkingEnabled(ctx android.ModuleContext) bool {
	if ctx.Config().Eng() || !d.effectiveOptimizeEnabled(ctx) {
		return false
	}
	shrinkResources := d.dexProperties.Optimize.Shrink_resources.GetOrDefault(ctx, false)
	return d.dexProperties.Optimize.Optimized_shrink_resources.GetOrDefault(ctx, shrinkResources)
}

func (d *dexer) optimizedResourceShrinkingEnabled(ctx android.ModuleContext) bool {
	if !d.resourceShrinkingEnabled(ctx) {
		return false
	}
	shrinkByDefault := ctx.Config().UseOptimizedResourceShrinkingByDefault()
	return d.dexProperties.Optimize.Optimized_shrink_resources.GetOrDefault(ctx, shrinkByDefault)
}

func (d *dexer) optimizeOrObfuscateEnabled(ctx android.ModuleContext) bool {
	if !d.effectiveOptimizeEnabled(ctx) {
		return false
	}
	return d.dexProperties.Optimize.Optimize.GetOrDefault(ctx, d.dexProperties.Optimize.OptimizeByDefault) || d.dexProperties.Optimize.Obfuscate.GetOrDefault(ctx, false)
}

func (d *dexer) shrinkEnabled(ctx android.ModuleContext) bool {
	if !d.effectiveOptimizeEnabled(ctx) {
		return false
	}
	return d.dexProperties.Optimize.Shrink.GetOrDefault(ctx, d.dexProperties.Optimize.ShrinkByDefault)
}

type proguardFlagsFiles struct {
	files    android.Paths
	included android.Paths
}

func (d *dexer) ProguardFlagsFiles(ctx android.ModuleContext) proguardFlagsFiles {
	return proguardFlagsFiles{
		files:    android.PathsForModuleSrc(ctx, d.dexProperties.Optimize.Proguard_flags_files.GetOrDefault(ctx, nil)),
		included: android.PathsForModuleSrc(ctx, d.dexProperties.Optimize.Included_proguard_flags_files.GetOrDefault(ctx, nil)),
	}
}

// Removes all outputs of d8Inc rule
var d8IncClean = pctx.AndroidStaticRule("d8Inc-partialcompileclean",
	blueprint.RuleParams{
		Command2: blueprint.NewCommand(
			android.Rm, ` -rf "${outDir}" "${builtOut}" "${d8Deps}" "${outDepfile}"`),
	}, "outDir", "d8Flags", "d8Deps", "zipFlags", "mergeZipsFlags", "builtOut", "outDepfile",
)

var d8Inc, d8IncRE = pctx.MultiCommandRemoteStaticRules("d8Inc",
	blueprint.RuleParams{
		Command: `mkdir -p "$outDir" "$outDir/packages" && ` +
			`. ${config.UsePartialCompileFile} && ` +
			`${config.IncrementalDexInputCmd} ` +
			`--classesJar $in --dexTarget $out --deps $d8Deps --outputDir $outDir --packageOutputDir $outDir/packages && ` +
			`PATH=${config.JavaToolchain}:$$PATH ` +
			`$d8Template${config.D8Cmd} ${config.D8Flags} $d8Flags --output $outDir --no-dex-input-jar $in --packages $out.rsp --mod-packages $out.inc.rsp --package-output $outDir/packages && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $mergeZipsFlags $out $outDir/classes.dex.jar $in && ` +
			`if [ -f "$out.input.pc_state.new" ]; then mv "$out.input.pc_state.new" "$out.input.pc_state" && ` +
			`rm -rf $out.input.pc_state.new; fi && ` +
			`if [ -f "$out.deps.pc_state.new" ]; then mv "$out.deps.pc_state.new" "$out.deps.pc_state" && ` +
			`rm -rf $out.deps.pc_state.new; fi && ` +
			`rm -f "$outDir"/classes*.dex "$outDir/classes.dex.jar"`,
		CommandDeps: []string{
			"${config.IncrementalDexInputCmd}",
			"${config.D8Cmd}",
			"${config.D8Jar}",
			"${config.JavaCmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
			"${config.UsePartialCompileFile}",
		},
		SandboxDisabled: true,
	}, map[string]*remoteexec.REParams{
		"$d8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "d8"},
			Inputs:          []string{"${config.D8Jar}"},
			ExecStrategy:    "${config.RED8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RED8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "d8Flags", "zipFlags", "mergeZipsFlags", "d8Deps"}, nil)

// Include all of the args for d8IncR8, so that we can generate the partialcompileclean target's build using the same list.
var d8IncR8Clean = pctx.AndroidStaticRule("d8Incr8-partialcompileclean",
	blueprint.RuleParams{
		Command2: blueprint.NewCommand(
			android.Rm, ` -rf "${outDir}" "${outDict}" "${outConfig}" "${outUsage}" `,
			`"${outUsageZip}" "${outUsageDir}" "${outDepfile}" "${resourcesOutput}" `,
			`"${outR8ArtProfile}" ${builtOut} ${d8Deps}`),
	}, "outDir", "outDict", "outConfig", "outUsage", "outUsageZip", "outUsageDir", "builtOut",
	"d8Flags", "d8Deps", "r8Flags", "zipFlags", "mergeZipsFlags", "resourcesOutput",
	"outR8ArtProfile", "implicits", "outDepfile",
)

var d8IncR8, d8IncR8RE = pctx.MultiCommandRemoteStaticRules("d8Incr8",
	blueprint.RuleParams{
		Command: `mkdir -p "$outDir" "$outDir/packages" && ` +
			`rm -rf "$outDict" "$outConfig" "${outUsageDir}" "${outDepfile}" && ` +
			`mkdir -p $$(dirname ${outUsage}) && ` +
			`. ${config.UsePartialCompileFile} && ` +
			`if [ -n "$${SOONG_USE_PARTIAL_COMPILE}" ]; then ` +
			` for f in "${outConfig}" "${outDict}" "${outUsage}" "${resourcesOutput}"; do ` +
			`   test -n "$${f}" && test ! -f "$${f}" && mkdir -p "$$(dirname "$${f}")" && touch "$${f}" || true; ` +
			` done && ` +
			` ${config.IncrementalDexInputCmd} --classesJar $in --dexTarget $out --deps $d8Deps --outputDir $outDir --packageOutputDir $outDir/packages && ` +
			` $d8Template${config.D8Cmd} ${config.D8Flags} $d8Flags --output $outDir --no-dex-input-jar $in --packages $out.rsp --mod-packages $out.inc.rsp --package-output $outDir/packages && ` +
			// outDepfile is expected as an output file but we don't use r8 in this branch
			` touch ${outDepfile}; ` +
			`else ` +
			` rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			` $r8Template${config.R8Cmd} ${config.R8Flags} $r8Flags -injars $in --output $outDir ` +
			` --no-data-resources ` +
			` -printmapping ${outDict} ` +
			` -printconfiguration ${outConfig} ` +
			` -printusage ${outUsage} ` +
			// Reclient seems to have a bug where you can't have a depfile be an outputfile, work
			// around it by outputting to the output file and copying to the depfile location.
			` --deps-file ${outDepfile} && ` +
			` cp ${outDepfile} ${out}.d && ` +
			` touch "${outDict}" "${outConfig}" "${outUsage}"; ` +
			`fi && ` +
			`${config.SoongZipCmd} -o ${outUsageZip} -C ${outUsageDir} -f ${outUsage} && ` +
			`rm -rf ${outUsageDir} && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $mergeZipsFlags $out $outDir/classes.dex.jar $in && ` +
			`if [ -f "$out.input.pc_state.new" ]; then mv "$out.input.pc_state.new" "$out.input.pc_state" && ` +
			`rm -rf $out.input.pc_state.new; fi && ` +
			`if [ -f "$out.deps.pc_state.new" ]; then mv "$out.deps.pc_state.new" "$out.deps.pc_state" && ` +
			`rm -rf $out.deps.pc_state.new; fi && ` +
			`rm -f "$outDir"/classes*.dex "$outDir/classes.dex.jar" `,
		Pool: android.HighmemPool(),
		CommandDeps: []string{
			"${config.IncrementalDexInputCmd}",
			"${config.D8Cmd}",
			"${config.R8Cmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
			"${config.UsePartialCompileFile}",
		},
		SandboxDisabled: true,
	}, map[string]*remoteexec.REParams{
		"$d8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "d8"},
			Inputs:          []string{"${config.D8Jar}"},
			ExecStrategy:    "${config.RED8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$r8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "r8"},
			Inputs:          []string{"$implicits", "${config.R8Jar}"},
			OutputFiles:     []string{"${outUsage}", "${outConfig}", "${outDict}", "${resourcesOutput}", "${outR8ArtProfile}"},
			ExecStrategy:    "${config.RER8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RED8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "outDict", "outConfig", "outUsage", "outUsageZip", "outUsageDir",
		"outDepfile", "d8Flags", "d8Deps", "r8Flags", "zipFlags", "mergeZipsFlags",
		"resourcesOutput", "outR8ArtProfile"}, []string{"implicits"})

var d8, d8RE = pctx.MultiCommandRemoteStaticRules("d8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`$d8Template${config.D8Cmd} ${config.D8Flags} $d8Flags --output $outDir --no-dex-input-jar $in && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $mergeZipsFlags $out $outDir/classes.dex.jar $in && ` +
			`rm -f "$outDir"/classes*.dex "$outDir/classes.dex.jar"`,
		CommandDeps: []string{
			"${config.D8Cmd}",
			"${config.D8Jar}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
		SandboxDisabled: true,
	}, map[string]*remoteexec.REParams{
		"$d8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "d8"},
			Inputs:          []string{"${config.D8Jar}"},
			ExecStrategy:    "${config.RED8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RED8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "d8Flags", "zipFlags", "mergeZipsFlags"}, nil)

// Include all of the args for d8r8, so that we can generate the partialcompileclean target's build using the same list.
var d8r8Clean = pctx.AndroidStaticRule("d8r8-partialcompileclean",
	blueprint.RuleParams{
		Command: `rm -rf "${outDir}" "${outDict}" "${outConfig}" "${outUsage}" "${outUsageZip}" "${outUsageDir}" "${outDepfile}" ` +
			`"${resourcesOutput}" "${outR8ArtProfile}" ${builtOut}`,
		SandboxDisabled: true,
	}, "outDir", "outDict", "outConfig", "outUsage", "outUsageZip", "outUsageDir", "builtOut",
	"d8Flags", "r8Flags", "zipFlags", "mergeZipsFlags", "resourcesOutput", "outR8ArtProfile",
	"implicits", "outDepfile",
)

var d8r8, d8r8RE = pctx.MultiCommandRemoteStaticRules("d8r8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`rm -f "$outDict" && rm -f "$outConfig" && rm -rf "${outUsageDir}" && ` +
			`mkdir -p $$(dirname ${outUsage}) && ` +
			`. ${config.UsePartialCompileFile} && ` +
			`if [ -n "$${SOONG_USE_PARTIAL_COMPILE}" ]; then ` +
			` for f in "${outConfig}" "${outDict}" "${outUsage}" "${resourcesOutput}"; do ` +
			`   test -n "$${f}" && test ! -f "$${f}" && mkdir -p "$$(dirname "$${f}")" && touch "$${f}" || true; ` +
			` done && ` +
			` $d8Template${config.D8Cmd} ${config.D8Flags} $d8Flags --output $outDir --no-dex-input-jar $in; ` +
			`else ` +
			` $r8Template${config.R8Cmd} ${config.R8Flags} $r8Flags -injars $in --output $outDir ` +
			` --no-data-resources ` +
			` -printmapping ${outDict} ` +
			` -printconfiguration ${outConfig} ` +
			` -printusage ${outUsage} ` +
			// Reclient seems to have a bug where you can't have a depfile be an outputfile, work
			// around it by outputting to the output file and copying to the depfile location.
			` --deps-file ${outDepfile} && ` +
			` cp ${outDepfile} ${out}.d && ` +
			` touch "${outDict}" "${outConfig}" "${outUsage}"; ` +
			`fi && ` +
			`${config.SoongZipCmd} -o ${outUsageZip} -C ${outUsageDir} -f ${outUsage} && ` +
			`rm -rf ${outUsageDir} && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $mergeZipsFlags $out $outDir/classes.dex.jar $in && ` +
			`rm -f "$outDir"/classes*.dex "$outDir/classes.dex.jar" `,
		Pool: android.HighmemPool(),
		CommandDeps: []string{
			"${config.D8Cmd}",
			"${config.D8Jar}",
			"${config.R8Cmd}",
			"${config.R8Jar}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
			"${config.UsePartialCompileFile}",
		},
		SandboxDisabled: true,
	}, map[string]*remoteexec.REParams{
		"$d8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "d8"},
			Inputs:          []string{"${config.D8Jar}"},
			ExecStrategy:    "${config.RED8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$r8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "r8"},
			Inputs:          []string{"$implicits", "${config.R8Jar}"},
			OutputFiles:     []string{"${outDepfile}", "${outUsage}", "${outConfig}", "${outDict}", "${resourcesOutput}", "${outR8ArtProfile}"},
			ExecStrategy:    "${config.RER8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RED8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "outDict", "outConfig", "outUsage", "outUsageZip", "outUsageDir", "outDepfile",
		"d8Flags", "r8Flags", "zipFlags", "mergeZipsFlags", "resourcesOutput", "outR8ArtProfile"}, []string{"implicits"})

var r8, r8RE = pctx.MultiCommandRemoteStaticRules("r8",
	blueprint.RuleParams{
		Command: `rm -rf "$outDir" && mkdir -p "$outDir" && ` +
			`rm -f "$outDict" && rm -f "$outConfig" && rm -rf "${outUsageDir}" && ` +
			`mkdir -p $$(dirname ${outUsage}) && ` +
			`PATH=${config.JavaToolchain}:$$PATH ` +
			`$r8Template${config.R8Cmd} ${config.R8Flags} $r8Flags -injars $in --output $outDir ` +
			`--no-data-resources ` +
			`-printmapping ${outDict} ` +
			`-printconfiguration ${outConfig} ` +
			`-printusage ${outUsage} ` +
			// Reclient seems to have a bug where you can't have a depfile be an outputfile, work
			// around it by outputting to the output file and copying to the depfile location.
			`--deps-file ${outDepfile} && ` +
			`cp ${outDepfile} ${out}.d && ` +
			`touch "${outDict}" "${outConfig}" "${outUsage}" && ` +
			`${config.SoongZipCmd} -o ${outUsageZip} -C ${outUsageDir} -f ${outUsage} && ` +
			`rm -rf ${outUsageDir} && ` +
			`$zipTemplate${config.SoongZipCmd} $zipFlags -o $outDir/classes.dex.jar -C $outDir -f "$outDir/classes*.dex" && ` +
			`${config.MergeZipsCmd} -D -stripFile "**/*.class" $mergeZipsFlags $out $outDir/classes.dex.jar $in && ` +
			`rm -f "$outDir"/classes*.dex "$outDir/classes.dex.jar"`,
		Depfile: "${out}.d",
		Deps:    blueprint.DepsGCC,
		Pool:    android.HighmemPool(),
		CommandDeps: []string{
			"${config.R8Cmd}",
			"${config.R8Jar}",
			"${config.JavaCmd}",
			"${config.SoongZipCmd}",
			"${config.MergeZipsCmd}",
		},
		SandboxDisabled: true,
	}, map[string]*remoteexec.REParams{
		"$r8Template": &remoteexec.REParams{
			Labels:          map[string]string{"type": "compile", "compiler": "r8"},
			Inputs:          []string{"$implicits", "${config.R8Jar}"},
			OutputFiles:     []string{"${outDepfile}", "${outUsage}", "${outConfig}", "${outDict}", "${resourcesOutput}", "${outR8ArtProfile}"},
			ExecStrategy:    "${config.RER8ExecStrategy}",
			ToolchainInputs: []string{"${config.JavaCmd}"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "$outDir"},
			OutputFiles:  []string{"$outDir/classes.dex.jar"},
			ExecStrategy: "${config.RER8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
		"$zipUsageTemplate": &remoteexec.REParams{
			Labels:       map[string]string{"type": "tool", "name": "soong_zip"},
			Inputs:       []string{"${config.SoongZipCmd}", "${outUsage}"},
			OutputFiles:  []string{"${outUsageZip}"},
			ExecStrategy: "${config.RER8ExecStrategy}",
			Platform:     map[string]string{remoteexec.PoolKey: "${config.REJavaPool}"},
		},
	}, []string{"outDir", "outDict", "outConfig", "outUsage", "outUsageZip", "outUsageDir", "outDepfile",
		"r8Flags", "zipFlags", "mergeZipsFlags", "resourcesOutput", "outR8ArtProfile"}, []string{"implicits"})

var proguardDictToProto = pctx.AndroidStaticRule("proguard_dict_to_proto", blueprint.RuleParams{
	Command:         `${symbols_map} -r8 $in -location $location -write_if_changed $out`,
	Restat:          true,
	CommandDeps:     []string{"${symbols_map}"},
	SandboxDisabled: true,
}, "location")

func (d *dexer) dexCommonFlags(ctx android.ModuleContext,
	dexParams *compileDexParams) (flags []string, deps android.Paths, incD8Compatible bool) {

	flags = d.dexProperties.Dxflags.GetOrDefault(ctx, nil)
	// Translate all the DX flags to D8 ones until all the build files have been migrated
	// to D8 flags. See: b/69377755
	flags = android.RemoveListFromList(flags,
		[]string{"--core-library", "--dex", "--multi-dex"})

	for _, f := range android.PathsForModuleSrc(ctx, d.dexProperties.Main_dex_rules) {
		flags = append(flags, "--main-dex-rules", f.String())
		deps = append(deps, f)
	}

	var requestReleaseMode, requestDebugMode bool
	requestReleaseMode, flags = android.RemoveFromList("--release", flags)
	requestDebugMode, flags = android.RemoveFromList("--debug", flags)

	if ctx.Config().Getenv("NO_OPTIMIZE_DX") != "" || ctx.Config().Getenv("GENERATE_DEX_DEBUG") != "" {
		requestDebugMode = true
		requestReleaseMode = false
	}

	// Don't strip out debug information for eng builds, unless the target
	// explicitly provided the `--release` build flag. This allows certain
	// test targets to remain optimized as part of eng test_suites builds.
	if requestDebugMode {
		flags = append(flags, "--debug")
	} else if requestReleaseMode {
		flags = append(flags, "--release")
	} else if ctx.Config().Eng() {
		flags = append(flags, "--debug")
	} else if !d.effectiveOptimizeEnabled(ctx) && d.dexProperties.Optimize.EnabledByDefault {
		// D8 uses --debug by default, whereas R8 uses --release by default.
		// For targets that default to R8 usage (e.g., apps), but override this default, we still
		// want D8 to run in release mode, preserving semantics as much as possible between the two.
		flags = append(flags, "--release")
	}

	// Supplying the platform build flag disables various features like API modeling and desugaring.
	// For targets with a stable min SDK version (i.e., when the min SDK is both explicitly specified
	// and managed+versioned), we suppress this flag to ensure portability.
	// Note: Targets with a min SDK kind of core_platform (e.g., framework.jar) or unspecified (e.g.,
	// services.jar), are not classified as stable, which is WAI.
	// TODO(b/232073181): Expand to additional min SDK cases after validation.
	var addAndroidPlatformBuildFlag = false
	if !dexParams.sdkVersion.Stable() {
		addAndroidPlatformBuildFlag = true
	}

	effectiveVersion, err := dexParams.minSdkVersion.EffectiveVersion(ctx)
	if err != nil {
		ctx.PropertyErrorf("min_sdk_version", "%s", err)
	}
	if !Bool(d.dexProperties.No_dex_container) && effectiveVersion.FinalOrFutureInt() >= 36 && ctx.Config().UseDexV41() {
		// W is 36, but we have not bumped the SDK version yet, so check for both.
		if ctx.Config().PlatformSdkVersion().FinalInt() >= 36 ||
			ctx.Config().PlatformSdkCodename() == "Baklava" {
			flags = append([]string{"-JDcom.android.tools.r8.dexContainerExperiment"}, flags...)
		}
	}

	if !ctx.Config().UseR8MinimizedSyntheticNames() {
		flags = append(flags, "--verbose-synthetic-names")
	}

	// Enable a safe form of list iteration rewriting for D8/R8, though note that D8 impact is
	// conditional on the use of release mode. We *prepend* this flag to ensure application
	// to the JVM environment, before other args that are passed along to D8/R8.
	flags = append([]string{"-JDcom.android.tools.r8.enableListIterationRewriting=1"}, flags...)

	// If the specified SDK level is 10000, then configure the compiler to use the
	// current platform SDK level and to compile the build as a platform build.
	var minApiFlagValue = effectiveVersion.FinalOrFutureInt()
	if minApiFlagValue == 10000 {
		minApiFlagValue = ctx.Config().PlatformSdkVersion().FinalInt()
		addAndroidPlatformBuildFlag = true
	}
	flags = append(flags, "--min-api "+strconv.Itoa(minApiFlagValue))

	if ctx.Config().PartialCompileFlags().Enable_inc_d8_outside_platform {
		incD8Compatible = true
	} else {
		incD8Compatible = false
	}

	if addAndroidPlatformBuildFlag {
		flags = append(flags, "--android-platform-build")
		incD8Compatible = true
	}
	return flags, deps, incD8Compatible
}

func (d *dexer) d8Flags(ctx android.ModuleContext, dexParams *compileDexParams, useD8Inc bool) (d8Flags []string, d8Deps android.Paths, artProfileOutput *android.OutputPath) {
	flags := dexParams.flags
	d8Flags = append(d8Flags, flags.bootClasspath.FormRepeatedClassPath("--lib ")...)
	d8Flags = append(d8Flags, flags.dexClasspath.FormRepeatedClassPath("--lib ")...)
	d8Deps = append(d8Deps, flags.bootClasspath...)
	d8Deps = append(d8Deps, flags.dexClasspath...)
	if !useD8Inc {
		if flags, deps, profileOutput := d.addArtProfile(ctx, dexParams); profileOutput != nil {
			d8Flags = append(d8Flags, flags...)
			d8Deps = append(d8Deps, deps...)
			artProfileOutput = profileOutput
		}
	}

	return d8Flags, d8Deps, artProfileOutput
}

func (d *dexer) r8DepLibFlags(ctx android.ModuleContext, dexParams *compileDexParams) (r8Flags []string, r8Deps android.Paths) {
	var depLibs classpath

	// When an app contains references to APIs that are not in the SDK specified by
	// its LOCAL_SDK_VERSION for example added by support library or by runtime
	// classes added by desugaring, we artificially raise the "SDK version" "linked" by
	// ProGuard, to
	// - suppress ProGuard warnings of referencing symbols unknown to the lower SDK version.
	// - prevent ProGuard stripping subclass in the support library that extends class added in the higher SDK version.
	// See b/20667396
	// TODO(b/360905238): Remove SdkSystemServer exception after resolving missing class references.
	if !dexParams.sdkVersion.Stable() || dexParams.sdkVersion.Kind == android.SdkSystemServer {
		var proguardRaiseDeps classpath
		ctx.VisitDirectDepsProxyWithTag(proguardRaiseTag, func(m android.ModuleProxy) {
			if dep, ok := android.OtherModuleProvider(ctx, m, JavaInfoProvider); ok {
				proguardRaiseDeps = append(proguardRaiseDeps, dep.RepackagedHeaderJars...)
			}
		})
		depLibs = append(depLibs, proguardRaiseDeps...)
	}

	depLibs = append(depLibs, dexParams.flags.bootClasspath...)
	depLibs = append(depLibs, dexParams.flags.dexClasspath...)

	transitiveStaticLibsLookupMap := map[android.Path]bool{}
	for _, jar := range d.transitiveStaticLibsHeaderJarsForR8.ToList() {
		transitiveStaticLibsLookupMap[jar] = true
	}
	transitiveHeaderJars := android.Paths{}
	for _, jar := range d.transitiveLibsHeaderJarsForR8.ToList() {
		if _, ok := transitiveStaticLibsLookupMap[jar]; ok {
			// don't include a lib if it is already packaged in the current JAR as a static lib
			continue
		}
		transitiveHeaderJars = append(transitiveHeaderJars, jar)
	}
	transitiveClasspath := classpath(transitiveHeaderJars)
	depLibs = append(depLibs, transitiveClasspath...)

	depLibs = depLibs.FirstUniquePaths()
	r8Deps = append(r8Deps, depLibs...)

	var libraryDepArg string
	if Bool(d.dexProperties.Optimize.Import_libraries_as_classpath) {
		libraryDepArg = "--classpath "
	} else {
		libraryDepArg = "--lib "
	}
	r8Flags = append(r8Flags, depLibs.FormRepeatedClassPath(libraryDepArg)...)

	return r8Flags, r8Deps
}

func (d *dexer) r8Flags(ctx android.ModuleContext, dexParams *compileDexParams, debugMode, useD8Inc bool) (r8Flags []string, r8Deps android.Paths, artProfileOutput *android.OutputPath) {
	r8Flags, r8Deps = d.r8DepLibFlags(ctx, dexParams)

	flags := dexParams.flags
	opt := d.dexProperties.Optimize

	// TODO(b/248580093): Get the base set of Proguard rules from a property
	// that defaults to a filegroup populated based on build flags.
	flagFiles := android.Paths{
		android.PathForSource(ctx, "build/make/core/proguard.flags"),
	}

	if ctx.Config().UseR8GlobalCheckNotNullFlags() {
		flagFiles = append(flagFiles, android.PathForSource(ctx,
			"build/make/core/proguard/checknotnull.flags"))
	}

	if ctx.Config().OmitR8GlobalEnumKeeps() {
		// Intentionally blank when global enum keep rules are explicitly omitted.
	} else {
		flagFiles = append(flagFiles, android.PathForSource(ctx,
			"build/make/core/proguard/enumvalues.flags"))
	}

	flagFiles = append(flagFiles, d.extraProguardFlagsFiles...)
	r8Deps = append(r8Deps, d.extraIncludedProguardFlagsFiles...)
	// TODO(ccross): static android library proguard files

	proguardFlagsFiles := d.ProguardFlagsFiles(ctx)
	flagFiles = append(flagFiles, proguardFlagsFiles.files...)
	r8Deps = append(r8Deps, proguardFlagsFiles.included...)

	// Note: This is a special case where we explicitly don't want SDK-injected
	// proguard rules to propagate to other targets or cross library boundaries,
	// otherwise we'd reuse existing propagation with ProguardSpecInfoProvider.
	ctx.VisitDirectDepsProxyWithTag(sdkDepProguardTag, func(m android.ModuleProxy) {
		flagFiles = append(flagFiles, android.OutputFilesForModule(ctx, m, "")...)
	})

	traceReferencesSources := android.Paths{}
	ctx.VisitDirectDepsProxyWithTag(traceReferencesTag, func(m android.ModuleProxy) {
		if dep, ok := android.OtherModuleProvider(ctx, m, JavaInfoProvider); ok {
			traceReferencesSources = append(traceReferencesSources, dep.ImplementationJars...)
		}
	})
	if len(traceReferencesSources) > 0 {
		traceTarget := dexParams.classesJar
		traceLibs := android.FirstUniquePaths(append(flags.bootClasspath.Paths(), flags.dexClasspath.Paths()...))
		traceReferencesFlags := android.PathForModuleOut(ctx, "proguard", "trace_references.flags")
		TraceReferences(ctx, traceReferencesSources, traceTarget, traceLibs, traceReferencesFlags)
		flagFiles = append(flagFiles, traceReferencesFlags)
	}

	flagFiles = android.FirstUniquePaths(flagFiles)

	r8Flags = append(r8Flags, android.JoinWithPrefix(flagFiles.Strings(), "-include "))
	r8Deps = append(r8Deps, flagFiles...)

	// TODO(b/70942988): This is included from build/make/core/proguard.flags
	r8Deps = append(r8Deps,
		android.PathForSource(ctx, "build/make/core/proguard_basic_keeps.flags"),
		android.PathForSource(ctx, "build/make/core/proguard/kotlin.flags"),
	)

	r8Flags = append(r8Flags, opt.Proguard_flags.GetOrDefault(ctx, nil)...)

	if BoolDefault(opt.Ignore_library_extends_program, false) {
		r8Flags = append(r8Flags, "--ignore-library-extends-program")
	}

	if !ctx.Config().UseR8OnlyRuntimeVisibleAnnotations() && BoolDefault(opt.Keep_runtime_invisible_annotations, false) {
		r8Flags = append(r8Flags, "--keep-runtime-invisible-annotations")
	}

	if opt.Proguard_compatibility.GetOrDefault(ctx, false) {
		r8Flags = append(r8Flags, "--force-proguard-compatibility")
	}

	if BoolDefault(opt.Protect_api_surface, false) {
		r8Flags = append(r8Flags, "--protect-api-surface")
	}

	// Avoid unnecessary stack frame noise by only injecting source map ids for non-debug
	// optimized or obfuscated targets.
	optimize := opt.Optimize.GetOrDefault(ctx, opt.OptimizeByDefault)
	if (optimize || opt.Obfuscate.GetOrDefault(ctx, false)) && !debugMode {
		// TODO(b/213833843): Allow configuration of the prefix via a build variable.
		var sourceFilePrefix = "go/retraceme "
		var sourceFileTemplate = "\"" + sourceFilePrefix + "%MAP_ID\""
		r8Flags = append(r8Flags, "--map-id-template", "%MAP_HASH")
		r8Flags = append(r8Flags, "--source-file-template", sourceFileTemplate)
	}

	// TODO(ccross): Don't shrink app instrumentation tests by default.
	if !d.shrinkEnabled(ctx) {
		r8Flags = append(r8Flags, "-dontshrink")
	}

	if !optimize {
		r8Flags = append(r8Flags, "-dontoptimize")
	}

	// TODO(ccross): error if obufscation + app instrumentation test.
	if !opt.Obfuscate.GetOrDefault(ctx, false) {
		r8Flags = append(r8Flags, "-dontobfuscate")
	}
	// TODO(ccross): if this is an instrumentation test of an obfuscated app, use the
	// dictionary of the app and move the app from libraryjars to injars.

	// For now, only enable warnings by default if the target is optimized, as that is generally
	// the riskiest scenario to ignore such warnings.
	// TODO(b/180878971): Address missing classes in remaining builds and default to not ignoring
	// warnings for all targets, not just fully optimized ones.
	ignoreWarningsByDefault := !optimize
	if proptools.BoolDefault(opt.Ignore_warnings, ignoreWarningsByDefault) {
		r8Flags = append(r8Flags, "-ignorewarnings")
	}

	// resourcesInput is empty when we don't use resource shrinking, if on, pass these to R8
	if d.resourcesInput.Valid() {
		r8Flags = append(r8Flags, "--resource-input", d.resourcesInput.Path().String())
		r8Deps = append(r8Deps, d.resourcesInput.Path())
		r8Flags = append(r8Flags, "--resource-output", d.resourcesOutput.Path().String())
		if d.optimizedResourceShrinkingEnabled(ctx) {
			r8Flags = append(r8Flags, "--optimized-resource-shrinking")
			if d.dexProperties.Optimize.Optimized_shrink_resources.GetOrDefault(ctx, false) {
				// Explicitly opted into optimized shrinking, no need for keeping R$id entries
				r8Flags = append(r8Flags, "--force-optimized-resource-shrinking")
			}
		}
	}

	if !useD8Inc {
		if flags, deps, profileOutput := d.addArtProfile(ctx, dexParams); profileOutput != nil {
			r8Flags = append(r8Flags, flags...)
			r8Deps = append(r8Deps, deps...)
			artProfileOutput = profileOutput
		}
	}

	if ctx.Config().UseR8StoreStoreFenceConstructorInlining() {
		r8Flags = append(r8Flags, "--store-store-fence-constructor-inlining")
	}

	if opt.Exclude != nil {
		excludeFile := android.PathForModuleSrc(ctx, *opt.Exclude)
		r8Flags = append(r8Flags, "--exclude", excludeFile.String())
		r8Deps = append(r8Deps, excludeFile)
	}

	if opt.Include != nil {
		includeFile := android.PathForModuleSrc(ctx, *opt.Include)
		r8Flags = append(r8Flags, "--include", includeFile.String())
		r8Deps = append(r8Deps, includeFile)
	}

	return r8Flags, r8Deps, artProfileOutput
}

type compileDexParams struct {
	flags           javaBuilderFlags
	sdkVersion      android.SdkSpec
	minSdkVersion   android.ApiLevel
	classesJar      android.Path
	jarName         string
	artProfileInput *string
}

// Adds --art-profile to r8/d8 command.
// r8/d8 will output a generated profile file to match the optimized dex code.
func (d *dexer) addArtProfile(ctx android.ModuleContext, dexParams *compileDexParams) (flags []string, deps android.Paths, artProfileOutputPath *android.OutputPath) {
	if dexParams.artProfileInput == nil {
		return nil, nil, nil
	}
	artProfileInputPath := android.PathForModuleSrc(ctx, *dexParams.artProfileInput)
	artProfileOutputPathValue := android.PathForModuleOut(ctx, "profile.prof.txt").OutputPath
	artProfileOutputPath = &artProfileOutputPathValue
	flags = []string{
		"--art-profile",
		artProfileInputPath.String(),
		artProfileOutputPath.String(),
	}
	deps = append(deps, artProfileInputPath)
	return flags, deps, artProfileOutputPath

}

// Return the compiled dex jar and (optional) profile _after_ r8 optimization
func (d *dexer) compileDex(ctx android.ModuleContext, dexParams *compileDexParams) (android.Path, android.Path) {

	// Compile classes.jar into classes.dex and then javalib.jar
	javalibJar := android.PathForModuleOut(ctx, "dex", dexParams.jarName).OutputPath
	cleanPhonyPath := android.PathForModuleOut(ctx, "dex", dexParams.jarName+"-partialcompileclean").OutputPath
	outDir := android.PathForModuleOut(ctx, "dex")

	zipFlags := "--ignore_missing_files --quiet"
	if proptools.Bool(d.dexProperties.Uncompress_dex) {
		zipFlags += " -L 0"
	}

	commonFlags, commonDeps, incD8Compatible := d.dexCommonFlags(ctx, dexParams)

	// Exclude kotlinc generated files when "exclude_kotlinc_generated_files" is set to true.
	mergeZipsFlags := ""
	if proptools.BoolDefault(d.dexProperties.Exclude_kotlinc_generated_files, false) {
		mergeZipsFlags = `-stripFile "META-INF/**/*.kotlin_module" -stripFile "**/*.kotlin_builtins"`
	}

	useR8 := d.effectiveOptimizeEnabled(ctx)
	useD8 := !useR8 || ctx.Config().PartialCompileFlags().Use_d8
	// d8Inc is applicable only when d8 is allowed.
	useD8Inc := useD8 && ctx.Config().PartialCompileFlags().Enable_inc_d8 && incD8Compatible
	rbeR8 := ctx.Config().UseREWrapper() && ctx.Config().IsEnvTrue("RBE_R8")
	rbeD8 := ctx.Config().UseREWrapper() && ctx.Config().IsEnvTrue("RBE_D8")
	var rule blueprint.Rule
	var description string
	var artProfileOutputPath *android.OutputPath
	var implicitOutputs android.WritablePaths
	var deps android.Paths
	args := map[string]string{
		"zipFlags":       zipFlags,
		"outDir":         outDir.String(),
		"mergeZipsFlags": mergeZipsFlags,
	}
	if useR8 {
		proguardDictionary := android.PathForModuleOut(ctx, "proguard_dictionary")
		d.proguardDictionary = android.OptionalPathForPath(proguardDictionary)
		proguardConfiguration := android.PathForModuleOut(ctx, "proguard_configuration")
		d.proguardConfiguration = android.OptionalPathForPath(proguardConfiguration)
		proguardUsageDir := android.PathForModuleOut(ctx, "proguard_usage")
		proguardUsage := proguardUsageDir.Join(ctx, ctx.Namespace().Path,
			android.ModuleNameWithPossibleOverride(ctx), "unused.txt")
		proguardUsageZip := android.PathForModuleOut(ctx, "proguard_usage.zip")
		d.proguardUsageZip = android.OptionalPathForPath(proguardUsageZip)
		resourcesOutput := android.PathForModuleOut(ctx, "package-res-shrunken.apk")
		d.resourcesOutput = android.OptionalPathForPath(resourcesOutput)
		implicitOutputs = append(implicitOutputs, android.WritablePaths{
			proguardDictionary,
			proguardUsageZip,
			proguardConfiguration,
		}...)
		description = "r8"
		debugMode := android.InList("--debug", commonFlags)
		r8Flags, r8Deps, r8ArtProfileOutputPath := d.r8Flags(ctx, dexParams, debugMode, useD8Inc)
		deps = append(deps, r8Deps...)

		var r8JvmFlags []string
		if ctx.Config().IsEnvTrue("R8_DUMP_INPUT") {
			r8Inputs := android.PathForModuleOut(ctx, "r8inputs.zip")
			r8JvmFlags = append(r8JvmFlags, "-JDcom.android.tools.r8.dumpinputtofile="+r8Inputs.String())
		}
		if ctx.Config().IsEnvTrue("R8_DUMP_BLAST_RADIUS") {
			r8BlastRadius := android.PathForModuleOut(ctx, "r8blastradius.pb")
			r8JvmFlags = append(r8JvmFlags, "-JDcom.android.tools.r8.dumpblastradiustofile="+r8BlastRadius.String())
		}
		if ctx.Config().IsEnvTrue("R8_DUMP_PERFETTO_TRACE") {
			r8PerfettoTrace := android.PathForModuleOut(ctx, "r8trace.ptrace")
			r8JvmFlags = append(r8JvmFlags, "-JDcom.android.tools.r8.dumptracetofile="+r8PerfettoTrace.String())
		}

		args["r8Flags"] = strings.Join(slices.Concat(r8JvmFlags, commonFlags, r8Flags), " ")
		if r8ArtProfileOutputPath != nil {
			artProfileOutputPath = r8ArtProfileOutputPath
			// Add the implicit r8 Art profile output to args so that r8RE knows
			// about this implicit output
			args["outR8ArtProfile"] = r8ArtProfileOutputPath.String()
		}
		args["outDict"] = proguardDictionary.String()
		args["outConfig"] = proguardConfiguration.String()
		args["outUsageDir"] = proguardUsageDir.String()
		args["outUsage"] = proguardUsage.String()
		args["outUsageZip"] = proguardUsageZip.String()
		if d.resourcesInput.Valid() {
			implicitOutputs = append(implicitOutputs, resourcesOutput)
			args["resourcesOutput"] = resourcesOutput.String()
		}

		rule = r8
		if rbeR8 {
			rule = r8RE
			args["implicits"] = strings.Join(deps.Strings(), ",")
		}
	}
	cleanupD8R8, cleanupD8IncR8, cleanupD8Inc := false, false, false
	if useD8 {
		description = "d8"
		d8Flags, d8Deps, d8ArtProfileOutputPath := d.d8Flags(ctx, dexParams, useD8Inc)
		deps = append(deps, d8Deps...)
		deps = append(deps, commonDeps...)
		args["d8Flags"] = strings.Join(append(commonFlags, d8Flags...), " ")
		if d8ArtProfileOutputPath != nil {
			artProfileOutputPath = d8ArtProfileOutputPath
		}
		// The file containing dependencies of the current module
		// Any change in them may warrant changes in the incremental dex compilation
		// source set.
		if useD8Inc {
			d8DepsFile := android.PathForModuleOut(ctx, dexParams.jarName+".dex.deps.rsp")
			android.WriteFileRule(ctx, d8DepsFile, strings.Join(deps.Strings(), "\n"))
			deps = append(deps, d8DepsFile)
			args["d8Deps"] = d8DepsFile.String()
		}
		// If we are generating both d8 and r8, only use RBE when both are enabled.
		switch {
		// r8 is the selected rule, useD8Inc is the override
		case useR8 && rule == r8 && useD8Inc:
			rule = d8IncR8
			cleanupD8IncR8 = true
			description = "d8IncR8"
		// r8 is the selected rule, useD8 is the override
		case useR8 && rule == r8:
			rule = d8r8
			cleanupD8R8 = true
			description = "d8r8"
		// rbeR8 is the selected rule, useD8Inc is the override
		case useR8 && rule == r8RE && useD8Inc:
			rule = d8IncR8RE
			cleanupD8IncR8 = true
			description = "d8IncR8"
		// rbeR8 is the selected rule, useD8 is the override
		case useR8 && rule == r8RE && rbeD8:
			rule = d8r8RE
			cleanupD8R8 = true
			description = "d8r8"
		// rbeD8 is the selected rule, useD8Inc is the override
		case rbeD8 && useD8Inc:
			rule = d8IncRE
			cleanupD8Inc = true
			description = "d8Inc"
		// rbeD8 is the selected rule
		case rbeD8:
			rule = d8RE
		// D8 is the selected rule, useD8Inc is the override
		case useD8Inc:
			rule = d8Inc
			cleanupD8Inc = true
			description = "d8Inc"
		// D8 is the selected rule
		default:
			rule = d8
		}
	}
	if artProfileOutputPath != nil {
		implicitOutputs = append(
			implicitOutputs,
			artProfileOutputPath,
		)
	}

	// Because proguard files can include other proguard files, check that the user listed
	// all of them in the bp files by checking the r8 depfile vs the input files.
	var depfileVerifier android.WritablePath
	if useR8 {
		depfile := android.PathForModuleOut(ctx, "dex_depfile_verifier", dexParams.jarName+".d")
		args["outDepfile"] = depfile.String()
		implicitOutputs = append(implicitOutputs, depfile)
		// Need to use a new dex_depfile_verifier folder because the r8 rule rm -rf's the dex folder
		depfileVerifier = android.PathForModuleOut(ctx, "dex_depfile_verifier", dexParams.jarName+".depfile_verifier")
		android.DepfileVerifierRule(ctx, depfileVerifier, depfile, append(deps, dexParams.classesJar))
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            rule,
		Description:     description,
		Output:          javalibJar,
		ImplicitOutputs: implicitOutputs,
		Input:           dexParams.classesJar,
		Implicits:       deps,
		Args:            args,
		Validation:      depfileVerifier,
	})
	// Run cleanup when d8r8 was used
	if cleanupD8R8 {
		// Generate the rule for partial compile clean.
		args["builtOut"] = javalibJar.String()
		ctx.Build(pctx, android.BuildParams{
			Rule:        d8r8Clean,
			Description: "d8r8Clean",
			Output:      cleanPhonyPath,
			Args:        args,
			PhonyOutput: true,
		})
		ctx.Phony("partialcompileclean", cleanPhonyPath)
	}
	// Run cleanup when d8IncR8 was used
	if cleanupD8IncR8 {
		// Generate the rule for partial compile clean.
		args["builtOut"] = javalibJar.String()
		ctx.Build(pctx, android.BuildParams{
			Rule:        d8IncR8Clean,
			Description: "d8IncR8Clean",
			Output:      cleanPhonyPath,
			Args:        args,
			PhonyOutput: true,
		})
		ctx.Phony("partialcompileclean", cleanPhonyPath)
	}
	// Run cleanup when d8Inc was used
	if cleanupD8Inc {
		// Generate the rule for partial compile clean.
		args["builtOut"] = javalibJar.String()
		ctx.Build(pctx, android.BuildParams{
			Rule:        d8IncClean,
			Description: "d8IncClean",
			Output:      cleanPhonyPath,
			Args:        args,
			PhonyOutput: true,
		})
		ctx.Phony("partialcompileclean", cleanPhonyPath)
	}

	if proptools.Bool(d.dexProperties.Uncompress_dex) {
		alignedJavalibJar := android.PathForModuleOut(ctx, "aligned", dexParams.jarName).OutputPath
		TransformZipAlign(ctx, alignedJavalibJar, javalibJar, nil)
		javalibJar = alignedJavalibJar
	}

	return javalibJar, artProfileOutputPath
}

type ProguardZips struct {
	DictZip     android.Path
	DictMapping android.Path
	UsageZip    android.Path
}

func BuildProguardZips(ctx android.ModuleContext, modules []android.ModuleProxy) ProguardZips {
	dictZip := android.PathForModuleOut(ctx, "proguard-dict.zip")
	dictZipBuilder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	dictZipCmd := dictZipBuilder.Command().BuiltTool("soong_zip").Flag("-d").FlagWithOutput("-o ", dictZip)

	dictMapping := android.PathForModuleOut(ctx, "proguard-dict-mapping.textproto")
	dictMappingBuilder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	dictMappingCmd := dictMappingBuilder.Command().BuiltTool("symbols_map").Flag("-merge").Output(dictMapping)

	protosDir := android.PathForModuleOut(ctx, "proguard_mapping_protos")

	usageZip := android.PathForModuleOut(ctx, "proguard-usage.zip")
	usageZipBuilder := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	usageZipCmd := usageZipBuilder.Command().BuiltTool("merge_zips").Output(usageZip)

	visitedProguardInfo := make(map[string]bool)
	for _, mod := range modules {
		if proguardInfos, ok := android.OtherModuleProvider(ctx, mod, ProguardProvider); ok {
			for _, proguardInfo := range proguardInfos {
				// Maintain these out/target/common paths for backwards compatibility. They may be able
				// to be changed if tools look up file locations from the protobuf, but I'm not
				// exactly sure how that works.
				dictionaryFakePath := fmt.Sprintf("out/target/common/obj/%s/%s_intermediates/proguard_dictionary", proguardInfo.Class, proguardInfo.ModuleName)

				// It's technically possible for some aggregate module types (e.g., apex modules) to bundle
				// targets that are directly included in the installed image. Since they point to the same
				// proguard information, de-duplicate them here to avoid any collisions.
				if visitedProguardInfo[dictionaryFakePath] {
					continue
				}
				visitedProguardInfo[dictionaryFakePath] = true

				dictZipCmd.FlagWithArg("-e ", dictionaryFakePath)
				dictZipCmd.FlagWithInput("-f ", proguardInfo.ProguardDictionary)
				dictZipCmd.Textf("-e out/target/common/obj/%s/%s_intermediates/classes.jar", proguardInfo.Class, proguardInfo.ModuleName)
				dictZipCmd.FlagWithInput("-f ", proguardInfo.ClassesJar)

				protoFile := protosDir.Join(ctx, filepath.Dir(dictionaryFakePath), "proguard_dictionary.textproto")
				ctx.Build(pctx, android.BuildParams{
					Rule:   proguardDictToProto,
					Input:  proguardInfo.ProguardDictionary,
					Output: protoFile,
					Args: map[string]string{
						"location": dictionaryFakePath,
					},
				})
				dictMappingCmd.Input(protoFile)

				usageZipCmd.Input(proguardInfo.ProguardUsageZip)
			}
		}
	}

	dictZipBuilder.Build("proguard_dict_zip", "Building proguard dictionary zip")
	dictMappingBuilder.Build("proguard_dict_mapping_proto", "Building proguard mapping proto")
	usageZipBuilder.Build("proguard_usage_zip", "Building proguard usage zip")

	return ProguardZips{
		DictZip:     dictZip,
		DictMapping: dictMapping,
		UsageZip:    usageZip,
	}
}

// @auto-generate: gob
type ProguardInfo struct {
	ModuleName         string
	Class              string
	ProguardDictionary android.Path
	ProguardUsageZip   android.Path
	ClassesJar         android.Path
}

// @auto-generate: gob
type ProguardInfos []ProguardInfo

var ProguardProvider = blueprint.NewProvider[ProguardInfos]()
