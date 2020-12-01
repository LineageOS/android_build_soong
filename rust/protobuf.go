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
)

var (
	defaultProtobufFlags = []string{""}
)

const (
	grpcSuffix = "_grpc"
)

type PluginType int

const (
	Protobuf PluginType = iota
	Grpc
)

func init() {
	android.RegisterModuleType("rust_protobuf", RustProtobufFactory)
	android.RegisterModuleType("rust_protobuf_host", RustProtobufHostFactory)
	android.RegisterModuleType("rust_grpcio", RustGrpcioFactory)
	android.RegisterModuleType("rust_grpcio_host", RustGrpcioHostFactory)
}

var _ SourceProvider = (*protobufDecorator)(nil)

type ProtobufProperties struct {
	// List of realtive paths to proto files that will be used to generate the source
	Protos []string `android:"path,arch_variant"`

	// List of additional flags to pass to aprotoc
	Proto_flags []string `android:"arch_variant"`

	// List of libraries which export include paths required for this module
	Header_libs []string `android:"arch_variant,variant_prepend"`
}

type protobufDecorator struct {
	*BaseSourceProvider

	Properties ProtobufProperties
	plugin     PluginType
}

func (proto *protobufDecorator) GenerateSource(ctx ModuleContext, deps PathDeps) android.Path {
	var protoFlags android.ProtoFlags
	var pluginPaths android.Paths
	var protoNames []string

	protoFlags.OutTypeFlag = "--rust_out"
	outDir := android.PathForModuleOut(ctx)

	pluginPaths, protoFlags = proto.setupPlugin(ctx, protoFlags, outDir)

	protoFlags.Flags = append(protoFlags.Flags, defaultProtobufFlags...)
	protoFlags.Flags = append(protoFlags.Flags, proto.Properties.Proto_flags...)

	protoFlags.Deps = append(protoFlags.Deps, pluginPaths...)

	protoFiles := android.PathsForModuleSrc(ctx, proto.Properties.Protos)

	// Add exported dependency include paths
	for _, include := range deps.depIncludePaths {
		protoFlags.Flags = append(protoFlags.Flags, "-I"+include.String())
	}

	stem := proto.BaseSourceProvider.getStem(ctx)

	// The mod_stem.rs file is used to avoid collisions if this is not included as a crate.
	stemFile := android.PathForModuleOut(ctx, "mod_"+stem+".rs")

	// stemFile must be first here as the first path in BaseSourceProvider.OutputFiles is the library entry-point.
	outputs := android.WritablePaths{stemFile}

	rule := android.NewRuleBuilder()
	for _, protoFile := range protoFiles {
		protoName := strings.TrimSuffix(protoFile.Base(), ".proto")
		protoNames = append(protoNames, protoName)

		protoOut := android.PathForModuleOut(ctx, protoName+".rs")
		ruleOutputs := android.WritablePaths{android.WritablePath(protoOut)}

		if proto.plugin == Grpc {
			grpcOut := android.PathForModuleOut(ctx, protoName+grpcSuffix+".rs")
			ruleOutputs = append(ruleOutputs, android.WritablePath(grpcOut))
		}

		depFile := android.PathForModuleOut(ctx, protoName+".d")

		android.ProtoRule(ctx, rule, protoFile, protoFlags, protoFlags.Deps, outDir, depFile, ruleOutputs)
		outputs = append(outputs, ruleOutputs...)
	}

	rule.Command().
		Implicits(outputs.Paths()).
		Text("printf '" + proto.genModFileContents(ctx, protoNames) + "' >").
		Output(stemFile)

	rule.Build(pctx, ctx, "protoc_"+ctx.ModuleName(), "protoc "+ctx.ModuleName())

	proto.BaseSourceProvider.OutputFiles = outputs.Paths()

	// mod_stem.rs is the entry-point for our library modules, so this is what we return.
	return stemFile
}

func (proto *protobufDecorator) genModFileContents(ctx ModuleContext, protoNames []string) string {
	lines := []string{
		"// @Soong generated Source",
	}
	for _, protoName := range protoNames {
		lines = append(lines, fmt.Sprintf("pub mod %s;", protoName))

		if proto.plugin == Grpc {
			lines = append(lines, fmt.Sprintf("pub mod %s%s;", protoName, grpcSuffix))
		}
	}

	if proto.plugin == Grpc {
		lines = append(
			lines,
			"pub mod empty {",
			"    pub use protobuf::well_known_types::Empty;",
			"}")
	}

	return strings.Join(lines, "\\n")
}

func (proto *protobufDecorator) setupPlugin(ctx ModuleContext, protoFlags android.ProtoFlags, outDir android.ModuleOutPath) (android.Paths, android.ProtoFlags) {
	pluginPaths := []android.Path{}

	if proto.plugin == Protobuf {
		pluginPath := ctx.Config().HostToolPath(ctx, "protoc-gen-rust")
		pluginPaths = append(pluginPaths, pluginPath)
		protoFlags.Flags = append(protoFlags.Flags, "--plugin="+pluginPath.String())
	} else if proto.plugin == Grpc {
		grpcPath := ctx.Config().HostToolPath(ctx, "grpc_rust_plugin")
		protobufPath := ctx.Config().HostToolPath(ctx, "protoc-gen-rust")
		pluginPaths = append(pluginPaths, grpcPath, protobufPath)
		protoFlags.Flags = append(protoFlags.Flags, "--grpc_out="+outDir.String())
		protoFlags.Flags = append(protoFlags.Flags, "--plugin=protoc-gen-grpc="+grpcPath.String())
		protoFlags.Flags = append(protoFlags.Flags, "--plugin=protoc-gen-rust="+protobufPath.String())
	} else {
		ctx.ModuleErrorf("Unknown protobuf plugin type requested")
	}

	return pluginPaths, protoFlags
}

func (proto *protobufDecorator) SourceProviderProps() []interface{} {
	return append(proto.BaseSourceProvider.SourceProviderProps(), &proto.Properties)
}

func (proto *protobufDecorator) SourceProviderDeps(ctx DepsContext, deps Deps) Deps {
	deps = proto.BaseSourceProvider.SourceProviderDeps(ctx, deps)
	deps.Rustlibs = append(deps.Rustlibs, "libprotobuf")
	deps.HeaderLibs = append(deps.SharedLibs, proto.Properties.Header_libs...)

	if proto.plugin == Grpc {
		deps.Rustlibs = append(deps.Rustlibs, "libgrpcio", "libfutures")
		deps.HeaderLibs = append(deps.HeaderLibs, "libprotobuf-cpp-full")
	}

	return deps
}

// rust_protobuf generates protobuf rust code from the provided proto file. This uses the protoc-gen-rust plugin for
// protoc. Additional flags to the protoc command can be passed via the proto_flags property. This module type will
// create library variants that can be used as a crate dependency by adding it to the rlibs, dylibs, and rustlibs
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

func RustGrpcioFactory() android.Module {
	module, _ := NewRustGrpcio(android.HostAndDeviceSupported)
	return module.Init()
}

// A host-only variant of rust_protobuf. Refer to rust_protobuf for more details.
func RustGrpcioHostFactory() android.Module {
	module, _ := NewRustGrpcio(android.HostSupported)
	return module.Init()
}

func NewRustProtobuf(hod android.HostOrDeviceSupported) (*Module, *protobufDecorator) {
	protobuf := &protobufDecorator{
		BaseSourceProvider: NewSourceProvider(),
		Properties:         ProtobufProperties{},
		plugin:             Protobuf,
	}

	module := NewSourceProviderModule(hod, protobuf, false)

	return module, protobuf
}

func NewRustGrpcio(hod android.HostOrDeviceSupported) (*Module, *protobufDecorator) {
	protobuf := &protobufDecorator{
		BaseSourceProvider: NewSourceProvider(),
		Properties:         ProtobufProperties{},
		plugin:             Grpc,
	}

	module := NewSourceProviderModule(hod, protobuf, false)

	return module, protobuf
}
