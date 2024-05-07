package main

import (
	"cmp"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	rc_lib "android/soong/cmd/release_config/release_config_lib"
	rc_proto "android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

type Flags struct {
	// The path to the top of the workspace.  Default: ".".
	top string

	// Pathlist of release config map textproto files.
	// If not specified, then the value is (if present):
	// - build/release/release_config_map.textproto
	// - vendor/google_shared/build/release/release_config_map.textproto
	// - vendor/google/release/release_config_map.textproto
	//
	// Additionally, any maps specified in the environment variable
	// `PRODUCT_RELEASE_CONFIG_MAPS` are used.
	maps rc_lib.StringList

	// Output directory (relative to `top`).
	outDir string

	// Which $TARGET_RELEASE(s) should we use.  Some commands will only
	// accept one value, others also accept `--release --all`.
	targetReleases rc_lib.StringList

	// Disable warning messages
	quiet bool

	// Show all release configs
	allReleases bool

	// Call get_build_var PRODUCT_RELEASE_CONFIG_MAPS to get the
	// product-specific map directories.
	useGetBuildVar bool

	// Panic on errors.
	debug bool
}

type CommandFunc func(*rc_lib.ReleaseConfigs, Flags, string, []string) error

var commandMap map[string]CommandFunc = map[string]CommandFunc{
	"get":   GetCommand,
	"set":   SetCommand,
	"trace": GetCommand, // Also handled by GetCommand
}

// Find the top of the release config contribution directory.
// Returns the parent of the flag_declarations and flag_values directories.
func GetMapDir(path string) (string, error) {
	for p := path; p != "."; p = filepath.Dir(p) {
		switch filepath.Base(p) {
		case "flag_declarations":
			return filepath.Dir(p), nil
		case "flag_values":
			return filepath.Dir(p), nil
		}
	}
	return "", fmt.Errorf("Could not determine directory from %s", path)
}

func MarshalFlagDefaultValue(config *rc_lib.ReleaseConfig, name string) (ret string, err error) {
	fa, ok := config.FlagArtifacts[name]
	if !ok {
		return "", fmt.Errorf("%s not found in %s", name, config.Name)
	}
	return rc_lib.MarshalValue(fa.Traces[0].Value), nil
}

func MarshalFlagValue(config *rc_lib.ReleaseConfig, name string) (ret string, err error) {
	fa, ok := config.FlagArtifacts[name]
	if !ok {
		return "", fmt.Errorf("%s not found in %s", name, config.Name)
	}
	return rc_lib.MarshalValue(fa.Value), nil
}

// Returns a list of ReleaseConfig objects for which to process flags.
func GetReleaseArgs(configs *rc_lib.ReleaseConfigs, commonFlags Flags) ([]*rc_lib.ReleaseConfig, error) {
	var all bool
	relFlags := flag.NewFlagSet("releaseFlags", flag.ExitOnError)
	relFlags.BoolVar(&all, "all", false, "Display all releases")
	relFlags.Parse(commonFlags.targetReleases)
	var ret []*rc_lib.ReleaseConfig
	if all || commonFlags.allReleases {
		sortMap := map[string]int{
			"trunk_staging": 0,
			"trunk_food":    10,
			"trunk":         20,
			// Anything not listed above, uses this for key 1 in the sort.
			"-default": 100,
		}

		for _, config := range configs.ReleaseConfigs {
			ret = append(ret, config)
		}
		slices.SortFunc(ret, func(a, b *rc_lib.ReleaseConfig) int {
			mapValue := func(v *rc_lib.ReleaseConfig) int {
				if v, ok := sortMap[v.Name]; ok {
					return v
				}
				return sortMap["-default"]
			}
			if n := cmp.Compare(mapValue(a), mapValue(b)); n != 0 {
				return n
			}
			return cmp.Compare(a.Name, b.Name)
		})
		return ret, nil
	}
	for _, arg := range relFlags.Args() {
		// Return releases in the order that they were given.
		config, err := configs.GetReleaseConfig(arg)
		if err != nil {
			return nil, err
		}
		ret = append(ret, config)
	}
	return ret, nil
}

func GetCommand(configs *rc_lib.ReleaseConfigs, commonFlags Flags, cmd string, args []string) error {
	isTrace := cmd == "trace"
	isSet := cmd == "set"

	var all bool
	getFlags := flag.NewFlagSet("get", flag.ExitOnError)
	getFlags.BoolVar(&all, "all", false, "Display all flags")
	getFlags.Parse(args)
	args = getFlags.Args()

	if isSet {
		commonFlags.allReleases = true
	}
	releaseConfigList, err := GetReleaseArgs(configs, commonFlags)
	if err != nil {
		return err
	}
	if isTrace && len(releaseConfigList) > 1 {
		return fmt.Errorf("trace command only allows one --release argument.  Got: %s", strings.Join(commonFlags.targetReleases, " "))
	}

	if all {
		args = []string{}
		for _, fa := range configs.FlagArtifacts {
			args = append(args, *fa.FlagDeclaration.Name)
		}
	}

	var maxVariableNameLen, maxReleaseNameLen int
	var releaseNameFormat, variableNameFormat string
	valueFormat := "%s"
	showReleaseName := len(releaseConfigList) > 1
	showVariableName := len(args) > 1
	if showVariableName {
		for _, arg := range args {
			maxVariableNameLen = max(len(arg), maxVariableNameLen)
		}
		variableNameFormat = fmt.Sprintf("%%-%ds ", maxVariableNameLen)
		valueFormat = "'%s'"
	}
	if showReleaseName {
		for _, config := range releaseConfigList {
			maxReleaseNameLen = max(len(config.Name), maxReleaseNameLen)
		}
		releaseNameFormat = fmt.Sprintf("%%-%ds ", maxReleaseNameLen)
		valueFormat = "'%s'"
	}

	outputOneLine := func(variable, release, value, valueFormat string) {
		var outStr string
		if showVariableName {
			outStr += fmt.Sprintf(variableNameFormat, variable)
		}
		if showReleaseName {
			outStr += fmt.Sprintf(releaseNameFormat, release)
		}
		outStr += fmt.Sprintf(valueFormat, value)
		fmt.Println(outStr)
	}

	for _, arg := range args {
		if _, ok := configs.FlagArtifacts[arg]; !ok {
			return fmt.Errorf("%s is not a defined build flag", arg)
		}
	}

	for _, arg := range args {
		for _, config := range releaseConfigList {
			if isSet {
				// If this is from the set command, format the output as:
				// <default>           ""
				// trunk_staging       ""
				// trunk               ""
				//
				// ap1a                ""
				// ...
				switch {
				case config.Name == "trunk_staging":
					defaultValue, err := MarshalFlagDefaultValue(config, arg)
					if err != nil {
						return err
					}
					outputOneLine(arg, "<default>", defaultValue, valueFormat)
				case config.AconfigFlagsOnly:
					continue
				case config.Name == "trunk":
					fmt.Println()
				}
			}
			val, err := MarshalFlagValue(config, arg)
			if err == nil {
				outputOneLine(arg, config.Name, val, valueFormat)
			} else {
				outputOneLine(arg, config.Name, "REDACTED", "%s")
			}
			if isTrace {
				for _, trace := range config.FlagArtifacts[arg].Traces {
					fmt.Printf("  => \"%s\" in %s\n", rc_lib.MarshalValue(trace.Value), *trace.Source)
				}
			}
		}
	}
	return nil
}

func SetCommand(configs *rc_lib.ReleaseConfigs, commonFlags Flags, cmd string, args []string) error {
	var valueDir string
	if len(commonFlags.targetReleases) > 1 {
		return fmt.Errorf("set command only allows one --release argument.  Got: %s", strings.Join(commonFlags.targetReleases, " "))
	}
	targetRelease := commonFlags.targetReleases[0]

	setFlags := flag.NewFlagSet("set", flag.ExitOnError)
	setFlags.StringVar(&valueDir, "dir", "", "Directory in which to place the value")
	setFlags.Parse(args)
	setArgs := setFlags.Args()
	if len(setArgs) != 2 {
		return fmt.Errorf("set command expected flag and value, got: %s", strings.Join(setArgs, " "))
	}
	name := setArgs[0]
	value := setArgs[1]
	release, err := configs.GetReleaseConfig(targetRelease)
	targetRelease = release.Name
	if err != nil {
		return err
	}
	if release.AconfigFlagsOnly {
		return fmt.Errorf("%s does not allow build flag overrides", targetRelease)
	}
	flagArtifact, ok := release.FlagArtifacts[name]
	if !ok {
		return fmt.Errorf("Unknown build flag %s", name)
	}
	if valueDir == "" {
		mapDir, err := configs.GetFlagValueDirectory(release, flagArtifact)
		if err != nil {
			return err
		}
		valueDir = mapDir
	}

	flagValue := &rc_proto.FlagValue{
		Name:  proto.String(name),
		Value: rc_lib.UnmarshalValue(value),
	}
	flagPath := filepath.Join(valueDir, "flag_values", targetRelease, fmt.Sprintf("%s.textproto", name))
	err = rc_lib.WriteMessage(flagPath, flagValue)
	if err != nil {
		return err
	}

	// Reload the release configs.
	configs, err = rc_lib.ReadReleaseConfigMaps(commonFlags.maps, commonFlags.targetReleases[0], commonFlags.useGetBuildVar)
	if err != nil {
		return err
	}
	err = GetCommand(configs, commonFlags, cmd, args[0:1])
	if err != nil {
		return err
	}
	fmt.Printf("Updated: %s\n", flagPath)
	return nil
}

func main() {
	var commonFlags Flags
	var configs *rc_lib.ReleaseConfigs
	topDir, err := rc_lib.GetTopDir()

	// Handle the common arguments
	flag.StringVar(&commonFlags.top, "top", topDir, "path to top of workspace")
	flag.BoolVar(&commonFlags.quiet, "quiet", false, "disable warning messages")
	flag.Var(&commonFlags.maps, "map", "path to a release_config_map.textproto. may be repeated")
	flag.StringVar(&commonFlags.outDir, "out-dir", rc_lib.GetDefaultOutDir(), "basepath for the output. Multiple formats are created")
	flag.Var(&commonFlags.targetReleases, "release", "TARGET_RELEASE for this build")
	flag.BoolVar(&commonFlags.allReleases, "all-releases", false, "operate on all releases. (Ignored for set command)")
	flag.BoolVar(&commonFlags.useGetBuildVar, "use-get-build-var", true, "use get_build_var PRODUCT_RELEASE_CONFIG_MAPS to get needed maps")
	flag.BoolVar(&commonFlags.debug, "debug", false, "turn on debugging output for errors")
	flag.Parse()

	errorExit := func(err error) {
		if commonFlags.debug {
			panic(err)
		}
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	if commonFlags.quiet {
		rc_lib.DisableWarnings()
	}

	if len(commonFlags.targetReleases) == 0 {
		release, ok := os.LookupEnv("TARGET_RELEASE")
		if ok {
			commonFlags.targetReleases = rc_lib.StringList{release}
		} else {
			commonFlags.targetReleases = rc_lib.StringList{"trunk_staging"}
		}
	}

	if err = os.Chdir(commonFlags.top); err != nil {
		errorExit(err)
	}

	// Get the current state of flagging.
	relName := commonFlags.targetReleases[0]
	if relName == "--all" || relName == "-all" {
		commonFlags.allReleases = true
	}
	configs, err = rc_lib.ReadReleaseConfigMaps(commonFlags.maps, relName, commonFlags.useGetBuildVar)
	if err != nil {
		errorExit(err)
	}

	if cmd, ok := commandMap[flag.Arg(0)]; ok {
		args := flag.Args()
		if err = cmd(configs, commonFlags, args[0], args[1:]); err != nil {
			errorExit(err)
		}
	}
}
