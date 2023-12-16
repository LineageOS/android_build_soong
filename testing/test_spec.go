// Copyright 2020 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testing

import (
	"path/filepath"
	"strconv"

	"android/soong/android"
	"android/soong/testing/test_spec_proto"
	"github.com/google/blueprint"
	"google.golang.org/protobuf/proto"
)

// ErrTestModuleDataNotFound is the error message for missing test module provider data.
const ErrTestModuleDataNotFound = "The module '%s' does not provide test specification data. Hint: This issue could arise if either the module is not a valid testing module or if it lacks the required 'TestModuleProviderKey' provider.\n"

func TestSpecFactory() android.Module {
	module := &TestSpecModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)

	return module
}

type TestSpecModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// Properties for "test_spec"
	properties struct {
		// Specifies the name of the test config.
		Name string
		// Specifies the team ID.
		TeamId string
		// Specifies the list of tests covered under this module.
		Tests []string
	}
}

type testsDepTagType struct {
	blueprint.BaseDependencyTag
}

var testsDepTag = testsDepTagType{}

func (module *TestSpecModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Validate Properties
	if len(module.properties.TeamId) == 0 {
		ctx.PropertyErrorf("TeamId", "Team Id not found in the test_spec module. Hint: Maybe the TeamId property hasn't been properly specified.")
	}
	if !isInt(module.properties.TeamId) {
		ctx.PropertyErrorf("TeamId", "Invalid value for Team ID. The Team ID must be an integer.")
	}
	if len(module.properties.Tests) == 0 {
		ctx.PropertyErrorf("Tests", "Expected to attribute some test but none found. Hint: Maybe the test property hasn't been properly specified.")
	}
	ctx.AddDependency(ctx.Module(), testsDepTag, module.properties.Tests...)
}
func isInt(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// Provider published by TestSpec
type TestSpecProviderData struct {
	IntermediatePath android.WritablePath
}

var TestSpecProviderKey = blueprint.NewProvider(TestSpecProviderData{})

type TestModuleProviderData struct {
}

var TestModuleProviderKey = blueprint.NewProvider(TestModuleProviderData{})

func (module *TestSpecModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	for _, m := range ctx.GetDirectDepsWithTag(testsDepTag) {
		if !ctx.OtherModuleHasProvider(m, TestModuleProviderKey) {
			ctx.ModuleErrorf(ErrTestModuleDataNotFound, m.Name())
		}
	}
	bpFilePath := filepath.Join(ctx.ModuleDir(), ctx.BlueprintsFile())
	metadataList := make(
		[]*test_spec_proto.TestSpec_OwnershipMetadata, 0,
		len(module.properties.Tests),
	)
	for _, test := range module.properties.Tests {
		targetName := test
		metadata := test_spec_proto.TestSpec_OwnershipMetadata{
			TrendyTeamId: &module.properties.TeamId,
			TargetName:   &targetName,
			Path:         &bpFilePath,
		}
		metadataList = append(metadataList, &metadata)
	}
	intermediatePath := android.PathForModuleOut(
		ctx, "intermediateTestSpecMetadata.pb",
	)
	testSpecMetadata := test_spec_proto.TestSpec{OwnershipMetadataList: metadataList}
	protoData, err := proto.Marshal(&testSpecMetadata)
	if err != nil {
		ctx.ModuleErrorf("Error: %s", err.Error())
	}
	android.WriteFileRule(ctx, intermediatePath, string(protoData))

	ctx.SetProvider(
		TestSpecProviderKey, TestSpecProviderData{
			IntermediatePath: intermediatePath,
		},
	)
}
