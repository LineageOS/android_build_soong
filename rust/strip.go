// Copyright 2020 Google Inc. All rights reserved.
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
	"android/soong/cc"
)

// Stripper defines the stripping actions and properties for a module. The Rust
// implementation reuses the C++ implementation.
type Stripper struct {
	cc.Stripper
}

// StripExecutableOrSharedLib strips a binary or shared library from its debug
// symbols and other debug information.
func (s *Stripper) StripExecutableOrSharedLib(ctx ModuleContext, in android.Path, out android.ModuleOutPath) {
	ccFlags := cc.StripFlags{Toolchain: ctx.RustModule().ccToolchain(ctx)}
	s.Stripper.StripExecutableOrSharedLib(ctx, in, out, ccFlags)
}
