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
}

type procMacroDecorator struct {
	*baseCompiler
	*flagExporter

	Properties ProcMacroCompilerProperties
}

type procMacroInterface interface {
	ProcMacro() bool
}

var _ compiler = (*procMacroDecorator)(nil)
var _ exportedFlagsProducer = (*procMacroDecorator)(nil)

func ProcMacroFactory() android.Module {
	module, _ := NewProcMacro(android.HostSupportedNoCross)
	return module.Init()
}

func NewProcMacro(hod android.HostOrDeviceSupported) (*Module, *procMacroDecorator) {
	module := newModule(hod, android.MultilibFirst)

	procMacro := &procMacroDecorator{
		baseCompiler: NewBaseCompiler("lib", "lib64", InstallInSystem),
		flagExporter: NewFlagExporter(),
	}

	// Don't sanitize procMacros
	module.sanitize = nil
	module.compiler = procMacro

	return module, procMacro
}

func (procMacro *procMacroDecorator) compilerProps() []interface{} {
	return append(procMacro.baseCompiler.compilerProps(),
		&procMacro.Properties)
}

func (procMacro *procMacroDecorator) compilerFlags(ctx ModuleContext, flags Flags) Flags {
	flags = procMacro.baseCompiler.compilerFlags(ctx, flags)
	flags.RustFlags = append(flags.RustFlags, "--extern proc_macro")
	return flags
}

func (procMacro *procMacroDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) buildOutput {
	fileName := procMacro.getStem(ctx) + ctx.toolchain().ProcMacroSuffix()
	outputFile := android.PathForModuleOut(ctx, fileName)
	srcPath := crateRootPath(ctx, procMacro)
	ret := TransformSrctoProcMacro(ctx, srcPath, deps, flags, outputFile)
	procMacro.baseCompiler.unstrippedOutputFile = outputFile
	return ret
}

func (procMacro *procMacroDecorator) getStem(ctx ModuleContext) string {
	stem := procMacro.baseCompiler.getStemWithoutSuffix(ctx)
	validateLibraryStem(ctx, stem, procMacro.crateName())

	return stem + String(procMacro.baseCompiler.Properties.Suffix)
}

func (procMacro *procMacroDecorator) autoDep(ctx android.BottomUpMutatorContext) autoDep {
	return rlibAutoDep
}

func (procMacro *procMacroDecorator) ProcMacro() bool {
	return true
}

func (procMacro *procMacroDecorator) everInstallable() bool {
	// Proc_macros are never installed
	return false
}
