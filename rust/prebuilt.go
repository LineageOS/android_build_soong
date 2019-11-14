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
	android.RegisterModuleType("rust_prebuilt_dylib", PrebuiltDylibFactory)
}

type PrebuiltProperties struct {
	// path to the prebuilt file
	Srcs []string `android:"path,arch_variant"`
}

type prebuiltLibraryDecorator struct {
	*libraryDecorator
	Properties PrebuiltProperties
}

var _ compiler = (*prebuiltLibraryDecorator)(nil)

func PrebuiltDylibFactory() android.Module {
	module, _ := NewPrebuiltDylib(android.HostAndDeviceSupported)
	return module.Init()
}

func NewPrebuiltDylib(hod android.HostOrDeviceSupported) (*Module, *prebuiltLibraryDecorator) {
	module, library := NewRustLibrary(hod)
	library.BuildOnlyDylib()
	library.setNoStdlibs()
	library.setDylib()
	prebuilt := &prebuiltLibraryDecorator{
		libraryDecorator: library,
	}
	module.compiler = prebuilt
	module.AddProperties(&library.Properties)
	return module, prebuilt
}

func (prebuilt *prebuiltLibraryDecorator) compilerProps() []interface{} {
	return append(prebuilt.baseCompiler.compilerProps(),
		&prebuilt.Properties)
}

func (prebuilt *prebuiltLibraryDecorator) compile(ctx ModuleContext, flags Flags, deps PathDeps) android.Path {
	srcPath := srcPathFromModuleSrcs(ctx, prebuilt.Properties.Srcs)

	prebuilt.unstrippedOutputFile = srcPath

	return srcPath
}

func (prebuilt *prebuiltLibraryDecorator) compilerDeps(ctx DepsContext, deps Deps) Deps {
	deps = prebuilt.baseCompiler.compilerDeps(ctx, deps)
	return deps
}
