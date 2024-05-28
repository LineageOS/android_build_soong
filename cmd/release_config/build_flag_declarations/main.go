package main

import (
	"flag"
	"fmt"
	"os"

	rc_lib "android/soong/cmd/release_config/release_config_lib"
	rc_proto "android/soong/cmd/release_config/release_config_proto"
)

type Flags struct {
	// The path to the top of the workspace.  Default: ".".
	top string

	// Output file.
	output string

	// Format for output file
	format string

	// List of flag_declaration files to add.
	decls rc_lib.StringList

	// List of flag_artifacts files to merge.
	intermediates rc_lib.StringList

	// Disable warning messages
	quiet bool

	// Panic on errors.
	debug bool
}

func main() {
	var flags Flags
	topDir, err := rc_lib.GetTopDir()

	// Handle the common arguments
	flag.StringVar(&flags.top, "top", topDir, "path to top of workspace")
	flag.Var(&flags.decls, "decl", "path to a flag_declaration file. May be repeated")
	flag.Var(&flags.intermediates, "intermediate", "path to a flag_artifacts file (output from a prior run). May be repeated")
	flag.StringVar(&flags.format, "format", "pb", "output file format")
	flag.StringVar(&flags.output, "output", "build_flags.pb", "output file")
	flag.BoolVar(&flags.debug, "debug", false, "turn on debugging output for errors")
	flag.BoolVar(&flags.quiet, "quiet", false, "disable warning messages")
	flag.Parse()

	errorExit := func(err error) {
		if flags.debug {
			panic(err)
		}
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	if flags.quiet {
		rc_lib.DisableWarnings()
	}

	if err = os.Chdir(flags.top); err != nil {
		errorExit(err)
	}

	flagArtifacts := rc_lib.FlagArtifactsFactory("")
	intermediates := []*rc_proto.FlagDeclarationArtifacts{}
	for _, intermediate := range flags.intermediates {
		fda := rc_lib.FlagDeclarationArtifactsFactory(intermediate)
		intermediates = append(intermediates, fda)
	}
	for _, decl := range flags.decls {
		fa := rc_lib.FlagArtifactFactory(decl)
		(*flagArtifacts)[*fa.FlagDeclaration.Name] = fa
	}

	message := flagArtifacts.GenerateFlagDeclarationArtifacts(intermediates)
	err = rc_lib.WriteFormattedMessage(flags.output, flags.format, message)
	if err != nil {
		errorExit(err)
	}
}
