// Copyright 2019 The Android Open Source Project
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

package rust

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/depset"

	"android/soong/android"
	"android/soong/cc"
	cc_config "android/soong/cc/config"
	"android/soong/remoteexec"
	"android/soong/rust/config"
)

var (
	_            = pctx.SourcePathVariable("rustcCmd", "${config.RustBin}/rustc")
	rustcWrapper = pctx.HostTool("rustc_wrapper")

	rustc, rustcRbe = pctx.RemoteStaticRules("rustc",
		blueprint.RuleParams{
			Command: "$relPwd $reTemplate ${SoongEnvCmd} -i --allow PWD --allow TMPDIR $envVars ${RustcWrapper} ${rustcCmd} " +
				"-C linker=${RustcLinkerCmd} -C link-args=\"--android-clang-bin=${config.ClangCmd} ${linkerScriptFlags}\" " +
				"-C link-args=@${out}.clang.rsp " +
				"--emit ${emitType} -o $out --emit dep-info=$out.d.raw $in ${libFlags} $rustcFlags" +
				" && ${DepfileVerifier} --check-suffix-only $out.d ${soongSrcsFile}",
			CommandDeps:    []string{"$rustcCmd", "${RustcLinkerCmd}", "${config.ClangCmd}", "${DepfileVerifier}", "${RustcWrapper}", "${SoongEnvCmd}"},
			Rspfile:        "${out}.clang.rsp",
			RspfileContent: "${crtBegin} ${earlyLinkFlags} ${linkFlags} ${crtEnd}",
		}, &remoteexec.REParams{
			// Until there's a "rust" tool, use clang. This interprets "-L" flags
			// to help identify potential build dependencies.
			Labels:               map[string]string{"type": "link", "tool": "clang"},
			Inputs:               []string{"${out}.clang.rsp"},
			RSPFiles:             []string{"$rbeRspFile"},
			OutputFiles:          []string{"${out}.d", "${out}.d.raw", "${out}"},
			ExecStrategy:         "${config.RERustExecStrategy}",
			EnvironmentVariables: []string{"PWD"},
			ToolchainInputs: []string{
				"${rustcCmd}",
				"${RustcLinkerCmd}",
				"${config.ClangCmd}",
				"${config.LlvmDlltool}",
				"${DepfileVerifier}",
				"${RustcWrapper}",
				"${SoongEnvCmd}",
			},
			Platform: map[string]string{remoteexec.PoolKey: "${config.RERustPool}"},
		},
		[]string{"rustcFlags", "linkerScriptFlags", "earlyLinkFlags", "linkFlags", "libFlags", "crtBegin", "crtEnd", "emitType", "envVars", "soongSrcsFile"},
		[]string{"rbeRspFile"})

	_       = pctx.SourcePathVariable("rustdocCmd", "${config.RustBin}/rustdoc")
	rustdoc = pctx.AndroidStaticRule("rustdoc",
		blueprint.RuleParams{
			Command2: blueprint.NewCommand(
				"$envVars ", rustcWrapper, " $rustdocCmd $rustdocFlags $in -o $outDir && ",
				android.Touch, " $out"),
			CommandDeps: []string{"$rustdocCmd"},
		},
		"rustdocFlags", "outDir", "envVars")

	_                  = pctx.SourcePathVariable("clippyCmd", "${config.RustBin}/clippy-driver")
	generateClippyRule = func(ruleName string, extraFlags string) blueprint.Rule {
		return pctx.AndroidStaticRule(ruleName,
			blueprint.RuleParams{
				Command2: blueprint.NewCommand(
					"$envVars ", rustcWrapper, " ${clippyCmd} ",
					// Because clippy-driver uses rustc as backend, we need to have some output even during the linting.
					// Use the metadata output as it has the smallest footprint.
					"--emit metadata -o $out --emit dep-info=$out.d.raw $in ${libFlags} ",
					"$rustcFlags $clippyFlags ", extraFlags, " && ",
					android.DepfileVerifier, " --check-suffix-only $out.d ${soongSrcsFile}"),
				CommandDeps: []string{"$clippyCmd"},
			},
			"rustcFlags", "libFlags", "clippyFlags", "envVars", "soongSrcsFile")
	}
	clippyDriver = generateClippyRule("clippy", "")

	// Same as clippyDriver, but outputting any errors or warnings in a JSON schema to a
	// $out.error file. This file will then be parsed by rust_check.py to provide clippy-driver
	// errors and warnings to rust-analyzer.
	clippyJsonDriver = generateClippyRule("clippyJson", "--error-format=json 2> $out.error")

	cp = pctx.AndroidStaticRule("cp",
		blueprint.RuleParams{
			Command2: blueprint.NewCommand(
				android.Cp, " `", android.Cat, " $outDir.rsp` $outDir"),
			Rspfile:        "${outDir}.rsp",
			RspfileContent: "$in",
		},
		"outDir")
)

type buildOutput struct {
	outputFile android.Path
	kytheFile  android.Path
}

func init() {
	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
	pctx.HostBinToolVariable("RustcLinkerCmd", "rustc_linker")
	pctx.HostBinToolVariable("RustcWrapper", "rustc_wrapper")
	pctx.HostBinToolVariable("SoongEnvCmd", "soong_env")
	pctx.HostBinToolVariable("DepfileVerifier", "depfile_verifier")
	pctx.StaticVariable("relPwd", cc.PwdPrefix())

	cc.TransformRlibstoStaticlib = TransformRlibstoStaticlib
}

type transformProperties struct {
	crateName               string
	targetTriple            string
	is64Bit                 bool
	bootstrap               bool
	inRecovery              bool
	inRamdisk               bool
	inVendorRamdisk         bool
	cargoOutDir             android.OptionalPath
	synthetic               bool
	crateType               string
	emitType                string
	useExpansiveDefaultSrcs bool
}

// Populates a standard transformProperties struct for Rust modules
func getTransformProperties(ctx ModuleContext, crateType string) transformProperties {
	module := ctx.RustModule()
	return transformProperties{
		crateName:       module.CrateName(),
		is64Bit:         ctx.toolchain().Is64Bit(),
		targetTriple:    ctx.toolchain().RustTriple(),
		bootstrap:       module.Bootstrap(),
		inRecovery:      module.InRecovery(),
		inRamdisk:       module.InRamdisk(),
		inVendorRamdisk: module.InVendorRamdisk(),
		cargoOutDir:     module.compiler.cargoOutDir(ctx),

		// crateType indicates what type of crate to build
		crateType: crateType,

		// synthetic indicates whether this is an actual Rust module or not
		synthetic: false,

		emitType: module.compiler.emitType(),

		useExpansiveDefaultSrcs: module.compiler.useExpansiveDefaultSrcs(),
	}
}

func TransformSrcToBinary(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath) buildOutput {
	if ctx.RustModule().compiler.Thinlto() {
		flags.GlobalRustFlags = flags.GlobalRustFlags.AppendNoDeps("-C lto=thin")
	}

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, "bin"))
}

func TransformSrcToObject(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath) buildOutput {

	// There's no "obj" crate-type since it doesn't get linked, so don't define one.
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, ""))
}

func TransformSrctoRlib(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, "rlib"))
}

// TransformRlibstoStaticlib is assumed to be callable from the cc module, and
// thus needs to reconstruct the common set of flags which need to be passed
// to the rustc compiler.
func TransformRlibstoStaticlib(ctx android.ModuleContext, mainSrc android.Path, deps []cc.RustRlibDep,
	outputFile android.WritablePath) android.Path {

	var rustPathDeps PathDeps
	var rustFlags Flags
	IsHwasan := false
	for _, rlibDep := range deps {
		if ctx.Os() == android.LinuxMusl && ctx.Config().BuildArch == android.Arm64 && rlibDep.CrateName == "std" {
			continue
		}
		rustPathDeps.RLibs = append(rustPathDeps.RLibs, RustLibrary{Path: rlibDep.LibPath, CrateName: rlibDep.CrateName})
		rustPathDeps.linkDirs = append(rustPathDeps.linkDirs, rlibDep.LinkDirs...)
		IsHwasan = IsHwasan || rlibDep.IsHwasan
		rustPathDeps.linkDirsDeps = append(rustPathDeps.linkDirsDeps, rlibDep.LinkDirsDeps...)
	}
	rustPathDeps.linkDirs = android.FirstUniqueStrings(rustPathDeps.linkDirs)
	rustPathDeps.linkDirsDeps = android.FirstUniquePaths(rustPathDeps.linkDirsDeps)

	mod := ctx.Module().(cc.LinkableInterface)
	toolchain := config.FindToolchain(ctx.Os(), ctx.Arch())
	t := transformProperties{
		// Crate name can be a predefined value as this is a staticlib and
		// it does not need to be unique. The crate name is used for name
		// mangling, but it is mixed with the metadata for that purpose, which we
		// already set to the module name.
		crateName:       "generated_rust_staticlib",
		is64Bit:         toolchain.Is64Bit(),
		targetTriple:    toolchain.RustTriple(),
		bootstrap:       mod.Bootstrap(),
		inRecovery:      mod.InRecovery(),
		inRamdisk:       mod.InRamdisk(),
		inVendorRamdisk: mod.InVendorRamdisk(),
		emitType:        "link",

		// crateType indicates what type of crate to build
		crateType: "staticlib",

		// synthetic indicates whether this is an actual Rust module or not
		synthetic: true,

		// To match the default value of useExpansiveDefaultSrcs() from rust'd baseCompiler.
		// Should switch to false in the future.
		useExpansiveDefaultSrcs: true,
	}

	rustFlags = CommonDefaultFlags(ctx, toolchain, rustFlags)
	rustFlags = CommonLibraryCompilerFlags(ctx, rustFlags)
	if IsHwasan {
		rustFlags.RustFlags = append(rustFlags.RustFlags, "-Zsanitizer=hwaddress")
	}
	rustFlags.GlobalRustFlags = rustFlags.GlobalRustFlags.AppendNoDeps("-C lto=thin")

	rustFlags.EmitXrefs = false

	return transformSrctoCrate(ctx, mainSrc, rustPathDeps, rustFlags, outputFile, nil, t).outputFile
}

func TransformSrctoDylib(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath) buildOutput {
	if ctx.RustModule().compiler.Thinlto() {
		flags.GlobalRustFlags = flags.GlobalRustFlags.AppendNoDeps("-C lto=thin")
	}

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, "dylib"))
}

func TransformSrctoStatic(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath) buildOutput {
	if ctx.RustModule().compiler.Thinlto() {
		flags.GlobalRustFlags = flags.GlobalRustFlags.AppendNoDeps("-C lto=thin")
	}

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, "staticlib"))
}

func TransformSrctoShared(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath) buildOutput {
	if ctx.RustModule().compiler.Thinlto() {
		flags.GlobalRustFlags = flags.GlobalRustFlags.AppendNoDeps("-C lto=thin")
	}

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, "cdylib"))
}

func TransformSrctoProcMacro(ctx ModuleContext, mainSrc android.Path, deps PathDeps,
	flags Flags, outputFile, checkJsonFile android.WritablePath) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, checkJsonFile, getTransformProperties(ctx, "proc-macro"))
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func makeLibFlags(deps PathDeps) ([]string, android.Paths) {
	var libFlags []string
	var depFiles []android.Path

	// Collect library/crate flags
	for _, lib := range deps.RLibs {
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+lib.Path.String())
	}
	for _, lib := range deps.DyLibs {
		libFlags = append(libFlags, "--extern force:"+lib.CrateName+"="+lib.Path.String())
	}
	for _, proc_macro := range deps.ProcMacros {
		libFlags = append(libFlags, "--extern "+proc_macro.CrateName+"="+proc_macro.Path.String())
	}

	for _, path := range deps.linkDirs {
		libFlags = append(libFlags, "-L "+path)
	}
	depFiles = append(depFiles, deps.linkDirsDeps...)

	return libFlags, depFiles
}

func rustStringifyEnvVars(envVars map[string]string) string {
	envVarStrings := []string{}
	for _, key := range android.SortedKeys(envVars) {
		envVarStrings = append(envVarStrings, key+"="+envVars[key])
	}
	return strings.Join(envVarStrings, " ")
}

func rustEnvVars(ctx android.ModuleContext, deps PathDeps, crateName string, cargoOutDir android.OptionalPath) map[string]string {
	envVars := make(map[string]string)

	// libstd requires a specific environment variable to be set. This is
	// not officially documented and may be removed in the future. See
	// https://github.com/rust-lang/rust/blob/master/library/std/src/env.rs#L866.
	if crateName == "std" {
		envVars["STD_ENV_ARCH"] = config.StdEnvArch[ctx.Arch().ArchType]
	}

	if len(deps.SrcDeps) > 0 && cargoOutDir.Valid() {
		moduleGenDir := cargoOutDir
		envVars["SOONG_RUST_GEN_DIR"] = moduleGenDir.String()
	}
	envVars["ANDROID_RUST_VERSION"] = config.GetRustVersion(ctx)

	if rustMod, ok := ctx.Module().(*Module); ok && rustMod.compiler.cargoEnvCompat() {
		// We only emulate cargo environment variables for 3p code, which is only ever built
		// by defining a Rust module, so we only need to set these for true Rust modules.
		if bin, ok := rustMod.compiler.(*binaryDecorator); ok {
			envVars["CARGO_BIN_NAME"] = bin.getStem(ctx)
		}
		envVars["CARGO_CRATE_NAME"] = crateName
		envVars["CARGO_MANIFEST_DIR"] = ctx.ModuleDir()
		envVars["CARGO_PKG_NAME"] = crateName
		pkgVersion := rustMod.compiler.cargoPkgVersion()
		if pkgVersion != "" {
			envVars["CARGO_PKG_VERSION"] = pkgVersion
			// Ensure the version is in the form of "x.y.z" (approximately semver compliant).
			//
			// For our purposes, we don't care to enforce that these are integers since they may
			// include other characters at times (e.g. sometimes the patch version is more than an integer).
			if strings.Count(pkgVersion, ".") == 2 {
				var semver_parts = strings.Split(pkgVersion, ".")
				envVars["CARGO_PKG_VERSION_MAJOR"] = semver_parts[0]
				envVars["CARGO_PKG_VERSION_MINOR"] = semver_parts[1]
				envVars["CARGO_PKG_VERSION_PATCH"] = semver_parts[2]
			}
		}
	}

	if rustModule, ok := ctx.Module().(*Module); ok {
		if _, ok := rustModule.compiler.(*toolchainLibraryDecorator); ok {
			// some toolchain libraries rely on sources which are pregenerated
			// during the toolchain build. This ensures that `include!` macros
			// work.
			// Note this may be overwritten by rustc_wrapper.sh if SOONG_RUST_GEN_DIR
			// is set.
			envVars["OUT_DIR"] = "out/"
		}
	}

	if ctx.Darwin() {
		envVars["ANDROID_RUST_DARWIN"] = "true"
	}

	// TODO: Remove, there are existing users.
	envVars["TARGET_BUILD_VARIANT"] = func() string {
		if ctx.Config().Eng() {
			return "eng"
		} else if ctx.Config().Debuggable() {
			return "userdebug"
		} else {
			return "user"
		}
	}()

	return envVars
}

func transformSrctoCrate(ctx android.ModuleContext, main android.Path, deps PathDeps, flags Flags,
	outputFile, checkJsonFile android.WritablePath, t transformProperties) buildOutput {

	var inputs android.Paths
	var implicits android.Paths
	var validations android.Paths
	var orderOnly android.Paths
	var output buildOutput
	var linkerScriptFlags, rustcFlags, linkFlags []string
	var earlyLinkFlags string

	output.outputFile = outputFile

	envVars := rustEnvVars(ctx, deps, t.crateName, t.cargoOutDir)

	inputs = append(inputs, main)

	// Collect rustc flags
	rustcFlags = append(rustcFlags, flags.GlobalRustFlags.Flags)
	implicits = append(implicits, flags.GlobalRustFlags.Deps...)
	rustcFlags = append(rustcFlags, flags.RustFlags...)
	if t.crateType != "" {
		rustcFlags = append(rustcFlags, "--crate-type="+t.crateType)
	}
	if t.crateName != "" {
		rustcFlags = append(rustcFlags, "--crate-name="+t.crateName)
	}
	if t.targetTriple != "" {
		rustcFlags = append(rustcFlags, "--target="+t.targetTriple)
		linkFlags = append(linkFlags, "-target "+t.targetTriple)
	}

	// Suppress an implicit sysroot
	rustcFlags = append(rustcFlags, "--sysroot=/dev/null")

	// Enable incremental compilation if requested by user
	if ctx.Config().IsEnvTrue("SOONG_RUSTC_INCREMENTAL") {
		incrementalPath := android.PathForOutput(ctx, "rustc").String()

		rustcFlags = append(rustcFlags, "-C incremental="+incrementalPath)
	} else if ctx.Config().Eng() {
		rustcFlags = append(rustcFlags, "-C codegen-units=16")
	} else {
		rustcFlags = append(rustcFlags, "-C codegen-units=1")
	}
	// Disallow experimental features
	modulePath := ctx.ModuleDir()
	if !android.IsThirdPartyPath(modulePath) && !strings.HasPrefix(modulePath, "prebuilts") {
		rustcFlags = append(rustcFlags, "-Zallow-features=\"\"")
	}

	// Collect linker flags
	if !ctx.Darwin() {
		earlyLinkFlags = "-Wl,--as-needed"
	}

	linkFlags = append(linkFlags, flags.GlobalLinkFlags.Flags)
	implicits = append(implicits, flags.GlobalLinkFlags.Deps...)
	linkFlags = append(linkFlags, flags.LinkFlags.Flags)
	implicits = append(implicits, flags.LinkFlags.Deps...)
	linkerScriptFlags = append(linkerScriptFlags, flags.LinkerScriptFlags...)

	// Check if this module needs to use the bootstrap linker
	useBootstrap := t.bootstrap && !t.inRecovery && !t.inRamdisk && !t.inVendorRamdisk
	if ctx.Config().GetBuildFlagBool("RELEASE_DEPRECATE_RUNTIME_APEX") {
		useBootstrap = false
	}

	if useBootstrap {
		dynamicLinker := "-Wl,-dynamic-linker,/system/bin/bootstrap/linker"
		if t.is64Bit {
			dynamicLinker += "64"
		}
		linkFlags = append(linkFlags, dynamicLinker)
	}

	if generatedLib := cc.GenerateRustStaticlib(ctx, deps.ccRlibDeps); generatedLib != nil {
		deps.StaticLibs = append(deps.StaticLibs, generatedLib)
		linkFlags = append(linkFlags, generatedLib.String())
	}

	libFlags, libFlagsDeps := makeLibFlags(deps)

	// Collect dependencies
	orderOnly = append(orderOnly, deps.SharedLibs...)

	srcInputs := android.Paths{}
	srcInputs = append(srcInputs, main)
	srcInputs = append(srcInputs, deps.srcProviderFiles...)
	srcInputs = append(srcInputs, deps.SrcFiles...)
	// AFDO profiles are included in Rusts deps file, so include these here
	// so they're not missing when comparing inputs.
	srcInputs = append(srcInputs, deps.AfdoProfiles...)

	if !t.synthetic {
		// Only worry about OUT_DIR for actual Rust modules.
		// Libraries built from cc use generated source, and do not utilize OUT_DIR.
		if len(deps.SrcDeps) > 0 {
			var outputs android.WritablePaths

			for _, genSrc := range deps.SrcDeps {
				if android.SuffixInList(outputs.Strings(), genSubDir+genSrc.Base()) {
					ctx.PropertyErrorf("srcs",
						"multiple source providers generate the same filename output: "+genSrc.Base())
				}
				copiedFile := android.PathForModuleOut(ctx, genSubDir+genSrc.Base())
				outputs = append(outputs, copiedFile)
				srcInputs = append(srcInputs, copiedFile)
			}

			ctx.Build(pctx, android.BuildParams{
				Rule:        cp,
				Description: "cp " + t.cargoOutDir.Path().Rel(),
				Outputs:     outputs,
				Inputs:      deps.SrcDeps,
				Args: map[string]string{
					"outDir": t.cargoOutDir.String(),
				},
			})
		}
	}
	var implicitOutputs android.WritablePaths

	if ctx.Windows() && (t.crateType == "cdylib" || t.crateType == "dylib") {
		// On windows, an additional .lib file is produced for dynamic linkages.
		importLibraryPath := android.PathForModuleOut(ctx, outputFile.ReplaceExtension(ctx, "lib").Base())
		linkFlags = append(linkFlags, "-Wl,--out-implib="+importLibraryPath.String())
		implicitOutputs = append(implicitOutputs, importLibraryPath)

		// On Windows, we always generate a PDB file
		// --strip-debug is needed to also keep COFF symbols which are needed when
		// we patch binaries with symbol_inject.
		pdb := outputFile.ReplaceExtension(ctx, "pdb")
		linkFlags = append(linkFlags, " -Wl,--strip-debug -Wl,--pdb="+pdb.String()+" ")
		implicitOutputs = append(implicitOutputs, pdb)
	}

	implicits = append(implicits, rustLibsToPaths(deps.RLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.DyLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.ProcMacros)...)
	implicits = append(implicits, deps.StaticLibs...)
	implicits = append(implicits, deps.SharedLibDeps...)
	implicits = append(implicits, deps.LinkerDeps...)
	implicits = append(implicits, deps.CrtBegin...)
	implicits = append(implicits, deps.CrtEnd...)
	implicits = append(implicits, srcInputs...)
	implicits = append(implicits, libFlagsDeps...)

	toolchainImplicitsPhony := toolchainImplicitsPhony(ctx)
	implicits = append(implicits, toolchainImplicitsPhony)

	soongDepsFile := android.PathForModuleOut(ctx, outputFile.Base()+".soong_deps")
	android.WriteFileRule(ctx, soongDepsFile, strings.Join(srcInputs.Strings(), "\n"))
	implicits = append(implicits, soongDepsFile)

	if !t.synthetic {
		// Only worry about clippy for actual Rust modules.
		// Libraries built from cc use generated source, and don't need to run clippy.
		args := map[string]string{
			"rustcFlags":    strings.Join(rustcFlags, " "),
			"libFlags":      strings.Join(libFlags, " "),
			"clippyFlags":   strings.Join(flags.ClippyFlags, " "),
			"envVars":       rustStringifyEnvVars(envVars),
			"soongSrcsFile": soongDepsFile.String(),
		}
		if flags.Clippy {
			ctx.Build(pctx, android.BuildParams{
				Rule:        clippyJsonDriver,
				Description: "clippy check json " + main.Rel(),
				Output:      checkJsonFile,
				Inputs:      inputs,
				Implicits:   implicits,
				Args:        args,
			})
			ctx.Phony("rustClippyJson", checkJsonFile)
			clippyFile := android.PathForModuleOut(ctx, outputFile.Base()+".clippy")
			ctx.Build(pctx, android.BuildParams{
				Rule:            clippyDriver,
				Description:     "clippy " + main.Rel(),
				Output:          clippyFile,
				ImplicitOutputs: nil,
				Inputs:          inputs,
				Implicits:       implicits,
				OrderOnly:       orderOnly,
				Args:            args,
			})
			// Declare the clippy build as a validation dependency of the original crate.  It will
			// be run in any build that builds the crate, but not block building the crate or anything
			// that depends on the crate.
			validations = append(validations, clippyFile)
		}
	}

	rule := rustc
	args := map[string]string{
		"rustcFlags":        strings.Join(rustcFlags, " "),
		"linkerScriptFlags": strings.Join(linkerScriptFlags, " "),
		"earlyLinkFlags":    earlyLinkFlags,
		"linkFlags":         strings.Join(linkFlags, " "),
		"libFlags":          strings.Join(libFlags, " "),
		"crtBegin":          strings.Join(deps.CrtBegin.Strings(), " "),
		"crtEnd":            strings.Join(deps.CrtEnd.Strings(), " "),
		"envVars":           rustStringifyEnvVars(envVars),
		"emitType":          t.emitType,
		"soongSrcsFile":     soongDepsFile.String(),
	}

	// If SrcFiles populating is ever tied to some other property being set
	// (e.g. crate_root), a check against whether its populated should be added here.
	if ctx.Config().UseREWrapper() && ctx.Config().IsEnvTrue("RBE_RUST") {
		rule = rustcRbe
		rbeInputs := android.Paths{}
		rbeInputs = append(rbeInputs, implicits...)
		rbeInputs = append(rbeInputs, depset.New(depset.PREORDER, deps.directApexImplementationDeps, deps.transitiveApexImplementationDeps).ToList()...)
		rbeInputs = append(rbeInputs, depset.New(depset.PREORDER, deps.directNonApexImplementationDeps, deps.transitiveNonApexImplementationDeps).ToList()...)
		rbeInputs = android.FirstUniquePaths(rbeInputs)
		// Produce an rsp file for RBE as the inputs list can easily grow too large.
		rbeRustRspFile := android.PathForModuleOut(ctx, "", outputFile.Base()+".rbe.rsp")
		android.WriteFileRule(ctx, rbeRustRspFile, strings.Join(rbeInputs.Strings(), "\n"))
		implicits = append(implicits, rbeRustRspFile)

		args["rbeRspFile"] = rbeRustRspFile.String()
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            rule,
		Description:     "rustc " + main.Rel(),
		Output:          outputFile,
		Inputs:          inputs,
		Implicits:       implicits,
		ImplicitOutputs: implicitOutputs,
		OrderOnly:       orderOnly,
		Validations:     validations,
		Args:            args,
	})

	return output
}

func Rustdoc(ctx ModuleContext, main android.Path, deps PathDeps,
	flags Flags) android.ModuleOutPath {

	var implicits android.Paths

	rustdocFlags := slices.Clone(flags.RustdocFlags)
	rustdocFlags = append(rustdocFlags, flags.GlobalRustFlags.Flags)
	implicits = append(implicits, flags.GlobalRustFlags.Deps...)
	rustdocFlags = append(rustdocFlags, "--sysroot=/dev/null")

	// Build an index for all our crates. -Z unstable options is required to use
	// this flag.
	rustdocFlags = append(rustdocFlags, "-Z", "unstable-options", "--enable-index-page")

	// Ensure we use any special-case code-paths for Soong.
	rustdocFlags = append(rustdocFlags, "--cfg", "soong")

	targetTriple := ctx.toolchain().RustTriple()

	// Collect rustc flags
	if targetTriple != "" {
		rustdocFlags = append(rustdocFlags, "--target="+targetTriple)
	}

	crateName := ctx.RustModule().CrateName()
	rustdocFlags = append(rustdocFlags, "--crate-name "+crateName)

	libFlags, libFlagsDeps := makeLibFlags(deps)
	rustdocFlags = append(rustdocFlags, libFlags...)
	docTimestampFile := android.PathForModuleOut(ctx, "rustdoc.timestamp")

	// Silence warnings about renamed lints for third-party crates
	modulePath := ctx.ModuleDir()
	if android.IsThirdPartyPath(modulePath) {
		rustdocFlags = append(rustdocFlags, " --cap-lints allow")
	}

	// Yes, the same out directory is used simultaneously by all rustdoc builds.
	// This is what cargo does. The docs for individual crates get generated to
	// a subdirectory named for the crate, and rustdoc synchronizes writes to
	// shared pieces like the index and search data itself.
	// https://github.com/rust-lang/rust/blob/master/src/librustdoc/html/render/write_shared.rs#L144-L146
	docDir := android.PathForOutput(ctx, "rustdoc")

	toolchainImplicitsPhony := toolchainImplicitsPhony(ctx)
	implicits = append(implicits, toolchainImplicitsPhony)

	implicits = append(implicits, rustLibsToPaths(deps.RLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.DyLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.ProcMacros)...)
	implicits = append(implicits, libFlagsDeps...)
	implicits = append(implicits, deps.SrcFiles...)
	envVars := rustEnvVars(ctx, deps, crateName, ctx.RustModule().compiler.cargoOutDir(ctx))
	ctx.Build(pctx, android.BuildParams{
		Rule:        rustdoc,
		Description: "rustdoc " + main.Rel(),
		Output:      docTimestampFile,
		Input:       main,
		Implicit:    ctx.RustModule().UnstrippedOutputFile(),
		Implicits:   implicits,
		Args: map[string]string{
			"rustdocFlags": strings.Join(rustdocFlags, " "),
			"outDir":       docDir.String(),
			"envVars":      rustStringifyEnvVars(envVars),
		},
	})

	return docTimestampFile
}

func toolchainImplicitsPhony(ctx android.ModuleContext) android.Path {
	rustPath := config.RustPath(ctx)
	clangPath := cc_config.ClangPath(ctx, "").String()

	return ctx.CreateNinjaPhonyOnce("rustToolchainImplicits", []string{
		filepath.Join(rustPath, "lib/librustc_driver-*.so"),
		filepath.Join(rustPath, "lib/rustlib/*/lib/**/*"),
		filepath.Join(rustPath, "lib/libLLVM*"),
		filepath.Join(rustPath, "lib64/*.so"),
		filepath.Join(clangPath, "lib/clang/*/include/**/*"),
		filepath.Join(clangPath, "lib/clang/*/lib", runtime.GOOS, "**/*"),
		filepath.Join(clangPath, "lib/libxml2.so*"),
		filepath.Join(clangPath, "bin/**/*"),
	})
}
