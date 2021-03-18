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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"android/soong/android"
)

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
		"srcs: the path %q contains invalid subpath %q."
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
		mockFiles android.MockFS

		errors           []string
		expectedBinaries []pyModule
	}{
		{
			desc: "module without any src files",
			mockFiles: map[string][]byte{
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

					python_binary_host {
						name: "bin",
						pkg_path: "e/",
						srcs: [
							"bin.py",
						],
						libs: [
							"lib2",
						],
					}
					`,
				),
				"dir/c/file1.py": nil,
				"dir/file1.py":   nil,
				"dir/bin.py":     nil,
			},
			errors: []string{
				fmt.Sprintf(dupRunfileErrTemplate, "dir/Android.bp:20:6",
					"bin", "PY3", "a/b/c/file1.py", "bin", "dir/file1.py",
					"lib1", "dir/c/file1.py"),
			},
		},
		{
			desc: "module for testing dependencies",
			mockFiles: map[string][]byte{
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
				StubTemplateHost: []byte(`PYTHON_BINARY = '%interpreter%'
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
					srcsZip: "out/soong/.intermediates/dir/bin/PY3/bin.py.srcszip",
					depsSrcsZips: []string{
						"out/soong/.intermediates/dir/lib5/PY3/lib5.py.srcszip",
						"out/soong/.intermediates/dir/lib6/PY3/lib6.py.srcszip",
					},
				},
			},
		},
	}
)

func TestPythonModule(t *testing.T) {
	for _, d := range data {
		if d.desc != "module with duplicate runfile path" {
			continue
		}
		errorPatterns := make([]string, len(d.errors))
		for i, s := range d.errors {
			errorPatterns[i] = regexp.QuoteMeta(s)
		}

		t.Run(d.desc, func(t *testing.T) {
			result := emptyFixtureFactory.
				ExtendWithErrorHandler(android.FixtureExpectsAllErrorsToMatchAPattern(errorPatterns)).
				RunTest(t,
					android.PrepareForTestWithDefaults,
					PrepareForTestWithPythonBuildComponents,
					d.mockFiles.AddToFixture(),
				)

			if len(result.Errs) > 0 {
				return
			}

			for _, e := range d.expectedBinaries {
				t.Run(e.name, func(t *testing.T) {
					expectModule(t, result.TestContext, e.name, e.actualVersion, e.srcsZip, e.pyRunfiles, e.depsSrcsZips)
				})
			}
		})
	}
}

func expectModule(t *testing.T, ctx *android.TestContext, name, variant, expectedSrcsZip string, expectedPyRunfiles, expectedDepsSrcsZips []string) {
	module := ctx.ModuleForTests(name, variant)

	base, baseOk := module.Module().(*Module)
	if !baseOk {
		t.Fatalf("%s is not Python module!", name)
	}

	actualPyRunfiles := []string{}
	for _, path := range base.srcsPathMappings {
		actualPyRunfiles = append(actualPyRunfiles, path.dest)
	}

	android.AssertDeepEquals(t, "pyRunfiles", expectedPyRunfiles, actualPyRunfiles)

	android.AssertPathRelativeToTopEquals(t, "srcsZip", expectedSrcsZip, base.srcsZip)

	android.AssertPathsRelativeToTopEquals(t, "depsSrcsZips", expectedDepsSrcsZips, base.depsSrcsZips)
}

var emptyFixtureFactory = android.NewFixtureFactory(nil)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
