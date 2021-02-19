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
	"android/soong/bloaty"
	"android/soong/rust/config"
)

var (
	_     = pctx.SourcePathVariable("rustcCmd", "${config.RustBin}/rustc")
	rustc = pctx.AndroidStaticRule("rustc",
		blueprint.RuleParams{
			Command: "$envVars $rustcCmd " +
				"-C linker=${config.RustLinker} " +
				"-C link-args=\"${crtBegin} ${config.RustLinkerArgs} ${linkFlags} ${crtEnd}\" " +
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
		"rustcFlags", "linkFlags", "libFlags", "crtBegin", "crtEnd", "envVars")

	_            = pctx.SourcePathVariable("clippyCmd", "${config.RustBin}/clippy-driver")
	clippyDriver = pctx.AndroidStaticRule("clippy",
		blueprint.RuleParams{
			Command: "$envVars $clippyCmd " +
				// Because clippy-driver uses rustc as backend, we need to have some output even during the linting.
				// Use the metadata output as it has the smallest footprint.
				"--emit metadata -o $out $in ${libFlags} " +
				"$rustcFlags $clippyFlags",
			CommandDeps: []string{"$clippyCmd"},
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
)

type buildOutput struct {
	outputFile android.Path
}

func init() {
	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
}

func TransformSrcToBinary(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, linkDirs []string) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto")

	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "bin", linkDirs)
}

func TransformSrctoRlib(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, linkDirs []string) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "rlib", linkDirs)
}

func TransformSrctoDylib(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, linkDirs []string) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "dylib", linkDirs)
}

func TransformSrctoStatic(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, linkDirs []string) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto")
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "staticlib", linkDirs)
}

func TransformSrctoShared(ctx ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, linkDirs []string) buildOutput {
	flags.GlobalRustFlags = append(flags.GlobalRustFlags, "-C lto")
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "cdylib", linkDirs)
}

func TransformSrctoProcMacro(ctx ModuleContext, mainSrc android.Path, deps PathDeps,
	flags Flags, outputFile android.WritablePath, linkDirs []string) buildOutput {
	return transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "proc-macro", linkDirs)
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func transformSrctoCrate(ctx ModuleContext, main android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, crate_type string, linkDirs []string) buildOutput {

	var inputs android.Paths
	var implicits android.Paths
	var envVars []string
	var output buildOutput
	var libFlags, rustcFlags, linkFlags []string
	var implicitOutputs android.WritablePaths

	output.outputFile = outputFile
	crateName := ctx.RustModule().CrateName()
	targetTriple := ctx.toolchain().RustTriple()

	// libstd requires a specific environment variable to be set. This is
	// not officially documented and may be removed in the future. See
	// https://github.com/rust-lang/rust/blob/master/library/std/src/env.rs#L866.
	if crateName == "std" {
		envVars = append(envVars, "STD_ENV_ARCH="+config.StdEnvArch[ctx.RustModule().Arch().ArchType])
	}

	inputs = append(inputs, main)

	// Collect rustc flags
	rustcFlags = append(rustcFlags, flags.GlobalRustFlags...)
	rustcFlags = append(rustcFlags, flags.RustFlags...)
	rustcFlags = append(rustcFlags, "--crate-type="+crate_type)
	if crateName != "" {
		rustcFlags = append(rustcFlags, "--crate-name="+crateName)
	}
	if targetTriple != "" {
		rustcFlags = append(rustcFlags, "--target="+targetTriple)
		linkFlags = append(linkFlags, "-target "+targetTriple)
	}

	// Suppress an implicit sysroot
	rustcFlags = append(rustcFlags, "--sysroot=/dev/null")

	// Collect linker flags
	linkFlags = append(linkFlags, flags.GlobalLinkFlags...)
	linkFlags = append(linkFlags, flags.LinkFlags...)

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

	for _, path := range linkDirs {
		libFlags = append(libFlags, "-L "+path)
	}

	// Collect dependencies
	implicits = append(implicits, rustLibsToPaths(deps.RLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.DyLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.ProcMacros)...)
	implicits = append(implicits, deps.StaticLibs...)
	implicits = append(implicits, deps.SharedLibDeps...)
	implicits = append(implicits, deps.srcProviderFiles...)

	if deps.CrtBegin.Valid() {
		implicits = append(implicits, deps.CrtBegin.Path(), deps.CrtEnd.Path())
	}

	if len(deps.SrcDeps) > 0 {
		genSubDir := "out/"
		moduleGenDir := android.PathForModuleOut(ctx, genSubDir)
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
			Description: "cp " + moduleGenDir.Rel(),
			Outputs:     outputs,
			Inputs:      deps.SrcDeps,
			Args: map[string]string{
				"outDir": moduleGenDir.String(),
			},
		})
		implicits = append(implicits, outputs.Paths()...)

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

	bloaty.MeasureSizeForPath(ctx, outputFile)

	ctx.Build(pctx, android.BuildParams{
		Rule:            rustc,
		Description:     "rustc " + main.Rel(),
		Output:          outputFile,
		ImplicitOutputs: implicitOutputs,
		Inputs:          inputs,
		Implicits:       implicits,
		Args: map[string]string{
			"rustcFlags": strings.Join(rustcFlags, " "),
			"linkFlags":  strings.Join(linkFlags, " "),
			"libFlags":   strings.Join(libFlags, " "),
			"crtBegin":   deps.CrtBegin.String(),
			"crtEnd":     deps.CrtEnd.String(),
			"envVars":    strings.Join(envVars, " "),
		},
	})

	return output
}
