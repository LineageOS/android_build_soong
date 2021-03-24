// Copyright 2020 The Android Open Source Project
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
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	cc_config "android/soong/cc/config"
)

var (
	defaultBindgenFlags = []string{""}

	// bindgen should specify its own Clang revision so updating Clang isn't potentially blocked on bindgen failures.
	bindgenClangVersion = "clang-r412851"

	_ = pctx.VariableFunc("bindgenClangVersion", func(ctx android.PackageVarContext) string {
		if override := ctx.Config().Getenv("LLVM_BINDGEN_PREBUILTS_VERSION"); override != "" {
			return override
		}
		return bindgenClangVersion
	})

	//TODO(b/160803703) Use a prebuilt bindgen instead of the built bindgen.
	_ = pctx.HostBinToolVariable("bindgenCmd", "bindgen")
	_ = pctx.SourcePathVariable("bindgenClang",
		"${cc_config.ClangBase}/${config.HostPrebuiltTag}/${bindgenClangVersion}/bin/clang")
	_ = pctx.SourcePathVariable("bindgenLibClang",
		"${cc_config.ClangBase}/${config.HostPrebuiltTag}/${bindgenClangVersion}/lib64/")

	//TODO(ivanlozano) Switch this to RuleBuilder
	bindgen = pctx.AndroidStaticRule("bindgen",
		blueprint.RuleParams{
			Command: "CLANG_PATH=$bindgenClang LIBCLANG_PATH=$bindgenLibClang RUSTFMT=${config.RustBin}/rustfmt " +
				"$cmd $flags $in -o $out -- -MD -MF $out.d $cflags",
			CommandDeps: []string{"$cmd"},
			Deps:        blueprint.DepsGCC,
			Depfile:     "$out.d",
		},
		"cmd", "flags", "cflags")
)

func init() {
	android.RegisterModuleType("rust_bindgen", RustBindgenFactory)
	android.RegisterModuleType("rust_bindgen_host", RustBindgenHostFactory)
}

var _ SourceProvider = (*bindgenDecorator)(nil)

type BindgenProperties struct {
	// The wrapper header file. By default this is assumed to be a C header unless the extension is ".hh" or ".hpp".
	// This is used to specify how to interpret the header and determines which '-std' flag to use by default.
	//
	// If your C++ header must have some other extension, then the default behavior can be overridden by setting the
	// cpp_std property.
	Wrapper_src *string `android:"path,arch_variant"`

	// list of bindgen-specific flags and options
	Bindgen_flags []string `android:"arch_variant"`

	// module name of a custom binary/script which should be used instead of the 'bindgen' binary. This custom
	// binary must expect arguments in a similar fashion to bindgen, e.g.
	//
	// "my_bindgen [flags] wrapper_header.h -o [output_path] -- [clang flags]"
	Custom_bindgen string `android:"path"`
}

type bindgenDecorator struct {
	*BaseSourceProvider

	Properties      BindgenProperties
	ClangProperties cc.RustBindgenClangProperties
}

func (b *bindgenDecorator) getStdVersion(ctx ModuleContext, src android.Path) (string, bool) {
	// Assume headers are C headers
	isCpp := false
	stdVersion := ""

	switch src.Ext() {
	case ".hpp", ".hh":
		isCpp = true
	}

	if String(b.ClangProperties.Cpp_std) != "" && String(b.ClangProperties.C_std) != "" {
		ctx.PropertyErrorf("c_std", "c_std and cpp_std cannot both be defined at the same time.")
	}

	if String(b.ClangProperties.Cpp_std) != "" {
		if String(b.ClangProperties.Cpp_std) == "experimental" {
			stdVersion = cc_config.ExperimentalCppStdVersion
		} else if String(b.ClangProperties.Cpp_std) == "default" {
			stdVersion = cc_config.CppStdVersion
		} else {
			stdVersion = String(b.ClangProperties.Cpp_std)
		}
	} else if b.ClangProperties.C_std != nil {
		if String(b.ClangProperties.C_std) == "experimental" {
			stdVersion = cc_config.ExperimentalCStdVersion
		} else if String(b.ClangProperties.C_std) == "default" {
			stdVersion = cc_config.CStdVersion
		} else {
			stdVersion = String(b.ClangProperties.C_std)
		}
	} else if isCpp {
		stdVersion = cc_config.CppStdVersion
	} else {
		stdVersion = cc_config.CStdVersion
	}

	return stdVersion, isCpp
}

func (b *bindgenDecorator) GenerateSource(ctx ModuleContext, deps PathDeps) android.Path {
	ccToolchain := ctx.RustModule().ccToolchain(ctx)

	var cflags []string
	var implicits android.Paths

	implicits = append(implicits, deps.depGeneratedHeaders...)

	// Default clang flags
	cflags = append(cflags, "${cc_config.CommonClangGlobalCflags}")
	if ctx.Device() {
		cflags = append(cflags, "${cc_config.DeviceClangGlobalCflags}")
	}

	// Toolchain clang flags
	cflags = append(cflags, "-target "+ccToolchain.ClangTriple())
	cflags = append(cflags, strings.ReplaceAll(ccToolchain.ToolchainClangCflags(), "${config.", "${cc_config."))

	// Dependency clang flags and include paths
	cflags = append(cflags, deps.depClangFlags...)
	for _, include := range deps.depIncludePaths {
		cflags = append(cflags, "-I"+include.String())
	}
	for _, include := range deps.depSystemIncludePaths {
		cflags = append(cflags, "-isystem "+include.String())
	}

	esc := proptools.NinjaAndShellEscapeList

	// Filter out invalid cflags
	for _, flag := range b.ClangProperties.Cflags {
		if flag == "-x c++" || flag == "-xc++" {
			ctx.PropertyErrorf("cflags",
				"-x c++ should not be specified in cflags; setting cpp_std specifies this is a C++ header, or change the file extension to '.hpp' or '.hh'")
		}
		if strings.HasPrefix(flag, "-std=") {
			ctx.PropertyErrorf("cflags",
				"-std should not be specified in cflags; instead use c_std or cpp_std")
		}
	}

	// Module defined clang flags and include paths
	cflags = append(cflags, esc(b.ClangProperties.Cflags)...)
	for _, include := range b.ClangProperties.Local_include_dirs {
		cflags = append(cflags, "-I"+android.PathForModuleSrc(ctx, include).String())
		implicits = append(implicits, android.PathForModuleSrc(ctx, include))
	}

	bindgenFlags := defaultBindgenFlags
	bindgenFlags = append(bindgenFlags, esc(b.Properties.Bindgen_flags)...)

	wrapperFile := android.OptionalPathForModuleSrc(ctx, b.Properties.Wrapper_src)
	if !wrapperFile.Valid() {
		ctx.PropertyErrorf("wrapper_src", "invalid path to wrapper source")
	}

	// Add C std version flag
	stdVersion, isCpp := b.getStdVersion(ctx, wrapperFile.Path())
	cflags = append(cflags, "-std="+stdVersion)

	// Specify the header source language to avoid ambiguity.
	if isCpp {
		cflags = append(cflags, "-x c++")
		// Add any C++ only flags.
		cflags = append(cflags, esc(b.ClangProperties.Cppflags)...)
	} else {
		cflags = append(cflags, "-x c")
	}

	outputFile := android.PathForModuleOut(ctx, b.BaseSourceProvider.getStem(ctx)+".rs")

	var cmd, cmdDesc string
	if b.Properties.Custom_bindgen != "" {
		cmd = ctx.GetDirectDepWithTag(b.Properties.Custom_bindgen, customBindgenDepTag).(*Module).HostToolPath().String()
		cmdDesc = b.Properties.Custom_bindgen
	} else {
		cmd = "$bindgenCmd"
		cmdDesc = "bindgen"
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        bindgen,
		Description: strings.Join([]string{cmdDesc, wrapperFile.Path().Rel()}, " "),
		Output:      outputFile,
		Input:       wrapperFile.Path(),
		Implicits:   implicits,
		Args: map[string]string{
			"cmd":    cmd,
			"flags":  strings.Join(bindgenFlags, " "),
			"cflags": strings.Join(cflags, " "),
		},
	})

	b.BaseSourceProvider.OutputFiles = android.Paths{outputFile}
	return outputFile
}

func (b *bindgenDecorator) SourceProviderProps() []interface{} {
	return append(b.BaseSourceProvider.SourceProviderProps(),
		&b.Properties, &b.ClangProperties)
}

// rust_bindgen generates Rust FFI bindings to C libraries using bindgen given a wrapper header as the primary input.
// Bindgen has a number of flags to control the generated source, and additional flags can be passed to clang to ensure
// the header and generated source is appropriately handled. It is recommended to add it as a dependency in the
// rlibs, dylibs or rustlibs property. It may also be added in the srcs property for external crates, using the ":"
// prefix.
func RustBindgenFactory() android.Module {
	module, _ := NewRustBindgen(android.HostAndDeviceSupported)
	return module.Init()
}

func RustBindgenHostFactory() android.Module {
	module, _ := NewRustBindgen(android.HostSupported)
	return module.Init()
}

func NewRustBindgen(hod android.HostOrDeviceSupported) (*Module, *bindgenDecorator) {
	bindgen := &bindgenDecorator{
		BaseSourceProvider: NewSourceProvider(),
		Properties:         BindgenProperties{},
		ClangProperties:    cc.RustBindgenClangProperties{},
	}

	module := NewSourceProviderModule(hod, bindgen, false)

	return module, bindgen
}

func (b *bindgenDecorator) SourceProviderDeps(ctx DepsContext, deps Deps) Deps {
	deps = b.BaseSourceProvider.SourceProviderDeps(ctx, deps)
	if ctx.toolchain().Bionic() {
		deps = bionicDeps(ctx, deps, false)
	}

	deps.SharedLibs = append(deps.SharedLibs, b.ClangProperties.Shared_libs...)
	deps.StaticLibs = append(deps.StaticLibs, b.ClangProperties.Static_libs...)
	deps.HeaderLibs = append(deps.StaticLibs, b.ClangProperties.Header_libs...)
	return deps
}
