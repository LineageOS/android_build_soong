package bp2build

import (
	"testing"

	"android/soong/python"
)

func TestPythonLibrarySimple(t *testing.T) {
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        "simple python_library converts to a native py_library",
		moduleTypeUnderTest:                "python_library",
		moduleTypeUnderTestFactory:         python.PythonLibraryFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonLibraryBp2Build,
		filesystem: map[string]string{
			"a.py":           "",
			"b/c.py":         "",
			"b/d.py":         "",
			"b/e.py":         "",
			"files/data.txt": "",
		},
		blueprint: `python_library {
    name: "foo",
    srcs: ["**/*.py"],
    exclude_srcs: ["b/e.py"],
    data: ["files/data.txt",],
    bazel_module: { bp2build_available: true },
}
`,
		expectedBazelTargets: []string{`py_library(
    name = "foo",
    data = ["files/data.txt"],
    srcs = [
        "a.py",
        "b/c.py",
        "b/d.py",
    ],
    srcs_version = "PY3",
)`,
		},
	})
}

func TestPythonLibraryPy2(t *testing.T) {
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        "py2 python_library",
		moduleTypeUnderTest:                "python_library",
		moduleTypeUnderTestFactory:         python.PythonLibraryFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonLibraryBp2Build,
		blueprint: `python_library {
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
}
`,
		expectedBazelTargets: []string{`py_library(
    name = "foo",
    srcs = ["a.py"],
    srcs_version = "PY2",
)`,
		},
	})
}

func TestPythonLibraryPy3(t *testing.T) {
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        "py3 python_library",
		moduleTypeUnderTest:                "python_library",
		moduleTypeUnderTestFactory:         python.PythonLibraryFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonLibraryBp2Build,
		blueprint: `python_library {
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
}
`,
		expectedBazelTargets: []string{
			`py_library(
    name = "foo",
    srcs = ["a.py"],
    srcs_version = "PY3",
)`,
		},
	})
}

func TestPythonLibraryPyBoth(t *testing.T) {
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        "py3 python_library",
		moduleTypeUnderTest:                "python_library",
		moduleTypeUnderTestFactory:         python.PythonLibraryFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonLibraryBp2Build,
		blueprint: `python_library {
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
}
`,
		expectedBazelTargets: []string{
			// srcs_version is PY2ANDPY3 by default.
			`py_library(
    name = "foo",
    srcs = ["a.py"],
)`,
		},
	})
}
