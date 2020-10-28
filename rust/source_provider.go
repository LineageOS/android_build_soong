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
	"android/soong/android"
)

type SourceProviderProperties struct {
	// filename for the generated source file (<source_stem>.rs). This field is required.
	// The inherited "stem" property sets the output filename for the generated library variants only.
	Source_stem *string `android:"arch_variant"`

	// crate name, used for the library variant of this source provider. See additional details in rust_library.
	Crate_name string `android:"arch_variant"`
}

type BaseSourceProvider struct {
	Properties SourceProviderProperties

	// The first file in OutputFiles must be the library entry point.
	OutputFiles      android.Paths
	subAndroidMkOnce map[SubAndroidMkProvider]bool
	subName          string
}

var _ SourceProvider = (*BaseSourceProvider)(nil)

type SourceProvider interface {
	GenerateSource(ctx ModuleContext, deps PathDeps) android.Path
	Srcs() android.Paths
	SourceProviderProps() []interface{}
	SourceProviderDeps(ctx DepsContext, deps Deps) Deps
	setSubName(subName string)
	setOutputFiles(outputFiles android.Paths)
}

func (sp *BaseSourceProvider) Srcs() android.Paths {
	return sp.OutputFiles
}

func (sp *BaseSourceProvider) GenerateSource(ctx ModuleContext, deps PathDeps) android.Path {
	panic("BaseSourceProviderModule does not implement GenerateSource()")
}

func (sp *BaseSourceProvider) SourceProviderProps() []interface{} {
	return []interface{}{&sp.Properties}
}

func NewSourceProvider() *BaseSourceProvider {
	return &BaseSourceProvider{
		Properties: SourceProviderProperties{},
	}
}

func NewSourceProviderModule(hod android.HostOrDeviceSupported, sourceProvider SourceProvider, enableLints bool) *Module {
	_, library := NewRustLibrary(hod)
	library.BuildOnlyRust()
	library.sourceProvider = sourceProvider

	module := newModule(hod, android.MultilibBoth)
	module.sourceProvider = sourceProvider
	module.compiler = library

	if !enableLints {
		library.disableLints()
		module.disableClippy()
	}

	return module
}

func (sp *BaseSourceProvider) getStem(ctx android.ModuleContext) string {
	if String(sp.Properties.Source_stem) == "" {
		ctx.PropertyErrorf("source_stem",
			"source_stem property is undefined but required for rust_bindgen modules")
	}
	return String(sp.Properties.Source_stem)
}

func (sp *BaseSourceProvider) SourceProviderDeps(ctx DepsContext, deps Deps) Deps {
	return deps
}

func (sp *BaseSourceProvider) setSubName(subName string) {
	sp.subName = subName
}

func (sp *BaseSourceProvider) setOutputFiles(outputFiles android.Paths) {
	sp.OutputFiles = outputFiles
}
