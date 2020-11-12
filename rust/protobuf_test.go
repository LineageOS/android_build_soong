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

func TestRustProtobuf(t *testing.T) {
	ctx := testRust(t, `
		rust_protobuf {
			name: "librust_proto",
			proto: "buf.proto",
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
}

func TestRustGrpcio(t *testing.T) {
	ctx := testRust(t, `
		rust_grpcio {
			name: "librust_grpcio",
			proto: "buf.proto",
			crate_name: "rust_grpcio",
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
	librust_grpcio_module := ctx.ModuleForTests("librust_grpcio", "android_arm64_armv8-a_dylib").Module().(*Module)
	if !android.InList("libprotobuf", librust_grpcio_module.Properties.AndroidMkDylibs) {
		t.Errorf("libprotobuf dependency missing for rust_grpcio (dependency missing from AndroidMkDylibs)")
	}

	// Check that libgrpcio is added as a dependency.
	if !android.InList("libgrpcio", librust_grpcio_module.Properties.AndroidMkDylibs) {
		t.Errorf("libgrpcio dependency missing for rust_grpcio (dependency missing from AndroidMkDylibs)")
	}

	// Check that libfutures is added as a dependency.
	if !android.InList("libfutures", librust_grpcio_module.Properties.AndroidMkDylibs) {
		t.Errorf("libfutures dependency missing for rust_grpcio (dependency missing from AndroidMkDylibs)")
	}

	// Make sure the correct plugin is being used.
	librust_grpcio_out := ctx.ModuleForTests("librust_grpcio", "android_arm64_armv8-a_source").Output("buf_grpc.rs")
	cmd := librust_grpcio_out.RuleParams.Command
	if w := "protoc-gen-grpc"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

	// Check exported include directories
	if w := "-Ishared_include"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}
	if w := "-Istatic_include"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}

	// Check that we're including the exported directory from libprotobuf-cpp-full
	if w := "-Ilibprotobuf-cpp-full-includes"; !strings.Contains(cmd, w) {
		t.Errorf("expected %q in %q", w, cmd)
	}
}
