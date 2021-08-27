package bp2build

import (
	"fmt"
	"testing"

	"android/soong/android"
	"android/soong/python"
)

// TODO(alexmarquez): Should be lifted into a generic Bp2Build file
type PythonLibBp2Build func(ctx android.TopDownMutatorContext)

func TestPythonLibrary(t *testing.T) {
	testPythonLib(t, "python_library",
		python.PythonLibraryFactory, python.PythonLibraryBp2Build,
		func(ctx android.RegistrationContext) {})
}

func TestPythonLibraryHost(t *testing.T) {
	testPythonLib(t, "python_library_host",
		python.PythonLibraryHostFactory, python.PythonLibraryHostBp2Build,
		func(ctx android.RegistrationContext) {
			ctx.RegisterModuleType("python_library", python.PythonLibraryFactory)
		})
}

func testPythonLib(t *testing.T, modType string,
	factory android.ModuleFactory, mutator PythonLibBp2Build,
	registration func(ctx android.RegistrationContext)) {
	t.Helper()
	// Simple
	runBp2BuildTestCase(t, registration, bp2buildTestCase{
		description:                        fmt.Sprintf("simple %s converts to a native py_library", modType),
		moduleTypeUnderTest:                modType,
		moduleTypeUnderTestFactory:         factory,
		moduleTypeUnderTestBp2BuildMutator: mutator,
		filesystem: map[string]string{
			"a.py":           "",
			"b/c.py":         "",
			"b/d.py":         "",
			"b/e.py":         "",
			"files/data.txt": "",
		},
		blueprint: fmt.Sprintf(`%s {
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
    }`, modType),
		expectedBazelTargets: []string{`py_library(
    name = "foo",
    data = ["files/data.txt"],
    deps = [":bar"],
    srcs = [
        "a.py",
        "b/c.py",
        "b/d.py",
    ],
    srcs_version = "PY3",
)`,
		},
	})

	// PY2
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        fmt.Sprintf("py2 %s converts to a native py_library", modType),
		moduleTypeUnderTest:                modType,
		moduleTypeUnderTestFactory:         factory,
		moduleTypeUnderTestBp2BuildMutator: mutator,
		blueprint: fmt.Sprintf(`%s {
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
}`, modType),
		expectedBazelTargets: []string{`py_library(
    name = "foo",
    srcs = ["a.py"],
    srcs_version = "PY2",
)`,
		},
	})

	// PY3
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        fmt.Sprintf("py3 %s converts to a native py_library", modType),
		moduleTypeUnderTest:                modType,
		moduleTypeUnderTestFactory:         factory,
		moduleTypeUnderTestBp2BuildMutator: mutator,
		blueprint: fmt.Sprintf(`%s {
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
}`, modType),
		expectedBazelTargets: []string{`py_library(
    name = "foo",
    srcs = ["a.py"],
    srcs_version = "PY3",
)`,
		},
	})

	// Both
	runBp2BuildTestCaseSimple(t, bp2buildTestCase{
		description:                        fmt.Sprintf("py2&3 %s converts to a native py_library", modType),
		moduleTypeUnderTest:                modType,
		moduleTypeUnderTestFactory:         factory,
		moduleTypeUnderTestBp2BuildMutator: mutator,
		blueprint: fmt.Sprintf(`%s {
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
}`, modType),
		expectedBazelTargets: []string{
			// srcs_version is PY2ANDPY3 by default.
			`py_library(
    name = "foo",
    srcs = ["a.py"],
)`,
		},
	})
}
