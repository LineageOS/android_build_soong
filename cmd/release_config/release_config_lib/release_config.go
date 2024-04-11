// Copyright 2024 Google Inc. All rights reserved.
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

package release_config_lib

import (
	"fmt"

	"android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

// One directory's contribution to the a release config.
type ReleaseConfigContribution struct {
	// Paths to files providing this config.
	path string

	// The index of the config directory where this release config
	// contribution was declared.
	// Flag values cannot be set in a location with a lower index.
	DeclarationIndex int

	// Protobufs relevant to the config.
	proto release_config_proto.ReleaseConfig

	FlagValues []*FlagValue
}

// A generated release config.
type ReleaseConfig struct {
	// the Name of the release config
	Name string

	// The index of the config directory where this release config was
	// first declared.
	// Flag values cannot be set in a location with a lower index.
	DeclarationIndex int

	// What contributes to this config.
	Contributions []*ReleaseConfigContribution

	// Aliases for this release
	OtherNames []string

	// The names of release configs that we inherit
	InheritNames []string

	// Unmarshalled flag artifacts
	FlagArtifacts FlagArtifacts

	// Generated release config
	ReleaseConfigArtifact *release_config_proto.ReleaseConfigArtifact

	// We have begun compiling this release config.
	compileInProgress bool
}

func ReleaseConfigFactory(name string, index int) (c *ReleaseConfig) {
	return &ReleaseConfig{Name: name, DeclarationIndex: index}
}

func (config *ReleaseConfig) GenerateReleaseConfig(configs *ReleaseConfigs) error {
	if config.ReleaseConfigArtifact != nil {
		return nil
	}
	if config.compileInProgress {
		return fmt.Errorf("Loop detected for release config %s", config.Name)
	}
	config.compileInProgress = true

	// Generate any configs we need to inherit.  This will detect loops in
	// the config.
	contributionsToApply := []*ReleaseConfigContribution{}
	myInherits := []string{}
	myInheritsSet := make(map[string]bool)
	for _, inherit := range config.InheritNames {
		if _, ok := myInheritsSet[inherit]; ok {
			continue
		}
		myInherits = append(myInherits, inherit)
		myInheritsSet[inherit] = true
		iConfig, err := configs.GetReleaseConfig(inherit)
		if err != nil {
			return err
		}
		iConfig.GenerateReleaseConfig(configs)
		contributionsToApply = append(contributionsToApply, iConfig.Contributions...)
	}
	contributionsToApply = append(contributionsToApply, config.Contributions...)

	myAconfigValueSets := []string{}
	myFlags := configs.FlagArtifacts.Clone()
	myDirsMap := make(map[int]bool)
	for _, contrib := range contributionsToApply {
		myAconfigValueSets = append(myAconfigValueSets, contrib.proto.AconfigValueSets...)
		myDirsMap[contrib.DeclarationIndex] = true
		for _, value := range contrib.FlagValues {
			fa, ok := myFlags[*value.proto.Name]
			if !ok {
				return fmt.Errorf("Setting value for undefined flag %s in %s\n", *value.proto.Name, value.path)
			}
			myDirsMap[fa.DeclarationIndex] = true
			if fa.DeclarationIndex > contrib.DeclarationIndex {
				// Setting location is to the left of declaration.
				return fmt.Errorf("Setting value for flag %s not allowed in %s\n", *value.proto.Name, value.path)
			}
			if err := fa.UpdateValue(*value); err != nil {
				return err
			}
		}
	}

	directories := []string{}
	for idx, confDir := range configs.ConfigDirs {
		if _, ok := myDirsMap[idx]; ok {
			directories = append(directories, confDir)
		}
	}

	config.FlagArtifacts = myFlags
	config.ReleaseConfigArtifact = &release_config_proto.ReleaseConfigArtifact{
		Name:       proto.String(config.Name),
		OtherNames: config.OtherNames,
		FlagArtifacts: func() []*release_config_proto.FlagArtifact {
			ret := []*release_config_proto.FlagArtifact{}
			for _, flag := range myFlags {
				ret = append(ret, &release_config_proto.FlagArtifact{
					FlagDeclaration: flag.FlagDeclaration,
					Traces:          flag.Traces,
					Value:           flag.Value,
				})
			}
			return ret
		}(),
		AconfigValueSets: myAconfigValueSets,
		Inherits:         myInherits,
		Directories:      directories,
	}

	config.compileInProgress = false
	return nil
}
