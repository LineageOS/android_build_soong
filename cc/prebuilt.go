// Copyright 2016 Google Inc. All rights reserved.
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

package cc

import (
	"path/filepath"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	RegisterPrebuiltBuildComponents(android.InitRegistrationContext)
}

func RegisterPrebuiltBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_prebuilt_library", PrebuiltLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_shared", PrebuiltSharedLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_static", PrebuiltStaticLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_test_library_shared", PrebuiltSharedTestLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_object", PrebuiltObjectFactory)
	ctx.RegisterModuleType("cc_prebuilt_binary", PrebuiltBinaryFactory)
}

type prebuiltLinkerInterface interface {
	Name(string) string
	prebuilt() *android.Prebuilt
	sourceModuleName() string
}

type prebuiltLinkerProperties struct {
	// Name of the source soong module that gets shadowed by this prebuilt
	// If unspecified, follows the naming convention that the source module of
	// the prebuilt is Name() without "prebuilt_" prefix
	Source_module_name *string

	// a prebuilt library or binary. Can reference a genrule module that generates an executable file.
	Srcs []string `android:"path,arch_variant"`

	Sanitized Sanitized `android:"arch_variant"`

	// Check the prebuilt ELF files (e.g. DT_SONAME, DT_NEEDED, resolution of undefined
	// symbols, etc), default true.
	Check_elf_files *bool

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols *string

	// Optionally provide an import library if this is a Windows PE DLL prebuilt.
	// This is needed only if this library is linked by other modules in build time.
	// Only makes sense for the Windows target.
	Windows_import_lib *string `android:"path,arch_variant"`
}

type prebuiltLinker struct {
	android.Prebuilt

	properties prebuiltLinkerProperties
}

func (p *prebuiltLinker) prebuilt() *android.Prebuilt {
	return &p.Prebuilt
}

func (p *prebuiltLinker) PrebuiltSrcs() []string {
	return p.properties.Srcs
}

type prebuiltLibraryInterface interface {
	libraryInterface
	prebuiltLinkerInterface
	disablePrebuilt()
}

type prebuiltLibraryLinker struct {
	*libraryDecorator
	prebuiltLinker
}

var _ prebuiltLinkerInterface = (*prebuiltLibraryLinker)(nil)
var _ prebuiltLibraryInterface = (*prebuiltLibraryLinker)(nil)

func (p *prebuiltLibraryLinker) linkerInit(ctx BaseModuleContext) {}

func (p *prebuiltLibraryLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	return p.libraryDecorator.linkerDeps(ctx, deps)
}

func (p *prebuiltLibraryLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	return flags
}

func (p *prebuiltLibraryLinker) linkerProps() []interface{} {
	return p.libraryDecorator.linkerProps()
}

func (p *prebuiltLibraryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	p.libraryDecorator.flagExporter.exportIncludes(ctx)
	p.libraryDecorator.flagExporter.reexportDirs(deps.ReexportedDirs...)
	p.libraryDecorator.flagExporter.reexportSystemDirs(deps.ReexportedSystemDirs...)
	p.libraryDecorator.flagExporter.reexportFlags(deps.ReexportedFlags...)
	p.libraryDecorator.flagExporter.reexportDeps(deps.ReexportedDeps...)
	p.libraryDecorator.flagExporter.addExportedGeneratedHeaders(deps.ReexportedGeneratedHeaders...)

	p.libraryDecorator.flagExporter.setProvider(ctx)

	// TODO(ccross): verify shared library dependencies
	srcs := p.prebuiltSrcs(ctx)
	if len(srcs) > 0 {
		if len(srcs) > 1 {
			ctx.PropertyErrorf("srcs", "multiple prebuilt source files")
			return nil
		}

		p.libraryDecorator.exportVersioningMacroIfNeeded(ctx)

		in := android.PathForModuleSrc(ctx, srcs[0])

		if String(p.prebuiltLinker.properties.Prefix_symbols) != "" {
			prefixed := android.PathForModuleOut(ctx, "prefixed", srcs[0])
			transformBinaryPrefixSymbols(ctx, String(p.prebuiltLinker.properties.Prefix_symbols),
				in, flagsToBuilderFlags(flags), prefixed)
			in = prefixed
		}

		if p.static() {
			depSet := android.NewDepSetBuilder[android.Path](android.TOPOLOGICAL).Direct(in).Build()
			android.SetProvider(ctx, StaticLibraryInfoProvider, StaticLibraryInfo{
				StaticLibrary: in,

				TransitiveStaticLibrariesForOrdering: depSet,
			})
			return in
		}

		if p.shared() {
			p.unstrippedOutputFile = in
			libName := p.libraryDecorator.getLibName(ctx) + flags.Toolchain.ShlibSuffix()
			outputFile := android.PathForModuleOut(ctx, libName)
			var implicits android.Paths

			if p.stripper.NeedsStrip(ctx) {
				stripFlags := flagsToStripFlags(flags)
				stripped := android.PathForModuleOut(ctx, "stripped", libName)
				p.stripper.StripExecutableOrSharedLib(ctx, in, stripped, stripFlags)
				in = stripped
			}

			// Optimize out relinking against shared libraries whose interface hasn't changed by
			// depending on a table of contents file instead of the library itself.
			tocFile := android.PathForModuleOut(ctx, libName+".toc")
			p.tocFile = android.OptionalPathForPath(tocFile)
			TransformSharedObjectToToc(ctx, outputFile, tocFile)

			if ctx.Windows() && p.properties.Windows_import_lib != nil {
				// Consumers of this library actually links to the import library in build
				// time and dynamically links to the DLL in run time. i.e.
				// a.exe <-- static link --> foo.lib <-- dynamic link --> foo.dll
				importLibSrc := android.PathForModuleSrc(ctx, String(p.properties.Windows_import_lib))
				importLibName := p.libraryDecorator.getLibName(ctx) + ".lib"
				importLibOutputFile := android.PathForModuleOut(ctx, importLibName)
				implicits = append(implicits, importLibOutputFile)

				ctx.Build(pctx, android.BuildParams{
					Rule:        android.Cp,
					Description: "prebuilt import library",
					Input:       importLibSrc,
					Output:      importLibOutputFile,
					Args: map[string]string{
						"cpFlags": "-L",
					},
				})
			}

			ctx.Build(pctx, android.BuildParams{
				Rule:        android.Cp,
				Description: "prebuilt shared library",
				Implicits:   implicits,
				Input:       in,
				Output:      outputFile,
				Args: map[string]string{
					"cpFlags": "-L",
				},
			})

			android.SetProvider(ctx, SharedLibraryInfoProvider, SharedLibraryInfo{
				SharedLibrary: outputFile,
				Target:        ctx.Target(),

				TableOfContents: p.tocFile,
			})

			// TODO(b/220898484): Mainline module sdk prebuilts of stub libraries use a stub
			// library as their source and must not be installed, but other prebuilts like
			// libclang_rt.* libraries set `stubs` property because they are LLNDK libraries,
			// but use an implementation library as their source and need to be installed.
			// This discrepancy should be resolved without the prefix hack below.
			isModuleSdkPrebuilts := android.HasAnyPrefix(ctx.ModuleDir(), []string{
				"prebuilts/runtime/mainline/", "prebuilts/module_sdk/"})
			if p.hasStubsVariants() && !p.buildStubs() && !ctx.Host() && isModuleSdkPrebuilts {
				ctx.Module().MakeUninstallable()
			}

			return outputFile
		}
	}

	if p.header() {
		android.SetProvider(ctx, HeaderLibraryInfoProvider, HeaderLibraryInfo{})

		// Need to return an output path so that the AndroidMk logic doesn't skip
		// the prebuilt header. For compatibility, in case Android.mk files use a
		// header lib in LOCAL_STATIC_LIBRARIES, create an empty ar file as
		// placeholder, just like non-prebuilt header modules do in linkStatic().
		ph := android.PathForModuleOut(ctx, ctx.ModuleName()+staticLibraryExtension)
		transformObjToStaticLib(ctx, nil, nil, builderFlags{}, ph, nil, nil)
		return ph
	}

	return nil
}

func (p *prebuiltLibraryLinker) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	sanitize := ctx.Module().(*Module).sanitize
	srcs := p.properties.Srcs
	srcs = append(srcs, srcsForSanitizer(sanitize, p.properties.Sanitized)...)
	if p.static() {
		srcs = append(srcs, p.libraryDecorator.StaticProperties.Static.Srcs...)
		srcs = append(srcs, srcsForSanitizer(sanitize, p.libraryDecorator.StaticProperties.Static.Sanitized)...)
	}
	if p.shared() {
		srcs = append(srcs, p.libraryDecorator.SharedProperties.Shared.Srcs...)
		srcs = append(srcs, srcsForSanitizer(sanitize, p.libraryDecorator.SharedProperties.Shared.Sanitized)...)
	}
	return srcs
}

func (p *prebuiltLibraryLinker) shared() bool {
	return p.libraryDecorator.shared()
}

func (p *prebuiltLibraryLinker) nativeCoverage() bool {
	return false
}

func (p *prebuiltLibraryLinker) disablePrebuilt() {
	p.properties.Srcs = nil
	p.properties.Sanitized.None.Srcs = nil
	p.properties.Sanitized.Address.Srcs = nil
	p.properties.Sanitized.Hwaddress.Srcs = nil
}

// Implements versionedInterface
func (p *prebuiltLibraryLinker) implementationModuleName(name string) string {
	return android.RemoveOptionalPrebuiltPrefix(name)
}

func NewPrebuiltLibrary(hod android.HostOrDeviceSupported, srcsProperty string) (*Module, *libraryDecorator) {
	module, library := NewLibrary(hod)
	module.compiler = nil

	prebuilt := &prebuiltLibraryLinker{
		libraryDecorator: library,
	}
	module.linker = prebuilt
	module.library = prebuilt

	module.AddProperties(&prebuilt.properties)

	if srcsProperty == "" {
		android.InitPrebuiltModuleWithoutSrcs(module)
	} else {
		srcsSupplier := func(ctx android.BaseModuleContext, _ android.Module) []string {
			return prebuilt.prebuiltSrcs(ctx)
		}

		android.InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, srcsProperty)
	}

	return module, library
}

// cc_prebuilt_library installs a precompiled shared library that are
// listed in the srcs property in the device's directory.
func PrebuiltLibraryFactory() android.Module {
	module, _ := NewPrebuiltLibrary(android.HostAndDeviceSupported, "srcs")

	// Prebuilt shared libraries can be included in APEXes
	android.InitApexModule(module)

	return module.Init()
}

// cc_prebuilt_library_shared installs a precompiled shared library that are
// listed in the srcs property in the device's directory.
func PrebuiltSharedLibraryFactory() android.Module {
	module, _ := NewPrebuiltSharedLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

// cc_prebuilt_test_library_shared installs a precompiled shared library
// to be used as a data dependency of a test-related module (such as cc_test, or
// cc_test_library).
func PrebuiltSharedTestLibraryFactory() android.Module {
	module, library := NewPrebuiltLibrary(android.HostAndDeviceSupported, "srcs")
	library.BuildOnlyShared()
	library.baseInstaller = NewTestInstaller()
	return module.Init()
}

func NewPrebuiltSharedLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module, library := NewPrebuiltLibrary(hod, "srcs")
	library.BuildOnlyShared()

	// Prebuilt shared libraries can be included in APEXes
	android.InitApexModule(module)

	return module, library
}

// cc_prebuilt_library_static installs a precompiled static library that are
// listed in the srcs property in the device's directory.
func PrebuiltStaticLibraryFactory() android.Module {
	module, _ := NewPrebuiltStaticLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

func NewPrebuiltStaticLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module, library := NewPrebuiltLibrary(hod, "srcs")
	library.BuildOnlyStatic()

	return module, library
}

type prebuiltObjectProperties struct {
	// Name of the source soong module that gets shadowed by this prebuilt
	// If unspecified, follows the naming convention that the source module of
	// the prebuilt is Name() without "prebuilt_" prefix
	Source_module_name *string
	Srcs               []string `android:"path,arch_variant"`
}

type prebuiltObjectLinker struct {
	android.Prebuilt
	objectLinker

	properties prebuiltObjectProperties
}

func (p *prebuiltObjectLinker) prebuilt() *android.Prebuilt {
	return &p.Prebuilt
}

func (p *prebuiltObjectLinker) sourceModuleName() string {
	return proptools.String(p.properties.Source_module_name)
}

var _ prebuiltLinkerInterface = (*prebuiltObjectLinker)(nil)

func (p *prebuiltObjectLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	if len(p.properties.Srcs) > 0 {
		// Copy objects to a name matching the final installed name
		in := p.Prebuilt.SingleSourcePath(ctx)
		outputFile := android.PathForModuleOut(ctx, ctx.ModuleName()+".o")
		ctx.Build(pctx, android.BuildParams{
			Rule:        android.CpExecutable,
			Description: "prebuilt",
			Output:      outputFile,
			Input:       in,
		})
		return outputFile
	}
	return nil
}

func (p *prebuiltObjectLinker) object() bool {
	return true
}

func NewPrebuiltObject(hod android.HostOrDeviceSupported) *Module {
	module := newObject(hod)
	prebuilt := &prebuiltObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.linker = prebuilt
	module.AddProperties(&prebuilt.properties)
	android.InitPrebuiltModule(module, &prebuilt.properties.Srcs)
	return module
}

func PrebuiltObjectFactory() android.Module {
	module := NewPrebuiltObject(android.HostAndDeviceSupported)
	return module.Init()
}

type prebuiltBinaryLinker struct {
	*binaryDecorator
	prebuiltLinker

	toolPath android.OptionalPath
}

var _ prebuiltLinkerInterface = (*prebuiltBinaryLinker)(nil)

func (p *prebuiltBinaryLinker) hostToolPath() android.OptionalPath {
	return p.toolPath
}

func (p *prebuiltBinaryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	// TODO(ccross): verify shared library dependencies
	if len(p.properties.Srcs) > 0 {
		fileName := p.getStem(ctx) + flags.Toolchain.ExecutableSuffix()
		in := p.Prebuilt.SingleSourcePath(ctx)
		outputFile := android.PathForModuleOut(ctx, fileName)
		p.unstrippedOutputFile = in

		if ctx.Host() {
			// Host binaries are symlinked to their prebuilt source locations. That
			// way they are executed directly from there so the linker resolves their
			// shared library dependencies relative to that location (using
			// $ORIGIN/../lib(64):$ORIGIN/lib(64) as RUNPATH). This way the prebuilt
			// repository can supply the expected versions of the shared libraries
			// without interference from what is in the out tree.

			// These shared lib paths may point to copies of the libs in
			// .intermediates, which isn't where the binary will load them from, but
			// it's fine for dependency tracking. If a library dependency is updated,
			// the symlink will get a new timestamp, along with any installed symlinks
			// handled in make.
			sharedLibPaths := deps.EarlySharedLibs
			sharedLibPaths = append(sharedLibPaths, deps.SharedLibs...)
			sharedLibPaths = append(sharedLibPaths, deps.LateSharedLibs...)

			var fromPath = in.String()
			if !filepath.IsAbs(fromPath) {
				fromPath = "$$PWD/" + fromPath
			}

			ctx.Build(pctx, android.BuildParams{
				Rule:      android.Symlink,
				Output:    outputFile,
				Input:     in,
				Implicits: sharedLibPaths,
				Args: map[string]string{
					"fromPath": fromPath,
				},
			})

			p.toolPath = android.OptionalPathForPath(outputFile)
		} else {
			if p.stripper.NeedsStrip(ctx) {
				stripped := android.PathForModuleOut(ctx, "stripped", fileName)
				p.stripper.StripExecutableOrSharedLib(ctx, in, stripped, flagsToStripFlags(flags))
				in = stripped
			}

			// Copy binaries to a name matching the final installed name
			ctx.Build(pctx, android.BuildParams{
				Rule:        android.CpExecutable,
				Description: "prebuilt",
				Output:      outputFile,
				Input:       in,
			})
		}

		return outputFile
	}

	return nil
}

func (p *prebuiltBinaryLinker) binary() bool {
	return true
}

// cc_prebuilt_binary installs a precompiled executable in srcs property in the
// device's directory, for both the host and device
func PrebuiltBinaryFactory() android.Module {
	module, _ := NewPrebuiltBinary(android.HostAndDeviceSupported)
	return module.Init()
}

func NewPrebuiltBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module, binary := newBinary(hod)
	module.compiler = nil

	prebuilt := &prebuiltBinaryLinker{
		binaryDecorator: binary,
	}
	module.linker = prebuilt
	module.installer = prebuilt

	module.AddProperties(&prebuilt.properties)

	android.InitPrebuiltModule(module, &prebuilt.properties.Srcs)
	return module, binary
}

type Sanitized struct {
	None struct {
		Srcs []string `android:"path,arch_variant"`
	} `android:"arch_variant"`
	Address struct {
		Srcs []string `android:"path,arch_variant"`
	} `android:"arch_variant"`
	Hwaddress struct {
		Srcs []string `android:"path,arch_variant"`
	} `android:"arch_variant"`
}

func srcsForSanitizer(sanitize *sanitize, sanitized Sanitized) []string {
	if sanitize == nil {
		return nil
	}
	if sanitize.isSanitizerEnabled(Asan) && sanitized.Address.Srcs != nil {
		return sanitized.Address.Srcs
	}
	if sanitize.isSanitizerEnabled(Hwasan) && sanitized.Hwaddress.Srcs != nil {
		return sanitized.Hwaddress.Srcs
	}
	return sanitized.None.Srcs
}

func (p *prebuiltLinker) sourceModuleName() string {
	return proptools.String(p.properties.Source_module_name)
}
