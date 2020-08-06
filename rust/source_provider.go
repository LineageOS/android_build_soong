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

type baseSourceProvider struct {
	Properties SourceProviderProperties

	outputFile       android.Path
	subAndroidMkOnce map[subAndroidMkProvider]bool
	subName          string
}

var _ SourceProvider = (*baseSourceProvider)(nil)

type SourceProvider interface {
	generateSource(ctx android.ModuleContext, deps PathDeps) android.Path
	Srcs() android.Paths
	sourceProviderProps() []interface{}
	sourceProviderDeps(ctx DepsContext, deps Deps) Deps
	setSubName(subName string)
}

func (sp *baseSourceProvider) Srcs() android.Paths {
	return android.Paths{sp.outputFile}
}

func (sp *baseSourceProvider) generateSource(ctx android.ModuleContext, deps PathDeps) android.Path {
	panic("baseSourceProviderModule does not implement generateSource()")
}

func (sp *baseSourceProvider) sourceProviderProps() []interface{} {
	return []interface{}{&sp.Properties}
}

func NewSourceProvider() *baseSourceProvider {
	return &baseSourceProvider{
		Properties: SourceProviderProperties{},
	}
}

func (sp *baseSourceProvider) getStem(ctx android.ModuleContext) string {
	if String(sp.Properties.Source_stem) == "" {
		ctx.PropertyErrorf("source_stem",
			"source_stem property is undefined but required for rust_bindgen modules")
	}
	return String(sp.Properties.Source_stem)
}

func (sp *baseSourceProvider) sourceProviderDeps(ctx DepsContext, deps Deps) Deps {
	return deps
}

func (sp *baseSourceProvider) setSubName(subName string) {
	sp.subName = subName
}
