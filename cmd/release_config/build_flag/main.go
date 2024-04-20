package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

func MarshalFlagValue(config *rc_lib.ReleaseConfig, name string) (ret string, err error) {
	fa, ok := config.FlagArtifacts[name]
	if !ok {
		return "", fmt.Errorf("%s not found in %s", name, config.Name)
	}
	return rc_lib.MarshalValue(fa.Value), nil
}

func GetReleaseArgs(configs *rc_lib.ReleaseConfigs, commonFlags Flags) ([]*rc_lib.ReleaseConfig, error) {
	var all bool
	relFlags := flag.NewFlagSet("set", flag.ExitOnError)
	relFlags.BoolVar(&all, "all", false, "Display all flags")
	relFlags.Parse(commonFlags.targetReleases)
	var ret []*rc_lib.ReleaseConfig
	if all {
		for _, config := range configs.ReleaseConfigs {
			ret = append(ret, config)
		}
		return ret, nil
	}
	for _, arg := range relFlags.Args() {
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
	var all bool
	getFlags := flag.NewFlagSet("set", flag.ExitOnError)
	getFlags.BoolVar(&all, "all", false, "Display all flags")
	getFlags.Parse(args)
	args = getFlags.Args()

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

	showName := len(releaseConfigList) > 1 || len(args) > 1
	for _, config := range releaseConfigList {
		var configName string
		if len(releaseConfigList) > 1 {
			configName = fmt.Sprintf("%s.", config.Name)
		}
		for _, arg := range args {
			val, err := MarshalFlagValue(config, arg)
			if err != nil {
				return err
			}
			if showName {
				fmt.Printf("%s%s=%s\n", configName, arg, val)
			} else {
				fmt.Printf("%s\n", val)
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
	flagArtifact, ok := release.FlagArtifacts[name]
	if !ok {
		return fmt.Errorf("Unknown build flag %s", name)
	}
	if valueDir == "" {
		mapDir, err := GetMapDir(*flagArtifact.Traces[len(flagArtifact.Traces)-1].Source)
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
	return rc_lib.WriteMessage(flagPath, flagValue)
}

func main() {
	var err error
	var commonFlags Flags
	var configs *rc_lib.ReleaseConfigs

	outEnv := os.Getenv("OUT_DIR")
	if outEnv == "" {
		outEnv = "out"
	}
	// Handle the common arguments
	flag.StringVar(&commonFlags.top, "top", ".", "path to top of workspace")
	flag.BoolVar(&commonFlags.quiet, "quiet", false, "disable warning messages")
	flag.Var(&commonFlags.maps, "map", "path to a release_config_map.textproto. may be repeated")
	flag.StringVar(&commonFlags.outDir, "out_dir", rc_lib.GetDefaultOutDir(), "basepath for the output. Multiple formats are created")
	flag.Var(&commonFlags.targetReleases, "release", "TARGET_RELEASE for this build")
	flag.Parse()

	if commonFlags.quiet {
		rc_lib.DisableWarnings()
	}

	if len(commonFlags.targetReleases) == 0 {
		commonFlags.targetReleases = rc_lib.StringList{"trunk_staging"}
	}

	if err = os.Chdir(commonFlags.top); err != nil {
		panic(err)
	}

	// Get the current state of flagging.
	relName := commonFlags.targetReleases[0]
	if relName == "--all" || relName == "-all" {
		// If the users said `--release --all`, grab trunk staging for simplicity.
		relName = "trunk_staging"
	}
	configs, err = rc_lib.ReadReleaseConfigMaps(commonFlags.maps, relName)
	if err != nil {
		panic(err)
	}

	if cmd, ok := commandMap[flag.Arg(0)]; ok {
		args := flag.Args()
		if err = cmd(configs, commonFlags, args[0], args[1:]); err != nil {
			panic(err)
		}
	}
}
