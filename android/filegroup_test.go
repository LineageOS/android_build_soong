package android

import (
	"path/filepath"
	"testing"
)

func TestFileGroupWithPathProp(t *testing.T) {
	outBaseDir := "outputbase"
	pathPrefix := outBaseDir + "/execroot/__main__"
	expectedOutputfile := filepath.Join(pathPrefix, "a/b/c/d/test.aidl")

	testCases := []struct {
		bp  string
		rel string
	}{
		{
			bp: `
	filegroup {
		name: "baz",
		srcs: ["a/b/c/d/test.aidl"],
		path: "a/b",
		bazel_module: { label: "//:baz" },
	}
`,
			rel: "c/d/test.aidl",
		},
		{
			bp: `
	filegroup {
		name: "baz",
		srcs: ["a/b/c/d/test.aidl"],
		bazel_module: { label: "//:baz" },
	}
`,
			rel: "a/b/c/d/test.aidl",
		},
	}

	for _, testCase := range testCases {
		outBaseDir := "outputbase"
		result := GroupFixturePreparers(
			PrepareForTestWithFilegroup,
			FixtureModifyConfig(func(config Config) {
				config.BazelContext = MockBazelContext{
					OutputBaseDir: outBaseDir,
					LabelToOutputFiles: map[string][]string{
						"//:baz": []string{"a/b/c/d/test.aidl"},
					},
				}
			}),
		).RunTestWithBp(t, testCase.bp)

		fg := result.Module("baz", "").(*fileGroup)
		AssertStringEquals(t, "src relativeRoot", testCase.rel, fg.srcs[0].Rel())
		AssertStringEquals(t, "src full path", expectedOutputfile, fg.srcs[0].String())
	}
}
