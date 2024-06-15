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
	"strings"
	"testing"

	"android/soong/android"
)

func TestRustProtobuf3(t *testing.T) {
	ctx := testRust(t, `
		rust_protobuf {
			name: "librust_proto",
			protos: ["buf.proto", "proto.proto"],
			crate_name: "rust_proto",
			source_stem: "buf",
			shared_libs: ["libfoo_shared"],
			static_libs: ["libfoo_static"],
		}
		cc_library_shared {
			name: "libfoo_shared",
			export_include_dirs: ["shared_include"],
		}
		cc_library_static {
			name: "libfoo_static",
			export_include_dirs: ["static_include"],
		}
	`)
	// Check that libprotobuf is added as a dependency.
	librust_proto := ctx.ModuleForTests("librust_proto", "android_arm64_armv8-a_dylib").Module().(*Module)
	if !android.InList("libprotobuf", librust_proto.Properties.AndroidMkDylibs) {
		t.Errorf("libprotobuf dependency missing for rust_protobuf (dependency missing from AndroidMkDylibs)")
	}

	// Make sure the correct plugin is being used.
	librust_proto_out := ctx.ModuleForTests("librust_proto", "android_arm64_armv8-a_source").Output("buf.rs")
	cmd := librust_proto_out.RuleParams.Command
	if w := "protoc-gen-rust"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

	// Check exported include directories
	if w := "-Ishared_include"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}
	if w := "-Istatic_include"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

	// Check proto.rs, the second protobuf, is listed as an output
	librust_proto_outputs := ctx.ModuleForTests("librust_proto", "android_arm64_armv8-a_source").AllOutputs()
	if android.InList("proto.rs", librust_proto_outputs) {
		t.Errorf("rust_protobuf is not producing multiple outputs; expected 'proto.rs' in list, got: %#v ",
			librust_proto_outputs)
	}
}

func TestRustProtobufInclude(t *testing.T) {
	ctx := testRust(t, `
		rust_protobuf {
			name: "librust_proto",
			protos: ["proto.proto"],
			crate_name: "rust_proto",
			source_stem: "proto",
			rustlibs: ["librust_exported_proto", "libfoo"],
		}
		rust_protobuf {
			name: "librust_exported_proto",
			protos: ["proto.proto"],
			crate_name: "rust_exported_proto",
			source_stem: "exported_proto",
			exported_include_dirs: ["proto"]
		}
		rust_library {
			name: "libfoo",
			crate_name: "foo",
			srcs: ["foo.rs"],
		}
	`)
	// Check that librust_exported_proto is added as additional crate to generate source.
	librust_proto := ctx.ModuleForTests("librust_proto", "android_arm64_armv8-a_source").Module().(*Module).sourceProvider.(*protobufDecorator)
	if !android.InList("rust_exported_proto", librust_proto.additionalCrates) {
		t.Errorf("librust_proto should have librust_exported_proto included as an additional crate for generated source, instead got: %#v", librust_proto.additionalCrates)
	}

	// Make sure the default crates aren't being included.
	if android.InList("std", librust_proto.additionalCrates) {
		t.Errorf("librust_proto should not have included libstd as an additional crate for generated source, instead got: %#v", librust_proto.additionalCrates)
	}
	if android.InList("protobuf", librust_proto.additionalCrates) {
		t.Errorf("librust_proto should not have included libprotobuf as an additional crate for generated source, instead got: %#v", librust_proto.additionalCrates)
	}

	// And make sure that non-protobuf crates aren't getting included either.
	if android.InList("foo", librust_proto.additionalCrates) {
		t.Errorf("librust_proto should not have included libfoo as an additional crate for generated source, instead got: %#v", librust_proto.additionalCrates)
	}

	// Check librust_proto args includes -Iproto
	librust_proto_rule := ctx.ModuleForTests("librust_proto", "android_arm64_armv8-a_source").Output("proto.rs")
	cmd := librust_proto_rule.RuleParams.Command
	if w := "-Iproto"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

}

func TestRustGrpc(t *testing.T) {
	ctx := testRust(t, `
		rust_protobuf {
			name: "librust_grpcio",
			protos: ["buf.proto"],
			grpc_protos: ["foo.proto", "proto.proto"],
			crate_name: "rust_grpcio",
			source_stem: "buf",
		}
	`)

	// Check that libprotobuf is added as a dependency.
	librust_grpcio_module := ctx.ModuleForTests("librust_grpcio", "android_arm64_armv8-a_dylib").Module().(*Module)

	// Check that libgrpcio is added as a dependency.
	if !android.InList("libgrpcio", librust_grpcio_module.Properties.AndroidMkDylibs) {
		t.Errorf("libgrpcio dependency missing for rust_grpcio (dependency missing from AndroidMkDylibs)")
	}

	// Check that libfutures is added as a dependency.
	if !android.InList("libfutures", librust_grpcio_module.Properties.AndroidMkDylibs) {
		t.Errorf("libfutures dependency missing for rust_grpcio (dependency missing from AndroidMkDylibs)")
	}

	// Make sure the correct plugin is being used.
	librust_grpcio_out := ctx.ModuleForTests("librust_grpcio", "android_arm64_armv8-a_source").Output("foo_grpc.rs")
	cmd := librust_grpcio_out.RuleParams.Command
	if w := "protoc-gen-grpc"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

	// Check that we're including the exported directory from libprotobuf-cpp-full
	if w := "-I" + rustDefaultsDir + "libprotobuf-cpp-full-includes"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

	// Check proto.rs, the second protobuf, is listed as an output
	librust_grpcio_outputs := ctx.ModuleForTests("librust_grpcio", "android_arm64_armv8-a_source").AllOutputs()
	if android.InList("proto_grpc.rs", librust_grpcio_outputs) {
		t.Errorf("rust_protobuf is not producing multiple outputs; expected 'proto_grpc.rs' in list, got: %#v ",
			librust_grpcio_outputs)
	}
}

func TestRustProtoErrors(t *testing.T) {
	testRustError(t, "A proto can only be added once to either grpc_protos or protos.*", `
		rust_protobuf {
			name: "librust_grpcio",
			protos: ["buf.proto"],
			grpc_protos: ["buf.proto"],
			crate_name: "rust_grpcio",
			source_stem: "buf",
		}
	`)

	testRustError(t, "proto filenames must be unique across  'protos' and 'grpc_protos'.*", `
		rust_protobuf {
			name: "librust_grpcio",
			protos: ["buf.proto"],
			grpc_protos: ["proto/buf.proto"],
			crate_name: "rust_grpcio",
			source_stem: "buf",
		}
	`)
}
