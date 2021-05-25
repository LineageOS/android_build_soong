package bp2build

import (
	"testing"

	"android/soong/android"
	"android/soong/python"
)

func runPythonTestCase(t *testing.T, tc bp2buildTestCase) {
	t.Helper()
	runBp2BuildTestCase(t, func(ctx android.RegistrationContext) {}, tc)
}

func TestPythonBinaryHostSimple(t *testing.T) {
	runPythonTestCase(t, bp2buildTestCase{
		description:                        "simple python_binary_host converts to a native py_binary",
		moduleTypeUnderTest:                "python_binary_host",
		moduleTypeUnderTestFactory:         python.PythonBinaryHostFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonBinaryBp2Build,
		filesystem: map[string]string{
			"a.py":           "",
			"b/c.py":         "",
			"b/d.py":         "",
			"b/e.py":         "",
			"files/data.txt": "",
		},
		blueprint: `python_binary_host {
    name: "foo",
    main: "a.py",
    srcs: ["**/*.py"],
    exclude_srcs: ["b/e.py"],
    data: ["files/data.txt",],
    bazel_module: { bp2build_available: true },
}
`,
		expectedBazelTargets: []string{`py_binary(
    name = "foo",
    data = ["files/data.txt"],
    main = "a.py",
    srcs = [
        "a.py",
        "b/c.py",
        "b/d.py",
    ],
)`,
		},
	})
}

func TestPythonBinaryHostPy2(t *testing.T) {
	runPythonTestCase(t, bp2buildTestCase{
		description:                        "py2 python_binary_host",
		moduleTypeUnderTest:                "python_binary_host",
		moduleTypeUnderTestFactory:         python.PythonBinaryHostFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonBinaryBp2Build,
		blueprint: `python_binary_host {
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
		expectedBazelTargets: []string{`py_binary(
    name = "foo",
    python_version = "PY2",
    srcs = ["a.py"],
)`,
		},
	})
}

func TestPythonBinaryHostPy3(t *testing.T) {
	runPythonTestCase(t, bp2buildTestCase{
		description:                        "py3 python_binary_host",
		moduleTypeUnderTest:                "python_binary_host",
		moduleTypeUnderTestFactory:         python.PythonBinaryHostFactory,
		moduleTypeUnderTestBp2BuildMutator: python.PythonBinaryBp2Build,
		blueprint: `python_binary_host {
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
			// python_version is PY3 by default.
			`py_binary(
    name = "foo",
    srcs = ["a.py"],
)`,
		},
	})
}
