// Copyright 2017 Google Inc. All rights reserved.
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

// TODO(ccross): protos are often used to communicate between multiple modules.  If the only
// way to convert a proto to source is to reference it as a source file, and external modules cannot
// reference source files in other modules, then every module that owns a proto file will need to
// export a library for every type of external user (lite vs. full, c vs. c++ vs. java).  It would
// be better to support a proto module type that exported a proto file along with some include dirs,
// and then external modules could depend on the proto module but use their own settings to
// generate the source.

func ProtoFlags(ctx ModuleContext, p *ProtoProperties) []string {
	// -I . must come first, it affects where protoc places the output files.
	protoFlags := []string{"-I ."}

	if len(p.Proto.Local_include_dirs) > 0 {
		localProtoIncludeDirs := PathsForModuleSrc(ctx, p.Proto.Local_include_dirs)
		protoFlags = append(protoFlags, JoinWithPrefix(localProtoIncludeDirs.Strings(), "-I"))
	}
	if len(p.Proto.Include_dirs) > 0 {
		rootProtoIncludeDirs := PathsForSource(ctx, p.Proto.Include_dirs)
		protoFlags = append(protoFlags, JoinWithPrefix(rootProtoIncludeDirs.Strings(), "-I"))
	}

	return protoFlags
}

// ProtoDir returns the module's "gen/proto" directory
func ProtoDir(ctx ModuleContext) ModuleGenPath {
	return PathForModuleGen(ctx, "proto")
}

// ProtoSubDir returns the module's "gen/proto/path/to/module" directory
func ProtoSubDir(ctx ModuleContext) ModuleGenPath {
	return PathForModuleGen(ctx, "proto", ctx.ModuleDir())
}

type ProtoProperties struct {
	Proto struct {
		// Proto generator type.  C++: full or lite.  Java: micro, nano, stream, or lite.
		Type *string `android:"arch_variant"`

		// list of directories that will be added to the protoc include paths.
		Include_dirs []string

		// list of directories relative to the bp file that will
		// be added to the protoc include paths.
		Local_include_dirs []string
	} `android:"arch_variant"`
}
