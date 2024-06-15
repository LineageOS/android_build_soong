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
	"fmt"
	"strings"

	"android/soong/android"
	"android/soong/cc"
)

var (
	defaultProtobufFlags = []string{""}
)

const (
	grpcSuffix = "_grpc"
)

type PluginType int

func init() {
	android.RegisterModuleType("rust_protobuf", RustProtobufFactory)
	android.RegisterModuleType("rust_protobuf_host", RustProtobufHostFactory)
}

var _ SourceProvider = (*protobufDecorator)(nil)

type ProtobufProperties struct {
	// List of relative paths to proto files that will be used to generate the source.
	// Either this or grpc_protos must be defined.
	Protos []string `android:"path,arch_variant"`

	// List of relative paths to GRPC-containing proto files that will be used to generate the source.
	// Either this or protos must be defined.
	Grpc_protos []string `android:"path,arch_variant"`

	// List of additional flags to pass to aprotoc
	Proto_flags []string `android:"arch_variant"`

	// List of libraries which export include paths required for this module
	Header_libs []string `android:"arch_variant,variant_prepend"`

	// List of exported include paths containing proto files for dependent rust_protobuf modules.
	Exported_include_dirs []string
}

type protobufDecorator struct {
	*BaseSourceProvider

	Properties       ProtobufProperties
	protoNames       []string
	additionalCrates []string
	grpcNames        []string

	grpcProtoFlags android.ProtoFlags
	protoFlags     android.ProtoFlags
}

func (proto *protobufDecorator) GenerateSource(ctx ModuleContext, deps PathDeps) android.Path {
	var protoFlags android.ProtoFlags
	var grpcProtoFlags android.ProtoFlags
	var commonProtoFlags []string

	outDir := android.PathForModuleOut(ctx)
	protoFiles := android.PathsForModuleSrc(ctx, proto.Properties.Protos)
	grpcFiles := android.PathsForModuleSrc(ctx, proto.Properties.Grpc_protos)

	protoPluginPath := ctx.Config().HostToolPath(ctx, "protoc-gen-rust")

	commonProtoFlags = append(commonProtoFlags, defaultProtobufFlags...)
	commonProtoFlags = append(commonProtoFlags, proto.Properties.Proto_flags...)
	commonProtoFlags = append(commonProtoFlags, "--plugin=protoc-gen-rust="+protoPluginPath.String())

	if len(protoFiles) > 0 {
		protoFlags.OutTypeFlag = "--rust_out"
		protoFlags.Flags = append(protoFlags.Flags, commonProtoFlags...)

		protoFlags.Deps = append(protoFlags.Deps, protoPluginPath)
	}

	if len(grpcFiles) > 0 {
		grpcPath := ctx.Config().HostToolPath(ctx, "grpc_rust_plugin")

		grpcProtoFlags.OutTypeFlag = "--rust_out"
		grpcProtoFlags.Flags = append(grpcProtoFlags.Flags, "--grpc_out="+outDir.String())
		grpcProtoFlags.Flags = append(grpcProtoFlags.Flags, "--plugin=protoc-gen-grpc="+grpcPath.String())
		grpcProtoFlags.Flags = append(grpcProtoFlags.Flags, commonProtoFlags...)

		grpcProtoFlags.Deps = append(grpcProtoFlags.Deps, grpcPath, protoPluginPath)
	}

	if len(protoFiles) == 0 && len(grpcFiles) == 0 {
		ctx.PropertyErrorf("protos",
			"at least one protobuf must be defined in either protos or grpc_protos.")
	}

	// Add exported dependency include paths
	for _, include := range deps.depIncludePaths {
		protoFlags.Flags = append(protoFlags.Flags, "-I"+include.String())
		grpcProtoFlags.Flags = append(grpcProtoFlags.Flags, "-I"+include.String())
	}

	stem := proto.BaseSourceProvider.getStem(ctx)

	// The mod_stem.rs file is used to avoid collisions if this is not included as a crate.
	stemFile := android.PathForModuleOut(ctx, "mod_"+stem+".rs")

	// stemFile must be first here as the first path in BaseSourceProvider.OutputFiles is the library entry-point.
	var outputs android.WritablePaths

	rule := android.NewRuleBuilder(pctx, ctx)

	for _, protoFile := range protoFiles {
		// Since we're iterating over the protoFiles already, make sure they're not redeclared in grpcFiles
		if android.InList(protoFile.String(), grpcFiles.Strings()) {
			ctx.PropertyErrorf("protos",
				"A proto can only be added once to either grpc_protos or protos. %q is declared in both properties",
				protoFile.String())
		}

		protoName := strings.TrimSuffix(protoFile.Base(), ".proto")
		proto.protoNames = append(proto.protoNames, protoName)

		protoOut := android.PathForModuleOut(ctx, protoName+".rs")
		depFile := android.PathForModuleOut(ctx, protoName+".d")

		ruleOutputs := android.WritablePaths{protoOut, depFile}

		android.ProtoRule(rule, protoFile, protoFlags, protoFlags.Deps, outDir, depFile, ruleOutputs)
		outputs = append(outputs, ruleOutputs...)
	}

	for _, grpcFile := range grpcFiles {
		grpcName := strings.TrimSuffix(grpcFile.Base(), ".proto")
		proto.grpcNames = append(proto.grpcNames, grpcName)

		// GRPC protos produce two files, a proto.rs and a proto_grpc.rs
		protoOut := android.WritablePath(android.PathForModuleOut(ctx, grpcName+".rs"))
		grpcOut := android.WritablePath(android.PathForModuleOut(ctx, grpcName+grpcSuffix+".rs"))
		depFile := android.PathForModuleOut(ctx, grpcName+".d")

		ruleOutputs := android.WritablePaths{protoOut, grpcOut, depFile}

		android.ProtoRule(rule, grpcFile, grpcProtoFlags, grpcProtoFlags.Deps, outDir, depFile, ruleOutputs)
		outputs = append(outputs, ruleOutputs...)
	}

	// Check that all proto base filenames are unique as outputs are written to the same directory.
	baseFilenames := append(proto.protoNames, proto.grpcNames...)
	if len(baseFilenames) != len(android.FirstUniqueStrings(baseFilenames)) {
		ctx.PropertyErrorf("protos", "proto filenames must be unique across  'protos' and 'grpc_protos' "+
			"to be used in the same rust_protobuf module. For example, foo.proto and src/foo.proto will conflict.")
	}

	android.WriteFileRule(ctx, stemFile, proto.genModFileContents())

	rule.Build("protoc_"+ctx.ModuleName(), "protoc "+ctx.ModuleName())

	// stemFile must be first here as the first path in BaseSourceProvider.OutputFiles is the library entry-point.
	proto.BaseSourceProvider.OutputFiles = append(android.Paths{stemFile}, outputs.Paths()...)

	android.SetProvider(ctx, cc.FlagExporterInfoProvider, cc.FlagExporterInfo{
		IncludeDirs: android.PathsForModuleSrc(ctx, proto.Properties.Exported_include_dirs),
	})

	// mod_stem.rs is the entry-point for our library modules, so this is what we return.
	return stemFile
}

func (proto *protobufDecorator) genModFileContents() string {
	lines := []string{
		"// @Soong generated Source",
	}

	for _, protoName := range proto.protoNames {
		lines = append(lines, fmt.Sprintf("pub mod %s;", protoName))
	}

	for _, crate := range proto.additionalCrates {
		lines = append(lines, fmt.Sprintf("pub use %s::*;", crate))

	}

	for _, grpcName := range proto.grpcNames {
		lines = append(lines, fmt.Sprintf("pub mod %s;", grpcName))
		lines = append(lines, fmt.Sprintf("pub mod %s%s;", grpcName, grpcSuffix))
	}
	if len(proto.grpcNames) > 0 {
		lines = append(
			lines,
			"pub mod empty {",
			"    pub use protobuf::well_known_types::empty::Empty;",
			"}")
	}

	return strings.Join(lines, "\n")
}

func (proto *protobufDecorator) SourceProviderProps() []interface{} {
	return append(proto.BaseSourceProvider.SourceProviderProps(), &proto.Properties)
}

func (proto *protobufDecorator) SourceProviderDeps(ctx DepsContext, deps Deps) Deps {
	deps = proto.BaseSourceProvider.SourceProviderDeps(ctx, deps)
	deps.Rustlibs = append(deps.Rustlibs, "libprotobuf")
	deps.HeaderLibs = append(deps.SharedLibs, proto.Properties.Header_libs...)

	if len(proto.Properties.Grpc_protos) > 0 {
		deps.Rustlibs = append(deps.Rustlibs, "libgrpcio", "libfutures")
		deps.HeaderLibs = append(deps.HeaderLibs, "libprotobuf-cpp-full")
	}

	return deps
}

// rust_protobuf generates protobuf rust code from the provided proto file. This uses the protoc-gen-rust plugin for
// protoc. Additional flags to the protoc command can be passed via the proto_flags property. This module type will
// create library variants that can be used as a crate dependency by adding it to the rlibs and rustlibs
// properties of other modules.
func RustProtobufFactory() android.Module {
	module, _ := NewRustProtobuf(android.HostAndDeviceSupported)
	return module.Init()
}

// A host-only variant of rust_protobuf. Refer to rust_protobuf for more details.
func RustProtobufHostFactory() android.Module {
	module, _ := NewRustProtobuf(android.HostSupported)
	return module.Init()
}

func NewRustProtobuf(hod android.HostOrDeviceSupported) (*Module, *protobufDecorator) {
	protobuf := &protobufDecorator{
		BaseSourceProvider: NewSourceProvider(),
		Properties:         ProtobufProperties{},
	}

	module := NewSourceProviderModule(hod, protobuf, false, false)

	return module, protobuf
}
