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
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/rust/config"
)

var (
	_     = pctx.SourcePathVariable("rustcCmd", "${config.RustBin}/rustc")
	rustc = pctx.AndroidStaticRule("rustc",
		blueprint.RuleParams{
			Command: "$envVars $rustcCmd " +
				"-C linker=${config.RustLinker} " +
				"-C link-args=\"${crtBegin} ${earlyLinkFlags} ${linkFlags} ${crtEnd}\" " +
				"--emit link -o $out --emit dep-info=$out.d.raw $in ${libFlags} $rustcFlags" +
				" && grep \"^$out:\" $out.d.raw > $out.d",
			CommandDeps: []string{"$rustcCmd"},
			// Rustc deps-info writes out make compatible dep files: https://github.com/rust-lang/rust/issues/7633
			// Rustc emits unneeded dependency lines for the .d and input .rs files.
			// Those extra lines cause ninja warning:
			//     "warning: depfile has multiple output paths"
			// For ninja, we keep/grep only the dependency rule for the rust $out file.
			Deps:    blueprint.DepsGCC,
			Depfile: "$out.d",
		},
		"rustcFlags", "earlyLinkFlags", "linkFlags", "libFlags", "crtBegin", "crtEnd", "envVars")

	_       = pctx.SourcePathVariable("rustdocCmd", "${config.RustBin}/rustdoc")
	rustdoc = pctx.AndroidStaticRule("rustdoc",
		blueprint.RuleParams{
			Command: "$envVars $rustdocCmd $rustdocFlags $in -o $outDir && " +
				"touch $out",
			CommandDeps: []string{"$rustdocCmd"},
		},
		"rustdocFlags", "outDir", "envVars")

	_            = pctx.SourcePathVariable("clippyCmd", "${config.RustBin}/clippy-driver")
	clippyDriver = pctx.AndroidStaticRule("clippy",
		blueprint.RuleParams{
			Command: "$envVars $clippyCmd " +
				// Because clippy-driver uses rustc as backend, we need to have some output even during the linting.
				// Use the metadata output as it has the smallest footprint.
				"--emit metadata -o $out --emit dep-info=$out.d.raw $in ${libFlags} " +
				"$rustcFlags $clippyFlags" +
				" && grep \"^$out:\" $out.d.raw > $out.d",
			CommandDeps: []string{"$clippyCmd"},
			Deps:        blueprint.DepsGCC,
			Depfile:     "$out.d",
		},
		"rustcFlags", "libFlags", "clippyFlags", "envVars")

	zip = pctx.AndroidStaticRule("zip",
		blueprint.RuleParams{
			Command:        "cat $out.rsp | tr ' ' '\\n' | tr -d \\' | sort -u > ${out}.tmp && ${SoongZipCmd} -o ${out} -C $$OUT_DIR -l ${out}.tmp",
			CommandDeps:    []string{"${SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		})

	cp = pctx.AndroidStaticRule("cp",
		blueprint.RuleParams{
			Command:        "cp `cat $outDir.rsp` $outDir",
			Rspfile:        "${outDir}.rsp",
			RspfileContent: "$in",
		},
		"outDir")

	// Cross-referencing:
	_ = pctx.SourcePathVariable("rustExtractor",
		"prebuilts/build-tools/${config.HostPrebuiltTag}/bin/rust_extractor")
	_ = pctx.VariableFunc("kytheCorpus",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCorpusName() })
	_ = pctx.VariableFunc("kytheCuEncoding",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCuEncoding() })
	_            = pctx.SourcePathVariable("kytheVnames", "build/soong/vnames.json")
	kytheExtract = pctx.AndroidStaticRule("kythe",
		blueprint.RuleParams{
			Command: `KYTHE_CORPUS=${kytheCorpus} ` +
				`KYTHE_OUTPUT_FILE=$out ` +
				`KYTHE_VNAMES=$kytheVnames ` +
				`KYTHE_KZIP_ENCODING=${kytheCuEncoding} ` +
				`KYTHE_CANONICALIZE_VNAME_PATHS=prefer-relative ` +
				`$rustExtractor $envVars ` +
				`$rustcCmd ` +
				`-C linker=${config.RustLinker} ` +
				`-C link-args="${crtBegin} ${linkFlags} ${crtEnd}" ` +
				`$in ${libFlags} $rustcFlags`,
			CommandDeps:    []string{"$rustExtractor", "$kytheVnames"},
			Rspfile:        "${out}.rsp",
			RspfileContent: "$in",
		},
		"rustcFlags", "linkFlags", "libFlags", "crtBegin", "crtEnd", "envVars")
)

type buildOutput struct {
	outputFile android.Path
	kytheFile  android.Path
}

func init() {
	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
}

func TransformSrcToBinary(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "bin")
}

func TransformSrctoRlib(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "rlib")
}

func TransformSrctoDylib(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "dylib")
}

func TransformSrctoStatic(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "staticlib")
}

func TransformSrctoShared(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto=thin")
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "cdylib")
}

func TransformSrctoProcMacro(ctx ModuleContext, mainSrc android.Path, deps PathDeps,
	flags Flags, outputFile android.WritablePath) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "proc-macro")
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func makeLibFlags(deps PathDeps) []string {
	var libFlags []string

	// Collect library/crate flags
	for _, lib := range deps.RLibs {
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+lib.Path.String())
	}
	for _, lib := range deps.DyLibs {
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+lib.Path.String())
	}
	for _, proc_macro := range deps.ProcMacros {
		libFlags = append(libFlags, "--extern "+proc_macro.CrateName+"="+proc_macro.Path.String())
	}

	for _, path := range deps.linkDirs {
		libFlags = append(libFlags, "-L "+path)
	}

	return libFlags
}

func rustEnvVars(ctx ModuleContext, deps PathDeps) []string {
	var envVars []string

	// libstd requires a specific environment variable to be set. This is
	// not officially documented and may be removed in the future. See
	// https://github.com/rust-lang/rust/blob/master/library/std/src/env.rs#L866.
	if ctx.RustModule().CrateName() == "std" {
		envVars = append(envVars, "STD_ENV_ARCH="+config.StdEnvArch[ctx.RustModule().Arch().ArchType])
	}

	if len(deps.SrcDeps) > 0 {
		moduleGenDir := ctx.RustModule().compiler.cargoOutDir()
		// We must calculate an absolute path for OUT_DIR since Rust's include! macro (which normally consumes this)
		// assumes that paths are relative to the source file.
		var outDirPrefix string
		if !filepath.IsAbs(moduleGenDir.String()) {
			// If OUT_DIR is not absolute, we use $$PWD to generate an absolute path (os.Getwd() returns '/')
			outDirPrefix = "$$PWD/"
		} else {
			// If OUT_DIR is absolute, then moduleGenDir will be an absolute path, so we don't need to set this to anything.
			outDirPrefix = ""
		}
		envVars = append(envVars, "OUT_DIR="+filepath.Join(outDirPrefix, moduleGenDir.String()))
	} else {
		// TODO(pcc): Change this to "OUT_DIR=" after fixing crates to not rely on this value.
		envVars = append(envVars, "OUT_DIR=out")
	}

	envVars = append(envVars, "ANDROID_RUST_VERSION="+config.GetRustVersion(ctx))

	if ctx.RustModule().compiler.cargoEnvCompat() {
		if bin, ok := ctx.RustModule().compiler.(*binaryDecorator); ok {
			envVars = append(envVars, "CARGO_BIN_NAME="+bin.getStem(ctx))
		}
		envVars = append(envVars, "CARGO_CRATE_NAME="+ctx.RustModule().CrateName())
		envVars = append(envVars, "CARGO_PKG_NAME="+ctx.RustModule().CrateName())
		pkgVersion := ctx.RustModule().compiler.cargoPkgVersion()
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

	if ctx.Darwin() {
		envVars = append(envVars, "ANDROID_RUST_DARWIN=true")
	}

	return envVars
}

func transformSrctoCrate(ctx ModuleContext, main android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, crateType string) buildOutput {

	var inputs android.Paths
	var implicits android.Paths
	var orderOnly android.Paths
	var output buildOutput
	var rustcFlags, linkFlags []string
	var earlyLinkFlags string

	output.outputFile = outputFile
	crateName := ctx.RustModule().CrateName()
	targetTriple := ctx.toolchain().RustTriple()

	envVars := rustEnvVars(ctx, deps)

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

		rustcFlags = append(rustcFlags, "-C incremental="+incrementalPath)
	} else {
		rustcFlags = append(rustcFlags, "-C codegen-units=1")
	}

	// Disallow experimental features
	modulePath := ctx.ModuleDir()
	if !(android.IsThirdPartyPath(modulePath) || strings.HasPrefix(modulePath, "prebuilts")) {
		rustcFlags = append(rustcFlags, "-Zallow-features=\"\"")
	}

	// Collect linker flags
	if !ctx.Darwin() {
		earlyLinkFlags = "-Wl,--as-needed"
	}

	linkFlags = append(linkFlags, flags.GlobalLinkFlags...)
	linkFlags = append(linkFlags, flags.LinkFlags...)

	// Check if this module needs to use the bootstrap linker
	if ctx.RustModule().Bootstrap() && !ctx.RustModule().InRecovery() && !ctx.RustModule().InRamdisk() && !ctx.RustModule().InVendorRamdisk() {
		dynamicLinker := "-Wl,-dynamic-linker,/system/bin/bootstrap/linker"
		if ctx.toolchain().Is64Bit() {
			dynamicLinker += "64"
		}
		linkFlags = append(linkFlags, dynamicLinker)
	}

	libFlags := makeLibFlags(deps)

	// Collect dependencies
	implicits = append(implicits, rustLibsToPaths(deps.RLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.DyLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.ProcMacros)...)
	implicits = append(implicits, deps.StaticLibs...)
	implicits = append(implicits, deps.SharedLibDeps...)
	implicits = append(implicits, deps.srcProviderFiles...)
	implicits = append(implicits, deps.AfdoProfiles...)

	implicits = append(implicits, deps.CrtBegin...)
	implicits = append(implicits, deps.CrtEnd...)

	orderOnly = append(orderOnly, deps.SharedLibs...)

	if len(deps.SrcDeps) > 0 {
		moduleGenDir := ctx.RustModule().compiler.cargoOutDir()
		var outputs android.WritablePaths

		for _, genSrc := range deps.SrcDeps {
			if android.SuffixInList(outputs.Strings(), genSubDir+genSrc.Base()) {
				ctx.PropertyErrorf("srcs",
					"multiple source providers generate the same filename output: "+genSrc.Base())
			}
			outputs = append(outputs, android.PathForModuleOut(ctx, genSubDir+genSrc.Base()))
		}

		ctx.Build(pctx, android.BuildParams{
			Rule:        cp,
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
		clippyFile := android.PathForModuleOut(ctx, outputFile.Base()+".clippy")
		ctx.Build(pctx, android.BuildParams{
			Rule:            clippyDriver,
			Description:     "clippy " + main.Rel(),
			Output:          clippyFile,
			ImplicitOutputs: nil,
			Inputs:          inputs,
			Implicits:       implicits,
			OrderOnly:       orderOnly,
			Args: map[string]string{
				"rustcFlags":  strings.Join(rustcFlags, " "),
				"libFlags":    strings.Join(libFlags, " "),
				"clippyFlags": strings.Join(flags.ClippyFlags, " "),
				"envVars":     strings.Join(envVars, " "),
			},
		})
		// Declare the clippy build as an implicit dependency of the original crate.
		implicits = append(implicits, clippyFile)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        rustc,
		Description: "rustc " + main.Rel(),
		Output:      outputFile,
		Inputs:      inputs,
		Implicits:   implicits,
		OrderOnly:   orderOnly,
		Args: map[string]string{
			"rustcFlags":     strings.Join(rustcFlags, " "),
			"earlyLinkFlags": earlyLinkFlags,
			"linkFlags":      strings.Join(linkFlags, " "),
			"libFlags":       strings.Join(libFlags, " "),
			"crtBegin":       strings.Join(deps.CrtBegin.Strings(), " "),
			"crtEnd":         strings.Join(deps.CrtEnd.Strings(), " "),
			"envVars":        strings.Join(envVars, " "),
		},
	})

	if flags.EmitXrefs {
		kytheFile := android.PathForModuleOut(ctx, outputFile.Base()+".kzip")
		ctx.Build(pctx, android.BuildParams{
			Rule:        kytheExtract,
			Description: "Xref Rust extractor " + main.Rel(),
			Output:      kytheFile,
			Inputs:      inputs,
			Implicits:   implicits,
			OrderOnly:   orderOnly,
			Args: map[string]string{
				"rustcFlags": strings.Join(rustcFlags, " "),
				"linkFlags":  strings.Join(linkFlags, " "),
				"libFlags":   strings.Join(libFlags, " "),
				"crtBegin":   strings.Join(deps.CrtBegin.Strings(), " "),
				"crtEnd":     strings.Join(deps.CrtEnd.Strings(), " "),
				"envVars":    strings.Join(envVars, " "),
			},
		})
		output.kytheFile = kytheFile
	}
	return output
}

func Rustdoc(ctx ModuleContext, main android.Path, deps PathDeps,
	flags Flags) android.ModuleOutPath {

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

	rustdocFlags = append(rustdocFlags, makeLibFlags(deps)...)
	docTimestampFile := android.PathForModuleOut(ctx, "rustdoc.timestamp")

	// Silence warnings about renamed lints for third-party crates
	modulePath := ctx.ModuleDir()
	if android.IsThirdPartyPath(modulePath) {
		rustdocFlags = append(rustdocFlags, " -A warnings")
	}

	// Yes, the same out directory is used simultaneously by all rustdoc builds.
	// This is what cargo does. The docs for individual crates get generated to
	// a subdirectory named for the crate, and rustdoc synchronizes writes to
	// shared pieces like the index and search data itself.
	// https://github.com/rust-lang/rust/blob/master/src/librustdoc/html/render/write_shared.rs#L144-L146
	docDir := android.PathForOutput(ctx, "rustdoc")

	ctx.Build(pctx, android.BuildParams{
		Rule:        rustdoc,
		Description: "rustdoc " + main.Rel(),
		Output:      docTimestampFile,
		Input:       main,
		Implicit:    ctx.RustModule().UnstrippedOutputFile(),
		Args: map[string]string{
			"rustdocFlags": strings.Join(rustdocFlags, " "),
			"outDir":       docDir.String(),
			"envVars":      strings.Join(rustEnvVars(ctx, deps), " "),
		},
	})

	return docTimestampFile
}
