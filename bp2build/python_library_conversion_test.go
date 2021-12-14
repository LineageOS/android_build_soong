package bp2build

import (
	"fmt"
	"testing"

	"android/soong/android"
	"android/soong/python"
)

// TODO(alexmarquez): Should be lifted into a generic Bp2Build file
type PythonLibBp2Build func(ctx android.TopDownMutatorContext)

func runPythonLibraryTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	testCase := tc
	testCase.description = fmt.Sprintf(testCase.description, "python_library")
	testCase.blueprint = fmt.Sprintf(testCase.blueprint, "python_library")
	testCase.moduleTypeUnderTest = "python_library"
	testCase.moduleTypeUnderTestFactory = python.PythonLibraryFactory
	runBp2BuildTestCaseSimple(t, testCase)
}

func runPythonLibraryHostTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	testCase := tc
	testCase.description = fmt.Sprintf(testCase.description, "python_library_host")
	testCase.blueprint = fmt.Sprintf(testCase.blueprint, "python_library_host")
	testCase.moduleTypeUnderTest = "python_library_host"
	testCase.moduleTypeUnderTestFactory = python.PythonLibraryHostFactory
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("python_library", python.PythonLibraryFactory)
	},
		testCase)
}

func runPythonLibraryTestCases(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runPythonLibraryTestCase(t, tc)
	runPythonLibraryHostTestCase(t, tc)
}

func TestSimplePythonLib(t *testing.T) {
	testCases := []bp2buildTestCase{
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
			expectedBazelTargets: []string{
				makeBazelTarget("py_library", "foo", attrNameToString{
					"data": `["files/data.txt"]`,
					"deps": `[":bar"]`,
					"srcs": `[
        "a.py",
        "b/c.py",
        "b/d.py",
    ]`,
					"srcs_version": `"PY3"`,
				}),
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
			expectedBazelTargets: []string{
				makeBazelTarget("py_library", "foo", attrNameToString{
					"srcs":         `["a.py"]`,
					"srcs_version": `"PY2"`,
				}),
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
			expectedBazelTargets: []string{
				makeBazelTarget("py_library", "foo", attrNameToString{
					"srcs":         `["a.py"]`,
					"srcs_version": `"PY3"`,
				}),
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
			expectedBazelTargets: []string{
				// srcs_version is PY2ANDPY3 by default.
				makeBazelTarget("py_library", "foo", attrNameToString{
					"srcs": `["a.py"]`,
				}),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			runPythonLibraryTestCases(t, tc)
		})
	}
}

func TestPythonArchVariance(t *testing.T) {
	runPythonLibraryTestCases(t, bp2buildTestCase{
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
		expectedBazelTargets: []string{
			makeBazelTarget("py_library", "foo", attrNameToString{
				"srcs": `select({
        "//build/bazel/platforms/arch:arm": ["arm.py"],
        "//build/bazel/platforms/arch:x86": ["x86.py"],
        "//conditions:default": [],
    })`,
				"srcs_version": `"PY3"`,
			}),
		},
	})
}
