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

	"android/soong/android"
	"android/soong/testing/code_metadata_internal_proto"
	"github.com/google/blueprint"
	"google.golang.org/protobuf/proto"
)

func CodeMetadataFactory() android.Module {
	module := &CodeMetadataModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)

	return module
}

type CodeMetadataModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// Properties for "code_metadata"
	properties struct {
		// Specifies the name of the code_config.
		Name string
		// Specifies the team ID.
		TeamId string
		// Specifies the list of modules that this code_metadata covers.
		Code []string
		// An optional field to specify if multiple ownerships for source files is allowed.
		MultiOwnership bool
	}
}

type codeDepTagType struct {
	blueprint.BaseDependencyTag
}

var codeDepTag = codeDepTagType{}

func (module *CodeMetadataModule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Validate Properties
	if len(module.properties.TeamId) == 0 {
		ctx.PropertyErrorf(
			"TeamId",
			"Team Id not found in the code_metadata module. Hint: Maybe the teamId property hasn't been properly specified.",
		)
	}
	if !isInt(module.properties.TeamId) {
		ctx.PropertyErrorf(
			"TeamId", "Invalid value for Team ID. The Team ID must be an integer.",
		)
	}
	if len(module.properties.Code) == 0 {
		ctx.PropertyErrorf(
			"Code",
			"Targets to be attributed cannot be empty. Hint: Maybe the code property hasn't been properly specified.",
		)
	}
	ctx.AddDependency(ctx.Module(), codeDepTag, module.properties.Code...)
}

// Provider published by CodeMetadata
type CodeMetadataProviderData struct {
	IntermediatePath android.WritablePath
}

var CodeMetadataProviderKey = blueprint.NewProvider(CodeMetadataProviderData{})

func (module *CodeMetadataModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	metadataList := make(
		[]*code_metadata_internal_proto.CodeMetadataInternal_TargetOwnership, 0,
		len(module.properties.Code),
	)
	bpFilePath := filepath.Join(ctx.ModuleDir(), ctx.BlueprintsFile())

	for _, m := range ctx.GetDirectDepsWithTag(codeDepTag) {
		targetName := m.Name()
		var moduleSrcs []string
		if ctx.OtherModuleHasProvider(m, blueprint.SrcsFileProviderKey) {
			moduleSrcs = ctx.OtherModuleProvider(
				m, blueprint.SrcsFileProviderKey,
			).(blueprint.SrcsFileProviderData).SrcPaths
		}
		if module.properties.MultiOwnership {
			metadata := &code_metadata_internal_proto.CodeMetadataInternal_TargetOwnership{
				TargetName:     &targetName,
				TrendyTeamId:   &module.properties.TeamId,
				Path:           &bpFilePath,
				MultiOwnership: &module.properties.MultiOwnership,
				SourceFiles:    moduleSrcs,
			}
			metadataList = append(metadataList, metadata)
		} else {
			metadata := &code_metadata_internal_proto.CodeMetadataInternal_TargetOwnership{
				TargetName:   &targetName,
				TrendyTeamId: &module.properties.TeamId,
				Path:         &bpFilePath,
				SourceFiles:  moduleSrcs,
			}
			metadataList = append(metadataList, metadata)
		}

	}
	codeMetadata := &code_metadata_internal_proto.CodeMetadataInternal{TargetOwnershipList: metadataList}
	protoData, err := proto.Marshal(codeMetadata)
	if err != nil {
		ctx.ModuleErrorf("Error marshaling code metadata: %s", err.Error())
		return
	}
	intermediatePath := android.PathForModuleOut(
		ctx, "intermediateCodeMetadata.pb",
	)
	android.WriteFileRule(ctx, intermediatePath, string(protoData))

	ctx.SetProvider(
		CodeMetadataProviderKey,
		CodeMetadataProviderData{IntermediatePath: intermediatePath},
	)
}
