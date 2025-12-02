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
	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

func init() {
	android.RegisterModuleType("rust_prebuilt_library", PrebuiltLibraryFactory)
	android.RegisterModuleType("rust_prebuilt_dylib", PrebuiltDylibFactory)
	android.RegisterModuleType("rust_prebuilt_rlib", PrebuiltRlibFactory)
	android.RegisterModuleType("rust_prebuilt_proc_macro", PrebuiltProcMacroFactory)
	android.RegisterModuleType("rust_prebuilt_binary", PrebuiltBinaryFactory)
}

type PrebuiltProperties struct {
	// path to the prebuilt file
	Srcs proptools.Configurable[[]string] `android:"path,arch_variant"`
	// directories containing associated rlib dependencies
	Link_dirs []string `android:"path,arch_variant"`

	Force_use_prebuilt *bool `android:"arch_variant"`
}

type prebuiltLibraryDecorator struct {
	android.Prebuilt

	*libraryDecorator
	Properties PrebuiltProperties
}

type prebuiltProcMacroDecorator struct {
	android.Prebuilt

	*procMacroDecorator
	Properties PrebuiltProperties
}

type prebuiltBinaryDecorator struct {
	android.Prebuilt

	*binaryDecorator
	Properties PrebuiltProperties
}

func PrebuiltProcMacroFactory() android.Module {
	module, _ := NewPrebuiltProcMacro(android.HostSupportedNoCross)
	return module.Init()
}

type rustPrebuilt interface {
	prebuiltSrcs(ctx android.BaseModuleContext) []string
	prebuilt() *android.Prebuilt
}

func NewPrebuiltProcMacro(hod android.HostOrDeviceSupported) (*Module, *prebuiltProcMacroDecorator) {
	module, library := NewProcMacro(hod)
	prebuilt := &prebuiltProcMacroDecorator{
		procMacroDecorator: library,
	}
	module.compiler = prebuilt

	addSrcSupplier(module, prebuilt)

	return module, prebuilt
}

var _ compiler = (*prebuiltLibraryDecorator)(nil)
var _ exportedFlagsProducer = (*prebuiltLibraryDecorator)(nil)
var _ rustPrebuilt = (*prebuiltLibraryDecorator)(nil)

var _ compiler = (*prebuiltProcMacroDecorator)(nil)
var _ exportedFlagsProducer = (*prebuiltProcMacroDecorator)(nil)
var _ rustPrebuilt = (*prebuiltProcMacroDecorator)(nil)

var _ compiler = (*prebuiltBinaryDecorator)(nil)
var _ rustPrebuilt = (*prebuiltBinaryDecorator)(nil)

func prebuiltPath(ctx ModuleContext, prebuilt rustPrebuilt) android.Path {
	srcs := android.PathsForModuleSrc(ctx, prebuilt.prebuiltSrcs(ctx))
	if len(srcs) == 0 {
		ctx.PropertyErrorf("srcs", "srcs must not be empty")
	}
	if len(srcs) > 1 {
		ctx.PropertyErrorf("srcs", "prebuilt libraries can only have one entry in srcs (the prebuilt path)")
	}
	return srcs[0]
}

func PrebuiltLibraryFactory() android.Module {
	module, _ := NewPrebuiltLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

func PrebuiltDylibFactory() android.Module {
	module, _ := NewPrebuiltDylib(android.HostAndDeviceSupported)
	return module.Init()
}

func PrebuiltRlibFactory() android.Module {
	module, _ := NewPrebuiltRlib(android.HostAndDeviceSupported)
	return module.Init()
}

func PrebuiltBinaryFactory() android.Module {
	module, _ := NewPrebuiltBinary(android.HostAndDeviceSupported)
	return module.Init()
}

func addSrcSupplier(module android.PrebuiltInterface, prebuilt rustPrebuilt) {
	srcsSupplier := func(ctx android.BaseModuleContext, _ android.Module) []string {
		return prebuilt.prebuiltSrcs(ctx)
	}
	android.InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "srcs")
}

func NewPrebuiltLibrary(hod android.HostOrDeviceSupported) (*Module, *prebuiltLibraryDecorator) {
	module, library := NewRustLibrary(hod)
	library.BuildOnlyRust()
	library.setNoStdlibs()
	library.setSysroot()
	prebuilt := &prebuiltLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = prebuilt

	addSrcSupplier(module, prebuilt)

	return module, prebuilt
}

func NewPrebuiltDylib(hod android.HostOrDeviceSupported) (*Module, *prebuiltLibraryDecorator) {
	module, library := NewRustLibrary(hod)
	library.BuildOnlyDylib()
	library.setNoStdlibs()
	library.setSysroot()
	prebuilt := &prebuiltLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = prebuilt

	addSrcSupplier(module, prebuilt)

	return module, prebuilt
}

func NewPrebuiltRlib(hod android.HostOrDeviceSupported) (*Module, *prebuiltLibraryDecorator) {
	module, library := NewRustLibrary(hod)
	library.BuildOnlyRlib()
	library.setNoStdlibs()
	library.setSysroot()
	prebuilt := &prebuiltLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = prebuilt

	addSrcSupplier(module, prebuilt)

	return module, prebuilt
}

func (prebuilt *prebuiltLibraryDecorator) compilerProps() []interface{} {
	return append(prebuilt.libraryDecorator.compilerProps(),
		&prebuilt.Properties)
}

func (prebuilt *prebuiltLibraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	linkDirs := android.PathsForModuleSrc(ctx, prebuilt.Properties.Link_dirs).Strings()
	linkDirs = android.FirstUniqueInPlace(linkDirs)
	var linkDirsDeps []android.Path
	for _, linkDir := range linkDirs {
		linkDirsDeps = append(linkDirsDeps, android.PathsForModuleSrc(ctx, []string{linkDir + "/*.rlib"})...)
	}
	prebuilt.flagExporter.exportLinkDirs(linkDirs, linkDirsDeps)
	prebuilt.flagExporter.setRustProvider(ctx)
	srcPath := prebuiltPath(ctx, prebuilt)
	prebuilt.baseCompiler.unstrippedOutputFile = srcPath
	return buildOutput{outputFile: srcPath}
}

func (prebuilt *prebuiltLibraryDecorator) rustdoc(ctx ModuleContext, flags Flags,
	deps PathDeps) android.OptionalPath {

	return android.OptionalPath{}
}

func (prebuilt *prebuiltLibraryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = prebuilt.baseCompiler.compilerDeps(ctx, deps)
	return deps
}

func (prebuilt *prebuiltLibraryDecorator) nativeCoverage() bool {
	return false
}

func (prebuilt *prebuiltLibraryDecorator) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	return prebuilt.Properties.Srcs.GetOrDefault(ctx, nil)
}

func (prebuilt *prebuiltLibraryDecorator) prebuilt() *android.Prebuilt {
	return &prebuilt.Prebuilt
}

func (prebuilt *prebuiltLibraryDecorator) crateRootPath(ctx ModuleContext) android.Path {
	if prebuilt.baseCompiler.Properties.Crate_root == nil {
		return srcPathFromModuleSrcs(ctx, prebuilt.prebuiltSrcs(ctx))
	} else {
		return android.PathForModuleSrc(ctx, *prebuilt.baseCompiler.Properties.Crate_root)
	}
}

func (prebuilt *prebuiltProcMacroDecorator) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	return prebuilt.Properties.Srcs.GetOrDefault(ctx, nil)
}

func (prebuilt *prebuiltProcMacroDecorator) prebuilt() *android.Prebuilt {
	return &prebuilt.Prebuilt
}

func (prebuilt *prebuiltProcMacroDecorator) compilerProps() []interface{} {
	return append(prebuilt.procMacroDecorator.compilerProps(),
		&prebuilt.Properties)
}

func (prebuilt *prebuiltProcMacroDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	linkDirs := android.PathsForModuleSrc(ctx, prebuilt.Properties.Link_dirs).Strings()
	linkDirs = android.FirstUniqueInPlace(linkDirs)
	var linkDirsDeps []android.Path
	for _, linkDir := range linkDirs {
		linkDirsDeps = append(linkDirsDeps, android.PathsForModuleSrc(ctx, []string{linkDir + "/*.rlib"})...)
	}
	prebuilt.flagExporter.exportLinkDirs(linkDirs, linkDirsDeps)
	prebuilt.flagExporter.setRustProvider(ctx)
	srcPath := prebuiltPath(ctx, prebuilt)
	prebuilt.baseCompiler.unstrippedOutputFile = srcPath
	return buildOutput{outputFile: srcPath}
}

func (prebuilt *prebuiltProcMacroDecorator) rustdoc(ctx ModuleContext, flags Flags,
	deps PathDeps) android.OptionalPath {

	return android.OptionalPath{}
}

func (prebuilt *prebuiltProcMacroDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = prebuilt.baseCompiler.compilerDeps(ctx, deps)
	return deps
}

func (prebuilt *prebuiltProcMacroDecorator) nativeCoverage() bool {
	return false
}

func (prebuilt *prebuiltProcMacroDecorator) crateRootPath(ctx ModuleContext) android.Path {
	if prebuilt.baseCompiler.Properties.Crate_root == nil {
		return srcPathFromModuleSrcs(ctx, prebuilt.prebuiltSrcs(ctx))
	} else {
		return android.PathForModuleSrc(ctx, *prebuilt.baseCompiler.Properties.Crate_root)
	}
}

func NewPrebuiltBinary(hod android.HostOrDeviceSupported) (*Module, *prebuiltBinaryDecorator) {
	module, binary := NewRustBinary(hod)
	binary.setNoStdlibs()

	prebuilt := &prebuiltBinaryDecorator{
		binaryDecorator: binary,
	}

	module.compiler = prebuilt

	addSrcSupplier(module, prebuilt)

	return module, prebuilt
}

func (prebuilt *prebuiltBinaryDecorator) prebuiltSrcs(ctx android.BaseModuleContext) []string {
	return prebuilt.Properties.Srcs.GetOrDefault(ctx, nil)
}

func (prebuilt *prebuiltBinaryDecorator) prebuilt() *android.Prebuilt {
	return &prebuilt.Prebuilt
}

func (prebuilt *prebuiltBinaryDecorator) compilerProps() []interface{} {
	return append(prebuilt.binaryDecorator.compilerProps(), &prebuilt.Properties)
}

func (prebuilt *prebuiltBinaryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	srcPath := prebuiltPath(ctx, prebuilt)
	fileName := prebuilt.getStem(ctx) + ctx.toolchain().ExecutableSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)

	unstrippedOutputFile := outputFile

	if prebuilt.stripper.NeedsStrip(ctx) {
		unstrippedOutputFile = android.PathForModuleOut(ctx, "unstripped", fileName)

		ctx.Build(pctx, android.BuildParams{
			Rule:   android.CpExecutable,
			Input:  srcPath,
			Output: unstrippedOutputFile,
			Args: map[string]string{
				"cpFlags": "-L",
			},
		})

		prebuilt.stripper.StripExecutableOrSharedLib(ctx, unstrippedOutputFile, outputFile)
		prebuilt.baseCompiler.strippedOutputFile = android.OptionalPathForPath(outputFile)
	} else {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.CpExecutable,
			Input:  srcPath,
			Output: unstrippedOutputFile,
			Args: map[string]string{
				"cpFlags": "-L",
			},
		})
	}

	prebuilt.baseCompiler.unstrippedOutputFile = unstrippedOutputFile

	return buildOutput{outputFile: outputFile}
}

func (prebuilt *prebuiltBinaryDecorator) rustdoc(ctx ModuleContext, flags Flags,
	deps PathDeps) android.OptionalPath {

	return android.OptionalPath{}
}

func (prebuilt *prebuiltBinaryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = prebuilt.baseCompiler.compilerDeps(ctx, deps)
	return deps
}

func (prebuilt *prebuiltBinaryDecorator) nativeCoverage() bool {
	return false
}

func (prebuilt *prebuiltBinaryDecorator) crateRootPath(ctx ModuleContext) android.Path {
	if prebuilt.baseCompiler.Properties.Crate_root == nil {
		return srcPathFromModuleSrcs(ctx, prebuilt.prebuiltSrcs(ctx))
	} else {
		return android.PathForModuleSrc(ctx, *prebuilt.baseCompiler.Properties.Crate_root)
	}
}
