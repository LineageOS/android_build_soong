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

package python

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"android/soong/android"
)

var buildDir string

type pyModule struct {
	name          string
	actualVersion string
	pyRunfiles    []string
	srcsZip       string
	depsSrcsZips  []string
}

var (
	buildNamePrefix          = "soong_python_test"
	moduleVariantErrTemplate = "%s: module %q variant %q: "
	pkgPathErrTemplate       = moduleVariantErrTemplate +
		"pkg_path: %q must be a relative path contained in par file."
	badIdentifierErrTemplate = moduleVariantErrTemplate +
		"srcs: the path %q contains invalid token %q."
	dupRunfileErrTemplate = moduleVariantErrTemplate +
		"found two files to be placed at the same location within zip %q." +
		" First file: in module %s at path %q." +
		" Second file: in module %s at path %q."
	noSrcFileErr      = moduleVariantErrTemplate + "doesn't have any source files!"
	badSrcFileExtErr  = moduleVariantErrTemplate + "srcs: found non (.py|.proto) file: %q!"
	badDataFileExtErr = moduleVariantErrTemplate + "data: found (.py|.proto) file: %q!"
	bpFile            = "Android.bp"

	data = []struct {
		desc      string
		mockFiles map[string][]byte

		errors           []string
		expectedBinaries []pyModule
	}{
		{
			desc: "module without any src files",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_library_host {
						name: "lib1",
					}`,
				),
			},
			errors: []string{
				fmt.Sprintf(noSrcFileErr,
					"dir/Android.bp:1:1", "lib1", "PY3"),
			},
		},
		{
			desc: "module with bad src file ext",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_library_host {
						name: "lib1",
						srcs: [
							"file1.exe",
						],
					}`,
				),
				"dir/file1.exe": nil,
			},
			errors: []string{
				fmt.Sprintf(badSrcFileExtErr,
					"dir/Android.bp:3:11", "lib1", "PY3", "dir/file1.exe"),
			},
		},
		{
			desc: "module with bad data file ext",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_library_host {
						name: "lib1",
						srcs: [
							"file1.py",
						],
						data: [
							"file2.py",
						],
					}`,
				),
				"dir/file1.py": nil,
				"dir/file2.py": nil,
			},
			errors: []string{
				fmt.Sprintf(badDataFileExtErr,
					"dir/Android.bp:6:11", "lib1", "PY3", "dir/file2.py"),
			},
		},
		{
			desc: "module with bad pkg_path format",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_library_host {
						name: "lib1",
						pkg_path: "a/c/../../",
						srcs: [
							"file1.py",
						],
					}

					python_library_host {
						name: "lib2",
						pkg_path: "a/c/../../../",
						srcs: [
							"file1.py",
						],
					}

					python_library_host {
						name: "lib3",
						pkg_path: "/a/c/../../",
						srcs: [
							"file1.py",
						],
					}`,
				),
				"dir/file1.py": nil,
			},
			errors: []string{
				fmt.Sprintf(pkgPathErrTemplate,
					"dir/Android.bp:11:15", "lib2", "PY3", "a/c/../../../"),
				fmt.Sprintf(pkgPathErrTemplate,
					"dir/Android.bp:19:15", "lib3", "PY3", "/a/c/../../"),
			},
		},
		{
			desc: "module with bad runfile src path format",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_library_host {
						name: "lib1",
						pkg_path: "a/b/c/",
						srcs: [
							".file1.py",
							"123/file1.py",
							"-e/f/file1.py",
						],
					}`,
				),
				"dir/.file1.py":     nil,
				"dir/123/file1.py":  nil,
				"dir/-e/f/file1.py": nil,
			},
			errors: []string{
				fmt.Sprintf(badIdentifierErrTemplate, "dir/Android.bp:4:11",
					"lib1", "PY3", "a/b/c/-e/f/file1.py", "-e"),
				fmt.Sprintf(badIdentifierErrTemplate, "dir/Android.bp:4:11",
					"lib1", "PY3", "a/b/c/.file1.py", ".file1"),
				fmt.Sprintf(badIdentifierErrTemplate, "dir/Android.bp:4:11",
					"lib1", "PY3", "a/b/c/123/file1.py", "123"),
			},
		},
		{
			desc: "module with duplicate runfile path",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_library_host {
						name: "lib1",
						pkg_path: "a/b/",
						srcs: [
							"c/file1.py",
						],
					}

					python_library_host {
						name: "lib2",
						pkg_path: "a/b/c/",
						srcs: [
							"file1.py",
						],
						libs: [
							"lib1",
						],
					}
					`,
				),
				"dir/c/file1.py": nil,
				"dir/file1.py":   nil,
			},
			errors: []string{
				fmt.Sprintf(dupRunfileErrTemplate, "dir/Android.bp:9:6",
					"lib2", "PY3", "a/b/c/file1.py", "lib2", "dir/file1.py",
					"lib1", "dir/c/file1.py"),
			},
		},
		{
			desc: "module for testing dependencies",
			mockFiles: map[string][]byte{
				bpFile: []byte(`subdirs = ["dir"]`),
				filepath.Join("dir", bpFile): []byte(
					`python_defaults {
						name: "default_lib",
						srcs: [
							"default.py",
						],
						version: {
							py2: {
								enabled: true,
								srcs: [
									"default_py2.py",
								],
							},
							py3: {
								enabled: false,
								srcs: [
									"default_py3.py",
								],
							},
						},
					}

					python_library_host {
						name: "lib5",
						pkg_path: "a/b/",
						srcs: [
							"file1.py",
						],
						version: {
							py2: {
								enabled: true,
							},
							py3: {
								enabled: true,
							},
						},
					}

					python_library_host {
						name: "lib6",
						pkg_path: "c/d/",
						srcs: [
							"file2.py",
						],
						libs: [
							"lib5",
						],
					}

					python_binary_host {
						name: "bin",
						defaults: ["default_lib"],
						pkg_path: "e/",
						srcs: [
							"bin.py",
						],
						libs: [
							"lib5",
						],
						version: {
							py3: {
								enabled: true,
								srcs: [
									"file4.py",
								],
								libs: [
									"lib6",
								],
							},
						},
					}`,
				),
				filepath.Join("dir", "default.py"):     nil,
				filepath.Join("dir", "default_py2.py"): nil,
				filepath.Join("dir", "default_py3.py"): nil,
				filepath.Join("dir", "file1.py"):       nil,
				filepath.Join("dir", "file2.py"):       nil,
				filepath.Join("dir", "bin.py"):         nil,
				filepath.Join("dir", "file4.py"):       nil,
				stubTemplateHost: []byte(`PYTHON_BINARY = '%interpreter%'
				MAIN_FILE = '%main%'`),
			},
			expectedBinaries: []pyModule{
				{
					name:          "bin",
					actualVersion: "PY3",
					pyRunfiles: []string{
						"e/default.py",
						"e/bin.py",
						"e/default_py3.py",
						"e/file4.py",
					},
					srcsZip: "@prefix@/.intermediates/dir/bin/PY3/bin.py.srcszip",
					depsSrcsZips: []string{
						"@prefix@/.intermediates/dir/lib5/PY3/lib5.py.srcszip",
						"@prefix@/.intermediates/dir/lib6/PY3/lib6.py.srcszip",
					},
				},
			},
		},
	}
)

func TestPythonModule(t *testing.T) {
	for _, d := range data {
		t.Run(d.desc, func(t *testing.T) {
			config := android.TestConfig(buildDir, nil, "", d.mockFiles)
			ctx := android.NewTestContext()
			ctx.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
				ctx.BottomUp("version_split", versionSplitMutator()).Parallel()
			})
			ctx.RegisterModuleType("python_library_host", PythonLibraryHostFactory)
			ctx.RegisterModuleType("python_binary_host", PythonBinaryHostFactory)
			ctx.RegisterModuleType("python_defaults", defaultsFactory)
			ctx.PreArchMutators(android.RegisterDefaultsPreArchMutators)
			ctx.Register(config)
			_, testErrs := ctx.ParseBlueprintsFiles(bpFile)
			android.FailIfErrored(t, testErrs)
			_, actErrs := ctx.PrepareBuildActions(config)
			if len(actErrs) > 0 {
				testErrs = append(testErrs, expectErrors(t, actErrs, d.errors)...)
			} else {
				for _, e := range d.expectedBinaries {
					testErrs = append(testErrs,
						expectModule(t, ctx, buildDir, e.name,
							e.actualVersion,
							e.srcsZip,
							e.pyRunfiles,
							e.depsSrcsZips)...)
				}
			}
			android.FailIfErrored(t, testErrs)
		})
	}
}

func expectErrors(t *testing.T, actErrs []error, expErrs []string) (testErrs []error) {
	actErrStrs := []string{}
	for _, v := range actErrs {
		actErrStrs = append(actErrStrs, v.Error())
	}
	sort.Strings(actErrStrs)
	if len(actErrStrs) != len(expErrs) {
		t.Errorf("got (%d) errors, expected (%d) errors!", len(actErrStrs), len(expErrs))
		for _, v := range actErrStrs {
			testErrs = append(testErrs, errors.New(v))
		}
	} else {
		sort.Strings(expErrs)
		for i, v := range actErrStrs {
			if v != expErrs[i] {
				testErrs = append(testErrs, errors.New(v))
			}
		}
	}

	return
}

func expectModule(t *testing.T, ctx *android.TestContext, buildDir, name, variant, expectedSrcsZip string,
	expectedPyRunfiles, expectedDepsSrcsZips []string) (testErrs []error) {
	module := ctx.ModuleForTests(name, variant)

	base, baseOk := module.Module().(*Module)
	if !baseOk {
		t.Fatalf("%s is not Python module!", name)
	}

	actualPyRunfiles := []string{}
	for _, path := range base.srcsPathMappings {
		actualPyRunfiles = append(actualPyRunfiles, path.dest)
	}

	if !reflect.DeepEqual(actualPyRunfiles, expectedPyRunfiles) {
		testErrs = append(testErrs, errors.New(fmt.Sprintf(
			`binary "%s" variant "%s" has unexpected pyRunfiles: %q!`,
			base.Name(),
			base.properties.Actual_version,
			actualPyRunfiles)))
	}

	if base.srcsZip.String() != strings.Replace(expectedSrcsZip, "@prefix@", buildDir, 1) {
		testErrs = append(testErrs, errors.New(fmt.Sprintf(
			`binary "%s" variant "%s" has unexpected srcsZip: %q!`,
			base.Name(),
			base.properties.Actual_version,
			base.srcsZip)))
	}

	for i, _ := range expectedDepsSrcsZips {
		expectedDepsSrcsZips[i] = strings.Replace(expectedDepsSrcsZips[i], "@prefix@", buildDir, 1)
	}
	if !reflect.DeepEqual(base.depsSrcsZips.Strings(), expectedDepsSrcsZips) {
		testErrs = append(testErrs, errors.New(fmt.Sprintf(
			`binary "%s" variant "%s" has unexpected depsSrcsZips: %q!`,
			base.Name(),
			base.properties.Actual_version,
			base.depsSrcsZips)))
	}

	return
}

func setUp() {
	var err error
	buildDir, err = ioutil.TempDir("", "soong_python_test")
	if err != nil {
		panic(err)
	}
}

func tearDown() {
	os.RemoveAll(buildDir)
}

func TestMain(m *testing.M) {
	run := func() int {
		setUp()
		defer tearDown()

		return m.Run()
	}

	os.Exit(run())
}
