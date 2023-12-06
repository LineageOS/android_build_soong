package java

import (
	"strings"
	"testing"

	"android/soong/android"
	soongTesting "android/soong/testing"
	"android/soong/testing/code_metadata_internal_proto"
	"google.golang.org/protobuf/proto"
)

func TestCodeMetadata(t *testing.T) {
	bp := `code_metadata {
		name: "module-name",
		teamId: "12345",
		code: [
			"foo",
		]
	}

	java_sdk_library {
		name: "foo",
		srcs: ["a.java"],
	}`
	result := runCodeMetadataTest(t, android.FixtureExpectsNoErrors, bp)

	module := result.ModuleForTests(
		"module-name", "",
	).Module().(*soongTesting.CodeMetadataModule)

	// Check that the provider has the right contents
	data := result.ModuleProvider(
		module, soongTesting.CodeMetadataProviderKey,
	).(soongTesting.CodeMetadataProviderData)
	if !strings.HasSuffix(
		data.IntermediatePath.String(), "/intermediateCodeMetadata.pb",
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

	metadataList := make([]*code_metadata_internal_proto.CodeMetadataInternal_TargetOwnership, 0, 2)
	teamId := "12345"
	bpFilePath := "Android.bp"
	targetName := "foo"
	srcFile := []string{"a.java"}
	expectedMetadataProto := code_metadata_internal_proto.CodeMetadataInternal_TargetOwnership{
		TrendyTeamId: &teamId,
		TargetName:   &targetName,
		Path:         &bpFilePath,
		SourceFiles:  srcFile,
	}
	metadataList = append(metadataList, &expectedMetadataProto)

	CodeMetadataMetadata := code_metadata_internal_proto.CodeMetadataInternal{TargetOwnershipList: metadataList}
	protoData, _ := proto.Marshal(&CodeMetadataMetadata)
	rawData := string(protoData)
	formattedData := strings.ReplaceAll(rawData, "\n", "\\n")
	expectedMetadata := "'" + formattedData + "\\n'"

	if metadata != expectedMetadata {
		t.Errorf(
			"Retrieved metadata: %s is not equal to expectedMetadata: %s", metadata,
			expectedMetadata,
		)
	}

	// Tests for all_test_spec singleton.
	singleton := result.SingletonForTests("all_code_metadata")
	rule := singleton.Rule("all_code_metadata_rule")
	prebuiltOs := result.Config.PrebuiltOS()
	expectedCmd := "out/soong/host/" + prebuiltOs + "/bin/metadata -rule code_metadata -inputFile out/soong/all_code_metadata_paths.rsp -outputFile out/soong/ownership/all_code_metadata.pb"
	expectedOutputFile := "out/soong/ownership/all_code_metadata.pb"
	expectedInputFile := "out/soong/.intermediates/module-name/intermediateCodeMetadata.pb"
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
			"Retrieved cmd: %s doesn't contain expectedCmd: %s",
			rule.RuleParams.Command, expectedCmd,
		)
	}
}
func runCodeMetadataTest(
		t *testing.T, errorHandler android.FixtureErrorHandler, bp string,
) *android.TestResult {
	return android.GroupFixturePreparers(
		soongTesting.PrepareForTestWithTestingBuildComponents, prepareForJavaTest,
		PrepareForTestWithJavaSdkLibraryFiles, FixtureWithLastReleaseApis("foo"),
	).
		ExtendWithErrorHandler(errorHandler).
		RunTestWithBp(t, bp)
}
