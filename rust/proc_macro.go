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
	android.RegisterModuleType("rust_proc_macro", ProcMacroFactory)
}

type ProcMacroCompilerProperties struct {
	// path to the source file that is the main entry point of the program (e.g. src/lib.rs)
	Srcs []string `android:"path,arch_variant"`

	// set name of the procMacro
	Stem   *string `android:"arch_variant"`
	Suffix *string `android:"arch_variant"`
}

type procMacroDecorator struct {
	*baseCompiler

	Properties           ProcMacroCompilerProperties
	distFile             android.OptionalPath
	unstrippedOutputFile android.Path
}

type procMacroInterface interface {
}

var _ compiler = (*procMacroDecorator)(nil)

func ProcMacroFactory() android.Module {
	module, _ := NewProcMacro(android.HostSupportedNoCross)
	return module.Init()
}

func NewProcMacro(hod android.HostOrDeviceSupported) (*Module, *procMacroDecorator) {
	module := newModule(hod, android.MultilibFirst)

	procMacro := &procMacroDecorator{
		baseCompiler: NewBaseCompiler("lib", "lib64", InstallInSystem),
	}

	module.compiler = procMacro

	return module, procMacro
}

func (procMacro *procMacroDecorator) compilerProps() []interface{} {
	return append(procMacro.baseCompiler.compilerProps(),
		&procMacro.Properties)
}

func (procMacro *procMacroDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	fileName := procMacro.getStem(ctx) + ctx.toolchain().ProcMacroSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)

	srcPath := srcPathFromModuleSrcs(ctx, procMacro.Properties.Srcs)

	procMacro.unstrippedOutputFile = outputFile

	TransformSrctoProcMacro(ctx, srcPath, deps, flags, outputFile, deps.linkDirs)
	return outputFile
}

func (procMacro *procMacroDecorator) getStem(ctx ModuleContext) string {
	stem := procMacro.baseCompiler.getStemWithoutSuffix(ctx)
	validateLibraryStem(ctx, stem, procMacro.crateName())

	return stem + String(procMacro.baseCompiler.Properties.Suffix)
}
