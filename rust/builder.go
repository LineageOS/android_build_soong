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
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	_     = pctx.SourcePathVariable("rustcCmd", "${config.RustBin}/rustc")
	rustc = pctx.AndroidStaticRule("rustc",
		blueprint.RuleParams{
			Command: "$rustcCmd " +
				"-C linker=${config.RustLinker} " +
				"-C link-args=\"${crtBegin} ${config.RustLinkerArgs} ${linkFlags} ${crtEnd}\" " +
				"--emit link -o $out --emit dep-info=$out.d $in ${libFlags} $rustcFlags",
			CommandDeps: []string{"$rustcCmd"},
			// Rustc deps-info writes out make compatible dep files: https://github.com/rust-lang/rust/issues/7633
			Deps:    blueprint.DepsGCC,
			Depfile: "$out.d",
		},
		"rustcFlags", "linkFlags", "libFlags", "crtBegin", "crtEnd")
)

func init() {

}

func TransformSrcToBinary(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, includeDirs []string) {
	flags.RustFlags = append(flags.RustFlags, "-C lto")

	transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "bin", includeDirs)
}

func TransformSrctoRlib(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "rlib", includeDirs)
}

func TransformSrctoDylib(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "dylib", includeDirs)
}

func TransformSrctoStatic(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, includeDirs []string) {
	flags.RustFlags = append(flags.RustFlags, "-C lto")
	transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "staticlib", includeDirs)
}

func TransformSrctoShared(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, includeDirs []string) {
	flags.RustFlags = append(flags.RustFlags, "-C lto")
	transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "cdylib", includeDirs)
}

func TransformSrctoProcMacro(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps,
	flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps, flags, outputFile, "proc-macro", includeDirs)
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func transformSrctoCrate(ctx android.ModuleContext, main android.Path, deps PathDeps, flags Flags,
	outputFile android.WritablePath, crate_type string, includeDirs []string) {

	var inputs android.Paths
	var implicits android.Paths
	var libFlags, rustcFlags, linkFlags []string
	crate_name := ctx.(ModuleContext).CrateName()
	targetTriple := ctx.(ModuleContext).toolchain().RustTriple()

	inputs = append(inputs, main)

	// Collect rustc flags
	rustcFlags = append(rustcFlags, flags.GlobalRustFlags...)
	rustcFlags = append(rustcFlags, flags.RustFlags...)
	rustcFlags = append(rustcFlags, "--crate-type="+crate_type)
	if crate_name != "" {
		rustcFlags = append(rustcFlags, "--crate-name="+crate_name)
	}
	if targetTriple != "" {
		rustcFlags = append(rustcFlags, "--target="+targetTriple)
		linkFlags = append(linkFlags, "-target "+targetTriple)
	}
	// TODO once we have static libraries in the host prebuilt .bp, this
	// should be unconditionally added.
	if !ctx.Host() {
		// If we're on a device build, do not use an implicit sysroot
		rustcFlags = append(rustcFlags, "--sysroot=/dev/null")
	}
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

	for _, path := range includeDirs {
		libFlags = append(libFlags, "-L "+path)
	}

	// Collect dependencies
	implicits = append(implicits, rustLibsToPaths(deps.RLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.DyLibs)...)
	implicits = append(implicits, rustLibsToPaths(deps.ProcMacros)...)
	implicits = append(implicits, deps.StaticLibs...)
	implicits = append(implicits, deps.SharedLibs...)
	if deps.CrtBegin.Valid() {
		implicits = append(implicits, deps.CrtBegin.Path(), deps.CrtEnd.Path())
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        rustc,
		Description: "rustc " + main.Rel(),
		Output:      outputFile,
		Inputs:      inputs,
		Implicits:   implicits,
		Args: map[string]string{
			"rustcFlags": strings.Join(rustcFlags, " "),
			"linkFlags":  strings.Join(linkFlags, " "),
			"libFlags":   strings.Join(libFlags, " "),
			"crtBegin":   deps.CrtBegin.String(),
			"crtEnd":     deps.CrtEnd.String(),
		},
	})

}
