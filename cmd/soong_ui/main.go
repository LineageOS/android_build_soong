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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/ui/build"
	"android/soong/ui/logger"
	"android/soong/ui/metrics"
	"android/soong/ui/status"
	"android/soong/ui/terminal"
	"android/soong/ui/tracer"
)

const (
	configDir  = "vendor/google/tools/soong_config"
	jsonSuffix = "json"
)

func indexList(s string, list []string) int {
	for i, l := range list {
		if l == s {
			return i
		}
	}

	return -1
}

func inList(s string, list []string) bool {
	return indexList(s, list) != -1
}

func loadEnvConfig(config build.Config) error {
	bc := os.Getenv("ANDROID_BUILD_ENVIRONMENT_CONFIG")
	if bc == "" {
		return nil
	}
	configDirs := []string{
		os.Getenv("ANDROID_BUILD_ENVIRONMENT_CONFIG_DIR"),
		config.OutDir(),
		configDir,
	}
	var cfgFile string
	for _, dir := range configDirs {
		cfgFile = filepath.Join(os.Getenv("TOP"), dir, fmt.Sprintf("%s.%s", bc, jsonSuffix))
		if _, err := os.Stat(cfgFile); err == nil {
			break
		}
	}

	envVarsJSON, err := ioutil.ReadFile(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[33mWARNING:\033[0m failed to open config file %s: %s\n", cfgFile, err.Error())
		return nil
	}

	var envVars map[string]map[string]string
	if err := json.Unmarshal(envVarsJSON, &envVars); err != nil {
		return fmt.Errorf("env vars config file: %s did not parse correctly: %s", cfgFile, err.Error())
	}
	for k, v := range envVars["env"] {
		if os.Getenv(k) != "" {
			continue
		}
		config.Environment().Set(k, v)
	}
	return nil
}

func main() {
	buildStarted := time.Now()
	var stdio terminal.StdioInterface
	stdio = terminal.StdioImpl{}
	simpleOutput := false
	logsPrefix := ""

	// dumpvar uses stdout, everything else should be in stderr
	if os.Args[1] == "--dumpvar-mode" || os.Args[1] == "--dumpvars-mode" {
		// Any metrics files add the prefix to distinguish the type of metrics being
		// collected to further aggregate the metrics. For dump-var mode, it is usually
		// related to the execution of lunch command.
		logsPrefix = "dumpvars-"
		simpleOutput = true
		stdio = terminal.NewCustomStdio(os.Stdin, os.Stderr, os.Stderr)
	}

	writer := terminal.NewWriter(stdio)
	defer writer.Finish()

	log := logger.New(writer)
	defer log.Cleanup()

	if len(os.Args) < 2 || !(inList("--make-mode", os.Args) ||
		os.Args[1] == "--dumpvars-mode" ||
		os.Args[1] == "--dumpvar-mode") {

		log.Fatalln("The `soong` native UI is not yet available.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trace := tracer.New(log)
	defer trace.Close()

	met := metrics.New()
	met.SetBuildDateTime(buildStarted)
	met.SetBuildCommand(os.Args)

	stat := &status.Status{}
	defer stat.Finish()
	stat.AddOutput(terminal.NewStatusOutput(writer, os.Getenv("NINJA_STATUS"),
		build.OsEnvironment().IsEnvTrue("ANDROID_QUIET_BUILD")))
	stat.AddOutput(trace.StatusTracer())

	build.SetupSignals(log, cancel, func() {
		trace.Close()
		log.Cleanup()
		stat.Finish()
	})

	buildCtx := build.Context{ContextImpl: &build.ContextImpl{
		Context: ctx,
		Logger:  log,
		Metrics: met,
		Tracer:  trace,
		Writer:  writer,
		Status:  stat,
	}}
	var config build.Config
	if os.Args[1] == "--dumpvars-mode" || os.Args[1] == "--dumpvar-mode" {
		config = build.NewConfig(buildCtx)
	} else {
		config = build.NewConfig(buildCtx, os.Args[1:]...)
	}

	if err := loadEnvConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse env config files: %v", err)
		os.Exit(1)
	}

	build.SetupOutDir(buildCtx, config)

	logsDir := config.OutDir()
	if config.Dist() {
		logsDir = filepath.Join(config.DistDir(), "logs")
	}

	buildErrorFile := filepath.Join(logsDir, logsPrefix+"build_error")
	rbeMetricsFile := filepath.Join(logsDir, logsPrefix+"rbe_metrics.pb")
	soongMetricsFile := filepath.Join(logsDir, logsPrefix+"soong_metrics")
	defer build.UploadMetrics(buildCtx, config, simpleOutput, buildStarted, buildErrorFile, rbeMetricsFile, soongMetricsFile)

	os.MkdirAll(logsDir, 0777)

	log.SetOutput(filepath.Join(logsDir, "soong.log"))
	trace.SetOutput(filepath.Join(logsDir, "build.trace"))
	stat.AddOutput(status.NewVerboseLog(log, filepath.Join(logsDir, "verbose.log")))
	stat.AddOutput(status.NewErrorLog(log, filepath.Join(logsDir, "error.log")))

	defer met.Dump(soongMetricsFile)
	defer build.DumpRBEMetrics(buildCtx, config, rbeMetricsFile)

	if start, ok := os.LookupEnv("TRACE_BEGIN_SOONG"); ok {
		if !strings.HasSuffix(start, "N") {
			if start_time, err := strconv.ParseUint(start, 10, 64); err == nil {
				log.Verbosef("Took %dms to start up.",
					time.Since(time.Unix(0, int64(start_time))).Nanoseconds()/time.Millisecond.Nanoseconds())
				buildCtx.CompleteTrace(metrics.RunSetupTool, "startup", start_time, uint64(time.Now().UnixNano()))
			}
		}

		if executable, err := os.Executable(); err == nil {
			trace.ImportMicrofactoryLog(filepath.Join(filepath.Dir(executable), "."+filepath.Base(executable)+".trace"))
		}
	}

	// Fix up the source tree due to a repo bug where it doesn't remove
	// linkfiles that have been removed
	fixBadDanglingLink(buildCtx, "hardware/qcom/sdm710/Android.bp")
	fixBadDanglingLink(buildCtx, "hardware/qcom/sdm710/Android.mk")

	f := build.NewSourceFinder(buildCtx, config)
	defer f.Shutdown()
	build.FindSources(buildCtx, config, f)

	if os.Args[1] == "--dumpvar-mode" {
		dumpVar(buildCtx, config, os.Args[2:])
	} else if os.Args[1] == "--dumpvars-mode" {
		dumpVars(buildCtx, config, os.Args[2:])
	} else {
		if config.IsVerbose() {
			writer.Print("! The argument `showcommands` is no longer supported.")
			writer.Print("! Instead, the verbose log is always written to a compressed file in the output dir:")
			writer.Print("!")
			writer.Print(fmt.Sprintf("!   gzip -cd %s/verbose.log.gz | less -R", logsDir))
			writer.Print("!")
			writer.Print("! Older versions are saved in verbose.log.#.gz files")
			writer.Print("")
			time.Sleep(5 * time.Second)
		}

		toBuild := build.BuildAll
		if config.Checkbuild() {
			toBuild |= build.RunBuildTests
		}
		build.Build(buildCtx, config, toBuild)
	}
}

func fixBadDanglingLink(ctx build.Context, name string) {
	_, err := os.Lstat(name)
	if err != nil {
		return
	}
	_, err = os.Stat(name)
	if os.IsNotExist(err) {
		err = os.Remove(name)
		if err != nil {
			ctx.Fatalf("Failed to remove dangling link %q: %v", name, err)
		}
	}
}

func dumpVar(ctx build.Context, config build.Config, args []string) {
	flags := flag.NewFlagSet("dumpvar", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s --dumpvar-mode [--abs] <VAR>\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "In dumpvar mode, print the value of the legacy make variable VAR to stdout")
		fmt.Fprintln(os.Stderr, "")

		fmt.Fprintln(os.Stderr, "'report_config' is a special case that prints the human-readable config banner")
		fmt.Fprintln(os.Stderr, "from the beginning of the build.")
		fmt.Fprintln(os.Stderr, "")
		flags.PrintDefaults()
	}
	abs := flags.Bool("abs", false, "Print the absolute path of the value")
	flags.Parse(args)

	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(1)
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
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s --dumpvars-mode [--vars=\"VAR VAR ...\"]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "In dumpvars mode, dump the values of one or more legacy make variables, in")
		fmt.Fprintln(os.Stderr, "shell syntax. The resulting output may be sourced directly into a shell to")
		fmt.Fprintln(os.Stderr, "set corresponding shell variables.")
		fmt.Fprintln(os.Stderr, "")

		fmt.Fprintln(os.Stderr, "'report_config' is a special case that dumps a variable containing the")
		fmt.Fprintln(os.Stderr, "human-readable config banner from the beginning of the build.")
		fmt.Fprintln(os.Stderr, "")
		flags.PrintDefaults()
	}

	varsStr := flags.String("vars", "", "Space-separated list of variables to dump")
	absVarsStr := flags.String("abs-vars", "", "Space-separated list of variables to dump (using absolute paths)")

	varPrefix := flags.String("var-prefix", "", "String to prepend to all variable names when dumping")
	absVarPrefix := flags.String("abs-var-prefix", "", "String to prepent to all absolute path variable names when dumping")

	flags.Parse(args)

	if flags.NArg() != 0 {
		flags.Usage()
		os.Exit(1)
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
