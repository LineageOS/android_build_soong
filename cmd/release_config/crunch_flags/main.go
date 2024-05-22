package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	rc_lib "android/soong/cmd/release_config/release_config_lib"
	rc_proto "android/soong/cmd/release_config/release_config_proto"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

var (
	// When a flag declaration has an initial value that is a string, the default workflow is PREBUILT.
	// If the flag name starts with any of prefixes in manualFlagNamePrefixes, it is MANUAL.
	manualFlagNamePrefixes []string = []string{
		"RELEASE_ACONFIG_",
		"RELEASE_PLATFORM_",
		"RELEASE_BUILD_FLAGS_",
	}

	// Set `aconfig_flags_only: true` in these release configs.
	aconfigFlagsOnlyConfigs map[string]bool = map[string]bool{
		"trunk_food": true,
	}

	// Default namespace value.  This is intentionally invalid.
	defaultFlagNamespace string = "android_UNKNOWN"

	// What is the current name for "next".
	nextName string = "ap3a"
)

func RenameNext(name string) string {
	if name == "next" {
		return nextName
	}
	return name
}

func WriteFile(path string, message proto.Message) error {
	data, err := prototext.MarshalOptions{Multiline: true}.Marshal(message)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(path), 0775)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func WalkValueFiles(dir string, Func fs.WalkDirFunc) error {
	valPath := filepath.Join(dir, "build_config")
	if _, err := os.Stat(valPath); err != nil {
		fmt.Printf("%s not found, ignoring.\n", valPath)
		return nil
	}

	return filepath.WalkDir(valPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(d.Name(), ".scl") && d.Type().IsRegular() {
			return Func(path, d, err)
		}
		return nil
	})
}

func ProcessBuildFlags(dir string, namespaceMap map[string]string) error {
	var rootAconfigModule string

	path := filepath.Join(dir, "build_flags.scl")
	if _, err := os.Stat(path); err != nil {
		fmt.Printf("%s not found, ignoring.\n", path)
		return nil
	} else {
		fmt.Printf("Processing %s\n", path)
	}
	commentRegexp, err := regexp.Compile("^[[:space:]]*#(?<comment>.+)")
	if err != nil {
		return err
	}
	declRegexp, err := regexp.Compile("^[[:space:]]*flag.\"(?<name>[A-Z_0-9]+)\",[[:space:]]*(?<container>[_A-Z]*),[[:space:]]*(?<value>(\"[^\"]*\"|[^\",)]*))")
	if err != nil {
		return err
	}
	declIn, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(declIn), "\n")
	var description string
	for _, line := range lines {
		if comment := commentRegexp.FindStringSubmatch(commentRegexp.FindString(line)); comment != nil {
			// Description is the text from any contiguous series of lines before a `flag()` call.
			descLine := strings.TrimSpace(comment[commentRegexp.SubexpIndex("comment")])
			if !strings.HasPrefix(descLine, "keep-sorted") {
				description += fmt.Sprintf(" %s", descLine)
			}
			continue
		}
		matches := declRegexp.FindStringSubmatch(declRegexp.FindString(line))
		if matches == nil {
			// The line is neither a comment nor a `flag()` call.
			// Discard any description we have gathered and process the next line.
			description = ""
			continue
		}
		declName := matches[declRegexp.SubexpIndex("name")]
		declValue := matches[declRegexp.SubexpIndex("value")]
		description = strings.TrimSpace(description)
		containers := []string{strings.ToLower(matches[declRegexp.SubexpIndex("container")])}
		if containers[0] == "all" {
			containers = []string{"product", "system", "system_ext", "vendor"}
		}
		var namespace string
		var ok bool
		if namespace, ok = namespaceMap[declName]; !ok {
			namespace = defaultFlagNamespace
		}
		flagDeclaration := &rc_proto.FlagDeclaration{
			Name:        proto.String(declName),
			Namespace:   proto.String(namespace),
			Description: proto.String(description),
			Containers:  containers,
		}
		description = ""
		// Most build flags are `workflow: PREBUILT`.
		workflow := rc_proto.Workflow(rc_proto.Workflow_PREBUILT)
		switch {
		case declName == "RELEASE_ACONFIG_VALUE_SETS":
			if strings.HasPrefix(declValue, "\"") {
				rootAconfigModule = declValue[1 : len(declValue)-1]
			}
			continue
		case strings.HasPrefix(declValue, "\""):
			// String values mean that the flag workflow is (most likely) either MANUAL or PREBUILT.
			declValue = declValue[1 : len(declValue)-1]
			flagDeclaration.Value = &rc_proto.Value{Val: &rc_proto.Value_StringValue{declValue}}
			for _, prefix := range manualFlagNamePrefixes {
				if strings.HasPrefix(declName, prefix) {
					workflow = rc_proto.Workflow(rc_proto.Workflow_MANUAL)
					break
				}
			}
		case declValue == "False" || declValue == "True":
			// Boolean values are LAUNCH flags.
			flagDeclaration.Value = &rc_proto.Value{Val: &rc_proto.Value_BoolValue{declValue == "True"}}
			workflow = rc_proto.Workflow(rc_proto.Workflow_LAUNCH)
		case declValue == "None":
			// Use PREBUILT workflow with no initial value.
		default:
			fmt.Printf("%s: Unexpected value %s=%s\n", path, declName, declValue)
		}
		flagDeclaration.Workflow = &workflow
		if flagDeclaration != nil {
			declPath := filepath.Join(dir, "flag_declarations", fmt.Sprintf("%s.textproto", declName))
			err := WriteFile(declPath, flagDeclaration)
			if err != nil {
				return err
			}
		}
	}
	if rootAconfigModule != "" {
		rootProto := &rc_proto.ReleaseConfig{
			Name:             proto.String("root"),
			AconfigValueSets: []string{rootAconfigModule},
		}
		return WriteFile(filepath.Join(dir, "release_configs", "root.textproto"), rootProto)
	}
	return nil
}

func ProcessBuildConfigs(dir, name string, paths []string, releaseProto *rc_proto.ReleaseConfig) error {
	valRegexp, err := regexp.Compile("[[:space:]]+value.\"(?<name>[A-Z_0-9]+)\",[[:space:]]*(?<value>(\"[^\"]*\"|[^\",)]*))")
	if err != nil {
		return err
	}
	for _, path := range paths {
		fmt.Printf("Processing %s\n", path)
		valIn, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("%s: error: %v\n", path, err)
			return err
		}
		vals := valRegexp.FindAllString(string(valIn), -1)
		for _, val := range vals {
			matches := valRegexp.FindStringSubmatch(val)
			valValue := matches[valRegexp.SubexpIndex("value")]
			valName := matches[valRegexp.SubexpIndex("name")]
			flagValue := &rc_proto.FlagValue{
				Name: proto.String(valName),
			}
			switch {
			case valName == "RELEASE_ACONFIG_VALUE_SETS":
				flagValue = nil
				if releaseProto.AconfigValueSets == nil {
					releaseProto.AconfigValueSets = []string{}
				}
				releaseProto.AconfigValueSets = append(releaseProto.AconfigValueSets, valValue[1:len(valValue)-1])
			case strings.HasPrefix(valValue, "\""):
				valValue = valValue[1 : len(valValue)-1]
				flagValue.Value = &rc_proto.Value{Val: &rc_proto.Value_StringValue{valValue}}
			case valValue == "None":
				// nothing to do here.
			case valValue == "True":
				flagValue.Value = &rc_proto.Value{Val: &rc_proto.Value_BoolValue{true}}
			case valValue == "False":
				flagValue.Value = &rc_proto.Value{Val: &rc_proto.Value_BoolValue{false}}
			default:
				fmt.Printf("%s: Unexpected value %s=%s\n", path, valName, valValue)
			}
			if flagValue != nil {
				if releaseProto.GetAconfigFlagsOnly() {
					return fmt.Errorf("%s does not allow build flag overrides", RenameNext(name))
				}
				valPath := filepath.Join(dir, "flag_values", RenameNext(name), fmt.Sprintf("%s.textproto", valName))
				err := WriteFile(valPath, flagValue)
				if err != nil {
					return err
				}
			}
		}
	}
	return err
}

var (
	allContainers = func() []string {
		return []string{"product", "system", "system_ext", "vendor"}
	}()
)

func ProcessReleaseConfigMap(dir string, descriptionMap map[string]string) error {
	path := filepath.Join(dir, "release_config_map.mk")
	if _, err := os.Stat(path); err != nil {
		fmt.Printf("%s not found, ignoring.\n", path)
		return nil
	} else {
		fmt.Printf("Processing %s\n", path)
	}
	configRegexp, err := regexp.Compile("^..call[[:space:]]+declare-release-config,[[:space:]]+(?<name>[_a-z0-9A-Z]+),[[:space:]]+(?<files>[^,]*)(,[[:space:]]*(?<inherits>.*)|[[:space:]]*)[)]$")
	if err != nil {
		return err
	}
	aliasRegexp, err := regexp.Compile("^..call[[:space:]]+alias-release-config,[[:space:]]+(?<name>[_a-z0-9A-Z]+),[[:space:]]+(?<target>[_a-z0-9A-Z]+)")
	if err != nil {
		return err
	}

	mapIn, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cleanDir := strings.TrimLeft(dir, "../")
	var defaultContainers []string
	switch {
	case strings.HasPrefix(cleanDir, "build/") || cleanDir == "vendor/google_shared/build":
		defaultContainers = allContainers
	case cleanDir == "vendor/google/release":
		defaultContainers = allContainers
	default:
		defaultContainers = []string{"vendor"}
	}
	releaseConfigMap := &rc_proto.ReleaseConfigMap{DefaultContainers: defaultContainers}
	// If we find a description for the directory, include it.
	if description, ok := descriptionMap[cleanDir]; ok {
		releaseConfigMap.Description = proto.String(description)
	}
	lines := strings.Split(string(mapIn), "\n")
	for _, line := range lines {
		alias := aliasRegexp.FindStringSubmatch(aliasRegexp.FindString(line))
		if alias != nil {
			fmt.Printf("processing alias %s\n", line)
			name := alias[aliasRegexp.SubexpIndex("name")]
			target := alias[aliasRegexp.SubexpIndex("target")]
			if target == "next" {
				if RenameNext(target) != name {
					return fmt.Errorf("Unexpected name for next (%s)", RenameNext(target))
				}
				target, name = name, target
			}
			releaseConfigMap.Aliases = append(releaseConfigMap.Aliases,
				&rc_proto.ReleaseAlias{
					Name:   proto.String(name),
					Target: proto.String(target),
				})
		}
		config := configRegexp.FindStringSubmatch(configRegexp.FindString(line))
		if config == nil {
			continue
		}
		name := config[configRegexp.SubexpIndex("name")]
		releaseConfig := &rc_proto.ReleaseConfig{
			Name: proto.String(RenameNext(name)),
		}
		if aconfigFlagsOnlyConfigs[name] {
			releaseConfig.AconfigFlagsOnly = proto.Bool(true)
		}
		configFiles := config[configRegexp.SubexpIndex("files")]
		files := strings.Split(strings.ReplaceAll(configFiles, "$(local_dir)", dir+"/"), " ")
		configInherits := config[configRegexp.SubexpIndex("inherits")]
		if len(configInherits) > 0 {
			releaseConfig.Inherits = strings.Split(configInherits, " ")
		}
		err := ProcessBuildConfigs(dir, name, files, releaseConfig)
		if err != nil {
			return err
		}

		releasePath := filepath.Join(dir, "release_configs", fmt.Sprintf("%s.textproto", RenameNext(name)))
		err = WriteFile(releasePath, releaseConfig)
		if err != nil {
			return err
		}
	}
	return WriteFile(filepath.Join(dir, "release_config_map.textproto"), releaseConfigMap)
}

func main() {
	var err error
	var top string
	var dirs rc_lib.StringList
	var namespacesFile string
	var descriptionsFile string
	var debug bool
	defaultTopDir, err := rc_lib.GetTopDir()

	flag.StringVar(&top, "top", defaultTopDir, "path to top of workspace")
	flag.Var(&dirs, "dir", "directory to process, relative to the top of the workspace")
	flag.StringVar(&namespacesFile, "namespaces", "", "location of file with 'flag_name namespace' information")
	flag.StringVar(&descriptionsFile, "descriptions", "", "location of file with 'directory description' information")
	flag.BoolVar(&debug, "debug", false, "turn on debugging output for errors")
	flag.Parse()

	errorExit := func(err error) {
		if debug {
			panic(err)
		}
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	if err = os.Chdir(top); err != nil {
		errorExit(err)
	}
	if len(dirs) == 0 {
		dirs = rc_lib.StringList{"build/release", "vendor/google_shared/build/release", "vendor/google/release"}
	}

	namespaceMap := make(map[string]string)
	if namespacesFile != "" {
		data, err := os.ReadFile(namespacesFile)
		if err != nil {
			errorExit(err)
		}
		for idx, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, " ")
			if len(fields) > 2 {
				errorExit(fmt.Errorf("line %d: too many fields: %s", idx, line))
			}
			namespaceMap[fields[0]] = fields[1]
		}

	}

	descriptionMap := make(map[string]string)
	descriptionMap["build/release"] = "Published open-source flags and declarations"
	if descriptionsFile != "" {
		data, err := os.ReadFile(descriptionsFile)
		if err != nil {
			errorExit(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) != "" {
				fields := strings.SplitN(line, " ", 2)
				descriptionMap[fields[0]] = fields[1]
			}
		}

	}

	for _, dir := range dirs {
		err = ProcessBuildFlags(dir, namespaceMap)
		if err != nil {
			errorExit(err)
		}

		err = ProcessReleaseConfigMap(dir, descriptionMap)
		if err != nil {
			errorExit(err)
		}
	}
}
