// Copyright 2015 Google Inc. All rights reserved.
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

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
)

type strsTestCase struct {
	in  []string
	out string
	err []error
}

var commonValidatePathTestCases = []strsTestCase{
	{
		in:  []string{""},
		out: "",
	},
	{
		in:  []string{"a/b"},
		out: "a/b",
	},
	{
		in:  []string{"a/b", "c"},
		out: "a/b/c",
	},
	{
		in:  []string{"a/.."},
		out: ".",
	},
	{
		in:  []string{"."},
		out: ".",
	},
	{
		in:  []string{".."},
		out: "",
		err: []error{errors.New("Path is outside directory: ..")},
	},
	{
		in:  []string{"../a"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../a")},
	},
	{
		in:  []string{"b/../../a"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../a")},
	},
	{
		in:  []string{"/a"},
		out: "",
		err: []error{errors.New("Path is outside directory: /a")},
	},
	{
		in:  []string{"a", "../b"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../b")},
	},
	{
		in:  []string{"a", "b/../../c"},
		out: "",
		err: []error{errors.New("Path is outside directory: ../c")},
	},
	{
		in:  []string{"a", "./.."},
		out: "",
		err: []error{errors.New("Path is outside directory: ..")},
	},
}

var validateSafePathTestCases = append(commonValidatePathTestCases, []strsTestCase{
	{
		in:  []string{"$host/../$a"},
		out: "$a",
	},
}...)

var validatePathTestCases = append(commonValidatePathTestCases, []strsTestCase{
	{
		in:  []string{"$host/../$a"},
		out: "",
		err: []error{errors.New("Path contains invalid character($): $host/../$a")},
	},
	{
		in:  []string{"$host/.."},
		out: "",
		err: []error{errors.New("Path contains invalid character($): $host/..")},
	},
}...)

func TestValidateSafePath(t *testing.T) {
	for _, testCase := range validateSafePathTestCases {
		t.Run(strings.Join(testCase.in, ","), func(t *testing.T) {
			ctx := &configErrorWrapper{}
			out, err := validateSafePath(testCase.in...)
			if err != nil {
				reportPathError(ctx, err)
			}
			check(t, "validateSafePath", p(testCase.in), out, ctx.errors, testCase.out, testCase.err)
		})
	}
}

func TestValidatePath(t *testing.T) {
	for _, testCase := range validatePathTestCases {
		t.Run(strings.Join(testCase.in, ","), func(t *testing.T) {
			ctx := &configErrorWrapper{}
			out, err := validatePath(testCase.in...)
			if err != nil {
				reportPathError(ctx, err)
			}
			check(t, "validatePath", p(testCase.in), out, ctx.errors, testCase.out, testCase.err)
		})
	}
}

func TestOptionalPath(t *testing.T) {
	var path OptionalPath
	checkInvalidOptionalPath(t, path)

	path = OptionalPathForPath(nil)
	checkInvalidOptionalPath(t, path)
}

func checkInvalidOptionalPath(t *testing.T, path OptionalPath) {
	t.Helper()
	if path.Valid() {
		t.Errorf("Uninitialized OptionalPath should not be valid")
	}
	if path.String() != "" {
		t.Errorf("Uninitialized OptionalPath String() should return \"\", not %q", path.String())
	}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected a panic when calling Path() on an uninitialized OptionalPath")
		}
	}()
	path.Path()
}

func check(t *testing.T, testType, testString string,
	got interface{}, err []error,
	expected interface{}, expectedErr []error) {
	t.Helper()

	printedTestCase := false
	e := func(s string, expected, got interface{}) {
		t.Helper()
		if !printedTestCase {
			t.Errorf("test case %s: %s", testType, testString)
			printedTestCase = true
		}
		t.Errorf("incorrect %s", s)
		t.Errorf("  expected: %s", p(expected))
		t.Errorf("       got: %s", p(got))
	}

	if !reflect.DeepEqual(expectedErr, err) {
		e("errors:", expectedErr, err)
	}

	if !reflect.DeepEqual(expected, got) {
		e("output:", expected, got)
	}
}

func p(in interface{}) string {
	if v, ok := in.([]interface{}); ok {
		s := make([]string, len(v))
		for i := range v {
			s[i] = fmt.Sprintf("%#v", v[i])
		}
		return "[" + strings.Join(s, ", ") + "]"
	} else {
		return fmt.Sprintf("%#v", in)
	}
}

type moduleInstallPathContextImpl struct {
	androidBaseContextImpl

	inData         bool
	inTestcases    bool
	inSanitizerDir bool
	inRecovery     bool
}

func (moduleInstallPathContextImpl) Fs() pathtools.FileSystem {
	return pathtools.MockFs(nil)
}

func (m moduleInstallPathContextImpl) Config() Config {
	return m.androidBaseContextImpl.config
}

func (moduleInstallPathContextImpl) AddNinjaFileDeps(deps ...string) {}

func (m moduleInstallPathContextImpl) InstallInData() bool {
	return m.inData
}

func (m moduleInstallPathContextImpl) InstallInTestcases() bool {
	return m.inTestcases
}

func (m moduleInstallPathContextImpl) InstallInSanitizerDir() bool {
	return m.inSanitizerDir
}

func (m moduleInstallPathContextImpl) InstallInRecovery() bool {
	return m.inRecovery
}

func TestPathForModuleInstall(t *testing.T) {
	testConfig := TestConfig("", nil)

	hostTarget := Target{Os: Linux}
	deviceTarget := Target{Os: Android}

	testCases := []struct {
		name string
		ctx  *moduleInstallPathContextImpl
		in   []string
		out  string
	}{
		{
			name: "host binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: hostTarget,
				},
			},
			in:  []string{"bin", "my_test"},
			out: "host/linux-x86/bin/my_test",
		},

		{
			name: "system binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
				},
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/system/bin/my_test",
		},
		{
			name: "vendor binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   socSpecificModule,
				},
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/vendor/bin/my_test",
		},
		{
			name: "odm binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   deviceSpecificModule,
				},
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/odm/bin/my_test",
		},
		{
			name: "product binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productSpecificModule,
				},
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/product/bin/my_test",
		},
		{
			name: "product_services binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productServicesSpecificModule,
				},
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/product_services/bin/my_test",
		},

		{
			name: "system native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
				},
				inData: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/nativetest/my_test",
		},
		{
			name: "vendor native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   socSpecificModule,
				},
				inData: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/nativetest/my_test",
		},
		{
			name: "odm native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   deviceSpecificModule,
				},
				inData: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/nativetest/my_test",
		},
		{
			name: "product native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productSpecificModule,
				},
				inData: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/nativetest/my_test",
		},

		{
			name: "product_services native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productServicesSpecificModule,
				},
				inData: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/nativetest/my_test",
		},

		{
			name: "sanitized system binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
				},
				inSanitizerDir: true,
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/data/asan/system/bin/my_test",
		},
		{
			name: "sanitized vendor binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   socSpecificModule,
				},
				inSanitizerDir: true,
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/data/asan/vendor/bin/my_test",
		},
		{
			name: "sanitized odm binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   deviceSpecificModule,
				},
				inSanitizerDir: true,
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/data/asan/odm/bin/my_test",
		},
		{
			name: "sanitized product binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productSpecificModule,
				},
				inSanitizerDir: true,
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/data/asan/product/bin/my_test",
		},

		{
			name: "sanitized product_services binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productServicesSpecificModule,
				},
				inSanitizerDir: true,
			},
			in:  []string{"bin", "my_test"},
			out: "target/product/test_device/data/asan/product_services/bin/my_test",
		},

		{
			name: "sanitized system native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/asan/data/nativetest/my_test",
		},
		{
			name: "sanitized vendor native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   socSpecificModule,
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/asan/data/nativetest/my_test",
		},
		{
			name: "sanitized odm native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   deviceSpecificModule,
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/asan/data/nativetest/my_test",
		},
		{
			name: "sanitized product native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productSpecificModule,
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/asan/data/nativetest/my_test",
		},
		{
			name: "sanitized product_services native test binary",
			ctx: &moduleInstallPathContextImpl{
				androidBaseContextImpl: androidBaseContextImpl{
					target: deviceTarget,
					kind:   productServicesSpecificModule,
				},
				inData:         true,
				inSanitizerDir: true,
			},
			in:  []string{"nativetest", "my_test"},
			out: "target/product/test_device/data/asan/data/nativetest/my_test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.ctx.androidBaseContextImpl.config = testConfig
			output := PathForModuleInstall(tc.ctx, tc.in...)
			if output.basePath.path != tc.out {
				t.Errorf("unexpected path:\n got: %q\nwant: %q\n",
					output.basePath.path,
					tc.out)
			}
		})
	}
}

func TestDirectorySortedPaths(t *testing.T) {
	config := TestConfig("out", nil)

	ctx := PathContextForTesting(config, map[string][]byte{
		"a.txt":   nil,
		"a/txt":   nil,
		"a/b/c":   nil,
		"a/b/d":   nil,
		"b":       nil,
		"b/b.txt": nil,
		"a/a.txt": nil,
	})

	makePaths := func() Paths {
		return Paths{
			PathForSource(ctx, "a.txt"),
			PathForSource(ctx, "a/txt"),
			PathForSource(ctx, "a/b/c"),
			PathForSource(ctx, "a/b/d"),
			PathForSource(ctx, "b"),
			PathForSource(ctx, "b/b.txt"),
			PathForSource(ctx, "a/a.txt"),
		}
	}

	expected := []string{
		"a.txt",
		"a/a.txt",
		"a/b/c",
		"a/b/d",
		"a/txt",
		"b",
		"b/b.txt",
	}

	paths := makePaths()
	reversePaths := ReversePaths(paths)

	sortedPaths := PathsToDirectorySortedPaths(paths)
	reverseSortedPaths := PathsToDirectorySortedPaths(reversePaths)

	if !reflect.DeepEqual(Paths(sortedPaths).Strings(), expected) {
		t.Fatalf("sorted paths:\n %#v\n != \n %#v", paths.Strings(), expected)
	}

	if !reflect.DeepEqual(Paths(reverseSortedPaths).Strings(), expected) {
		t.Fatalf("sorted reversed paths:\n %#v\n !=\n %#v", reversePaths.Strings(), expected)
	}

	expectedA := []string{
		"a/a.txt",
		"a/b/c",
		"a/b/d",
		"a/txt",
	}

	inA := sortedPaths.PathsInDirectory("a")
	if !reflect.DeepEqual(inA.Strings(), expectedA) {
		t.Errorf("FilesInDirectory(a):\n %#v\n != \n %#v", inA.Strings(), expectedA)
	}

	expectedA_B := []string{
		"a/b/c",
		"a/b/d",
	}

	inA_B := sortedPaths.PathsInDirectory("a/b")
	if !reflect.DeepEqual(inA_B.Strings(), expectedA_B) {
		t.Errorf("FilesInDirectory(a/b):\n %#v\n != \n %#v", inA_B.Strings(), expectedA_B)
	}

	expectedB := []string{
		"b/b.txt",
	}

	inB := sortedPaths.PathsInDirectory("b")
	if !reflect.DeepEqual(inB.Strings(), expectedB) {
		t.Errorf("FilesInDirectory(b):\n %#v\n != \n %#v", inA.Strings(), expectedA)
	}
}

func TestMaybeRel(t *testing.T) {
	testCases := []struct {
		name   string
		base   string
		target string
		out    string
		isRel  bool
	}{
		{
			name:   "normal",
			base:   "a/b/c",
			target: "a/b/c/d",
			out:    "d",
			isRel:  true,
		},
		{
			name:   "parent",
			base:   "a/b/c/d",
			target: "a/b/c",
			isRel:  false,
		},
		{
			name:   "not relative",
			base:   "a/b",
			target: "c/d",
			isRel:  false,
		},
		{
			name:   "abs1",
			base:   "/a",
			target: "a",
			isRel:  false,
		},
		{
			name:   "abs2",
			base:   "a",
			target: "/a",
			isRel:  false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := &configErrorWrapper{}
			out, isRel := MaybeRel(ctx, testCase.base, testCase.target)
			if len(ctx.errors) > 0 {
				t.Errorf("MaybeRel(..., %s, %s) reported unexpected errors %v",
					testCase.base, testCase.target, ctx.errors)
			}
			if isRel != testCase.isRel || out != testCase.out {
				t.Errorf("MaybeRel(..., %s, %s) want %v, %v got %v, %v",
					testCase.base, testCase.target, testCase.out, testCase.isRel, out, isRel)
			}
		})
	}
}

func TestPathForSource(t *testing.T) {
	testCases := []struct {
		name     string
		buildDir string
		src      string
		err      string
	}{
		{
			name:     "normal",
			buildDir: "out",
			src:      "a/b/c",
		},
		{
			name:     "abs",
			buildDir: "out",
			src:      "/a/b/c",
			err:      "is outside directory",
		},
		{
			name:     "in out dir",
			buildDir: "out",
			src:      "out/a/b/c",
			err:      "is in output",
		},
	}

	funcs := []struct {
		name string
		f    func(ctx PathContext, pathComponents ...string) (SourcePath, error)
	}{
		{"pathForSource", pathForSource},
		{"safePathForSource", safePathForSource},
	}

	for _, f := range funcs {
		t.Run(f.name, func(t *testing.T) {
			for _, test := range testCases {
				t.Run(test.name, func(t *testing.T) {
					testConfig := TestConfig(test.buildDir, nil)
					ctx := &configErrorWrapper{config: testConfig}
					_, err := f.f(ctx, test.src)
					if len(ctx.errors) > 0 {
						t.Fatalf("unexpected errors %v", ctx.errors)
					}
					if err != nil {
						if test.err == "" {
							t.Fatalf("unexpected error %q", err.Error())
						} else if !strings.Contains(err.Error(), test.err) {
							t.Fatalf("incorrect error, want substring %q got %q", test.err, err.Error())
						}
					} else {
						if test.err != "" {
							t.Fatalf("missing error %q", test.err)
						}
					}
				})
			}
		})
	}
}

type pathForModuleSrcTestModule struct {
	ModuleBase
	props struct {
		Srcs         []string `android:"path"`
		Exclude_srcs []string `android:"path"`

		Src *string `android:"path"`

		Module_handles_missing_deps bool
	}

	src string
	rel string

	srcs []string
	rels []string

	missingDeps []string
}

func pathForModuleSrcTestModuleFactory() Module {
	module := &pathForModuleSrcTestModule{}
	module.AddProperties(&module.props)
	InitAndroidModule(module)
	return module
}

func (p *pathForModuleSrcTestModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	var srcs Paths
	if p.props.Module_handles_missing_deps {
		srcs, p.missingDeps = PathsAndMissingDepsForModuleSrcExcludes(ctx, p.props.Srcs, p.props.Exclude_srcs)
	} else {
		srcs = PathsForModuleSrcExcludes(ctx, p.props.Srcs, p.props.Exclude_srcs)
	}
	p.srcs = srcs.Strings()

	for _, src := range srcs {
		p.rels = append(p.rels, src.Rel())
	}

	if p.props.Src != nil {
		src := PathForModuleSrc(ctx, *p.props.Src)
		if src != nil {
			p.src = src.String()
			p.rel = src.Rel()
		}
	}

	if !p.props.Module_handles_missing_deps {
		p.missingDeps = ctx.GetMissingDependencies()
	}
}

type pathForModuleSrcTestCase struct {
	name string
	bp   string
	srcs []string
	rels []string
	src  string
	rel  string
}

func testPathForModuleSrc(t *testing.T, buildDir string, tests []pathForModuleSrcTestCase) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := TestConfig(buildDir, nil)
			ctx := NewTestContext()

			ctx.RegisterModuleType("test", ModuleFactoryAdaptor(pathForModuleSrcTestModuleFactory))
			ctx.RegisterModuleType("filegroup", ModuleFactoryAdaptor(FileGroupFactory))

			fgBp := `
				filegroup {
					name: "a",
					srcs: ["src/a"],
				}
			`

			mockFS := map[string][]byte{
				"fg/Android.bp":     []byte(fgBp),
				"foo/Android.bp":    []byte(test.bp),
				"fg/src/a":          nil,
				"foo/src/b":         nil,
				"foo/src/c":         nil,
				"foo/src/d":         nil,
				"foo/src/e/e":       nil,
				"foo/src_special/$": nil,
			}

			ctx.MockFileSystem(mockFS)

			ctx.Register()
			_, errs := ctx.ParseFileList(".", []string{"fg/Android.bp", "foo/Android.bp"})
			FailIfErrored(t, errs)
			_, errs = ctx.PrepareBuildActions(config)
			FailIfErrored(t, errs)

			m := ctx.ModuleForTests("foo", "").Module().(*pathForModuleSrcTestModule)

			if g, w := m.srcs, test.srcs; !reflect.DeepEqual(g, w) {
				t.Errorf("want srcs %q, got %q", w, g)
			}

			if g, w := m.rels, test.rels; !reflect.DeepEqual(g, w) {
				t.Errorf("want rels %q, got %q", w, g)
			}

			if g, w := m.src, test.src; g != w {
				t.Errorf("want src %q, got %q", w, g)
			}

			if g, w := m.rel, test.rel; g != w {
				t.Errorf("want rel %q, got %q", w, g)
			}
		})
	}
}

func TestPathsForModuleSrc(t *testing.T) {
	tests := []pathForModuleSrcTestCase{
		{
			name: "path",
			bp: `
			test {
				name: "foo",
				srcs: ["src/b"],
			}`,
			srcs: []string{"foo/src/b"},
			rels: []string{"src/b"},
		},
		{
			name: "glob",
			bp: `
			test {
				name: "foo",
				srcs: [
					"src/*",
					"src/e/*",
				],
			}`,
			srcs: []string{"foo/src/b", "foo/src/c", "foo/src/d", "foo/src/e/e"},
			rels: []string{"src/b", "src/c", "src/d", "src/e/e"},
		},
		{
			name: "recursive glob",
			bp: `
			test {
				name: "foo",
				srcs: ["src/**/*"],
			}`,
			srcs: []string{"foo/src/b", "foo/src/c", "foo/src/d", "foo/src/e/e"},
			rels: []string{"src/b", "src/c", "src/d", "src/e/e"},
		},
		{
			name: "filegroup",
			bp: `
			test {
				name: "foo",
				srcs: [":a"],
			}`,
			srcs: []string{"fg/src/a"},
			rels: []string{"src/a"},
		},
		{
			name: "special characters glob",
			bp: `
			test {
				name: "foo",
				srcs: ["src_special/*"],
			}`,
			srcs: []string{"foo/src_special/$"},
			rels: []string{"src_special/$"},
		},
	}

	buildDir, err := ioutil.TempDir("", "soong_paths_for_module_src_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	testPathForModuleSrc(t, buildDir, tests)
}

func TestPathForModuleSrc(t *testing.T) {
	tests := []pathForModuleSrcTestCase{
		{
			name: "path",
			bp: `
			test {
				name: "foo",
				src: "src/b",
			}`,
			src: "foo/src/b",
			rel: "src/b",
		},
		{
			name: "glob",
			bp: `
			test {
				name: "foo",
				src: "src/e/*",
			}`,
			src: "foo/src/e/e",
			rel: "src/e/e",
		},
		{
			name: "filegroup",
			bp: `
			test {
				name: "foo",
				src: ":a",
			}`,
			src: "fg/src/a",
			rel: "src/a",
		},
		{
			name: "special characters glob",
			bp: `
			test {
				name: "foo",
				src: "src_special/*",
			}`,
			src: "foo/src_special/$",
			rel: "src_special/$",
		},
	}

	buildDir, err := ioutil.TempDir("", "soong_path_for_module_src_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	testPathForModuleSrc(t, buildDir, tests)
}

func TestPathsForModuleSrc_AllowMissingDependencies(t *testing.T) {
	buildDir, err := ioutil.TempDir("", "soong_paths_for_module_src_allow_missing_dependencies_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(buildDir)

	config := TestConfig(buildDir, nil)
	config.TestProductVariables.Allow_missing_dependencies = proptools.BoolPtr(true)

	ctx := NewTestContext()
	ctx.SetAllowMissingDependencies(true)

	ctx.RegisterModuleType("test", ModuleFactoryAdaptor(pathForModuleSrcTestModuleFactory))

	bp := `
		test {
			name: "foo",
			srcs: [":a"],
			exclude_srcs: [":b"],
			src: ":c",
		}

		test {
			name: "bar",
			srcs: [":d"],
			exclude_srcs: [":e"],
			module_handles_missing_deps: true,
		}
	`

	mockFS := map[string][]byte{
		"Android.bp": []byte(bp),
	}

	ctx.MockFileSystem(mockFS)

	ctx.Register()
	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)

	foo := ctx.ModuleForTests("foo", "").Module().(*pathForModuleSrcTestModule)

	if g, w := foo.missingDeps, []string{"a", "b", "c"}; !reflect.DeepEqual(g, w) {
		t.Errorf("want foo missing deps %q, got %q", w, g)
	}

	if g, w := foo.srcs, []string{}; !reflect.DeepEqual(g, w) {
		t.Errorf("want foo srcs %q, got %q", w, g)
	}

	if g, w := foo.src, ""; g != w {
		t.Errorf("want foo src %q, got %q", w, g)
	}

	bar := ctx.ModuleForTests("bar", "").Module().(*pathForModuleSrcTestModule)

	if g, w := bar.missingDeps, []string{"d", "e"}; !reflect.DeepEqual(g, w) {
		t.Errorf("want bar missing deps %q, got %q", w, g)
	}

	if g, w := bar.srcs, []string{}; !reflect.DeepEqual(g, w) {
		t.Errorf("want bar srcs %q, got %q", w, g)
	}
}

func ExampleOutputPath_ReplaceExtension() {
	ctx := &configErrorWrapper{
		config: TestConfig("out", nil),
	}
	p := PathForOutput(ctx, "system/framework").Join(ctx, "boot.art")
	p2 := p.ReplaceExtension(ctx, "oat")
	fmt.Println(p, p2)
	fmt.Println(p.Rel(), p2.Rel())

	// Output:
	// out/system/framework/boot.art out/system/framework/boot.oat
	// boot.art boot.oat
}

func ExampleOutputPath_FileInSameDir() {
	ctx := &configErrorWrapper{
		config: TestConfig("out", nil),
	}
	p := PathForOutput(ctx, "system/framework").Join(ctx, "boot.art")
	p2 := p.InSameDir(ctx, "oat", "arm", "boot.vdex")
	fmt.Println(p, p2)
	fmt.Println(p.Rel(), p2.Rel())

	// Output:
	// out/system/framework/boot.art out/system/framework/oat/arm/boot.vdex
	// boot.art oat/arm/boot.vdex
}
