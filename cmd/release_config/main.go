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

package main

import (
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

var verboseFlag bool

type StringList []string

func (l *StringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func (l *StringList) String() string {
	return fmt.Sprintf("%v", *l)
}

var releaseConfigMapPaths StringList

func DumpProtos(outDir string, message proto.Message) error {
	basePath := filepath.Join(outDir, "all_release_configs")
	writer := func(suffix string, marshal func() ([]byte, error)) error {
		data, err := marshal()
		if err != nil {
			return err
		}
		return os.WriteFile(fmt.Sprintf("%s.%s", basePath, suffix), data, 0644)
	}
	err := writer("textproto", func() ([]byte, error) { return prototext.MarshalOptions{Multiline: true}.Marshal(message) })
	if err != nil {
		return err
	}

	err = writer("pb", func() ([]byte, error) { return proto.Marshal(message) })
	if err != nil {
		return err
	}

	return writer("json", func() ([]byte, error) { return json.MarshalIndent(message, "", "  ") })
}

func LoadTextproto(path string, message proto.Message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ret := prototext.Unmarshal(data, message)
	if verboseFlag {
		debug, _ := prototext.Marshal(message)
		fmt.Printf("%s: %s\n", path, debug)
	}
	return ret
}

func WalkTextprotoFiles(root string, subdir string, Func fs.WalkDirFunc) error {
	path := filepath.Join(root, subdir)
	if _, err := os.Stat(path); err != nil {
		// Missing subdirs are not an error.
		return nil
	}
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(d.Name(), ".textproto") && d.Type().IsRegular() {
			return Func(path, d, err)
		}
		return nil
	})
}

type FlagValue struct {
	// The path providing this value.
	path string

	// Protobuf
	proto release_config_proto.FlagValue
}

func FlagValueFactory(protoPath string) (fv *FlagValue) {
	fv = &FlagValue{path: protoPath}
	if protoPath != "" {
		LoadTextproto(protoPath, &fv.proto)
	}
	return fv
}

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

// A single release_config_map.textproto and its associated data.
// Used primarily for debugging.
type ReleaseConfigMap struct {
	// The path to this release_config_map file.
	path string

	// Data received
	proto release_config_proto.ReleaseConfigMap

	ReleaseConfigContributions map[string]*ReleaseConfigContribution
	FlagDeclarations           []release_config_proto.FlagDeclaration
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

type FlagArtifact struct {
	FlagDeclaration *release_config_proto.FlagDeclaration

	// The index of the config directory where this flag was declared.
	// Flag values cannot be set in a location with a lower index.
	DeclarationIndex int

	Traces []*release_config_proto.Tracepoint

	// Assigned value
	Value *release_config_proto.Value
}

// Key is flag name.
type FlagArtifacts map[string]*FlagArtifact

type ReleaseConfigDirMap map[string]int

// The generated release configs.
type ReleaseConfigs struct {
	// Ordered list of release config maps processed.
	ReleaseConfigMaps []*ReleaseConfigMap

	// Aliases
	Aliases map[string]*string

	// Dictionary of flag_name:FlagDeclaration, with no overrides applied.
	FlagArtifacts FlagArtifacts

	// Dictionary of name:ReleaseConfig
	ReleaseConfigs map[string]*ReleaseConfig

	// Generated release configs
	Artifact release_config_proto.ReleaseConfigsArtifact

	// The list of config directories used.
	ConfigDirs []string

	// A map from the config directory to its order in the list of config
	// directories.
	ConfigDirIndexes ReleaseConfigDirMap
}

func (src *FlagArtifact) Clone() *FlagArtifact {
	value := &release_config_proto.Value{}
	proto.Merge(value, src.Value)
	return &FlagArtifact{
		FlagDeclaration: src.FlagDeclaration,
		Traces:          src.Traces,
		Value:           value,
	}
}

func (src FlagArtifacts) Clone() (dst FlagArtifacts) {
	if dst == nil {
		dst = make(FlagArtifacts)
	}
	for k, v := range src {
		dst[k] = v.Clone()
	}
	return
}

func ReleaseConfigFactory(name string, index int) (c *ReleaseConfig) {
	return &ReleaseConfig{Name: name, DeclarationIndex: index}
}

func ReleaseConfigsFactory() (c *ReleaseConfigs) {
	return &ReleaseConfigs{
		Aliases:          make(map[string]*string),
		FlagArtifacts:    make(map[string]*FlagArtifact),
		ReleaseConfigs:   make(map[string]*ReleaseConfig),
		ConfigDirs:       []string{},
		ConfigDirIndexes: make(ReleaseConfigDirMap),
	}
}

func ReleaseConfigMapFactory(protoPath string) (m *ReleaseConfigMap) {
	m = &ReleaseConfigMap{
		path:                       protoPath,
		ReleaseConfigContributions: make(map[string]*ReleaseConfigContribution),
	}
	if protoPath != "" {
		LoadTextproto(protoPath, &m.proto)
	}
	return m
}

func FlagDeclarationFactory(protoPath string) (fd *release_config_proto.FlagDeclaration) {
	fd = &release_config_proto.FlagDeclaration{}
	if protoPath != "" {
		LoadTextproto(protoPath, fd)
	}
	return fd
}

func (configs *ReleaseConfigs) LoadReleaseConfigMap(path string, ConfigDirIndex int) error {
	m := ReleaseConfigMapFactory(path)
	if m.proto.Origin == nil || *m.proto.Origin == "" {
		return fmt.Errorf("Release config map %s lacks origin", path)
	}
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
		// TODO: drop flag_declaration.origin from the proto.
		if flagDeclaration.Origin == nil {
			flagDeclaration.Origin = m.proto.Origin
		}
		// There is always a default value.
		if flagDeclaration.Value == nil {
			flagDeclaration.Value = &release_config_proto.Value{Val: &release_config_proto.Value_UnspecifiedValue{true}}
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
			FlagValue{path: path, proto: release_config_proto.FlagValue{
				Name: proto.String(name), Value: flagDeclaration.Value}})
		return nil
	})
	if err != nil {
		return err
	}

	err = WalkTextprotoFiles(dir, "release_configs", func(path string, d fs.DirEntry, err error) error {
		releaseConfigContribution := &ReleaseConfigContribution{path: path, DeclarationIndex: ConfigDirIndex}
		LoadTextproto(path, &releaseConfigContribution.proto)
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

func (configs *ReleaseConfigs) DumpMakefile(outDir, targetRelease string) error {
	outFile := filepath.Join(outDir, "release_config.mk")
	makeVars := make(map[string]string)
	config, err := configs.GetReleaseConfig(targetRelease)
	if err != nil {
		return err
	}
	// Sort the flags by name first.
	names := []string{}
	for k, _ := range config.FlagArtifacts {
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
		flag := config.FlagArtifacts[name]
		decl := flag.FlagDeclaration

		// cName := strings.ToLower(release_config_proto.Container_name[decl.GetContainer()])
		cName := strings.ToLower(decl.Container.String())
		if cName == strings.ToLower(release_config_proto.Container_ALL.String()) {
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
		addVar(name, "ORIGIN", *decl.Origin)
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
	data := fmt.Sprintf("_ALL_RELEASE_FLAGS :=$= %s\n", strings.Join(names, " "))
	for _, pName := range pNames {
		data += fmt.Sprintf("_ALL_RELEASE_FLAGS.PARTITIONS.%s :=$= %s\n", pName, strings.Join(partitions[pName], " "))
	}
	for _, vName := range vNames {
		data += fmt.Sprintf("%s :=$= %s\n", vName, makeVars[vName])
	}
	data += "\n\n# Values for all build flags\n"
	data += fmt.Sprintf("RELEASE_ACONFIG_VALUE_SETS :=$= %s\n",
		strings.Join(config.ReleaseConfigArtifact.AconfigValueSets, " "))
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
	configs.Artifact = release_config_proto.ReleaseConfigsArtifact{
		ReleaseConfig: releaseConfig.ReleaseConfigArtifact,
		OtherReleaseConfigs: func() []*release_config_proto.ReleaseConfigArtifact {
			orc := []*release_config_proto.ReleaseConfigArtifact{}
			for name, config := range configs.ReleaseConfigs {
				if name != releaseConfig.Name {
					orc = append(orc, config.ReleaseConfigArtifact)
				}
			}
			return orc
		}(),
	}
	return nil
}

func MarshalValue(value *release_config_proto.Value) string {
	switch val := value.Val.(type) {
	case *release_config_proto.Value_UnspecifiedValue:
		// Value was never set.
		return ""
	case *release_config_proto.Value_StringValue:
		return val.StringValue
	case *release_config_proto.Value_BoolValue:
		if val.BoolValue {
			return "true"
		}
		// False ==> empty string
		return ""
	case *release_config_proto.Value_Obsolete:
		return " #OBSOLETE"
	default:
		// Flagged as error elsewhere, so return empty string here.
		return ""
	}
}

func (fa *FlagArtifact) UpdateValue(flagValue FlagValue) error {
	name := *flagValue.proto.Name
	fa.Traces = append(fa.Traces, &release_config_proto.Tracepoint{Source: proto.String(flagValue.path), Value: flagValue.proto.Value})
	if fa.Value.GetObsolete() {
		return fmt.Errorf("Attempting to set obsolete flag %s. Trace=%v", name, fa.Traces)
	}
	switch val := flagValue.proto.Value.Val.(type) {
	case *release_config_proto.Value_StringValue:
		fa.Value = &release_config_proto.Value{Val: &release_config_proto.Value_StringValue{val.StringValue}}
	case *release_config_proto.Value_BoolValue:
		fa.Value = &release_config_proto.Value{Val: &release_config_proto.Value_BoolValue{val.BoolValue}}
	case *release_config_proto.Value_Obsolete:
		if !val.Obsolete {
			return fmt.Errorf("%s: Cannot set obsolete=false.  Trace=%v", name, fa.Traces)
		}
		fa.Value = &release_config_proto.Value{Val: &release_config_proto.Value_Obsolete{true}}
	default:
		return fmt.Errorf("Invalid type for flag_value: %T.  Trace=%v", val, fa.Traces)
	}
	return nil
}

func (fa *FlagArtifact) Marshal() (*release_config_proto.FlagArtifact, error) {
	return &release_config_proto.FlagArtifact{
		FlagDeclaration: fa.FlagDeclaration,
		Value:           fa.Value,
		Traces:          fa.Traces,
	}, nil
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

func main() {
	var targetRelease string
	var outputDir string

	outEnv := os.Getenv("OUT_DIR")
	if outEnv == "" {
		outEnv = "out"
	}
	defaultOutputDir := filepath.Join(outEnv, "soong", "release-config")
	var defaultMapPaths StringList
	defaultLocations := StringList{
		"build/release/release_config_map.textproto",
		"vendor/google_shared/build/release/release_config_map.textproto",
		"vendor/google/release/release_config_map.textproto",
	}
	for _, path := range defaultLocations {
		if _, err := os.Stat(path); err == nil {
			defaultMapPaths = append(defaultMapPaths, path)
		}
	}
	prodMaps := os.Getenv("PRODUCT_RELEASE_CONFIG_MAPS")
	if prodMaps != "" {
		defaultMapPaths = append(defaultMapPaths, strings.Split(prodMaps, " ")...)
	}

	flag.BoolVar(&verboseFlag, "debug", false, "print debugging information")
	flag.Var(&releaseConfigMapPaths, "map", "path to a release_config_map.textproto. may be repeated")
	flag.StringVar(&targetRelease, "release", "trunk_staging", "TARGET_RELEASE for this build")
	flag.StringVar(&outputDir, "out_dir", defaultOutputDir, "basepath for the output. Multiple formats are created")
	flag.Parse()

	if len(releaseConfigMapPaths) == 0 {
		releaseConfigMapPaths = defaultMapPaths
		fmt.Printf("No --map argument provided.  Using: --map %s\n", strings.Join(releaseConfigMapPaths, " --map "))
	}

	configs := ReleaseConfigsFactory()
	for idx, releaseConfigMapPath := range releaseConfigMapPaths {
		// Maintain an ordered list of release config directories.
		configDir := filepath.Dir(releaseConfigMapPath)
		configs.ConfigDirIndexes[configDir] = idx
		configs.ConfigDirs = append(configs.ConfigDirs, configDir)
		err := configs.LoadReleaseConfigMap(releaseConfigMapPath, idx)
		if err != nil {
			panic(err)
		}
	}

	// Now that we have all of the release config maps, can meld them and generate the artifacts.
	err := configs.GenerateReleaseConfigs(targetRelease)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(outputDir, 0775)
	if err != nil {
		panic(err)
	}
	err = configs.DumpMakefile(outputDir, targetRelease)
	if err != nil {
		panic(err)
	}
	DumpProtos(outputDir, &configs.Artifact)
}
