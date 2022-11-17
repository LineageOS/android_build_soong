package android

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"android/soong/bazel/cquery"
	analysis_v2_proto "prebuilts/bazel/common/proto/analysis_v2"

	"google.golang.org/protobuf/proto"
)

var testConfig = TestConfig("out", nil, "", nil)

func TestRequestResultsAfterInvokeBazel(t *testing.T) {
	label := "@//foo:bar"
	cfg := configKey{"arm64_armv8-a", Android}
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{
		bazelCommand{command: "cquery", expression: "deps(@soong_injection//mixed_builds:buildroot, 2)"}: `@//foo:bar|arm64_armv8-a|android>>out/foo/bar.txt`,
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
 "artifacts": [
   { "id": 1, "path_fragment_id": 1 },
   { "id": 2, "path_fragment_id": 2 }],
 "actions": [{
   "target_Id": 1,
   "action_Key": "x",
   "mnemonic": "x",
   "arguments": ["touch", "foo"],
   "input_dep_set_ids": [1],
   "output_Ids": [1],
   "primary_output_id": 1
 }],
 "dep_set_of_files": [
   { "id": 1, "direct_artifact_ids": [1, 2] }],
 "path_fragments": [
   { "id": 1, "label": "one" },
   { "id": 2, "label": "two" }]
}`,
			"cd 'test/exec_root' && rm -rf 'one' && touch foo",
		}, {`
{
 "artifacts": [
   { "id": 1, "path_fragment_id": 10 },
   { "id": 2, "path_fragment_id": 20 }],
 "actions": [{
   "target_Id": 100,
   "action_Key": "x",
   "mnemonic": "x",
   "arguments": ["bogus", "command"],
   "output_Ids": [1, 2],
   "primary_output_id": 1
 }],
 "path_fragments": [
   { "id": 10, "label": "one", "parent_id": 30 },
   { "id": 20, "label": "one.d", "parent_id": 30 },
   { "id": 30, "label": "parent" }]
}`,
			`cd 'test/exec_root' && rm -rf 'parent/one' && bogus command && sed -i'' -E 's@(^|\s|")bazel-out/@\1test/bazel_out/@g' 'parent/one.d'`,
		},
	}

	for i, testCase := range testCases {
		data, err := JsonToActionGraphContainer(testCase.input)
		if err != nil {
			t.Error(err)
		}
		bazelContext, _ := testBazelContext(t, map[bazelCommand]string{
			bazelCommand{command: "aquery", expression: "deps(@soong_injection//mixed_builds:buildroot)"}: string(data)})

		err = bazelContext.InvokeBazel(testConfig)
		if err != nil {
			t.Fatalf("testCase #%d: did not expect error invoking Bazel, but got %s", i+1, err)
		}

		got := bazelContext.BuildStatementsToRegister()
		if want := 1; len(got) != want {
			t.Fatalf("expected %d registered build statements, but got %#v", want, got)
		}

		cmd := RuleBuilderCommand{}
		createCommand(&cmd, got[0], "test/exec_root", "test/bazel_out", PathContextForTesting(TestConfig("out", nil, "", nil)))
		if actual, expected := cmd.buf.String(), testCase.command; expected != actual {
			t.Errorf("expected: [%s], actual: [%s]", expected, actual)
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

	testConfig.productVariables.NativeCoveragePaths = []string{"*"}
	testConfig.productVariables.NativeCoverageExcludePaths = nil
	verifyExtraFlags(t, testConfig, `--collect_code_coverage --instrumentation_filter=+.*`)

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

// Transform the json format to ActionGraphContainer
func JsonToActionGraphContainer(inputString string) ([]byte, error) {
	var aqueryProtoResult analysis_v2_proto.ActionGraphContainer
	err := json.Unmarshal([]byte(inputString), &aqueryProtoResult)
	if err != nil {
		return []byte(""), err
	}
	data, _ := proto.Marshal(&aqueryProtoResult)
	return data, err
}
