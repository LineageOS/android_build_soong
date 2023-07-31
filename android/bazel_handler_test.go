package android

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"android/soong/bazel"
	"android/soong/bazel/cquery"
	analysis_v2_proto "prebuilts/bazel/common/proto/analysis_v2"

	"github.com/google/blueprint/metrics"
	"google.golang.org/protobuf/proto"
)

var testConfig = TestConfig("out", nil, "", nil)

type testInvokeBazelContext struct{}

type mockBazelRunner struct {
	testHelper *testing.T
	// Stores mock behavior. If an issueBazelCommand request is made for command
	// k, and {k:v} is present in this map, then the mock will return v.
	bazelCommandResults map[bazelCommand]string
	// Requests actually made of the mockBazelRunner with issueBazelCommand,
	// keyed by the command they represent.
	bazelCommandRequests map[bazelCommand]bazel.CmdRequest
}

func (r *mockBazelRunner) bazelCommandForRequest(cmdRequest bazel.CmdRequest) bazelCommand {
	for _, arg := range cmdRequest.Argv {
		for _, cmdType := range allBazelCommands {
			if arg == cmdType.command {
				return cmdType
			}
		}
	}
	r.testHelper.Fatalf("Unrecognized bazel request: %s", cmdRequest)
	return cqueryCmd
}

func (r *mockBazelRunner) issueBazelCommand(cmdRequest bazel.CmdRequest, paths *bazelPaths, eventHandler *metrics.EventHandler) (string, string, error) {
	command := r.bazelCommandForRequest(cmdRequest)
	r.bazelCommandRequests[command] = cmdRequest
	return r.bazelCommandResults[command], "", nil
}

func (t *testInvokeBazelContext) GetEventHandler() *metrics.EventHandler {
	return &metrics.EventHandler{}
}

func TestRequestResultsAfterInvokeBazel(t *testing.T) {
	label_foo := "@//foo:foo"
	label_bar := "@//foo:bar"
	apexKey := ApexConfigKey{
		WithinApex:     true,
		ApexSdkVersion: "29",
		ApiDomain:      "myapex",
	}
	cfg_foo := configKey{"arm64_armv8-a", Android, apexKey}
	cfg_bar := configKey{arch: "arm64_armv8-a", osType: Android}
	cmd_results := []string{
		`@//foo:foo|arm64_armv8-a|android|within_apex|29|myapex>>out/foo/foo.txt`,
		`@//foo:bar|arm64_armv8-a|android>>out/foo/bar.txt`,
	}
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{cqueryCmd: strings.Join(cmd_results, "\n")})

	bazelContext.QueueBazelRequest(label_foo, cquery.GetOutputFiles, cfg_foo)
	bazelContext.QueueBazelRequest(label_bar, cquery.GetOutputFiles, cfg_bar)
	err := bazelContext.InvokeBazel(testConfig, &testInvokeBazelContext{})
	if err != nil {
		t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
	}
	verifyCqueryResult(t, bazelContext, label_foo, cfg_foo, "out/foo/foo.txt")
	verifyCqueryResult(t, bazelContext, label_bar, cfg_bar, "out/foo/bar.txt")
}

func verifyCqueryResult(t *testing.T, ctx *mixedBuildBazelContext, label string, cfg configKey, result string) {
	g, err := ctx.GetOutputFiles(label, cfg)
	if err != nil {
		t.Errorf("Expected cquery results after running InvokeBazel(), but got err %v", err)
	} else if w := []string{result}; !reflect.DeepEqual(w, g) {
		t.Errorf("Expected output %s, got %s", w, g)
	}
}

func TestInvokeBazelWritesBazelFiles(t *testing.T) {
	bazelContext, baseDir := testBazelContext(t, map[bazelCommand]string{})
	err := bazelContext.InvokeBazel(testConfig, &testInvokeBazelContext{})
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
		bazelContext, _ := testBazelContext(t, map[bazelCommand]string{aqueryCmd: string(data)})

		err = bazelContext.InvokeBazel(testConfig, &testInvokeBazelContext{})
		if err != nil {
			t.Fatalf("testCase #%d: did not expect error invoking Bazel, but got %s", i+1, err)
		}

		got := bazelContext.BuildStatementsToRegister()
		if want := 1; len(got) != want {
			t.Fatalf("expected %d registered build statements, but got %#v", want, got)
		}

		cmd := RuleBuilderCommand{}
		ctx := builderContextForTests{PathContextForTesting(TestConfig("out", nil, "", nil))}
		createCommand(&cmd, got[0], "test/exec_root", "test/bazel_out", ctx, map[string]bazel.AqueryDepset{}, "")
		if actual, expected := cmd.buf.String(), testCase.command; expected != actual {
			t.Errorf("expected: [%s], actual: [%s]", expected, actual)
		}
	}
}

func TestMixedBuildSandboxedAction(t *testing.T) {
	input := `{
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
}`
	data, err := JsonToActionGraphContainer(input)
	if err != nil {
		t.Error(err)
	}
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{aqueryCmd: string(data)})

	err = bazelContext.InvokeBazel(testConfig, &testInvokeBazelContext{})
	if err != nil {
		t.Fatalf("TestMixedBuildSandboxedAction did not expect error invoking Bazel, but got %s", err)
	}

	statement := bazelContext.BuildStatementsToRegister()[0]
	statement.ShouldRunInSbox = true

	cmd := RuleBuilderCommand{}
	ctx := builderContextForTests{PathContextForTesting(TestConfig("out", nil, "", nil))}
	createCommand(&cmd, statement, "test/exec_root", "test/bazel_out", ctx, map[string]bazel.AqueryDepset{}, "")
	// Assert that the output is generated in an intermediate directory
	// fe05bcdcdc4928012781a5f1a2a77cbb5398e106 is the sha1 checksum of "one"
	if actual, expected := cmd.outputs[0].String(), "out/soong/mixed_build_sbox_intermediates/fe05bcdcdc4928012781a5f1a2a77cbb5398e106/test/exec_root/one"; expected != actual {
		t.Errorf("expected: [%s], actual: [%s]", expected, actual)
	}

	// Assert the actual command remains unchanged inside the sandbox
	if actual, expected := cmd.buf.String(), "mkdir -p 'test/exec_root' && cd 'test/exec_root' && rm -rf 'one' && touch foo"; expected != actual {
		t.Errorf("expected: [%s], actual: [%s]", expected, actual)
	}
}

func TestCoverageFlagsAfterInvokeBazel(t *testing.T) {
	testConfig.productVariables.ClangCoverage = boolPtr(true)

	testConfig.productVariables.NativeCoveragePaths = []string{"foo1", "foo2"}
	testConfig.productVariables.NativeCoverageExcludePaths = []string{"bar1", "bar2"}
	verifyAqueryContainsFlags(t, testConfig, "--collect_code_coverage", "--instrumentation_filter=+foo1,+foo2,-bar1,-bar2")

	testConfig.productVariables.NativeCoveragePaths = []string{"foo1"}
	testConfig.productVariables.NativeCoverageExcludePaths = []string{"bar1"}
	verifyAqueryContainsFlags(t, testConfig, "--collect_code_coverage", "--instrumentation_filter=+foo1,-bar1")

	testConfig.productVariables.NativeCoveragePaths = []string{"foo1"}
	testConfig.productVariables.NativeCoverageExcludePaths = nil
	verifyAqueryContainsFlags(t, testConfig, "--collect_code_coverage", "--instrumentation_filter=+foo1")

	testConfig.productVariables.NativeCoveragePaths = nil
	testConfig.productVariables.NativeCoverageExcludePaths = []string{"bar1"}
	verifyAqueryContainsFlags(t, testConfig, "--collect_code_coverage", "--instrumentation_filter=-bar1")

	testConfig.productVariables.NativeCoveragePaths = []string{"*"}
	testConfig.productVariables.NativeCoverageExcludePaths = nil
	verifyAqueryContainsFlags(t, testConfig, "--collect_code_coverage", "--instrumentation_filter=+.*")

	testConfig.productVariables.ClangCoverage = boolPtr(false)
	verifyAqueryDoesNotContainSubstrings(t, testConfig, "collect_code_coverage", "instrumentation_filter")
}

func TestBazelRequestsSorted(t *testing.T) {
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{})

	cfgKeyArm64Android := configKey{arch: "arm64_armv8-a", osType: Android}
	cfgKeyArm64Linux := configKey{arch: "arm64_armv8-a", osType: Linux}
	cfgKeyOtherAndroid := configKey{arch: "otherarch", osType: Android}

	bazelContext.QueueBazelRequest("zzz", cquery.GetOutputFiles, cfgKeyArm64Android)
	bazelContext.QueueBazelRequest("ccc", cquery.GetApexInfo, cfgKeyArm64Android)
	bazelContext.QueueBazelRequest("duplicate", cquery.GetOutputFiles, cfgKeyArm64Android)
	bazelContext.QueueBazelRequest("duplicate", cquery.GetOutputFiles, cfgKeyArm64Android)
	bazelContext.QueueBazelRequest("xxx", cquery.GetOutputFiles, cfgKeyArm64Linux)
	bazelContext.QueueBazelRequest("aaa", cquery.GetOutputFiles, cfgKeyArm64Android)
	bazelContext.QueueBazelRequest("aaa", cquery.GetOutputFiles, cfgKeyOtherAndroid)
	bazelContext.QueueBazelRequest("bbb", cquery.GetOutputFiles, cfgKeyOtherAndroid)

	if len(bazelContext.requests) != 7 {
		t.Error("Expected 7 request elements, but got", len(bazelContext.requests))
	}

	lastString := ""
	for _, val := range bazelContext.requests {
		thisString := val.String()
		if thisString <= lastString {
			t.Errorf("Requests are not ordered correctly. '%s' came before '%s'", lastString, thisString)
		}
		lastString = thisString
	}
}

func TestIsModuleNameAllowed(t *testing.T) {
	libDisabled := "lib_disabled"
	libEnabled := "lib_enabled"
	libDclaWithinApex := "lib_dcla_within_apex"
	libDclaNonApex := "lib_dcla_non_apex"
	libNotConverted := "lib_not_converted"

	disabledModules := map[string]bool{
		libDisabled: true,
	}
	enabledModules := map[string]bool{
		libEnabled: true,
	}
	dclaEnabledModules := map[string]bool{
		libDclaWithinApex: true,
		libDclaNonApex:    true,
	}

	bazelContext := &mixedBuildBazelContext{
		bazelEnabledModules:     enabledModules,
		bazelDisabledModules:    disabledModules,
		bazelDclaEnabledModules: dclaEnabledModules,
	}

	if bazelContext.IsModuleNameAllowed(libDisabled, true) {
		t.Fatalf("%s shouldn't be allowed for mixed build", libDisabled)
	}

	if !bazelContext.IsModuleNameAllowed(libEnabled, true) {
		t.Fatalf("%s should be allowed for mixed build", libEnabled)
	}

	if !bazelContext.IsModuleNameAllowed(libDclaWithinApex, true) {
		t.Fatalf("%s should be allowed for mixed build", libDclaWithinApex)
	}

	if bazelContext.IsModuleNameAllowed(libDclaNonApex, false) {
		t.Fatalf("%s shouldn't be allowed for mixed build", libDclaNonApex)
	}

	if bazelContext.IsModuleNameAllowed(libNotConverted, true) {
		t.Fatalf("%s shouldn't be allowed for mixed build", libNotConverted)
	}
}

func verifyAqueryContainsFlags(t *testing.T, config Config, expected ...string) {
	t.Helper()
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{})

	err := bazelContext.InvokeBazel(config, &testInvokeBazelContext{})
	if err != nil {
		t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
	}

	sliceContains := func(slice []string, x string) bool {
		for _, s := range slice {
			if s == x {
				return true
			}
		}
		return false
	}

	aqueryArgv := bazelContext.bazelRunner.(*mockBazelRunner).bazelCommandRequests[aqueryCmd].Argv

	for _, expectedFlag := range expected {
		if !sliceContains(aqueryArgv, expectedFlag) {
			t.Errorf("aquery does not contain expected flag %#v. Argv was: %#v", expectedFlag, aqueryArgv)
		}
	}
}

func verifyAqueryDoesNotContainSubstrings(t *testing.T, config Config, substrings ...string) {
	t.Helper()
	bazelContext, _ := testBazelContext(t, map[bazelCommand]string{})

	err := bazelContext.InvokeBazel(config, &testInvokeBazelContext{})
	if err != nil {
		t.Fatalf("Did not expect error invoking Bazel, but got %s", err)
	}

	sliceContainsSubstring := func(slice []string, substring string) bool {
		for _, s := range slice {
			if strings.Contains(s, substring) {
				return true
			}
		}
		return false
	}

	aqueryArgv := bazelContext.bazelRunner.(*mockBazelRunner).bazelCommandRequests[aqueryCmd].Argv

	for _, substring := range substrings {
		if sliceContainsSubstring(aqueryArgv, substring) {
			t.Errorf("aquery contains unexpected substring %#v. Argv was: %#v", substring, aqueryArgv)
		}
	}
}

func testBazelContext(t *testing.T, bazelCommandResults map[bazelCommand]string) (*mixedBuildBazelContext, string) {
	t.Helper()
	p := bazelPaths{
		soongOutDir:  t.TempDir(),
		outputBase:   "outputbase",
		workspaceDir: "workspace_dir",
	}
	if _, exists := bazelCommandResults[aqueryCmd]; !exists {
		bazelCommandResults[aqueryCmd] = ""
	}
	runner := &mockBazelRunner{
		testHelper:           t,
		bazelCommandResults:  bazelCommandResults,
		bazelCommandRequests: map[bazelCommand]bazel.CmdRequest{},
	}
	return &mixedBuildBazelContext{
		bazelRunner: runner,
		paths:       &p,
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
