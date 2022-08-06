package bp2build

import (
	"testing"

	"android/soong/android"
	"android/soong/python"
)

func runBp2BuildTestCaseWithPythonLibraries(t *testing.T, tc Bp2buildTestCase) {
	t.Helper()
	RunBp2BuildTestCase(t, func(ctx android.RegistrationContext) {
		ctx.RegisterModuleType("python_library", python.PythonLibraryFactory)
		ctx.RegisterModuleType("python_library_host", python.PythonLibraryHostFactory)
	}, tc)
}

func TestPythonBinaryHostSimple(t *testing.T) {
	runBp2BuildTestCaseWithPythonLibraries(t, Bp2buildTestCase{
		Description:                "simple python_binary_host converts to a native py_binary",
		ModuleTypeUnderTest:        "python_binary_host",
		ModuleTypeUnderTestFactory: python.PythonBinaryHostFactory,
		Filesystem: map[string]string{
			"a.py":           "",
			"b/c.py":         "",
			"b/d.py":         "",
			"b/e.py":         "",
			"files/data.txt": "",
		},
		Blueprint: `python_binary_host {
    name: "foo",
    main: "a.py",
    srcs: ["**/*.py"],
    exclude_srcs: ["b/e.py"],
    data: ["files/data.txt",],
    libs: ["bar"],
    bazel_module: { bp2build_available: true },
}
    python_library_host {
      name: "bar",
      srcs: ["b/e.py"],
      bazel_module: { bp2build_available: false },
    }`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("py_binary", "foo", AttrNameToString{
				"data":    `["files/data.txt"]`,
				"deps":    `[":bar"]`,
				"main":    `"a.py"`,
				"imports": `["."]`,
				"srcs": `[
        "a.py",
        "b/c.py",
        "b/d.py",
    ]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestPythonBinaryHostPy2(t *testing.T) {
	runBp2BuildTestCaseSimple(t, Bp2buildTestCase{
		Description:                "py2 python_binary_host",
		ModuleTypeUnderTest:        "python_binary_host",
		ModuleTypeUnderTestFactory: python.PythonBinaryHostFactory,
		Blueprint: `python_binary_host {
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
		ExpectedBazelTargets: []string{
			makeBazelTarget("py_binary", "foo", AttrNameToString{
				"python_version": `"PY2"`,
				"imports":        `["."]`,
				"srcs":           `["a.py"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestPythonBinaryHostPy3(t *testing.T) {
	runBp2BuildTestCaseSimple(t, Bp2buildTestCase{
		Description:                "py3 python_binary_host",
		ModuleTypeUnderTest:        "python_binary_host",
		ModuleTypeUnderTestFactory: python.PythonBinaryHostFactory,
		Blueprint: `python_binary_host {
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
		ExpectedBazelTargets: []string{
			// python_version is PY3 by default.
			makeBazelTarget("py_binary", "foo", AttrNameToString{
				"imports": `["."]`,
				"srcs":    `["a.py"]`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}

func TestPythonBinaryHostArchVariance(t *testing.T) {
	runBp2BuildTestCaseSimple(t, Bp2buildTestCase{
		Description:                "test arch variants",
		ModuleTypeUnderTest:        "python_binary_host",
		ModuleTypeUnderTestFactory: python.PythonBinaryHostFactory,
		Filesystem: map[string]string{
			"dir/arm.py": "",
			"dir/x86.py": "",
		},
		Blueprint: `python_binary_host {
					 name: "foo-arm",
					 arch: {
						 arm: {
							 srcs: ["arm.py"],
						 },
						 x86: {
							 srcs: ["x86.py"],
						 },
					},
				 }`,
		ExpectedBazelTargets: []string{
			makeBazelTarget("py_binary", "foo-arm", AttrNameToString{
				"imports": `["."]`,
				"srcs": `select({
        "//build/bazel/platforms/arch:arm": ["arm.py"],
        "//build/bazel/platforms/arch:x86": ["x86.py"],
        "//conditions:default": [],
    })`,
				"target_compatible_with": `select({
        "//build/bazel/platforms/os:android": ["@platforms//:incompatible"],
        "//conditions:default": [],
    })`,
			}),
		},
	})
}
