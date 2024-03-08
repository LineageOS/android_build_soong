// Copyright 2019 The Android Open Source Project
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

func TestRustTest(t *testing.T) {
	ctx := testRust(t, `
		rust_test_host {
			name: "my_test",
			srcs: ["foo.rs"],
			data: ["data.txt"],
		}`)

	testingModule := ctx.ModuleForTests("my_test", "linux_glibc_x86_64")
	expectedOut := "my_test/linux_glibc_x86_64/my_test"
	outPath := testingModule.Output("my_test").Output.String()
	if !strings.Contains(outPath, expectedOut) {
		t.Errorf("wrong output path: %v;  expected: %v", outPath, expectedOut)
	}

	dataPaths := testingModule.Module().(*Module).compiler.(*testDecorator).dataPaths()
	if len(dataPaths) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%v]", dataPaths)
		return
	}
}

func TestRustTestLinkage(t *testing.T) {
	ctx := testRust(t, `
		rust_test {
			name: "my_test",
			srcs: ["foo.rs"],
			rustlibs: ["libfoo"],
            rlibs: ["libbar"],
		}
		rust_library {
			name: "libfoo",
			srcs: ["foo.rs"],
			crate_name: "foo",
		}
		rust_library {
			name: "libbar",
			srcs: ["foo.rs"],
			crate_name: "bar",
		}`)

	testingModule := ctx.ModuleForTests("my_test", "android_arm64_armv8-a").Module().(*Module)

	if !android.InList("libfoo.rlib-std", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("rlib-std variant for libfoo not detected as a rustlib-defined rlib dependency for device rust_test module")
	}
	if !android.InList("libbar.rlib-std", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("rlib-std variant for libbar not detected as an rlib dependency for device rust_test module")
	}
	if !android.InList("libstd", testingModule.Properties.AndroidMkRlibs) {
		t.Errorf("Device rust_test module 'my_test' does not link libstd as an rlib")
	}
}

func TestDataLibs(t *testing.T) {
	bp := `
		cc_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
		}

		rust_binary {
			name: "rusty",
			srcs: ["foo.rs"],
			compile_multilib: "both",
		}

		rust_ffi {
			name: "librust_test_lib",
			crate_name: "rust_test_lib",
			srcs: ["test_lib.rs"],
			relative_install_path: "foo/bar/baz",
			compile_multilib: "64",
		}

		rust_test {
			name: "main_test",
			srcs: ["foo.rs"],
			data_libs: ["test_lib"],
			data_bins: ["rusty"],
		}
 `

	ctx := testRust(t, bp)

	module := ctx.ModuleForTests("main_test", "android_arm64_armv8-a").Module()
	testBinary := module.(*Module).compiler.(*testDecorator)
	outputFiles, err := module.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Fatalf("Expected rust_test to produce output files, error: %s", err)
	}
	if len(outputFiles) != 1 {
		t.Fatalf("expected exactly one output file. output files: [%s]", outputFiles)
	}
	if len(testBinary.dataPaths()) != 2 {
		t.Fatalf("expected exactly two test data files. test data files: [%v]", testBinary.dataPaths())
	}

	outputPath := outputFiles[0].String()
	dataLibraryPath := testBinary.dataPaths()[0].SrcPath.String()
	dataBinaryPath := testBinary.dataPaths()[1].SrcPath.String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
	}
	if !strings.HasSuffix(dataLibraryPath, "/test_lib.so") {
		t.Errorf("expected test data file to be 'test_lib.so', but was '%s'", dataLibraryPath)
	}
	if !strings.HasSuffix(dataBinaryPath, "/rusty") {
		t.Errorf("expected test data file to be 'test_lib.so', but was '%s'", dataBinaryPath)
	}
}

func TestDataLibsRelativeInstallPath(t *testing.T) {
	bp := `
		cc_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
			relative_install_path: "foo/bar/baz",
			compile_multilib: "64",
		}

		rust_ffi {
			name: "librust_test_lib",
			crate_name: "rust_test_lib",
			srcs: ["test_lib.rs"],
			relative_install_path: "foo/bar/baz",
			compile_multilib: "64",
		}

		rust_binary {
			name: "rusty",
			srcs: ["foo.rs"],
			relative_install_path: "foo/bar/baz",
			compile_multilib: "64",
		}

		rust_test {
			name: "main_test",
			srcs: ["foo.rs"],
			data_libs: ["test_lib", "librust_test_lib"],
			data_bins: ["rusty"],
			compile_multilib: "64",
		}
 `

	ctx := testRust(t, bp)
	module := ctx.ModuleForTests("main_test", "android_arm64_armv8-a").Module()
	testBinary := module.(*Module).compiler.(*testDecorator)
	outputFiles, err := module.(android.OutputFileProducer).OutputFiles("")
	if err != nil {
		t.Fatalf("Expected rust_test to produce output files, error: %s", err)
	}
	if len(outputFiles) != 1 {
		t.Fatalf("expected exactly one output file. output files: [%s]", outputFiles)
	}
	if len(testBinary.dataPaths()) != 3 {
		t.Fatalf("expected exactly two test data files. test data files: [%v]", testBinary.dataPaths())
	}

	outputPath := outputFiles[0].String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
	}
	entries := android.AndroidMkEntriesForTest(t, ctx, module)[0]
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][0], ":test_lib.so:lib64/foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:test_lib.so:lib64/foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][0])
	}
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][1], ":librust_test_lib.so:lib64/foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:librust_test_lib.so:lib64/foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][1])
	}
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][2], ":rusty:foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:rusty:foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][2])
	}
}
