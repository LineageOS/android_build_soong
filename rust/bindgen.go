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
)

var (
	defaultBindgenFlags = []string{""}

	// bindgen should specify its own Clang revision so updating Clang isn't potentially blocked on bindgen failures.
	bindgenClangVersion  = "clang-r383902c"
	bindgenLibClangSoGit = "11git"

	//TODO(b/160803703) Use a prebuilt bindgen instead of the built bindgen.
	_ = pctx.SourcePathVariable("bindgenCmd", "out/host/${config.HostPrebuiltTag}/bin/bindgen")
	_ = pctx.SourcePathVariable("bindgenClang",
		"${cc_config.ClangBase}/${config.HostPrebuiltTag}/"+bindgenClangVersion+"/bin/clang")
	_ = pctx.SourcePathVariable("bindgenLibClang",
		"${cc_config.ClangBase}/${config.HostPrebuiltTag}/"+bindgenClangVersion+"/lib64/libclang.so."+bindgenLibClangSoGit)

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
	// The wrapper header file
	Wrapper_src *string `android:"path,arch_variant"`

	// list of bindgen-specific flags and options
	Bindgen_flags []string `android:"arch_variant"`

	// list of clang flags required to correctly interpret the headers.
	Cflags []string `android:"arch_variant"`

	// list of directories relative to the Blueprints file that will
	// be added to the include path using -I
	Local_include_dirs []string `android:"arch_variant,variant_prepend"`

	// list of static libraries that provide headers for this binding.
	Static_libs []string `android:"arch_variant,variant_prepend"`

	// list of shared libraries that provide headers for this binding.
	Shared_libs []string `android:"arch_variant"`

	// module name of a custom binary/script which should be used instead of the 'bindgen' binary. This custom
	// binary must expect arguments in a similar fashion to bindgen, e.g.
	//
	// "my_bindgen [flags] wrapper_header.h -o [output_path] -- [clang flags]"
	Custom_bindgen string `android:"path"`

	//TODO(b/161141999) Add support for headers from cc_library_header modules.
}

type bindgenDecorator struct {
	*BaseSourceProvider

	Properties BindgenProperties
}

func (b *bindgenDecorator) GenerateSource(ctx ModuleContext, deps PathDeps) android.Path {
	ccToolchain := ctx.RustModule().ccToolchain(ctx)

	var cflags []string
	var implicits android.Paths

	implicits = append(implicits, deps.depIncludePaths...)
	implicits = append(implicits, deps.depSystemIncludePaths...)

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

	// Module defined clang flags and include paths
	cflags = append(cflags, esc(b.Properties.Cflags)...)
	for _, include := range b.Properties.Local_include_dirs {
		cflags = append(cflags, "-I"+android.PathForModuleSrc(ctx, include).String())
		implicits = append(implicits, android.PathForModuleSrc(ctx, include))
	}

	bindgenFlags := defaultBindgenFlags
	bindgenFlags = append(bindgenFlags, esc(b.Properties.Bindgen_flags)...)

	wrapperFile := android.OptionalPathForModuleSrc(ctx, b.Properties.Wrapper_src)
	if !wrapperFile.Valid() {
		ctx.PropertyErrorf("wrapper_src", "invalid path to wrapper source")
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

	b.BaseSourceProvider.OutputFile = outputFile
	return outputFile
}

func (b *bindgenDecorator) SourceProviderProps() []interface{} {
	return append(b.BaseSourceProvider.SourceProviderProps(),
		&b.Properties)
}

// rust_bindgen generates Rust FFI bindings to C libraries using bindgen given a wrapper header as the primary input.
// Bindgen has a number of flags to control the generated source, and additional flags can be passed to clang to ensure
// the header and generated source is appropriately handled.
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
	}

	module := NewSourceProviderModule(hod, bindgen, false)

	return module, bindgen
}

func (b *bindgenDecorator) SourceProviderDeps(ctx DepsContext, deps Deps) Deps {
	deps = b.BaseSourceProvider.SourceProviderDeps(ctx, deps)
	if ctx.toolchain().Bionic() {
		deps = bionicDeps(deps)
	}

	deps.SharedLibs = append(deps.SharedLibs, b.Properties.Shared_libs...)
	deps.StaticLibs = append(deps.StaticLibs, b.Properties.Static_libs...)
	return deps
}
