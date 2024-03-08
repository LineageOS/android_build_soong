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
)

func init() {
	android.RegisterModuleType("rust_prebuilt_library", PrebuiltLibraryFactory)
	android.RegisterModuleType("rust_prebuilt_dylib", PrebuiltDylibFactory)
	android.RegisterModuleType("rust_prebuilt_rlib", PrebuiltRlibFactory)
	android.RegisterModuleType("rust_prebuilt_proc_macro", PrebuiltProcMacroFactory)
}

type PrebuiltProperties struct {
	// path to the prebuilt file
	Srcs []string `android:"path,arch_variant"`
	// directories containing associated rlib dependencies
	Link_dirs []string `android:"path,arch_variant"`
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

func PrebuiltProcMacroFactory() android.Module {
	module, _ := NewPrebuiltProcMacro(android.HostSupportedNoCross)
	return module.Init()
}

type rustPrebuilt interface {
	prebuiltSrcs() []string
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

func prebuiltPath(ctx ModuleContext, prebuilt rustPrebuilt) android.Path {
	srcs := android.PathsForModuleSrc(ctx, prebuilt.prebuiltSrcs())
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

func addSrcSupplier(module android.PrebuiltInterface, prebuilt rustPrebuilt) {
	srcsSupplier := func(_ android.BaseModuleContext, _ android.Module) []string {
		return prebuilt.prebuiltSrcs()
	}
	android.InitPrebuiltModuleWithSrcSupplier(module, srcsSupplier, "srcs")
}

func NewPrebuiltLibrary(hod android.HostOrDeviceSupported) (*Module, *prebuiltLibraryDecorator) {
	module, library := NewRustLibrary(hod)
	library.BuildOnlyRust()
	library.setNoStdlibs()
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
	prebuilt.flagExporter.exportLinkDirs(android.PathsForModuleSrc(ctx, prebuilt.Properties.Link_dirs).Strings()...)
	prebuilt.flagExporter.setProvider(ctx)
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

func (prebuilt *prebuiltLibraryDecorator) prebuiltSrcs() []string {
	srcs := prebuilt.Properties.Srcs
	if prebuilt.rlib() {
		srcs = append(srcs, prebuilt.libraryDecorator.Properties.Rlib.Srcs...)
	}
	if prebuilt.dylib() {
		srcs = append(srcs, prebuilt.libraryDecorator.Properties.Dylib.Srcs...)
	}

	return srcs
}

func (prebuilt *prebuiltLibraryDecorator) prebuilt() *android.Prebuilt {
	return &prebuilt.Prebuilt
}

func (prebuilt *prebuiltProcMacroDecorator) prebuiltSrcs() []string {
	srcs := prebuilt.Properties.Srcs
	return srcs
}

func (prebuilt *prebuiltProcMacroDecorator) prebuilt() *android.Prebuilt {
	return &prebuilt.Prebuilt
}

func (prebuilt *prebuiltProcMacroDecorator) compilerProps() []interface{} {
	return append(prebuilt.procMacroDecorator.compilerProps(),
		&prebuilt.Properties)
}

func (prebuilt *prebuiltProcMacroDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	prebuilt.flagExporter.exportLinkDirs(android.PathsForModuleSrc(ctx, prebuilt.Properties.Link_dirs).Strings()...)
	prebuilt.flagExporter.setProvider(ctx)
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
