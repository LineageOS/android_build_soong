// Copyright 2017 Google Inc. All rights reserved.
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
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"android/soong/shared"
	"android/soong/ui/build"
	"android/soong/ui/logger"
	"android/soong/ui/metrics"
	"android/soong/ui/signal"
	"android/soong/ui/status"
	"android/soong/ui/terminal"
	"android/soong/ui/tracer"
)

// A command represents an operation to be executed in the soong build
// system.
type command struct {
	// The flag name (must have double dashes).
	flag string

	// Description for the flag (to display when running help).
	description string

	// Stream the build status output into the simple terminal mode.
	simpleOutput bool

	// Sets a prefix string to use for filenames of log files.
	logsPrefix string

	// Creates the build configuration based on the args and build context.
	config func(ctx build.Context, args ...string) build.Config

	// Returns what type of IO redirection this Command requires.
	stdio func() terminal.StdioInterface

	// run the command
	run func(ctx build.Context, config build.Config, args []string)
}

// list of supported commands (flags) supported by soong ui
var commands = []command{
	{
		flag:        "--make-mode",
		description: "build the modules by the target name (i.e. soong_docs)",
		config:      build.NewConfig,
		stdio:       stdio,
		run:         runMake,
	}, {
		flag:         "--dumpvar-mode",
		description:  "print the value of the legacy make variable VAR to stdout",
		simpleOutput: true,
		logsPrefix:   "dumpvars-",
		config:       dumpVarConfig,
		stdio:        customStdio,
		run:          dumpVar,
	}, {
		flag:         "--dumpvars-mode",
		description:  "dump the values of one or more legacy make variables, in shell syntax",
		simpleOutput: true,
		logsPrefix:   "dumpvars-",
		config:       dumpVarConfig,
		stdio:        customStdio,
		run:          dumpVars,
	}, {
		flag:        "--build-mode",
		description: "build modules based on the specified build action",
		config:      buildActionConfig,
		stdio:       stdio,
		run:         runMake,
	},
}

// indexList returns the index of first found s. -1 is return if s is not
// found.
func indexList(s string, list []string) int {
	for i, l := range list {
		if l == s {
			return i
		}
	}
	return -1
}

// inList returns true if one or more of s is in the list.
func inList(s string, list []string) bool {
	return indexList(s, list) != -1
}

func deleteStaleMetrics(metricsFilePathSlice []string) error {
	for _, metricsFilePath := range metricsFilePathSlice {
		if err := os.Remove(metricsFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Failed to remove %s\nError message: %w", metricsFilePath, err)
		}
	}
	return nil
}

// Main execution of soong_ui. The command format is as follows:
//
//	soong_ui <command> [<arg 1> <arg 2> ... <arg n>]
//
// Command is the type of soong_ui execution. Only one type of
// execution is specified. The args are specific to the command.
func main() {
	shared.ReexecWithDelveMaybe(os.Getenv("SOONG_UI_DELVE"), shared.ResolveDelveBinary())

	buildStarted := time.Now()

	c, args, err := getCommand(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing `soong` args: %s.\n", err)
		os.Exit(1)
	}

	// Create a terminal output that mimics Ninja's.
	output := terminal.NewStatusOutput(c.stdio().Stdout(), os.Getenv("NINJA_STATUS"), c.simpleOutput,
		build.OsEnvironment().IsEnvTrue("ANDROID_QUIET_BUILD"),
		build.OsEnvironment().IsEnvTrue("SOONG_UI_ANSI_OUTPUT"))

	// Create and start a new metric record.
	met := metrics.New()
	met.SetBuildDateTime(buildStarted)
	met.SetBuildCommand(os.Args)

	// Attach a new logger instance to the terminal output.
	log := logger.NewWithMetrics(output, met)
	defer log.Cleanup()

	// Create a context to simplify the program termination process.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new trace file writer, making it log events to the log instance.
	trace := tracer.New(log)
	defer trace.Close()

	// Create a new Status instance, which manages action counts and event output channels.
	stat := &status.Status{}

	// Hook up the terminal output and tracer to Status.
	stat.AddOutput(output)
	stat.AddOutput(trace.StatusTracer())

	// Set up a cleanup procedure in case the normal termination process doesn't work.
	signal.SetupSignals(log, cancel, func() {
		trace.Close()
		log.Cleanup()
		stat.Finish()
	})
	criticalPath := status.NewCriticalPath()
	buildCtx := build.Context{ContextImpl: &build.ContextImpl{
		Context:      ctx,
		Logger:       log,
		Metrics:      met,
		Tracer:       trace,
		Writer:       output,
		Status:       stat,
		CriticalPath: criticalPath,
	}}

	freshConfig := func() build.Config {
		config := c.config(buildCtx, args...)
		config.SetLogsPrefix(c.logsPrefix)
		return config
	}
	config := freshConfig()
	logsDir := config.LogsDir()
	buildStarted = config.BuildStartedTimeOrDefault(buildStarted)

	buildErrorFile := filepath.Join(logsDir, c.logsPrefix+"build_error")
	soongMetricsFile := filepath.Join(logsDir, c.logsPrefix+"soong_metrics")
	rbeMetricsFile := filepath.Join(logsDir, c.logsPrefix+"rbe_metrics.pb")
	soongBuildMetricsFile := filepath.Join(logsDir, c.logsPrefix+"soong_build_metrics.pb")

	metricsFiles := []string{
		buildErrorFile,        // build error strings
		rbeMetricsFile,        // high level metrics related to remote build execution.
		soongMetricsFile,      // high level metrics related to this build system.
		soongBuildMetricsFile, // high level metrics related to soong build
	}

	os.MkdirAll(logsDir, 0777)

	log.SetOutput(filepath.Join(logsDir, c.logsPrefix+"soong.log"))

	trace.SetOutput(filepath.Join(logsDir, c.logsPrefix+"build.trace"))

	log.Verbose("Command Line: ")
	for i, arg := range os.Args {
		log.Verbosef("  [%d] %s", i, arg)
	}

	// We need to call preProductConfigSetup before we can do product config, which is how we get
	// PRODUCT_CONFIG_RELEASE_MAPS set for the final product config for the build.
	// When product config uses a declarative language, we won't need to rerun product config.
	preProductConfigSetup(buildCtx, config)
	if build.SetProductReleaseConfigMaps(buildCtx, config) {
		log.Verbose("Product release config maps found\n")
		config = freshConfig()
	}

	defer func() {
		stat.Finish()
		criticalPath.WriteToMetrics(met)
		met.Dump(soongMetricsFile)
		if !config.SkipMetricsUpload() {
			build.UploadMetrics(buildCtx, config, c.simpleOutput, buildStarted, metricsFiles...)
		}
	}()
	c.run(buildCtx, config, args)

}

// This function must not modify config, since product config may cause us to recreate the config,
// and we won't call this function a second time.
func preProductConfigSetup(buildCtx build.Context, config build.Config) {
	log := buildCtx.ContextImpl.Logger
	logsPrefix := config.GetLogsPrefix()
	build.SetupOutDir(buildCtx, config)
	logsDir := config.LogsDir()

	// Common list of metric file definition.
	buildErrorFile := filepath.Join(logsDir, logsPrefix+"build_error")
	rbeMetricsFile := filepath.Join(logsDir, logsPrefix+"rbe_metrics.pb")
	soongMetricsFile := filepath.Join(logsDir, logsPrefix+"soong_metrics")
	bp2buildMetricsFile := filepath.Join(logsDir, logsPrefix+"bp2build_metrics.pb")
	soongBuildMetricsFile := filepath.Join(logsDir, logsPrefix+"soong_build_metrics.pb")

	//Delete the stale metrics files
	staleFileSlice := []string{buildErrorFile, rbeMetricsFile, soongMetricsFile, bp2buildMetricsFile, soongBuildMetricsFile}
	if err := deleteStaleMetrics(staleFileSlice); err != nil {
		log.Fatalln(err)
	}

	build.PrintOutDirWarning(buildCtx, config)

	stat := buildCtx.Status
	stat.AddOutput(status.NewVerboseLog(log, filepath.Join(logsDir, logsPrefix+"verbose.log")))
	stat.AddOutput(status.NewErrorLog(log, filepath.Join(logsDir, logsPrefix+"error.log")))
	stat.AddOutput(status.NewProtoErrorLog(log, buildErrorFile))
	stat.AddOutput(status.NewCriticalPathLogger(log, buildCtx.CriticalPath))
	stat.AddOutput(status.NewBuildProgressLog(log, filepath.Join(logsDir, logsPrefix+"build_progress.pb")))

	buildCtx.Verbosef("Detected %.3v GB total RAM", float32(config.TotalRAM())/(1024*1024*1024))
	buildCtx.Verbosef("Parallelism (local/remote/highmem): %v/%v/%v",
		config.Parallel(), config.RemoteParallel(), config.HighmemParallel())

	setMaxFiles(buildCtx)

	defer build.CheckProdCreds(buildCtx, config)

	// Read the time at the starting point.
	if start, ok := os.LookupEnv("TRACE_BEGIN_SOONG"); ok {
		// soong_ui.bash uses the date command's %N (nanosec) flag when getting the start time,
		// which Darwin doesn't support. Check if it was executed properly before parsing the value.
		if !strings.HasSuffix(start, "N") {
			if start_time, err := strconv.ParseUint(start, 10, 64); err == nil {
				log.Verbosef("Took %dms to start up.",
					time.Since(time.Unix(0, int64(start_time))).Nanoseconds()/time.Millisecond.Nanoseconds())
				buildCtx.CompleteTrace(metrics.RunSetupTool, "startup", start_time, uint64(time.Now().UnixNano()))
			}
		}

		if executable, err := os.Executable(); err == nil {
			buildCtx.ContextImpl.Tracer.ImportMicrofactoryLog(filepath.Join(filepath.Dir(executable), "."+filepath.Base(executable)+".trace"))
		}
	}

	// Create a source finder.
	f := build.NewSourceFinder(buildCtx, config)
	defer f.Shutdown()
	build.FindSources(buildCtx, config, f)
}

func dumpVar(ctx build.Context, config build.Config, args []string) {
	flags := flag.NewFlagSet("dumpvar", flag.ExitOnError)
	flags.SetOutput(ctx.Writer)

	flags.Usage = func() {
		fmt.Fprintf(ctx.Writer, "usage: %s --dumpvar-mode [--abs] <VAR>\n\n", os.Args[0])
		fmt.Fprintln(ctx.Writer, "In dumpvar mode, print the value of the legacy make variable VAR to stdout")
		fmt.Fprintln(ctx.Writer, "")

		fmt.Fprintln(ctx.Writer, "'report_config' is a special case that prints the human-readable config banner")
		fmt.Fprintln(ctx.Writer, "from the beginning of the build.")
		fmt.Fprintln(ctx.Writer, "")
		flags.PrintDefaults()
	}
	abs := flags.Bool("abs", false, "Print the absolute path of the value")
	flags.Parse(args)

	if flags.NArg() != 1 {
		flags.Usage()
		ctx.Fatalf("Invalid usage")
	}

	varName := flags.Arg(0)
	if varName == "report_config" {
		varData, err := build.DumpMakeVars(ctx, config, nil, build.BannerVars)
		if err != nil {
			ctx.Fatal(err)
		}

		fmt.Println(build.Banner(varData))
	} else {
		varData, err := build.DumpMakeVars(ctx, config, nil, []string{varName})
		if err != nil {
			ctx.Fatal(err)
		}

		if *abs {
			var res []string
			for _, path := range strings.Fields(varData[varName]) {
				if abs, err := filepath.Abs(path); err == nil {
					res = append(res, abs)
				} else {
					ctx.Fatalln("Failed to get absolute path of", path, err)
				}
			}
			fmt.Println(strings.Join(res, " "))
		} else {
			fmt.Println(varData[varName])
		}
	}
}

func dumpVars(ctx build.Context, config build.Config, args []string) {

	flags := flag.NewFlagSet("dumpvars", flag.ExitOnError)
	flags.SetOutput(ctx.Writer)

	flags.Usage = func() {
		fmt.Fprintf(ctx.Writer, "usage: %s --dumpvars-mode [--vars=\"VAR VAR ...\"]\n\n", os.Args[0])
		fmt.Fprintln(ctx.Writer, "In dumpvars mode, dump the values of one or more legacy make variables, in")
		fmt.Fprintln(ctx.Writer, "shell syntax. The resulting output may be sourced directly into a shell to")
		fmt.Fprintln(ctx.Writer, "set corresponding shell variables.")
		fmt.Fprintln(ctx.Writer, "")

		fmt.Fprintln(ctx.Writer, "'report_config' is a special case that dumps a variable containing the")
		fmt.Fprintln(ctx.Writer, "human-readable config banner from the beginning of the build.")
		fmt.Fprintln(ctx.Writer, "")
		flags.PrintDefaults()
	}

	varsStr := flags.String("vars", "", "Space-separated list of variables to dump")
	absVarsStr := flags.String("abs-vars", "", "Space-separated list of variables to dump (using absolute paths)")

	varPrefix := flags.String("var-prefix", "", "String to prepend to all variable names when dumping")
	absVarPrefix := flags.String("abs-var-prefix", "", "String to prepent to all absolute path variable names when dumping")

	flags.Parse(args)

	if flags.NArg() != 0 {
		flags.Usage()
		ctx.Fatalf("Invalid usage")
	}

	vars := strings.Fields(*varsStr)
	absVars := strings.Fields(*absVarsStr)

	allVars := append([]string{}, vars...)
	allVars = append(allVars, absVars...)

	if i := indexList("report_config", allVars); i != -1 {
		allVars = append(allVars[:i], allVars[i+1:]...)
		allVars = append(allVars, build.BannerVars...)
	}

	if len(allVars) == 0 {
		return
	}

	varData, err := build.DumpMakeVars(ctx, config, nil, allVars)
	if err != nil {
		ctx.Fatal(err)
	}

	for _, name := range vars {
		if name == "report_config" {
			fmt.Printf("%sreport_config='%s'\n", *varPrefix, build.Banner(varData))
		} else {
			fmt.Printf("%s%s='%s'\n", *varPrefix, name, varData[name])
		}
	}
	for _, name := range absVars {
		var res []string
		for _, path := range strings.Fields(varData[name]) {
			abs, err := filepath.Abs(path)
			if err != nil {
				ctx.Fatalln("Failed to get absolute path of", path, err)
			}
			res = append(res, abs)
		}
		fmt.Printf("%s%s='%s'\n", *absVarPrefix, name, strings.Join(res, " "))
	}
}

func stdio() terminal.StdioInterface {
	return terminal.StdioImpl{}
}

// dumpvar and dumpvars use stdout to output variable values, so use stderr instead of stdout when
// reporting events to keep stdout clean from noise.
func customStdio() terminal.StdioInterface {
	return terminal.NewCustomStdio(os.Stdin, os.Stderr, os.Stderr)
}

// dumpVarConfig does not require any arguments to be parsed by the NewConfig.
func dumpVarConfig(ctx build.Context, args ...string) build.Config {
	return build.NewConfig(ctx)
}

func buildActionConfig(ctx build.Context, args ...string) build.Config {
	flags := flag.NewFlagSet("build-mode", flag.ContinueOnError)
	flags.SetOutput(ctx.Writer)

	flags.Usage = func() {
		fmt.Fprintf(ctx.Writer, "usage: %s --build-mode --dir=<path> <build action> [<build arg 1> <build arg 2> ...]\n\n", os.Args[0])
		fmt.Fprintln(ctx.Writer, "In build mode, build the set of modules based on the specified build")
		fmt.Fprintln(ctx.Writer, "action. The --dir flag is required to determine what is needed to")
		fmt.Fprintln(ctx.Writer, "build in the source tree based on the build action. See below for")
		fmt.Fprintln(ctx.Writer, "the list of acceptable build action flags.")
		fmt.Fprintln(ctx.Writer, "")
		flags.PrintDefaults()
	}

	buildActionFlags := []struct {
		name        string
		description string
		action      build.BuildAction
		set         bool
	}{{
		name:        "all-modules",
		description: "Build action: build from the top of the source tree.",
		action:      build.BUILD_MODULES,
	}, {
		// This is redirecting to mma build command behaviour. Once it has soaked for a
		// while, the build command is deleted from here once it has been removed from the
		// envsetup.sh.
		name:        "modules-in-a-dir-no-deps",
		description: "Build action: builds all of the modules in the current directory without their dependencies.",
		action:      build.BUILD_MODULES_IN_A_DIRECTORY,
	}, {
		// This is redirecting to mmma build command behaviour. Once it has soaked for a
		// while, the build command is deleted from here once it has been removed from the
		// envsetup.sh.
		name:        "modules-in-dirs-no-deps",
		description: "Build action: builds all of the modules in the supplied directories without their dependencies.",
		action:      build.BUILD_MODULES_IN_DIRECTORIES,
	}, {
		name:        "modules-in-a-dir",
		description: "Build action: builds all of the modules in the current directory and their dependencies.",
		action:      build.BUILD_MODULES_IN_A_DIRECTORY,
	}, {
		name:        "modules-in-dirs",
		description: "Build action: builds all of the modules in the supplied directories and their dependencies.",
		action:      build.BUILD_MODULES_IN_DIRECTORIES,
	}}
	for i, flag := range buildActionFlags {
		flags.BoolVar(&buildActionFlags[i].set, flag.name, false, flag.description)
	}
	dir := flags.String("dir", "", "Directory of the executed build command.")

	// Only interested in the first two args which defines the build action and the directory.
	// The remaining arguments are passed down to the config.
	const numBuildActionFlags = 2
	if len(args) < numBuildActionFlags {
		flags.Usage()
		ctx.Fatalln("Improper build action arguments: too few arguments")
	}
	parseError := flags.Parse(args[0:numBuildActionFlags])

	// The next block of code is to validate that exactly one build action is set and the dir flag
	// is specified.
	buildActionFound := false
	var buildAction build.BuildAction
	for _, f := range buildActionFlags {
		if f.set {
			if buildActionFound {
				if parseError == nil {
					//otherwise Parse() already called Usage()
					flags.Usage()
				}
				ctx.Fatalf("Build action already specified, omit: --%s\n", f.name)
			}
			buildActionFound = true
			buildAction = f.action
		}
	}
	if !buildActionFound {
		if parseError == nil {
			//otherwise Parse() already called Usage()
			flags.Usage()
		}
		ctx.Fatalln("Build action not defined.")
	}
	if *dir == "" {
		ctx.Fatalln("-dir not specified.")
	}

	// Remove the build action flags from the args as they are not recognized by the config.
	args = args[numBuildActionFlags:]
	return build.NewBuildActionConfig(buildAction, *dir, ctx, args...)
}

func runMake(ctx build.Context, config build.Config, _ []string) {
	logsDir := config.LogsDir()
	if config.IsVerbose() {
		writer := ctx.Writer
		fmt.Fprintln(writer, "! The argument `showcommands` is no longer supported.")
		fmt.Fprintln(writer, "! Instead, the verbose log is always written to a compressed file in the output dir:")
		fmt.Fprintln(writer, "!")
		fmt.Fprintf(writer, "!   gzip -cd %s/verbose.log.gz | less -R\n", logsDir)
		fmt.Fprintln(writer, "!")
		fmt.Fprintln(writer, "! Older versions are saved in verbose.log.#.gz files")
		fmt.Fprintln(writer, "")
		ctx.Fatal("Invalid argument")
	}

	if _, ok := config.Environment().Get("ONE_SHOT_MAKEFILE"); ok {
		writer := ctx.Writer
		fmt.Fprintln(writer, "! The variable `ONE_SHOT_MAKEFILE` is obsolete.")
		fmt.Fprintln(writer, "!")
		fmt.Fprintln(writer, "! If you're using `mm`, you'll need to run `source build/envsetup.sh` to update.")
		fmt.Fprintln(writer, "!")
		fmt.Fprintln(writer, "! Otherwise, either specify a module name with m, or use mma / MODULES-IN-...")
		fmt.Fprintln(writer, "")
		ctx.Fatal("Invalid environment")
	}

	build.Build(ctx, config)
}

// getCommand finds the appropriate command based on args[1] flag. args[0]
// is the soong_ui filename.
func getCommand(args []string) (*command, []string, error) {
	listFlags := func() []string {
		flags := make([]string, len(commands))
		for i, c := range commands {
			flags[i] = c.flag
		}
		return flags
	}

	if len(args) < 2 {
		return nil, nil, fmt.Errorf("Too few arguments: %q\nUse one of these: %q", args, listFlags())
	}

	for _, c := range commands {
		if c.flag == args[1] {
			return &c, args[2:], nil
		}
	}
	return nil, nil, fmt.Errorf("Command not found: %q\nDid you mean one of these: %q", args[1], listFlags())
}

func setMaxFiles(ctx build.Context) {
	var limits syscall.Rlimit

	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limits)
	if err != nil {
		ctx.Println("Failed to get file limit:", err)
		return
	}

	ctx.Verbosef("Current file limits: %d soft, %d hard", limits.Cur, limits.Max)

	// Go 1.21 modifies the file limit but restores the original when
	// execing subprocesses if it hasn't be overridden.  Call Setrlimit
	// here even if it doesn't appear to be necessary so that the
	// syscall package considers it set.

	limits.Cur = limits.Max
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limits)
	if err != nil {
		ctx.Println("Failed to increase file limit:", err)
	}
}
