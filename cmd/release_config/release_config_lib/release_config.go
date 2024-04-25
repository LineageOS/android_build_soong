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
	"strings"

	rc_proto "android/soong/cmd/release_config/release_config_proto"

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
	proto rc_proto.ReleaseConfig

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
	ReleaseConfigArtifact *rc_proto.ReleaseConfigArtifact

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
	isRoot := config.Name == "root"

	// Generate any configs we need to inherit.  This will detect loops in
	// the config.
	contributionsToApply := []*ReleaseConfigContribution{}
	myInherits := []string{}
	myInheritsSet := make(map[string]bool)
	// If there is a "root" release config, it is the start of every inheritance chain.
	_, err := configs.GetReleaseConfig("root")
	if err == nil && !isRoot {
		config.InheritNames = append([]string{"root"}, config.InheritNames...)
	}
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
	myAconfigValueSetsMap := map[string]bool{}
	myFlags := configs.FlagArtifacts.Clone()
	workflowManual := rc_proto.Workflow(rc_proto.Workflow_MANUAL)
	container := rc_proto.Container(rc_proto.Container_ALL)
	releaseAconfigValueSets := FlagArtifact{
		FlagDeclaration: &rc_proto.FlagDeclaration{
			Name:        proto.String("RELEASE_ACONFIG_VALUE_SETS"),
			Namespace:   proto.String("android_UNKNOWN"),
			Description: proto.String("Aconfig value sets assembled by release-config"),
			Workflow:    &workflowManual,
			Container:   &container,
			Value:       &rc_proto.Value{Val: &rc_proto.Value_StringValue{""}},
		},
		DeclarationIndex: -1,
		Traces: []*rc_proto.Tracepoint{
			&rc_proto.Tracepoint{
				Source: proto.String("$release-config"),
				Value:  &rc_proto.Value{Val: &rc_proto.Value_StringValue{""}},
			},
		},
	}
	myFlags["RELEASE_ACONFIG_VALUE_SETS"] = &releaseAconfigValueSets
	myDirsMap := make(map[int]bool)
	for _, contrib := range contributionsToApply {
		if len(contrib.proto.AconfigValueSets) > 0 {
			contribAconfigValueSets := []string{}
			for _, v := range contrib.proto.AconfigValueSets {
				if _, ok := myAconfigValueSetsMap[v]; !ok {
					contribAconfigValueSets = append(contribAconfigValueSets, v)
					myAconfigValueSetsMap[v] = true
				}
			}
			myAconfigValueSets = append(myAconfigValueSets, contribAconfigValueSets...)
			releaseAconfigValueSets.Traces = append(
				releaseAconfigValueSets.Traces,
				&rc_proto.Tracepoint{
					Source: proto.String(contrib.path),
					Value:  &rc_proto.Value{Val: &rc_proto.Value_StringValue{strings.Join(contribAconfigValueSets, " ")}},
				})
		}
		myDirsMap[contrib.DeclarationIndex] = true
		for _, value := range contrib.FlagValues {
			name := *value.proto.Name
			fa, ok := myFlags[name]
			if !ok {
				return fmt.Errorf("Setting value for undefined flag %s in %s\n", name, value.path)
			}
			myDirsMap[fa.DeclarationIndex] = true
			if fa.DeclarationIndex > contrib.DeclarationIndex {
				// Setting location is to the left of declaration.
				return fmt.Errorf("Setting value for flag %s not allowed in %s\n", name, value.path)
			}
			if isRoot && *fa.FlagDeclaration.Workflow != workflowManual {
				// The "root" release config can only contain workflow: MANUAL flags.
				return fmt.Errorf("Setting value for non-MANUAL flag %s is not allowed in %s", name, value.path)
			}
			if err := fa.UpdateValue(*value); err != nil {
				return err
			}
			if fa.Redacted {
				delete(myFlags, name)
			}
		}
	}
	releaseAconfigValueSets.Value = &rc_proto.Value{Val: &rc_proto.Value_StringValue{strings.Join(myAconfigValueSets, " ")}}

	directories := []string{}
	for idx, confDir := range configs.configDirs {
		if _, ok := myDirsMap[idx]; ok {
			directories = append(directories, confDir)
		}
	}

	config.FlagArtifacts = myFlags
	config.ReleaseConfigArtifact = &rc_proto.ReleaseConfigArtifact{
		Name:       proto.String(config.Name),
		OtherNames: config.OtherNames,
		FlagArtifacts: func() []*rc_proto.FlagArtifact {
			ret := []*rc_proto.FlagArtifact{}
			for _, flag := range myFlags {
				ret = append(ret, &rc_proto.FlagArtifact{
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
