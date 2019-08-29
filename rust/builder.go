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
				"-C link-args=\"${config.RustLinkerArgs} ${linkFlags}\" " +
				"-o $out $in ${libFlags} $rustcFlags " +
				"&& $rustcCmd --emit=dep-info -o $out.d $in ${libFlags} $rustcFlags",
			CommandDeps: []string{"$rustcCmd"},
			Depfile:     "$out.d",
			Deps:        blueprint.DepsGCC, // Rustc deps-info writes out make compatible dep files: https://github.com/rust-lang/rust/issues/7633
		},
		"rustcFlags", "linkFlags", "libFlags")
)

func init() {

}

func TransformSrcToBinary(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	targetTriple := ctx.(ModuleContext).toolchain().RustTriple()

	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, flags, outputFile, "bin", includeDirs, targetTriple)
}

func TransformSrctoRlib(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	targetTriple := ctx.(ModuleContext).toolchain().RustTriple()

	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, flags, outputFile, "rlib", includeDirs, targetTriple)
}

func TransformSrctoDylib(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	targetTriple := ctx.(ModuleContext).toolchain().RustTriple()

	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, flags, outputFile, "dylib", includeDirs, targetTriple)
}

func TransformSrctoProcMacro(ctx android.ModuleContext, mainSrc android.Path, deps PathDeps, flags Flags, outputFile android.WritablePath, includeDirs []string) {
	// Proc macros are compiler plugins, and thus should target the host compiler
	targetTriple := ""

	transformSrctoCrate(ctx, mainSrc, deps.RLibs, deps.DyLibs, deps.ProcMacros, deps.StaticLibs, deps.SharedLibs, flags, outputFile, "proc-macro", includeDirs, targetTriple)
}

func rustLibsToPaths(libs RustLibraries) android.Paths {
	var paths android.Paths
	for _, lib := range libs {
		paths = append(paths, lib.Path)
	}
	return paths
}

func transformSrctoCrate(ctx android.ModuleContext, main android.Path,
	rlibs, dylibs, proc_macros RustLibraries, static_libs, shared_libs android.Paths, flags Flags, outputFile android.WritablePath, crate_type string, includeDirs []string, targetTriple string) {

	var inputs android.Paths
	var deps android.Paths
	var libFlags, rustcFlags []string
	crate_name := ctx.(ModuleContext).CrateName()

	inputs = append(inputs, main)

	// Collect rustc flags
	rustcFlags = append(rustcFlags, flags.GlobalFlags...)
	rustcFlags = append(rustcFlags, flags.RustFlags...)
	rustcFlags = append(rustcFlags, "--crate-type="+crate_type)
	rustcFlags = append(rustcFlags, "--crate-name="+crate_name)
	if targetTriple != "" {
		rustcFlags = append(rustcFlags, "--target="+targetTriple)
	}

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

	ctx.Build(pctx, android.BuildParams{
		Rule:        rustc,
		Description: "rustc " + main.Rel(),
		Output:      outputFile,
		Inputs:      inputs,
		Implicits:   deps,
		Args: map[string]string{
			"rustcFlags": strings.Join(rustcFlags, " "),
			"linkFlags":  strings.Join(flags.LinkFlags, " "),
			"libFlags":   strings.Join(libFlags, " "),
		},
	})

}
