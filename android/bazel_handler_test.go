package android

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"android/soong/bazel/cquery"
)

var testConfig = TestConfig("out", nil, "", nil)

func TestRequestResultsAfterInvokeBazel(t *testing.T) {
	label := "//foo:bar"
	cfg := configKey{"arm64_armv8-a", Android}
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{
		bazelCommand{command: "cquery", expression: "deps(@soong_injection//mixed_builds:buildroot, 2)"}: `//foo:bar|arm64_armv8-a|android>>out/foo/bar.txt`,
	})
	bazelContext.QueueBazelRequest(label, cquery.GetOutputFiles, cfg)
	err := bazelContext.InvokeBazel(testConfig)
	if err != nil {
		t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
	}
	g, err := bazelContext.GetOutputFiles(label, cfg)
	if err != nil {
		t.Errorf("Expected cquery results after running InvokeBazel(), but got err %v", err)
	} else if w := []string{"out/foo/bar.txt"}; !reflect.DeepEqual(w, g) {
		t.Errorf("Expected output %s, got %s", w, g)
	}
}

func TestInvokeBazelWritesBazelFiles(t *testing.T) {
	bazelContext, baseDir := testBazelContext(t, map[bazelCommand]string{})
	err := bazelContext.InvokeBazel(testConfig)
	if err != nil {
		t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "soong_injection", "mixed_builds", "main.bzl")); os.IsNotExist(err) {
		t.Errorf("Expected main.bzl to exist, but it does not")
	} else if err != nil {
		t.Errorf("Unexpected error stating main.bzl %s", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "soong_injection", "mixed_builds", "BUILD.bazel")); os.IsNotExist(err) {
		t.Errorf("Expected BUILD.bazel to exist, but it does not")
	} else if err != nil {
		t.Errorf("Unexpected error stating BUILD.bazel %s", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "soong_injection", "WORKSPACE.bazel")); os.IsNotExist(err) {
		t.Errorf("Expected WORKSPACE.bazel to exist, but it does not")
	} else if err != nil {
		t.Errorf("Unexpected error stating WORKSPACE.bazel %s", err)
	}
}

func TestInvokeBazelPopulatesBuildStatements(t *testing.T) {
	type testCase struct {
		input   string
		command string
	}

	var testCases = []testCase{
		{`
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 1
  }, {
    "id": 2,
    "pathFragmentId": 2
  }],
  "actions": [{
    "targetId": 1,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["touch", "foo"],
    "inputDepSetIds": [1],
    "outputIds": [1],
    "primaryOutputId": 1
  }],
  "depSetOfFiles": [{
    "id": 1,
    "directArtifactIds": [1, 2]
  }],
  "pathFragments": [{
    "id": 1,
    "label": "one"
  }, {
    "id": 2,
    "label": "two"
  }]
}`,
			"cd 'er' && rm -f one && touch foo",
		}, {`
{
  "artifacts": [{
    "id": 1,
    "pathFragmentId": 10
  }, {
    "id": 2,
    "pathFragmentId": 20
  }],
  "actions": [{
    "targetId": 100,
    "actionKey": "x",
    "mnemonic": "x",
    "arguments": ["bogus", "command"],
    "outputIds": [1, 2],
    "primaryOutputId": 1
  }],
  "pathFragments": [{
    "id": 10,
    "label": "one",
    "parentId": 30
  }, {
    "id": 20,
    "label": "one.d",
    "parentId": 30
  }, {
    "id": 30,
    "label": "parent"
  }]
}`,
			`cd 'er' && rm -f parent/one && bogus command && sed -i'' -E 's@(^|\s|")bazel-out/@\1bo/@g' 'parent/one.d'`,
		},
	}

	for _, testCase := range testCases {
		bazelContext, _ := testBazelContext(t, map[bazelCommand]string{
			bazelCommand{command: "aquery", expression: "deps(@soong_injection//mixed_builds:buildroot)"}: testCase.input})

		err := bazelContext.InvokeBazel(testConfig)
		if err != nil {
			t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
		}

		got := bazelContext.BuildStatementsToRegister()
		if want := 1; len(got) != want {
			t.Errorf("expected %d registered build statements, but got %#v", want, got)
		}

		cmd := RuleBuilderCommand{}
		createCommand(&cmd, got[0], "er", "bo", PathContextForTesting(TestConfig("out", nil, "", nil)))
		if actual := cmd.buf.String(); testCase.command != actual {
			t.Errorf("expected: [%s], actual: [%s]", testCase.command, actual)
		}
	}
}

func TestCoverageFlagsAfterInvokeBazel(t *testing.T) {
	testConfig.productVariables.ClangCoverage = boolPtr(true)

	testConfig.productVariables.NativeCoveragePaths = []string{"foo1", "foo2"}
	testConfig.productVariables.NativeCoverageExcludePaths = []string{"bar1", "bar2"}
	verifyExtraFlags(t, testConfig, `--collect_code_coverage --instrumentation_filter=+foo1,+foo2,-bar1,-bar2`)

	testConfig.productVariables.NativeCoveragePaths = []string{"foo1"}
	testConfig.productVariables.NativeCoverageExcludePaths = []string{"bar1"}
	verifyExtraFlags(t, testConfig, `--collect_code_coverage --instrumentation_filter=+foo1,-bar1`)

	testConfig.productVariables.NativeCoveragePaths = []string{"foo1"}
	testConfig.productVariables.NativeCoverageExcludePaths = nil
	verifyExtraFlags(t, testConfig, `--collect_code_coverage --instrumentation_filter=+foo1`)

	testConfig.productVariables.NativeCoveragePaths = nil
	testConfig.productVariables.NativeCoverageExcludePaths = []string{"bar1"}
	verifyExtraFlags(t, testConfig, `--collect_code_coverage --instrumentation_filter=-bar1`)

	testConfig.productVariables.ClangCoverage = boolPtr(false)
	actual := verifyExtraFlags(t, testConfig, ``)
	if strings.Contains(actual, "--collect_code_coverage") ||
		strings.Contains(actual, "--instrumentation_filter=") {
		t.Errorf("Expected code coverage disabled, but got %#v", actual)
	}
}

func verifyExtraFlags(t *testing.T, config Config, expected string) string {
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{})

	err := bazelContext.InvokeBazel(config)
	if err != nil {
		t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
	}

	flags := bazelContext.bazelRunner.(*mockBazelRunner).extraFlags
	if expected := 3; len(flags) != expected {
		t.Errorf("Expected %d extra flags got %#v", expected, flags)
	}

	actual := flags[1]
	if !strings.Contains(actual, expected) {
		t.Errorf("Expected %#v got %#v", expected, actual)
	}

	return actual
}

func testBazelContext(t *testing.T, bazelCommandResults map[bazelCommand]string) (*bazelContext, string) {
	t.Helper()
	p := bazelPaths{
		soongOutDir:  t.TempDir(),
		outputBase:   "outputbase",
		workspaceDir: "workspace_dir",
	}
	aqueryCommand := bazelCommand{command: "aquery", expression: "deps(@soong_injection//mixed_builds:buildroot)"}
	if _, exists := bazelCommandResults[aqueryCommand]; !exists {
		bazelCommandResults[aqueryCommand] = "{}\n"
	}
	runner := &mockBazelRunner{bazelCommandResults: bazelCommandResults}
	return &bazelContext{
		bazelRunner: runner,
		paths:       &p,
		requests:    map[cqueryKey]bool{},
	}, p.soongOutDir
}
