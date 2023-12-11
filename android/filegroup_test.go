package android

import (
	"path/filepath"
	"testing"
)

func TestFileGroupWithPathProp(t *testing.T) {
	// TODO(b/247782695), TODO(b/242847534) Fix mixed builds for filegroups
	t.Skip("Re-enable once filegroups are corrected for mixed builds")
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
		result := GroupFixturePreparers(
			PrepareForTestWithFilegroup,
		).RunTestWithBp(t, testCase.bp)

		fg := result.Module("baz", "").(*fileGroup)
		AssertStringEquals(t, "src relativeRoot", testCase.rel, fg.srcs[0].Rel())
		AssertStringEquals(t, "src full path", expectedOutputfile, fg.srcs[0].String())
	}
}

func TestFilegroupDefaults(t *testing.T) {
	bp := FixtureAddTextFile("p/Android.bp", `
		filegroup_defaults {
			name: "defaults",
			visibility: ["//x"],
		}
		filegroup {
			name: "foo",
			defaults: ["defaults"],
			visibility: ["//y"],
		}
	`)
	result := GroupFixturePreparers(
		PrepareForTestWithFilegroup,
		PrepareForTestWithDefaults,
		PrepareForTestWithVisibility,
		bp).RunTest(t)
	rules := effectiveVisibilityRules(result.Config, qualifiedModuleName{pkg: "p", name: "foo"})
	AssertDeepEquals(t, "visibility", []string{"//x", "//y"}, rules.Strings())
}
