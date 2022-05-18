package bp2build

import (
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/python"
)

// TODO(alexmarquez): Should be lifted into a generic Bp2Build file
type PythonLibBp2Build func(ctx android.TopDownMutatorContext)

type pythonLibBp2BuildTestCase struct {
	description          string
	filesystem           map[string]string
	blueprint            string
	expectedBazelTargets []testBazelTarget
	dir                  string
	expectedError        error
}

func convertPythonLibTestCaseToBp2build_Host(tc pythonLibBp2BuildTestCase) bp2buildTestCase {
	for i := range tc.expectedBazelTargets {
		tc.expectedBazelTargets[i].attrs["target_compatible_with"] = `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`
	}

	return convertPythonLibTestCaseToBp2build(tc)
}

func convertPythonLibTestCaseToBp2build(tc pythonLibBp2BuildTestCase) bp2buildTestCase {
	var bp2BuildTargets []string
	for _, t := range tc.expectedBazelTargets {
		bp2BuildTargets = append(bp2BuildTargets, makeBazelTarget(t.typ, t.name, t.attrs))
	}
	// Copy the filesystem so that we can change stuff in it later without it
	// affecting the original pythonLibBp2BuildTestCase
	filesystemCopy := make(map[string]string)
	for k, v := range tc.filesystem {
		filesystemCopy[k] = v
	}
	return bp2buildTestCase{
		description:          tc.description,
		filesystem:           filesystemCopy,
		blueprint:            tc.blueprint,
		expectedBazelTargets: bp2BuildTargets,
		dir:                  tc.dir,
		expectedErr:          tc.expectedError,
	}
}

func runPythonLibraryTestCase(t *testing.T, tc pythonLibBp2BuildTestCase) {
	t.Helper()
	testCase := convertPythonLibTestCaseToBp2build(tc)
	testCase.description = fmt.Sprintf(testCase.description, "python_library")
	testCase.blueprint = fmt.Sprintf(testCase.blueprint, "python_library")
	for name, contents := range testCase.filesystem {
		if strings.HasSuffix(name, "Android.bp") {
			testCase.filesystem[name] = fmt.Sprintf(contents, "python_library")
		}
	}
	testCase.moduleTypeUnderTest = "python_library"
	testCase.moduleTypeUnderTestFactory = python.PythonLibraryFactory

	runBp2BuildTestCaseSimple(t, testCase)
}

func runPythonLibraryHostTestCase(t *testing.T, tc pythonLibBp2BuildTestCase) {
	t.Helper()
	testCase := convertPythonLibTestCaseToBp2build_Host(tc)
	testCase.description = fmt.Sprintf(testCase.description, "python_library_host")
	testCase.blueprint = fmt.Sprintf(testCase.blueprint, "python_library_host")
	for name, contents := range testCase.filesystem {
		if strings.HasSuffix(name, "Android.bp") {
			testCase.filesystem[name] = fmt.Sprintf(contents, "python_library_host")
		}
	}
	testCase.moduleTypeUnderTest = "python_library_host"
	testCase.moduleTypeUnderTestFactory = python.PythonLibraryHostFactory
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("python_library", python.PythonLibraryFactory)
	},
		testCase)
}

func runPythonLibraryTestCases(t *testing.T, tc pythonLibBp2BuildTestCase) {
	t.Helper()
	runPythonLibraryTestCase(t, tc)
	runPythonLibraryHostTestCase(t, tc)
}

func TestSimplePythonLib(t *testing.T) {
	testCases := []pythonLibBp2BuildTestCase{
		{
			description: "simple %s converts to a native py_library",
			filesystem: map[string]string{
				"a.py":           "",
				"b/c.py":         "",
				"b/d.py":         "",
				"b/e.py":         "",
				"files/data.txt": "",
			},
			blueprint: `%s {
    name: "foo",
    srcs: ["**/*.py"],
    exclude_srcs: ["b/e.py"],
    data: ["files/data.txt",],
    libs: ["bar"],
    bazel_module: { bp2build_available: true },
}
    python_library {
      name: "bar",
      srcs: ["b/e.py"],
      bazel_module: { bp2build_available: false },
    }`,
			expectedBazelTargets: []testBazelTarget{
				{
					typ:  "py_library",
					name: "foo",
					attrs: attrNameToString{
						"data": `["files/data.txt"]`,
						"deps": `[":bar"]`,
						"srcs": `[
        "a.py",
        "b/c.py",
        "b/d.py",
    ]`,
						"srcs_version": `"PY3"`,
						"imports":      `["."]`,
					},
				},
			},
		},
		{
			description: "py2 %s converts to a native py_library",
			blueprint: `%s {
    name: "foo",
    srcs: ["a.py"],
    version: {
        py2: {
            enabled: true,
        },
        py3: {
            enabled: false,
        },
    },

    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []testBazelTarget{
				{
					typ:  "py_library",
					name: "foo",
					attrs: attrNameToString{
						"srcs":         `["a.py"]`,
						"srcs_version": `"PY2"`,
						"imports":      `["."]`,
					},
				},
			},
		},
		{
			description: "py3 %s converts to a native py_library",
			blueprint: `%s {
    name: "foo",
    srcs: ["a.py"],
    version: {
        py2: {
            enabled: false,
        },
        py3: {
            enabled: true,
        },
    },

    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []testBazelTarget{
				{
					typ:  "py_library",
					name: "foo",
					attrs: attrNameToString{
						"srcs":         `["a.py"]`,
						"srcs_version": `"PY3"`,
						"imports":      `["."]`,
					},
				},
			},
		},
		{
			description: "py2&3 %s converts to a native py_library",
			blueprint: `%s {
    name: "foo",
    srcs: ["a.py"],
    version: {
        py2: {
            enabled: true,
        },
        py3: {
            enabled: true,
        },
    },

    bazel_module: { bp2build_available: true },
}`,
			expectedBazelTargets: []testBazelTarget{
				{
					// srcs_version is PY2ANDPY3 by default.
					typ:  "py_library",
					name: "foo",
					attrs: attrNameToString{
						"srcs":    `["a.py"]`,
						"imports": `["."]`,
					},
				},
			},
		},
		{
			description: "%s: pkg_path in a subdirectory of the same name converts correctly",
			dir:         "mylib/subpackage",
			filesystem: map[string]string{
				"mylib/subpackage/a.py": "",
				"mylib/subpackage/Android.bp": `%s {
				name: "foo",
				srcs: ["a.py"],
				pkg_path: "mylib/subpackage",

				bazel_module: { bp2build_available: true },
			}`,
			},
			blueprint: `%s {name: "bar"}`,
			expectedBazelTargets: []testBazelTarget{
				{
					// srcs_version is PY2ANDPY3 by default.
					typ:  "py_library",
					name: "foo",
					attrs: attrNameToString{
						"srcs":         `["a.py"]`,
						"imports":      `["../.."]`,
						"srcs_version": `"PY3"`,
					},
				},
			},
		},
		{
			description: "%s: pkg_path in a subdirectory of a different name fails",
			dir:         "mylib/subpackage",
			filesystem: map[string]string{
				"mylib/subpackage/a.py": "",
				"mylib/subpackage/Android.bp": `%s {
				name: "foo",
				srcs: ["a.py"],
				pkg_path: "mylib/subpackage2",
				bazel_module: { bp2build_available: true },
			}`,
			},
			blueprint:     `%s {name: "bar"}`,
			expectedError: fmt.Errorf("Currently, bp2build only supports pkg_paths that are the same as the folders the Android.bp file is in."),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPythonLibraryTestCases(t, tc)
		})
	}
}

func TestPythonArchVariance(t *testing.T) {
	runPythonLibraryTestCases(t, pythonLibBp2BuildTestCase{
		description: "test %s arch variants",
		filesystem: map[string]string{
			"dir/arm.py": "",
			"dir/x86.py": "",
		},
		blueprint: `%s {
					 name: "foo",
					 arch: {
						 arm: {
							 srcs: ["arm.py"],
						 },
						 x86: {
							 srcs: ["x86.py"],
						 },
					},
				 }`,
		expectedBazelTargets: []testBazelTarget{
			{
				typ:  "py_library",
				name: "foo",
				attrs: attrNameToString{
					"srcs": `select({
        "//build/bazel/platforms/arch:arm": ["arm.py"],
        "//build/bazel/platforms/arch:x86": ["x86.py"],
        "//conditions:default": [],
    })`,
					"srcs_version": `"PY3"`,
					"imports":      `["."]`,
				},
			},
		},
	})
}
