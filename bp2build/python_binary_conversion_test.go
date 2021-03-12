package bp2build

import (
	"android/soong/android"
	"android/soong/python"
	"fmt"
	"strings"
	"testing"
)

func TestPythonBinaryHost(t *testing.T) {
	testCases := []struct {
		description                        string
		moduleTypeUnderTest                string
		moduleTypeUnderTestFactory         android.ModuleFactory
		moduleTypeUnderTestBp2BuildMutator func(android.TopDownMutatorContext)
		blueprint                          string
		expectedBazelTargets               []string
		filesystem                         map[string]string
	}{
		{
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
    srcs: [
        "**/*.py"
    ],
    exclude_srcs: [
        "b/e.py"
    ],
    data: [
        "files/data.txt",
    ],

    bazel_module: { bp2build_available: true },
}
`,
			expectedBazelTargets: []string{`py_binary(
    name = "foo",
    data = [
        "files/data.txt",
    ],
    main = "a.py",
    srcs = [
        "a.py",
        "b/c.py",
        "b/d.py",
    ],
)`,
			},
		},
		{
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
    srcs = [
        "a.py",
    ],
)`,
			},
		},
		{
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
    srcs = [
        "a.py",
    ],
)`,
			},
		},
	}

	dir := "."
	for _, testCase := range testCases {
		filesystem := make(map[string][]byte)
		toParse := []string{
			"Android.bp",
		}
		for f, content := range testCase.filesystem {
			if strings.HasSuffix(f, "Android.bp") {
				toParse = append(toParse, f)
			}
			filesystem[f] = []byte(content)
		}
		config := android.TestConfig(buildDir, nil, testCase.blueprint, filesystem)
		ctx := android.NewTestContext(config)

		ctx.RegisterModuleType(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestFactory)
		ctx.RegisterBp2BuildMutator(testCase.moduleTypeUnderTest, testCase.moduleTypeUnderTestBp2BuildMutator)
		ctx.RegisterForBazelConversion()

		_, errs := ctx.ParseFileList(dir, toParse)
		if Errored(t, testCase.description, errs) {
			continue
		}
		_, errs = ctx.ResolveDependencies(config)
		if Errored(t, testCase.description, errs) {
			continue
		}

		codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)
		bazelTargets := generateBazelTargetsForDir(codegenCtx, dir)
		if actualCount, expectedCount := len(bazelTargets), len(testCase.expectedBazelTargets); actualCount != expectedCount {
			fmt.Println(bazelTargets)
			t.Errorf("%s: Expected %d bazel target, got %d", testCase.description, expectedCount, actualCount)
		} else {
			for i, target := range bazelTargets {
				if w, g := testCase.expectedBazelTargets[i], target.content; w != g {
					t.Errorf(
						"%s: Expected generated Bazel target to be '%s', got '%s'",
						testCase.description,
						w,
						g,
					)
				}
			}
		}
	}
}
