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
			Depfile:     "$out.d",
			Deps:        blueprint.DepsGCC, // Rustc deps-info writes out make compatible dep files: https://github.com/rust-lang/rust/issues/7633
		},
		"rustcFlags", "linkFlags", "libFlags", "crtBegin", "crtEnd")
)

func init() {

}

func TransformSrcToBinary(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, deps.CrtBegin, deps.CrtEnd, flags, outputFile, "bin", includeDirs)
}

func TransformSrctoRlib(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, deps.CrtBegin, deps.CrtEnd, flags, outputFile, "rlib", includeDirs)
}

func TransformSrctoDylib(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, deps.CrtBegin, deps.CrtEnd, flags, outputFile, "dylib", includeDirs)
}

func TransformSrctoStatic(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, deps.CrtBegin, deps.CrtEnd, flags, outputFile, "staticlib", includeDirs)
}

func TransformSrctoShared(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, deps.CrtBegin, deps.CrtEnd, flags, outputFile, "cdylib", includeDirs)
}

func TransformSrctoProcMacro(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, deps.CrtBegin, deps.CrtEnd, flags, outputFile, "proc-macro", includeDirs)
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func transformSrctoCrate(ctx android.ModuleContext, main android.Path,
	rlibs, dylibs, proc_macros RustLibraries, static_libs, shared_libs android.Paths, crtBegin, crtEnd android.OptionalPath, flags Flags, outputFile android.WritablePath, crate_type string, includeDirs []string) {

	var inputs android.Paths
	var deps android.Paths
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
	// Collect linker flags
	linkFlags = append(linkFlags, flags.GlobalLinkFlags...)
	linkFlags = append(linkFlags, flags.LinkFlags...)

	// Collect library/crate flags
	for _, lib := range rlibs {
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+lib.Path.String())
	}
	for _, lib := range dylibs {
		libFlags = append(libFlags, "--extern "+lib.CrateName+"="+lib.Path.String())
	}
	for _, proc_macro := range proc_macros {
		libFlags = append(libFlags, "--extern "+proc_macro.CrateName+"="+proc_macro.Path.String())
	}

	for _, path := range includeDirs {
		libFlags = append(libFlags, "-L "+path)
	}

	// Collect dependencies
	deps = append(deps, rustLibsToPaths(rlibs)...)
	deps = append(deps, rustLibsToPaths(dylibs)...)
	deps = append(deps, rustLibsToPaths(proc_macros)...)
	deps = append(deps, static_libs...)
	deps = append(deps, shared_libs...)
	if crtBegin.Valid() {
		deps = append(deps, crtBegin.Path(), crtEnd.Path())
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        rustc,
		Description: "rustc " + main.Rel(),
		Output:      outputFile,
		Inputs:      inputs,
		Implicits:   deps,
		Args: map[string]string{
			"rustcFlags": strings.Join(rustcFlags, " "),
			"linkFlags":  strings.Join(linkFlags, " "),
			"libFlags":   strings.Join(libFlags, " "),
			"crtBegin":   crtBegin.String(),
			"crtEnd":     crtEnd.String(),
		},
	})

}
