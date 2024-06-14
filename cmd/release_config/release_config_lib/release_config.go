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
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	rc_proto "android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

// One directory's contribution to the a release config.
type ReleaseConfigContribution struct {
	// Path of the file providing this config contribution.
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

	// True if this release config only allows inheritance and aconfig flag
	// overrides. Build flag value overrides are an error.
	AconfigFlagsOnly bool

	// Unmarshalled flag artifacts
	FlagArtifacts FlagArtifacts

	// The files used by this release config
	FilesUsedMap map[string]bool

	// Generated release config
	ReleaseConfigArtifact *rc_proto.ReleaseConfigArtifact

	// We have begun compiling this release config.
	compileInProgress bool

	// Partitioned artifacts for {partition}/etc/build_flags.json
	PartitionBuildFlags map[string]*rc_proto.FlagArtifacts

	// Prior stage(s) for flag advancement (during development).
	// Once a flag has met criteria in a prior stage, it can advance to this one.
	PriorStagesMap map[string]bool
}

func ReleaseConfigFactory(name string, index int) (c *ReleaseConfig) {
	return &ReleaseConfig{
		Name:             name,
		DeclarationIndex: index,
		FilesUsedMap:     make(map[string]bool),
		PriorStagesMap:   make(map[string]bool),
	}
}

func (config *ReleaseConfig) InheritConfig(iConfig *ReleaseConfig) error {
	for f := range iConfig.FilesUsedMap {
		config.FilesUsedMap[f] = true
	}
	for _, fa := range iConfig.FlagArtifacts {
		name := *fa.FlagDeclaration.Name
		myFa, ok := config.FlagArtifacts[name]
		if !ok {
			return fmt.Errorf("Could not inherit flag %s from %s", name, iConfig.Name)
		}
		if name == "RELEASE_ACONFIG_VALUE_SETS" {
			// If there is a value assigned, add the trace.
			if len(fa.Value.GetStringValue()) > 0 {
				myFa.Traces = append(myFa.Traces, fa.Traces...)
				myFa.Value = &rc_proto.Value{Val: &rc_proto.Value_StringValue{
					myFa.Value.GetStringValue() + " " + fa.Value.GetStringValue()}}
			}
		} else if len(fa.Traces) > 1 {
			// A value was assigned. Set our value.
			myFa.Traces = append(myFa.Traces, fa.Traces[1:]...)
			myFa.Value = fa.Value
		}
	}
	return nil
}

func (config *ReleaseConfig) GetSortedFileList() []string {
	return SortedMapKeys(config.FilesUsedMap)
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

	// Is this a build-prefix release config, such as 'ap3a'?
	isBuildPrefix, err := regexp.MatchString("^[a-z][a-z][0-9][0-9a-z]$", config.Name)
	if err != nil {
		return err
	}
	// Start with only the flag declarations.
	config.FlagArtifacts = configs.FlagArtifacts.Clone()
	releaseAconfigValueSets := config.FlagArtifacts["RELEASE_ACONFIG_VALUE_SETS"]
	releasePlatformVersion := config.FlagArtifacts["RELEASE_PLATFORM_VERSION"]

	// Generate any configs we need to inherit.  This will detect loops in
	// the config.
	contributionsToApply := []*ReleaseConfigContribution{}
	myInherits := []string{}
	myInheritsSet := make(map[string]bool)
	// If there is a "root" release config, it is the start of every inheritance chain.
	_, err = configs.GetReleaseConfig("root")
	if err == nil && !isRoot {
		config.InheritNames = append([]string{"root"}, config.InheritNames...)
	}
	for _, inherit := range config.InheritNames {
		if _, ok := myInheritsSet[inherit]; ok {
			continue
		}
		if isBuildPrefix && configs.Aliases[inherit] != nil {
			return fmt.Errorf("%s cannot inherit from alias %s", config.Name, inherit)
		}
		myInherits = append(myInherits, inherit)
		myInheritsSet[inherit] = true
		iConfig, err := configs.GetReleaseConfig(inherit)
		if err != nil {
			return err
		}
		err = iConfig.GenerateReleaseConfig(configs)
		if err != nil {
			return err
		}
		err = config.InheritConfig(iConfig)
		if err != nil {
			return err
		}
	}

	// If we inherited nothing, then we need to mark the global files as used for this
	// config.  If we inherited, then we already marked them as part of inheritance.
	if len(config.InheritNames) == 0 {
		for f := range configs.FilesUsedMap {
			config.FilesUsedMap[f] = true
		}
	}

	contributionsToApply = append(contributionsToApply, config.Contributions...)

	workflowManual := rc_proto.Workflow(rc_proto.Workflow_MANUAL)
	myDirsMap := make(map[int]bool)
	if isBuildPrefix && releasePlatformVersion != nil {
		if MarshalValue(releasePlatformVersion.Value) != strings.ToUpper(config.Name) {
			value := FlagValue{
				path: config.Contributions[0].path,
				proto: rc_proto.FlagValue{
					Name:  releasePlatformVersion.FlagDeclaration.Name,
					Value: UnmarshalValue(strings.ToUpper(config.Name)),
				},
			}
			if err := releasePlatformVersion.UpdateValue(value); err != nil {
				return err
			}
		}
	}
	for _, contrib := range contributionsToApply {
		contribAconfigValueSets := []string{}
		// Gather the aconfig_value_sets from this contribution, allowing duplicates for simplicity.
		for _, v := range contrib.proto.AconfigValueSets {
			contribAconfigValueSets = append(contribAconfigValueSets, v)
		}
		contribAconfigValueSetsString := strings.Join(contribAconfigValueSets, " ")
		releaseAconfigValueSets.Value = &rc_proto.Value{Val: &rc_proto.Value_StringValue{
			releaseAconfigValueSets.Value.GetStringValue() + " " + contribAconfigValueSetsString}}
		releaseAconfigValueSets.Traces = append(
			releaseAconfigValueSets.Traces,
			&rc_proto.Tracepoint{
				Source: proto.String(contrib.path),
				Value:  &rc_proto.Value{Val: &rc_proto.Value_StringValue{contribAconfigValueSetsString}},
			})

		for _, priorStage := range contrib.proto.PriorStages {
			config.PriorStagesMap[priorStage] = true
		}
		myDirsMap[contrib.DeclarationIndex] = true
		if config.AconfigFlagsOnly && len(contrib.FlagValues) > 0 {
			return fmt.Errorf("%s does not allow build flag overrides", config.Name)
		}
		for _, value := range contrib.FlagValues {
			name := *value.proto.Name
			fa, ok := config.FlagArtifacts[name]
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
				delete(config.FlagArtifacts, name)
			}
		}
	}
	// Now remove any duplicates from the actual value of RELEASE_ACONFIG_VALUE_SETS
	myAconfigValueSets := []string{}
	myAconfigValueSetsMap := map[string]bool{}
	for _, v := range strings.Split(releaseAconfigValueSets.Value.GetStringValue(), " ") {
		if myAconfigValueSetsMap[v] {
			continue
		}
		myAconfigValueSetsMap[v] = true
		myAconfigValueSets = append(myAconfigValueSets, v)
	}
	releaseAconfigValueSets.Value = &rc_proto.Value{Val: &rc_proto.Value_StringValue{strings.TrimSpace(strings.Join(myAconfigValueSets, " "))}}

	directories := []string{}
	for idx, confDir := range configs.configDirs {
		if _, ok := myDirsMap[idx]; ok {
			directories = append(directories, confDir)
		}
	}

	// Now build the per-partition artifacts
	config.PartitionBuildFlags = make(map[string]*rc_proto.FlagArtifacts)
	for _, v := range config.FlagArtifacts {
		artifact, err := v.MarshalWithoutTraces()
		if err != nil {
			return err
		}
		for _, container := range v.FlagDeclaration.Containers {
			if _, ok := config.PartitionBuildFlags[container]; !ok {
				config.PartitionBuildFlags[container] = &rc_proto.FlagArtifacts{}
			}
			config.PartitionBuildFlags[container].FlagArtifacts = append(config.PartitionBuildFlags[container].FlagArtifacts, artifact)
		}
	}
	config.ReleaseConfigArtifact = &rc_proto.ReleaseConfigArtifact{
		Name:       proto.String(config.Name),
		OtherNames: config.OtherNames,
		FlagArtifacts: func() []*rc_proto.FlagArtifact {
			ret := []*rc_proto.FlagArtifact{}
			flagNames := []string{}
			for k := range config.FlagArtifacts {
				flagNames = append(flagNames, k)
			}
			sort.Strings(flagNames)
			for _, flagName := range flagNames {
				flag := config.FlagArtifacts[flagName]
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
		PriorStages:      SortedMapKeys(config.PriorStagesMap),
	}

	config.compileInProgress = false
	return nil
}

// Write the makefile for this targetRelease.
func (config *ReleaseConfig) WriteMakefile(outFile, targetRelease string, configs *ReleaseConfigs) error {
	makeVars := make(map[string]string)

	myFlagArtifacts := config.FlagArtifacts.Clone()
	// Sort the flags by name first.
	names := myFlagArtifacts.SortedFlagNames()
	partitions := make(map[string][]string)

	vNames := []string{}
	addVar := func(name, suffix, value string) {
		fullName := fmt.Sprintf("_ALL_RELEASE_FLAGS.%s.%s", name, suffix)
		vNames = append(vNames, fullName)
		makeVars[fullName] = value
	}

	for _, name := range names {
		flag := myFlagArtifacts[name]
		decl := flag.FlagDeclaration

		for _, container := range decl.Containers {
			partitions[container] = append(partitions[container], name)
		}
		value := MarshalValue(flag.Value)
		makeVars[name] = value
		addVar(name, "TYPE", ValueType(flag.Value))
		addVar(name, "PARTITIONS", strings.Join(decl.Containers, " "))
		addVar(name, "DEFAULT", MarshalValue(decl.Value))
		addVar(name, "VALUE", value)
		addVar(name, "DECLARED_IN", *flag.Traces[0].Source)
		addVar(name, "SET_IN", *flag.Traces[len(flag.Traces)-1].Source)
		addVar(name, "NAMESPACE", *decl.Namespace)
	}
	pNames := []string{}
	for k := range partitions {
		pNames = append(pNames, k)
	}
	slices.Sort(pNames)

	// Now sort the make variables, and output them.
	slices.Sort(vNames)

	// Write the flags as:
	//   _ALL_RELELASE_FLAGS
	//   _ALL_RELEASE_FLAGS.PARTITIONS.*
	//   all _ALL_RELEASE_FLAGS.*, sorted by name
	//   Final flag values, sorted by name.
	data := fmt.Sprintf("# TARGET_RELEASE=%s\n", config.Name)
	if targetRelease != config.Name {
		data += fmt.Sprintf("# User specified TARGET_RELEASE=%s\n", targetRelease)
	}
	// As it stands this list is not per-product, but conceptually it is, and will be.
	data += fmt.Sprintf("ALL_RELEASE_CONFIGS_FOR_PRODUCT :=$= %s\n", strings.Join(configs.GetAllReleaseNames(), " "))
	data += fmt.Sprintf("_used_files := %s\n", strings.Join(config.GetSortedFileList(), " "))
	data += fmt.Sprintf("_ALL_RELEASE_FLAGS :=$= %s\n", strings.Join(names, " "))
	for _, pName := range pNames {
		data += fmt.Sprintf("_ALL_RELEASE_FLAGS.PARTITIONS.%s :=$= %s\n", pName, strings.Join(partitions[pName], " "))
	}
	for _, vName := range vNames {
		data += fmt.Sprintf("%s :=$= %s\n", vName, makeVars[vName])
	}
	data += "\n\n# Values for all build flags\n"
	for _, name := range names {
		data += fmt.Sprintf("%s :=$= %s\n", name, makeVars[name])
	}
	return os.WriteFile(outFile, []byte(data), 0644)
}

func (config *ReleaseConfig) WritePartitionBuildFlags(outDir string) error {
	var err error
	for partition, flags := range config.PartitionBuildFlags {
		slices.SortFunc(flags.FlagArtifacts, func(a, b *rc_proto.FlagArtifact) int {
			return cmp.Compare(*a.FlagDeclaration.Name, *b.FlagDeclaration.Name)
		})
		// The json file name must not be modified as this is read from
		// build_flags_json module
		if err = WriteMessage(filepath.Join(outDir, fmt.Sprintf("build_flags_%s.json", partition)), flags); err != nil {
			return err
		}
	}
	return nil
}
