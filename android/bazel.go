// Copyright 2021 Google Inc. All rights reserved.
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

package android

import "android/soong/bazel"

// BazelModuleBase contains the property structs with metadata for modules which can be converted to
// Bazel.
type BazelModuleBase struct {
	bazelProperties bazel.Properties
}

// Bazelable is specifies the interface for modules that can be converted to Bazel.
type Bazelable interface {
	bazelProps() *bazel.Properties
	GetBazelLabel() string
	ConvertWithBp2build() bool
}

// BazelModule is a lightweight wrapper interface around Module for Bazel-convertible modules.
type BazelModule interface {
	Module
	Bazelable
}

// InitBazelModule is a wrapper function that decorates a BazelModule with Bazel-conversion
// properties.
func InitBazelModule(module BazelModule) {
	module.AddProperties(module.bazelProps())
}

// bazelProps returns the Bazel properties for the given BazelModuleBase.
func (b *BazelModuleBase) bazelProps() *bazel.Properties {
	return &b.bazelProperties
}

// GetBazelLabel returns the Bazel label for the given BazelModuleBase.
func (b *BazelModuleBase) GetBazelLabel() string {
	return b.bazelProperties.Bazel_module.Label
}

// ConvertWithBp2build returns whether the given BazelModuleBase should be converted with bp2build.
func (b *BazelModuleBase) ConvertWithBp2build() bool {
	return b.bazelProperties.Bazel_module.Bp2build_available
}
