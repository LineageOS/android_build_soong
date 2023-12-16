package java

import (
	"strings"
	"testing"

	"android/soong/android"
	soongTesting "android/soong/testing"
	"android/soong/testing/test_spec_proto"
	"google.golang.org/protobuf/proto"
)

func TestTestSpec(t *testing.T) {
	bp := `test_spec {
		name: "module-name",
		teamId: "12345",
		tests: [
			"java-test-module-name-one",
			"java-test-module-name-two"
		]
	}

	java_test {
		name: "java-test-module-name-one",
	}

	java_test {
		name: "java-test-module-name-two",
	}`
	result := runTestSpecTest(t, android.FixtureExpectsNoErrors, bp)

	module := result.ModuleForTests(
		"module-name", "",
	).Module().(*soongTesting.TestSpecModule)

	// Check that the provider has the right contents
	data := result.ModuleProvider(
		module, soongTesting.TestSpecProviderKey,
	).(soongTesting.TestSpecProviderData)
	if !strings.HasSuffix(
		data.IntermediatePath.String(), "/intermediateTestSpecMetadata.pb",
	) {
		t.Errorf(
			"Missing intermediates path in provider: %s",
			data.IntermediatePath.String(),
		)
	}

	buildParamsSlice := module.BuildParamsForTests()
	var metadata = ""
	for _, params := range buildParamsSlice {
		if params.Rule.String() == "android/soong/android.writeFile" {
			metadata = params.Args["content"]
		}
	}

	metadataList := make([]*test_spec_proto.TestSpec_OwnershipMetadata, 0, 2)
	teamId := "12345"
	bpFilePath := "Android.bp"
	targetNames := []string{
		"java-test-module-name-one", "java-test-module-name-two",
	}

	for _, test := range targetNames {
		targetName := test
		metadata := test_spec_proto.TestSpec_OwnershipMetadata{
			TrendyTeamId: &teamId,
			TargetName:   &targetName,
			Path:         &bpFilePath,
		}
		metadataList = append(metadataList, &metadata)
	}
	testSpecMetadata := test_spec_proto.TestSpec{OwnershipMetadataList: metadataList}
	protoData, _ := proto.Marshal(&testSpecMetadata)
	rawData := string(protoData)
	formattedData := strings.ReplaceAll(rawData, "\n", "\\n")
	expectedMetadata := "'" + formattedData + "\\n'"

	if metadata != expectedMetadata {
		t.Errorf(
			"Retrieved metadata: %s doesn't contain expectedMetadata: %s", metadata,
			expectedMetadata,
		)
	}

	// Tests for all_test_spec singleton.
	singleton := result.SingletonForTests("all_test_specs")
	rule := singleton.Rule("all_test_specs_rule")
	prebuiltOs := result.Config.PrebuiltOS()
	expectedCmd := "out/soong/host/" + prebuiltOs + "/bin/metadata -rule test_spec -inputFile out/soong/all_test_spec_paths.rsp -outputFile out/soong/ownership/all_test_specs.pb"
	expectedOutputFile := "out/soong/ownership/all_test_specs.pb"
	expectedInputFile := "out/soong/.intermediates/module-name/intermediateTestSpecMetadata.pb"
	if !strings.Contains(
		strings.TrimSpace(rule.Output.String()),
		expectedOutputFile,
	) {
		t.Errorf(
			"Retrieved singletonOutputFile: %s is not equal to expectedSingletonOutputFile: %s",
			rule.Output.String(), expectedOutputFile,
		)
	}

	if !strings.Contains(
		strings.TrimSpace(rule.Inputs[0].String()),
		expectedInputFile,
	) {
		t.Errorf(
			"Retrieved singletonInputFile: %s is not equal to expectedSingletonInputFile: %s",
			rule.Inputs[0].String(), expectedInputFile,
		)
	}

	if !strings.Contains(
		strings.TrimSpace(rule.RuleParams.Command),
		expectedCmd,
	) {
		t.Errorf(
			"Retrieved cmd: %s is not equal to expectedCmd: %s",
			rule.RuleParams.Command, expectedCmd,
		)
	}
}

func runTestSpecTest(
		t *testing.T, errorHandler android.FixtureErrorHandler, bp string,
) *android.TestResult {
	return android.GroupFixturePreparers(
		soongTesting.PrepareForTestWithTestingBuildComponents,
		PrepareForIntegrationTestWithJava,
	).
		ExtendWithErrorHandler(errorHandler).
		RunTestWithBp(t, bp)
}
