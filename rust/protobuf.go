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

var (
	defaultProtobufFlags = []string{""}
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
	// Path to the proto file that will be used to generate the source
	Proto *string `android:"path,arch_variant"`

	// List of additional flags to pass to aprotoc
	Proto_flags []string `android:"arch_variant"`
}

type protobufDecorator struct {
	*BaseSourceProvider

	Properties ProtobufProperties
	plugin     PluginType
}

func (proto *protobufDecorator) GenerateSource(ctx ModuleContext, deps PathDeps) android.Path {
	var protoFlags android.ProtoFlags
	var pluginPaths android.Paths

	protoFlags.OutTypeFlag = "--rust_out"
	outDir := android.PathForModuleOut(ctx)

	pluginPaths, protoFlags = proto.setupPlugin(ctx, protoFlags, outDir)

	protoFlags.Flags = append(protoFlags.Flags, defaultProtobufFlags...)
	protoFlags.Flags = append(protoFlags.Flags, proto.Properties.Proto_flags...)

	protoFlags.Deps = append(protoFlags.Deps, pluginPaths...)

	protoFile := android.OptionalPathForModuleSrc(ctx, proto.Properties.Proto)
	if !protoFile.Valid() {
		ctx.PropertyErrorf("proto", "invalid path to proto file")
	}

	stem := proto.BaseSourceProvider.getStem(ctx)
	// rust protobuf-codegen output <stem>.rs
	stemFile := android.PathForModuleOut(ctx, stem+".rs")
	// add mod_<stem>.rs to import <stem>.rs
	modFile := android.PathForModuleOut(ctx, "mod_"+stem+".rs")
	// mod_<stem>.rs is the main/first output file to be included/compiled
	outputs := android.WritablePaths{modFile, stemFile}
	depFile := android.PathForModuleOut(ctx, "mod_"+stem+".d")

	rule := android.NewRuleBuilder()
	android.ProtoRule(ctx, rule, protoFile.Path(), protoFlags, protoFlags.Deps, outDir, depFile, outputs)
	rule.Command().Text("printf '// @generated\\npub mod %s;\\n' '" + stem + "' >").Output(modFile)
	rule.Build(pctx, ctx, "protoc_"+protoFile.Path().Rel(), "protoc "+protoFile.Path().Rel())

	proto.BaseSourceProvider.OutputFiles = android.Paths{modFile, stemFile}
	return modFile
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
