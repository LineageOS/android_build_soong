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
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	rc_proto "android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

// A single release_config_map.textproto and its associated data.
// Used primarily for debugging.
type ReleaseConfigMap struct {
	// The path to this release_config_map file.
	path string

	// Data received
	proto rc_proto.ReleaseConfigMap

	// Map of name:contribution for release config contributions.
	ReleaseConfigContributions map[string]*ReleaseConfigContribution

	// Flags declared this directory's flag_declarations/*.textproto
	FlagDeclarations []rc_proto.FlagDeclaration
}

type ReleaseConfigDirMap map[string]int

// The generated release configs.
type ReleaseConfigs struct {
	// Ordered list of release config maps processed.
	ReleaseConfigMaps []*ReleaseConfigMap

	// Aliases
	Aliases map[string]*string

	// Dictionary of flag_name:FlagDeclaration, with no overrides applied.
	FlagArtifacts FlagArtifacts

	// Generated release configs artifact
	Artifact rc_proto.ReleaseConfigsArtifact

	// Dictionary of name:ReleaseConfig
	// Use `GetReleaseConfigs(name)` to get a release config.
	ReleaseConfigs map[string]*ReleaseConfig

	// Map of directory to *ReleaseConfigMap
	releaseConfigMapsMap map[string]*ReleaseConfigMap

	// The list of config directories used.
	configDirs []string

	// A map from the config directory to its order in the list of config
	// directories.
	configDirIndexes ReleaseConfigDirMap
}

// Write the "all_release_configs" artifact.
//
// The file will be in "{outDir}/all_release_configs-{product}.{format}"
//
// Args:
//
//	outDir string: directory path. Will be created if not present.
//	product string: TARGET_PRODUCT for the release_configs.
//	format string: one of "json", "pb", or "textproto"
//
// Returns:
//
//	error: Any error encountered.
func (configs *ReleaseConfigs) WriteArtifact(outDir, product, format string) error {
	return WriteMessage(
		filepath.Join(outDir, fmt.Sprintf("all_release_configs-%s.%s", product, format)),
		&configs.Artifact)
}

func ReleaseConfigsFactory() (c *ReleaseConfigs) {
	return &ReleaseConfigs{
		Aliases:              make(map[string]*string),
		FlagArtifacts:        make(map[string]*FlagArtifact),
		ReleaseConfigs:       make(map[string]*ReleaseConfig),
		releaseConfigMapsMap: make(map[string]*ReleaseConfigMap),
		configDirs:           []string{},
		configDirIndexes:     make(ReleaseConfigDirMap),
	}
}

func ReleaseConfigMapFactory(protoPath string) (m *ReleaseConfigMap) {
	m = &ReleaseConfigMap{
		path:                       protoPath,
		ReleaseConfigContributions: make(map[string]*ReleaseConfigContribution),
	}
	if protoPath != "" {
		LoadMessage(protoPath, &m.proto)
	}
	return m
}

func (configs *ReleaseConfigs) LoadReleaseConfigMap(path string, ConfigDirIndex int) error {
	m := ReleaseConfigMapFactory(path)
	if m.proto.DefaultContainer == nil {
		return fmt.Errorf("Release config map %s lacks default_container", path)
	}
	dir := filepath.Dir(path)
	// Record any aliases, checking for duplicates.
	for _, alias := range m.proto.Aliases {
		name := *alias.Name
		oldTarget, ok := configs.Aliases[name]
		if ok {
			if *oldTarget != *alias.Target {
				return fmt.Errorf("Conflicting alias declarations: %s vs %s",
					*oldTarget, *alias.Target)
			}
		}
		configs.Aliases[name] = alias.Target
	}
	var err error
	err = WalkTextprotoFiles(dir, "flag_declarations", func(path string, d fs.DirEntry, err error) error {
		flagDeclaration := FlagDeclarationFactory(path)
		// Container must be specified.
		if flagDeclaration.Container == nil {
			flagDeclaration.Container = m.proto.DefaultContainer
		}
		// TODO: once we have namespaces initialized, we can throw an error here.
		if flagDeclaration.Namespace == nil {
			flagDeclaration.Namespace = proto.String("android_UNKNOWN")
		}
		// If the input didn't specify a value, create one (== UnspecifiedValue).
		if flagDeclaration.Value == nil {
			flagDeclaration.Value = &rc_proto.Value{Val: &rc_proto.Value_UnspecifiedValue{false}}
		}
		m.FlagDeclarations = append(m.FlagDeclarations, *flagDeclaration)
		name := *flagDeclaration.Name
		if def, ok := configs.FlagArtifacts[name]; !ok {
			configs.FlagArtifacts[name] = &FlagArtifact{FlagDeclaration: flagDeclaration, DeclarationIndex: ConfigDirIndex}
		} else if !proto.Equal(def.FlagDeclaration, flagDeclaration) {
			return fmt.Errorf("Duplicate definition of %s", *flagDeclaration.Name)
		}
		// Set the initial value in the flag artifact.
		configs.FlagArtifacts[name].UpdateValue(
			FlagValue{path: path, proto: rc_proto.FlagValue{
				Name: proto.String(name), Value: flagDeclaration.Value}})
		if configs.FlagArtifacts[name].Redacted {
			return fmt.Errorf("%s may not be redacted by default.", *flagDeclaration.Name)
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = WalkTextprotoFiles(dir, "release_configs", func(path string, d fs.DirEntry, err error) error {
		releaseConfigContribution := &ReleaseConfigContribution{path: path, DeclarationIndex: ConfigDirIndex}
		LoadMessage(path, &releaseConfigContribution.proto)
		name := *releaseConfigContribution.proto.Name
		if fmt.Sprintf("%s.textproto", name) != filepath.Base(path) {
			return fmt.Errorf("%s incorrectly declares release config %s", path, name)
		}
		if _, ok := configs.ReleaseConfigs[name]; !ok {
			configs.ReleaseConfigs[name] = ReleaseConfigFactory(name, ConfigDirIndex)
		}
		config := configs.ReleaseConfigs[name]
		config.InheritNames = append(config.InheritNames, releaseConfigContribution.proto.Inherits...)

		// Only walk flag_values/{RELEASE} for defined releases.
		err2 := WalkTextprotoFiles(dir, filepath.Join("flag_values", name), func(path string, d fs.DirEntry, err error) error {
			flagValue := FlagValueFactory(path)
			if fmt.Sprintf("%s.textproto", *flagValue.proto.Name) != filepath.Base(path) {
				return fmt.Errorf("%s incorrectly sets value for flag %s", path, *flagValue.proto.Name)
			}
			releaseConfigContribution.FlagValues = append(releaseConfigContribution.FlagValues, flagValue)
			return nil
		})
		if err2 != nil {
			return err2
		}
		m.ReleaseConfigContributions[name] = releaseConfigContribution
		config.Contributions = append(config.Contributions, releaseConfigContribution)
		return nil
	})
	if err != nil {
		return err
	}
	configs.ReleaseConfigMaps = append(configs.ReleaseConfigMaps, m)
	configs.releaseConfigMapsMap[dir] = m
	return nil
}

func (configs *ReleaseConfigs) GetReleaseConfig(name string) (*ReleaseConfig, error) {
	trace := []string{name}
	for target, ok := configs.Aliases[name]; ok; target, ok = configs.Aliases[name] {
		name = *target
		trace = append(trace, name)
	}
	if config, ok := configs.ReleaseConfigs[name]; ok {
		return config, nil
	}
	return nil, fmt.Errorf("Missing config %s.  Trace=%v", name, trace)
}

// Write the makefile for this targetRelease.
func (configs *ReleaseConfigs) WriteMakefile(outFile, targetRelease string) error {
	makeVars := make(map[string]string)
	var allReleaseNames []string
	for _, v := range configs.ReleaseConfigs {
		allReleaseNames = append(allReleaseNames, v.Name)
		allReleaseNames = append(allReleaseNames, v.OtherNames...)
	}
	config, err := configs.GetReleaseConfig(targetRelease)
	if err != nil {
		return err
	}

	myFlagArtifacts := config.FlagArtifacts.Clone()
	// Sort the flags by name first.
	names := []string{}
	for k, _ := range myFlagArtifacts {
		names = append(names, k)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Compare(a, b)
	})
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

		// cName := strings.ToLower(rc_proto.Container_name[decl.GetContainer()])
		cName := strings.ToLower(decl.Container.String())
		if cName == strings.ToLower(rc_proto.Container_ALL.String()) {
			partitions["product"] = append(partitions["product"], name)
			partitions["system"] = append(partitions["system"], name)
			partitions["system_ext"] = append(partitions["system_ext"], name)
			partitions["vendor"] = append(partitions["vendor"], name)
		} else {
			partitions[cName] = append(partitions[cName], name)
		}
		value := MarshalValue(flag.Value)
		makeVars[name] = value
		addVar(name, "PARTITIONS", cName)
		addVar(name, "DEFAULT", MarshalValue(decl.Value))
		addVar(name, "VALUE", value)
		addVar(name, "DECLARED_IN", *flag.Traces[0].Source)
		addVar(name, "SET_IN", *flag.Traces[len(flag.Traces)-1].Source)
		addVar(name, "NAMESPACE", *decl.Namespace)
	}
	pNames := []string{}
	for k, _ := range partitions {
		pNames = append(pNames, k)
	}
	slices.SortFunc(pNames, func(a, b string) int {
		return cmp.Compare(a, b)
	})

	// Now sort the make variables, and output them.
	slices.SortFunc(vNames, func(a, b string) int {
		return cmp.Compare(a, b)
	})

	// Write the flags as:
	//   _ALL_RELELASE_FLAGS
	//   _ALL_RELEASE_FLAGS.PARTITIONS.*
	//   all _ALL_RELEASE_FLAGS.*, sorted by name
	//   Final flag values, sorted by name.
	data := fmt.Sprintf("# TARGET_RELEASE=%s\n", config.Name)
	if targetRelease != config.Name {
		data += fmt.Sprintf("# User specified TARGET_RELEASE=%s\n", targetRelease)
	}
	// The variable _all_release_configs will get deleted during processing, so do not mark it read-only.
	data += fmt.Sprintf("_all_release_configs := %s\n", strings.Join(allReleaseNames, " "))
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

func (configs *ReleaseConfigs) GenerateReleaseConfigs(targetRelease string) error {
	otherNames := make(map[string][]string)
	for aliasName, aliasTarget := range configs.Aliases {
		if _, ok := configs.ReleaseConfigs[aliasName]; ok {
			return fmt.Errorf("Alias %s is a declared release config", aliasName)
		}
		if _, ok := configs.ReleaseConfigs[*aliasTarget]; !ok {
			if _, ok2 := configs.Aliases[*aliasTarget]; !ok2 {
				return fmt.Errorf("Alias %s points to non-existing config %s", aliasName, *aliasTarget)
			}
		}
		otherNames[*aliasTarget] = append(otherNames[*aliasTarget], aliasName)
	}
	for name, aliases := range otherNames {
		configs.ReleaseConfigs[name].OtherNames = aliases
	}

	for _, config := range configs.ReleaseConfigs {
		err := config.GenerateReleaseConfig(configs)
		if err != nil {
			return err
		}
	}

	releaseConfig, err := configs.GetReleaseConfig(targetRelease)
	if err != nil {
		return err
	}
	configs.Artifact = rc_proto.ReleaseConfigsArtifact{
		ReleaseConfig: releaseConfig.ReleaseConfigArtifact,
		OtherReleaseConfigs: func() []*rc_proto.ReleaseConfigArtifact {
			orc := []*rc_proto.ReleaseConfigArtifact{}
			for name, config := range configs.ReleaseConfigs {
				if name != releaseConfig.Name {
					orc = append(orc, config.ReleaseConfigArtifact)
				}
			}
			return orc
		}(),
		ReleaseConfigMapsMap: func() map[string]*rc_proto.ReleaseConfigMap {
			ret := make(map[string]*rc_proto.ReleaseConfigMap)
			for k, v := range configs.releaseConfigMapsMap {
				ret[k] = &v.proto
			}
			return ret
		}(),
	}
	return nil
}

func ReadReleaseConfigMaps(releaseConfigMapPaths StringList, targetRelease string) (*ReleaseConfigs, error) {
	var err error

	if len(releaseConfigMapPaths) == 0 {
		releaseConfigMapPaths = GetDefaultMapPaths()
		if len(releaseConfigMapPaths) == 0 {
			return nil, fmt.Errorf("No maps found")
		}
		fmt.Printf("No --map argument provided.  Using: --map %s\n", strings.Join(releaseConfigMapPaths, " --map "))
	}

	configs := ReleaseConfigsFactory()
	for idx, releaseConfigMapPath := range releaseConfigMapPaths {
		// Maintain an ordered list of release config directories.
		configDir := filepath.Dir(releaseConfigMapPath)
		configs.configDirIndexes[configDir] = idx
		configs.configDirs = append(configs.configDirs, configDir)
		err = configs.LoadReleaseConfigMap(releaseConfigMapPath, idx)
		if err != nil {
			return nil, err
		}
	}

	// Now that we have all of the release config maps, can meld them and generate the artifacts.
	err = configs.GenerateReleaseConfigs(targetRelease)
	return configs, err
}
