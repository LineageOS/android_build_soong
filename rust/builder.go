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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/rust/config"
)

var (
	zip = pctx.AndroidStaticRule("zip",
		blueprint.RuleParams{
			Command:        "cat $out.rsp | tr ' ' '\\n' | tr -d \\' | sort -u > ${out}.tmp && ${SoongZipCmd} -o ${out} -C $$OUT_DIR -l ${out}.tmp",
			CommandDeps:    []string{"${SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		})

	cpDir = pctx.AndroidStaticRule("cpDir",
		blueprint.RuleParams{
			Command:        "cp `cat $outDir.rsp` $outDir",
			Rspfile:        "${outDir}.rsp",
			RspfileContent: "$in",
		},
		"outDir")

	cp = pctx.AndroidStaticRule("cp",
		blueprint.RuleParams{
			Command:     "rm -f $out && cp $in $out",
			Description: "cp $out",
		})

	// Cross-referencing:
	_ = pctx.SourcePathVariable("rustExtractor",
		"prebuilts/build-tools/${config.HostPrebuiltTag}/bin/rust_extractor")
	_ = pctx.VariableFunc("kytheCorpus",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCorpusName() })
	_ = pctx.VariableFunc("kytheCuEncoding",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCuEncoding() })
)

type buildOutput struct {
	outputFile android.Path
	kytheFile  android.Path
}

func init() {
	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
}

func TransformSrcToBinary(ctx ModuleContext, c compiler, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")

	return transformSrctoCrate(ctx, c, mainSrc, deps, flags, outputFile, "bin")
}

func TransformSrctoRlib(ctx ModuleContext, c compiler, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	return transformSrctoCrate(ctx, c, mainSrc, deps, flags, outputFile, "rlib")
}

func TransformSrctoDylib(ctx ModuleContext, c compiler, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")

	return transformSrctoCrate(ctx, c, mainSrc, deps, flags, outputFile, "dylib")
}

func TransformSrctoStatic(ctx ModuleContext, c compiler, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")
	return transformSrctoCrate(ctx, c, mainSrc, deps, flags, outputFile, "staticlib")
}

func TransformSrctoShared(ctx ModuleContext, c compiler, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")
	return transformSrctoCrate(ctx, c, mainSrc, deps, flags, outputFile, "cdylib")
}

func TransformSrctoProcMacro(ctx ModuleContext, c compiler, mainSrc android.Path, deps PathDeps,
	flags Flags, outputFile android.WritablePath) buildOutput {
	return transformSrctoCrate(ctx, c, mainSrc, deps, flags, outputFile, "proc-macro")
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func makeLibFlags(deps PathDeps, ruleCmd *android.RuleBuilderCommand) []string {
	var libFlags []string

	// Collect library/crate flags
	for _, lib := range deps.Rlibs.ToListDirect() {
		libPath := ruleCmd.PathForInput(lib.Path)
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+libPath)
	}
	for _, lib := range deps.Dylibs.ToListDirect() {
		libPath := ruleCmd.PathForInput(lib.Path)
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+libPath)
	}
	for _, procMacro := range deps.ProcMacros.ToListDirect() {
		procMacroPath := ruleCmd.PathForInput(procMacro.Path)
		libFlags = append(libFlags, "--extern "+procMacro.CrateName+"="+procMacroPath)
	}

	for _, path := range deps.linkDirs {
		libFlags = append(libFlags, "-L "+ruleCmd.PathForInput(path))
	}

	return libFlags
}

func collectImplicits(deps PathDeps) android.Paths {
	depPaths := android.Paths{}
	depPaths = append(depPaths, rustLibsToPaths(deps.Rlibs.ToList())...)
	depPaths = append(depPaths, rustLibsToPaths(deps.Dylibs.ToList())...)
	depPaths = append(depPaths, rustLibsToPaths(deps.ProcMacros.ToList())...)
	depPaths = append(depPaths, deps.AfdoProfiles...)
	depPaths = append(depPaths, deps.WholeStaticLibs...)
	depPaths = append(depPaths, deps.SrcDeps...)
	depPaths = append(depPaths, deps.srcProviderFiles...)
	depPaths = append(depPaths, deps.LibDeps...)
	depPaths = append(depPaths, deps.linkObjects...)
	depPaths = append(depPaths, deps.BuildToolSrcDeps...)
	return depPaths
}

func rustEnvVars(ctx ModuleContext, deps PathDeps, cmd *android.RuleBuilderCommand) []string {
	var envVars []string

	// libstd requires a specific environment variable to be set. This is
	// not officially documented and may be removed in the future. See
	// https://github.com/rust-lang/rust/blob/master/library/std/src/env.rs#L866.
	if ctx.RustModule().CrateName() == "std" {
		envVars = append(envVars, "STD_ENV_ARCH="+config.StdEnvArch[ctx.RustModule().Arch().ArchType])
	}

	if len(deps.SrcDeps) > 0 {
		moduleGenDir := ctx.RustModule().compiler.CargoOutDir()
		// We must calculate an absolute path for OUT_DIR since Rust's include! macro (which normally consumes this)
		// assumes that paths are relative to the source file.
		var outDir string
		if filepath.IsAbs(moduleGenDir.String()) {
			// If OUT_DIR is absolute, then moduleGenDir will be an absolute path, so we don't need to set this to anything.
			outDir = moduleGenDir.String()
		} else if moduleGenDir.Valid() {
			// If OUT_DIR is not absolute, we use $$PWD to generate an absolute path (os.Getwd() returns '/')
			outDir = filepath.Join("$$PWD/", cmd.PathForInput(moduleGenDir.Path()))
		} else {
			outDir = "$$PWD/"
		}
		envVars = append(envVars, "OUT_DIR="+outDir)
	} else {
		// TODO(pcc): Change this to "OUT_DIR=" after fixing crates to not rely on this value.
		envVars = append(envVars, "OUT_DIR=out")
	}

	envVars = append(envVars, "ANDROID_RUST_VERSION="+config.GetRustVersion(ctx))

	if ctx.RustModule().compiler.CargoEnvCompat() {
		if bin, ok := ctx.RustModule().compiler.(*binaryDecorator); ok {
			envVars = append(envVars, "CARGO_BIN_NAME="+bin.getStem(ctx))
		}
		envVars = append(envVars, "CARGO_CRATE_NAME="+ctx.RustModule().CrateName())
		envVars = append(envVars, "CARGO_PKG_NAME="+ctx.RustModule().CrateName())
		pkgVersion := ctx.RustModule().compiler.CargoPkgVersion()
		if pkgVersion != "" {
			envVars = append(envVars, "CARGO_PKG_VERSION="+pkgVersion)

			// Ensure the version is in the form of "x.y.z" (approximately semver compliant).
			//
			// For our purposes, we don't care to enforce that these are integers since they may
			// include other characters at times (e.g. sometimes the patch version is more than an integer).
			if strings.Count(pkgVersion, ".") == 2 {
				var semver_parts = strings.Split(pkgVersion, ".")
				envVars = append(envVars, "CARGO_PKG_VERSION_MAJOR="+semver_parts[0])
				envVars = append(envVars, "CARGO_PKG_VERSION_MINOR="+semver_parts[1])
				envVars = append(envVars, "CARGO_PKG_VERSION_PATCH="+semver_parts[2])
			}
		}
	}

	envVars = append(envVars, "AR="+cmd.PathForTool(deps.Llvm_ar))

	if ctx.Darwin() {
		envVars = append(envVars, "ANDROID_RUST_DARWIN=true")
	}

	return envVars
}

func transformSrctoCrate(ctx ModuleContext, comp compiler, main android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, crateType string) buildOutput {

	var inputs android.Paths
	var output buildOutput
	var rustcFlags, linkFlags []string
	var earlyLinkFlags []string

	output.outputFile = outputFile
	crateName := ctx.RustModule().CrateName()
	targetTriple := ctx.toolchain().RustTriple()

	inputs = append(inputs, main)

	// Collect rustc flags
	rustcFlags = append(rustcFlags, flags.GlobalRustFlags...)
	rustcFlags = append(rustcFlags, flags.RustFlags...)
	rustcFlags = append(rustcFlags, "--crate-type="+crateType)
	if crateName != "" {
		rustcFlags = append(rustcFlags, "--crate-name="+crateName)
	}
	if targetTriple != "" {
		rustcFlags = append(rustcFlags, "--target="+targetTriple)
		linkFlags = append(linkFlags, "-target "+targetTriple)
	}

	// Suppress an implicit sysroot
	rustcFlags = append(rustcFlags, "--sysroot=/dev/null")

	// Enable incremental compilation if requested by user
	if ctx.Config().IsEnvTrue("SOONG_RUSTC_INCREMENTAL") {
		incrementalPath := android.PathForOutput(ctx, "rustc").String()
		rustcFlags = append(rustcFlags, "-Cincremental="+incrementalPath)
	}

	// Disallow experimental features
	modulePath := android.PathForModuleSrc(ctx).String()
	if !(android.IsThirdPartyPath(modulePath) || strings.HasPrefix(modulePath, "prebuilts")) {
		rustcFlags = append(rustcFlags, "-Zallow-features=\"\"")
	}

	// Collect linker flags
	if !ctx.Darwin() {
		earlyLinkFlags = append(earlyLinkFlags, "-Wl,--as-needed")
	}

	// Collect dependencies
	var linkImplicits android.Paths
	implicits := collectImplicits(deps)
	toolImplicits := android.Concat(deps.BuildToolDeps)
	linkImplicits = append(linkImplicits, deps.CrtBegin...)
	linkImplicits = append(linkImplicits, deps.CrtEnd...)
	implicits = append(implicits, comp.compilationSourcesAndData(ctx)...)

	if len(deps.SrcDeps) > 0 {
		moduleGenDir := ctx.RustModule().compiler.CargoOutDir()
		var outputs android.WritablePaths

		for _, genSrc := range deps.SrcDeps {
			if android.SuffixInList(outputs.Strings(), genSubDir+genSrc.Base()) {
				ctx.PropertyErrorf("srcs",
					"multiple source providers generate the same filename output: "+genSrc.Base())
			}
			outputs = append(outputs, android.PathForModuleOut(ctx, genSubDir+genSrc.Base()))
		}

		ctx.Build(pctx, android.BuildParams{
			Rule:        cpDir,
			Description: "cp " + moduleGenDir.Path().Rel(),
			Outputs:     outputs,
			Inputs:      deps.SrcDeps,
			Args: map[string]string{
				"outDir": moduleGenDir.String(),
			},
		})
		implicits = append(implicits, outputs.Paths()...)
	}

	if flags.Clippy {
		// TODO(b/298461712) remove this hack to let slim manifest branches build
		if deps.Clippy_driver == nil {
			deps.Clippy_driver = config.RustPath(ctx, "bin/clippy-driver")
		}

		clippyRule := getRuleBuilder(ctx, pctx, false, "clippy")
		clippyCmd := clippyRule.Command()
		clippyFile := android.PathForModuleOut(ctx, outputFile.Base()+".clippy")
		clippyDepInfoFile := android.PathForModuleOut(ctx, outputFile.Base()+".clippy.d.raw")
		clippyDepFile := android.PathForModuleOut(ctx, outputFile.Base()+".clippy.d")

		clippyCmd.
			Flags(rustEnvVars(ctx, deps, clippyCmd)).
			Tool(deps.Clippy_driver).
			Flag("--emit metadata").
			FlagWithOutput("-o ", clippyFile).
			FlagWithOutput("--emit dep-info=", clippyDepInfoFile).
			Inputs(inputs).
			Flags(makeLibFlags(deps, clippyCmd)).
			Flags(rustcFlags).
			Flags(flags.ClippyFlags).
			ImplicitTools(toolImplicits).
			Implicits(implicits)

		depfileCreationCmd := clippyRule.Command()
		depfileCreationCmd.
			Flag(fmt.Sprintf(
				`grep "^%s:" %s >`,
				depfileCreationCmd.PathForOutput(clippyFile),
				depfileCreationCmd.PathForOutput(clippyDepInfoFile),
			)).
			DepFile(clippyDepFile)

		clippyRule.BuildWithUnescapedNinjaVars("clippy", "clippy "+main.Rel())

		// Declare the clippy build as an implicit dependency of the original crate.
		implicits = append(implicits, clippyFile)
	}

	sboxDirectory := "rustc"
	rustSboxOutputFile := android.PathForModuleOut(ctx, sboxDirectory, outputFile.Base())
	depFile := android.PathForModuleOut(ctx, sboxDirectory, rustSboxOutputFile.Base()+".d")
	depInfoFile := android.PathForModuleOut(ctx, sboxDirectory, rustSboxOutputFile.Base()+".d.raw")
	var rustcImplicitOutputs android.WritablePaths

	sandboxedCompilation := comp.crateRoot(ctx) != nil
	rustcRule := getRuleBuilder(ctx, pctx, sandboxedCompilation, sboxDirectory)
	rustcCmd := rustcRule.Command()

	linkFlags = append(linkFlags, flags.GlobalLinkFlags...)
	linkFlags = append(linkFlags, flags.LinkFlags...)
	linkFlags = append(linkFlags, rustcCmd.PathsForInputs(deps.linkObjects)...)

	// Check if this module needs to use the bootstrap linker
	if ctx.RustModule().Bootstrap() && !ctx.RustModule().InRecovery() && !ctx.RustModule().InRamdisk() && !ctx.RustModule().InVendorRamdisk() {
		dynamicLinker := "-Wl,-dynamic-linker,/system/bin/bootstrap/linker"
		if ctx.toolchain().Is64Bit() {
			dynamicLinker += "64"
		}
		linkFlags = append(linkFlags, dynamicLinker)
	}

	libFlags := makeLibFlags(deps, rustcCmd)

	usesLinker := crateType == "bin" || crateType == "dylib" || crateType == "cdylib" || crateType == "proc-macro"
	if usesLinker {
		rustSboxOutputFile = android.PathForModuleOut(ctx, sboxDirectory, rustSboxOutputFile.Base()+".rsp")
		rustcImplicitOutputs = android.WritablePaths{
			android.PathForModuleOut(ctx, sboxDirectory, rustSboxOutputFile.Base()+".whole.a"),
			android.PathForModuleOut(ctx, sboxDirectory, rustSboxOutputFile.Base()+".a"),
		}
	}

	// TODO(b/298461712) remove this hack to let slim manifest branches build
	if deps.Rustc == nil {
		deps.Rustc = config.RustPath(ctx, "bin/rustc")
	}

	rustcCmd.
		Flags(rustEnvVars(ctx, deps, rustcCmd)).
		Tool(deps.Rustc).
		FlagWithInput("-C linker=", android.PathForSource(ctx, "build", "soong", "scripts", "mkcratersp.py")).
		Flag("--emit link").
		Flag("-o").
		Output(rustSboxOutputFile).
		FlagWithOutput("--emit dep-info=", depInfoFile).
		Inputs(inputs).
		Flags(libFlags).
		ImplicitTools(toolImplicits).
		Implicits(implicits).
		Flags(rustcFlags).
		ImplicitOutputs(rustcImplicitOutputs)

	depfileCreationCmd := rustcRule.Command()
	depfileCreationCmd.
		Flag(fmt.Sprintf(
			`grep "^%s:" %s >`,
			depfileCreationCmd.PathForOutput(rustSboxOutputFile),
			depfileCreationCmd.PathForOutput(depInfoFile),
		)).
		DepFile(depFile)

	if !usesLinker {
		ctx.Build(pctx, android.BuildParams{
			Rule:   cp,
			Input:  rustSboxOutputFile,
			Output: outputFile,
		})
	} else {
		// TODO: delmerico - separate rustLink into its own rule
		// mkcratersp.py hardcodes paths to files within the sandbox, so
		// those need to be renamed/symlinked to something in the rustLink sandbox
		// if we want to separate the rules
		linkerSboxOutputFile := android.PathForModuleOut(ctx, sboxDirectory, outputFile.Base())
		rustLinkCmd := rustcRule.Command()
		rustLinkCmd.
			Tool(deps.Clang).
			Flag("-o").
			Output(linkerSboxOutputFile).
			Inputs(deps.CrtBegin).
			Flags(earlyLinkFlags).
			FlagWithInput("@", rustSboxOutputFile).
			Flags(linkFlags).
			Inputs(deps.CrtEnd).
			ImplicitTools(toolImplicits).
			Implicits(rustcImplicitOutputs.Paths()).
			Implicits(implicits).
			Implicits(linkImplicits)
		ctx.Build(pctx, android.BuildParams{
			Rule:   cp,
			Input:  linkerSboxOutputFile,
			Output: outputFile,
		})
	}

	rustcRule.BuildWithUnescapedNinjaVars("rustc", "rustc "+main.Rel())

	if flags.EmitXrefs {
		kytheRule := getRuleBuilder(ctx, pctx, false, "kythe")
		kytheCmd := kytheRule.Command()
		kytheFile := android.PathForModuleOut(ctx, outputFile.Base()+".kzip")
		kytheCmd.
			Flag("KYTHE_CORPUS=${kytheCorpus}").
			FlagWithOutput("KYTHE_OUTPUT_FILE=", kytheFile).
			FlagWithInput("KYTHE_VNAMES=", android.PathForSource(ctx, "build", "soong", "vnames.json")).
			Flag("KYTHE_KZIP_ENCODING=${kytheCuEncoding}").
			Flag("KYTHE_CANONICALIZE_VNAME_PATHS=prefer-relative").
			Tool(ctx.Config().PrebuiltBuildTool(ctx, "rust_extractor")).
			Flags(rustEnvVars(ctx, deps, kytheCmd)).
			Tool(deps.Rustc).
			Flag("-C linker=true").
			Inputs(inputs).
			Flags(makeLibFlags(deps, kytheCmd)).
			Flags(rustcFlags).
			ImplicitTools(toolImplicits).
			Implicits(implicits)
		kytheRule.BuildWithUnescapedNinjaVars("kythe", "Xref Rust extractor "+main.Rel())
		output.kytheFile = kytheFile
	}
	return output
}

func Rustdoc(ctx ModuleContext, main android.Path, deps PathDeps, flags Flags) android.ModuleOutPath {
	// TODO(b/298461712) remove this hack to let slim manifest branches build
	if deps.Rustdoc == nil {
		deps.Rustdoc = config.RustPath(ctx, "bin/rustdoc")
	}

	rustdocRule := getRuleBuilder(ctx, pctx, false, "rustdoc")
	rustdocCmd := rustdocRule.Command()

	rustdocFlags := append([]string{}, flags.RustdocFlags...)
	rustdocFlags = append(rustdocFlags, "--sysroot=/dev/null")

	// Build an index for all our crates. -Z unstable options is required to use
	// this flag.
	rustdocFlags = append(rustdocFlags, "-Z", "unstable-options", "--enable-index-page")

	targetTriple := ctx.toolchain().RustTriple()

	// Collect rustc flags
	if targetTriple != "" {
		rustdocFlags = append(rustdocFlags, "--target="+targetTriple)
	}

	crateName := ctx.RustModule().CrateName()
	rustdocFlags = append(rustdocFlags, "--crate-name "+crateName)

	rustdocFlags = append(rustdocFlags, makeLibFlags(deps, rustdocCmd)...)
	docTimestampFile := android.PathForModuleOut(ctx, "rustdoc.timestamp")

	// Silence warnings about renamed lints for third-party crates
	modulePath := android.PathForModuleSrc(ctx).String()
	if android.IsThirdPartyPath(modulePath) {
		rustdocFlags = append(rustdocFlags, " -A warnings")
	}

	// Yes, the same out directory is used simultaneously by all rustdoc builds.
	// This is what cargo does. The docs for individual crates get generated to
	// a subdirectory named for the crate, and rustdoc synchronizes writes to
	// shared pieces like the index and search data itself.
	// https://github.com/rust-lang/rust/blob/master/src/librustdoc/html/render/write_shared.rs#L144-L146
	docDir := android.PathForOutput(ctx, "rustdoc")

	rustdocCmd.
		Flags(rustEnvVars(ctx, deps, rustdocCmd)).
		Tool(deps.Rustdoc).
		Flags(rustdocFlags).
		Input(main).
		Flag("-o "+docDir.String()).
		FlagWithOutput("&& touch ", docTimestampFile).
		Implicit(ctx.RustModule().UnstrippedOutputFile())

	rustdocRule.BuildWithUnescapedNinjaVars("rustdoc", "rustdoc "+main.Rel())
	return docTimestampFile
}

func getRuleBuilder(ctx android.ModuleContext, pctx android.PackageContext, sbox bool, sboxDirectory string) *android.RuleBuilder {
	r := android.NewRuleBuilder(pctx, ctx)
	if sbox {
		r = r.Sbox(
			android.PathForModuleOut(ctx, sboxDirectory),
			android.PathForModuleOut(ctx, sboxDirectory+".sbox.textproto"),
		).SandboxInputs()
	}
	return r
}
