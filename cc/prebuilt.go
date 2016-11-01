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

	"github.com/google/blueprint"
)

func init() {
	android.RegisterModuleType("cc_prebuilt_shared_library", prebuiltSharedLibraryFactory)
}

type prebuiltLinkerInterface interface {
	Name(string) string
	prebuilt() *android.Prebuilt
}

type prebuiltLibraryLinker struct {
	*libraryDecorator
	android.Prebuilt
}

var _ prebuiltLinkerInterface = (*prebuiltLibraryLinker)(nil)

func (p *prebuiltLibraryLinker) prebuilt() *android.Prebuilt {
	return &p.Prebuilt
}

func (p *prebuiltLibraryLinker) linkerProps() []interface{} {
	props := p.libraryDecorator.linkerProps()
	return append(props, &p.Prebuilt.Properties)
}

func (p *prebuiltLibraryLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {
	// TODO(ccross): verify shared library dependencies
	if len(p.Prebuilt.Properties.Srcs) > 0 {
		p.libraryDecorator.exportIncludes(ctx, "-I")
		p.libraryDecorator.reexportFlags(deps.ReexportedFlags)
		p.libraryDecorator.reexportDeps(deps.ReexportedFlagsDeps)
		// TODO(ccross): .toc optimization, stripping, packing
		return p.Prebuilt.Path(ctx)
	}

	return nil
}

func prebuiltSharedLibraryFactory() (blueprint.Module, []interface{}) {
	module, library := NewLibrary(android.HostAndDeviceSupported, true, false)
	module.compiler = nil

	prebuilt := &prebuiltLibraryLinker{
		libraryDecorator: library,
	}
	module.linker = prebuilt

	return module.Init()
}
