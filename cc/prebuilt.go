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
	"android/soong/android"
)

func init() {
	RegisterPrebuiltBuildComponents(android.InitRegistrationContext)
}

func RegisterPrebuiltBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("cc_prebuilt_library", PrebuiltLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_shared", PrebuiltSharedLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_library_static", PrebuiltStaticLibraryFactory)
	ctx.RegisterModuleType("cc_prebuilt_object", prebuiltObjectFactory)
	ctx.RegisterModuleType("cc_prebuilt_binary", prebuiltBinaryFactory)
}

type prebuiltLinkerInterface interface {
	Name(string) string
	prebuilt() *android.Prebuilt
}

type prebuiltLinkerProperties struct {

	// a prebuilt library or binary. Can reference a genrule module that generates an executable file.
	Srcs []string `android:"path,arch_variant"`

	// Check the prebuilt ELF files (e.g. DT_SONAME, DT_NEEDED, resolution of undefined
	// symbols, etc), default true.
	Check_elf_files *bool

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

	p.libraryDecorator.exportIncludes(ctx)
	p.libraryDecorator.reexportDirs(deps.ReexportedDirs...)
	p.libraryDecorator.reexportSystemDirs(deps.ReexportedSystemDirs...)
	p.libraryDecorator.reexportFlags(deps.ReexportedFlags...)
	p.libraryDecorator.reexportDeps(deps.ReexportedDeps...)
	p.libraryDecorator.addExportedGeneratedHeaders(deps.ReexportedGeneratedHeaders...)

	// TODO(ccross): verify shared library dependencies
	srcs := p.prebuiltSrcs()
	if len(srcs) > 0 {
		builderFlags := flagsToBuilderFlags(flags)

		if len(srcs) > 1 {
			ctx.PropertyErrorf("srcs", "multiple prebuilt source files")
			return nil
		}

		in := android.PathForModuleSrc(ctx, srcs[0])

		if p.static() {
			return in
		}

		if p.shared() {
			p.unstrippedOutputFile = in
			libName := p.libraryDecorator.getLibName(ctx) + flags.Toolchain.ShlibSuffix()
			outputFile := android.PathForModuleOut(ctx, libName)
			var implicits android.Paths

			if p.needsStrip(ctx) {
				stripped := android.PathForModuleOut(ctx, "stripped", libName)
				p.stripExecutableOrSharedLib(ctx, in, stripped, builderFlags)
				in = stripped
			}

			// Optimize out relinking against shared libraries whose interface hasn't changed by
			// depending on a table of contents file instead of the library itself.
			tocFile := android.PathForModuleOut(ctx, libName+".toc")
			p.tocFile = android.OptionalPathForPath(tocFile)
			TransformSharedObjectToToc(ctx, outputFile, tocFile, builderFlags)

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

			return outputFile
		}
	}

	return nil
}

func (p *prebuiltLibraryLinker) prebuiltSrcs() []string {
	srcs := p.properties.Srcs
	if p.static() {
		srcs = append(srcs, p.libraryDecorator.StaticProperties.Static.Srcs...)
	}
	if p.shared() {
		srcs = append(srcs, p.libraryDecorator.SharedProperties.Shared.Srcs...)
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
}

func (p *prebuiltLibraryLinker) skipInstall(mod *Module) {
	mod.ModuleBase.SkipInstall()
}

func NewPrebuiltLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module, library := NewLibrary(hod)
	module.compiler = nil

	prebuilt := &prebuiltLibraryLinker{
		libraryDecorator: library,
	}
	module.linker = prebuilt
	module.installer = prebuilt

	module.AddProperties(&prebuilt.properties)

	srcsSupplier := func() []string {
		return prebuilt.prebuiltSrcs()
	}

	android.InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "srcs")

	// Prebuilt libraries can be used in SDKs.
	android.InitSdkAwareModule(module)
	return module, library
}

// cc_prebuilt_library installs a precompiled shared library that are
// listed in the srcs property in the device's directory.
func PrebuiltLibraryFactory() android.Module {
	module, _ := NewPrebuiltLibrary(android.HostAndDeviceSupported)

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

func NewPrebuiltSharedLibrary(hod android.HostOrDeviceSupported) (*Module, *libraryDecorator) {
	module, library := NewPrebuiltLibrary(hod)
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
	module, library := NewPrebuiltLibrary(hod)
	library.BuildOnlyStatic()
	return module, library
}

type prebuiltObjectProperties struct {
	Srcs []string `android:"path,arch_variant"`
}

type prebuiltObjectLinker struct {
	android.Prebuilt
	objectLinker

	properties prebuiltObjectProperties
}

func (p *prebuiltObjectLinker) prebuilt() *android.Prebuilt {
	return &p.Prebuilt
}

var _ prebuiltLinkerInterface = (*prebuiltObjectLinker)(nil)

func (p *prebuiltObjectLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	if len(p.properties.Srcs) > 0 {
		return p.Prebuilt.SingleSourcePath(ctx)
	}
	return nil
}

func newPrebuiltObject() *Module {
	module := newObject()
	prebuilt := &prebuiltObjectLinker{
		objectLinker: objectLinker{
			baseLinker: NewBaseLinker(nil),
		},
	}
	module.linker = prebuilt
	module.AddProperties(&prebuilt.properties)
	android.InitPrebuiltModule(module, &prebuilt.properties.Srcs)
	android.InitSdkAwareModule(module)
	return module
}

func prebuiltObjectFactory() android.Module {
	module := newPrebuiltObject()
	return module.Init()
}

type prebuiltBinaryLinker struct {
	*binaryDecorator
	prebuiltLinker
}

var _ prebuiltLinkerInterface = (*prebuiltBinaryLinker)(nil)

func (p *prebuiltBinaryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	// TODO(ccross): verify shared library dependencies
	if len(p.properties.Srcs) > 0 {
		builderFlags := flagsToBuilderFlags(flags)

		fileName := p.getStem(ctx) + flags.Toolchain.ExecutableSuffix()
		in := p.Prebuilt.SingleSourcePath(ctx)

		p.unstrippedOutputFile = in

		if p.needsStrip(ctx) {
			stripped := android.PathForModuleOut(ctx, "stripped", fileName)
			p.stripExecutableOrSharedLib(ctx, in, stripped, builderFlags)
			in = stripped
		}

		// Copy binaries to a name matching the final installed name
		outputFile := android.PathForModuleOut(ctx, fileName)
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

// cc_prebuilt_binary installs a precompiled executable in srcs property in the
// device's directory.
func prebuiltBinaryFactory() android.Module {
	module, _ := NewPrebuiltBinary(android.HostAndDeviceSupported)
	return module.Init()
}

func NewPrebuiltBinary(hod android.HostOrDeviceSupported) (*Module, *binaryDecorator) {
	module, binary := NewBinary(hod)
	module.compiler = nil

	prebuilt := &prebuiltBinaryLinker{
		binaryDecorator: binary,
	}
	module.linker = prebuilt

	module.AddProperties(&prebuilt.properties)

	android.InitPrebuiltModule(module, &prebuilt.properties.Srcs)
	return module, binary
}
