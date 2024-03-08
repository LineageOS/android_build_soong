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
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"android/soong/ui/logger"
	"android/soong/ui/signal"
	"android/soong/ui/status"
	"android/soong/ui/terminal"
	"android/soong/ui/tracer"
	"android/soong/zip"
)

var numJobs = flag.Int("j", 0, "number of parallel jobs [0=autodetect]")

var keepArtifacts = flag.Bool("keep", false, "keep archives of artifacts")
var incremental = flag.Bool("incremental", false, "run in incremental mode (saving intermediates)")

var outDir = flag.String("out", "", "path to store output directories (defaults to tmpdir under $OUT when empty)")
var alternateResultDir = flag.Bool("dist", false, "write select results to $DIST_DIR (or <out>/dist when empty)")

var bazelMode = flag.Bool("bazel-mode", false, "use bazel for analysis of certain modules")
var bazelModeStaging = flag.Bool("bazel-mode-staging", false, "use bazel for analysis of certain near-ready modules")

var onlyConfig = flag.Bool("only-config", false, "Only run product config (not Soong or Kati)")
var onlySoong = flag.Bool("only-soong", false, "Only run product config and Soong (not Kati)")

var buildVariant = flag.String("variant", "eng", "build variant to use")

var shardCount = flag.Int("shard-count", 1, "split the products into multiple shards (to spread the build onto multiple machines, etc)")
var shard = flag.Int("shard", 1, "1-indexed shard to execute")

var skipProducts multipleStringArg
var includeProducts multipleStringArg

func init() {
	flag.Var(&skipProducts, "skip-products", "comma-separated list of products to skip (known failures, etc)")
	flag.Var(&includeProducts, "products", "comma-separated list of products to build")
}

// multipleStringArg is a flag.Value that takes comma separated lists and converts them to a
// []string.  The argument can be passed multiple times to append more values.
type multipleStringArg []string

func (m *multipleStringArg) String() string {
	return strings.Join(*m, `, `)
}

func (m *multipleStringArg) Set(s string) error {
	*m = append(*m, strings.Split(s, ",")...)
	return nil
}

const errorLeadingLines = 20
const errorTrailingLines = 20

func errMsgFromLog(filename string) string {
	if filename == "" {
		return ""
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > errorLeadingLines+errorTrailingLines+1 {
		lines[errorLeadingLines] = fmt.Sprintf("... skipping %d lines ...",
			len(lines)-errorLeadingLines-errorTrailingLines)

		lines = append(lines[:errorLeadingLines+1],
			lines[len(lines)-errorTrailingLines:]...)
	}
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString("> ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return buf.String()
}

// TODO(b/70370883): This tool uses a lot of open files -- over the default
// soft limit of 1024 on some systems. So bump up to the hard limit until I fix
// the algorithm.
func setMaxFiles(log logger.Logger) {
	var limits syscall.Rlimit

	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limits)
	if err != nil {
		log.Println("Failed to get file limit:", err)
		return
	}

	log.Verbosef("Current file limits: %d soft, %d hard", limits.Cur, limits.Max)
	if limits.Cur == limits.Max {
		return
	}

	limits.Cur = limits.Max
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limits)
	if err != nil {
		log.Println("Failed to increase file limit:", err)
	}
}

func inList(str string, list []string) bool {
	for _, other := range list {
		if str == other {
			return true
		}
	}
	return false
}

func copyFile(from, to string) error {
	fromFile, err := os.Open(from)
	if err != nil {
		return err
	}
	defer fromFile.Close()

	toFile, err := os.Create(to)
	if err != nil {
		return err
	}
	defer toFile.Close()

	_, err = io.Copy(toFile, fromFile)
	return err
}

type mpContext struct {
	Logger logger.Logger
	Status status.ToolStatus

	SoongUi     string
	MainOutDir  string
	MainLogsDir string
}

func findNamedProducts(soongUi string, log logger.Logger) []string {
	cmd := exec.Command(soongUi, "--dumpvars-mode", "--vars=all_named_products")
	output, err := cmd.Output()
	if err != nil {
		log.Fatalf("Cannot determine named products: %v", err)
	}

	rx := regexp.MustCompile(`^all_named_products='(.*)'$`)
	match := rx.FindStringSubmatch(strings.TrimSpace(string(output)))
	return strings.Fields(match[1])
}

// ensureEmptyFileExists ensures that the containing directory exists, and the
// specified file exists. If it doesn't exist, it will write an empty file.
func ensureEmptyFileExists(file string, log logger.Logger) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		f, err := os.Create(file)
		if err != nil {
			log.Fatalf("Error creating %s: %q\n", file, err)
		}
		f.Close()
	} else if err != nil {
		log.Fatalf("Error checking %s: %q\n", file, err)
	}
}

func outDirBase() string {
	outDirBase := os.Getenv("OUT_DIR")
	if outDirBase == "" {
		return "out"
	} else {
		return outDirBase
	}
}

func distDir(outDir string) string {
	if distDir := os.Getenv("DIST_DIR"); distDir != "" {
		return filepath.Clean(distDir)
	} else {
		return filepath.Join(outDir, "dist")
	}
}

func forceAnsiOutput() bool {
	value := os.Getenv("SOONG_UI_ANSI_OUTPUT")
	return value == "1" || value == "y" || value == "yes" || value == "on" || value == "true"
}

func getBazelArg() string {
	count := 0
	str := ""
	if *bazelMode {
		count++
		str = "--bazel-mode"
	}
	if *bazelModeStaging {
		count++
		str = "--bazel-mode-staging"
	}

	if count > 1 {
		// Can't set more than one
		fmt.Errorf("Only one bazel mode is permitted to be set.")
		os.Exit(1)
	}

	return str
}

func main() {
	stdio := terminal.StdioImpl{}

	output := terminal.NewStatusOutput(stdio.Stdout(), "", false, false,
		forceAnsiOutput())
	log := logger.New(output)
	defer log.Cleanup()

	for _, v := range os.Environ() {
		log.Println("Environment: " + v)
	}

	log.Printf("Argv: %v\n", os.Args)

	flag.Parse()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	trace := tracer.New(log)
	defer trace.Close()

	stat := &status.Status{}
	defer stat.Finish()
	stat.AddOutput(output)

	var failures failureCount
	stat.AddOutput(&failures)

	signal.SetupSignals(log, cancel, func() {
		trace.Close()
		log.Cleanup()
		stat.Finish()
	})

	soongUi := "build/soong/soong_ui.bash"

	var outputDir string
	if *outDir != "" {
		outputDir = *outDir
	} else {
		name := "multiproduct"
		if !*incremental {
			name += "-" + time.Now().Format("20060102150405")
		}
		outputDir = filepath.Join(outDirBase(), name)
	}

	log.Println("Output directory:", outputDir)

	// The ninja_build file is used by our buildbots to understand that the output
	// can be parsed as ninja output.
	if err := os.MkdirAll(outputDir, 0777); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}
	ensureEmptyFileExists(filepath.Join(outputDir, "ninja_build"), log)

	logsDir := filepath.Join(outputDir, "logs")
	os.MkdirAll(logsDir, 0777)

	var configLogsDir string
	if *alternateResultDir {
		configLogsDir = filepath.Join(distDir(outDirBase()), "logs")
	} else {
		configLogsDir = outputDir
	}

	log.Println("Logs dir: " + configLogsDir)

	os.MkdirAll(configLogsDir, 0777)
	log.SetOutput(filepath.Join(configLogsDir, "soong.log"))
	trace.SetOutput(filepath.Join(configLogsDir, "build.trace"))

	var jobs = *numJobs
	if jobs < 1 {
		jobs = runtime.NumCPU() / 4

		ramGb := int(detectTotalRAM() / (1024 * 1024 * 1024))
		if ramJobs := ramGb / 40; ramGb > 0 && jobs > ramJobs {
			jobs = ramJobs
		}

		if jobs < 1 {
			jobs = 1
		}
	}
	log.Verbosef("Using %d parallel jobs", jobs)

	setMaxFiles(log)

	allProducts := findNamedProducts(soongUi, log)
	var productsList []string

	if len(includeProducts) > 0 {
		var missingProducts []string
		for _, product := range includeProducts {
			if inList(product, allProducts) {
				productsList = append(productsList, product)
			} else {
				missingProducts = append(missingProducts, product)
			}
		}
		if len(missingProducts) > 0 {
			log.Fatalf("Products don't exist: %s\n", missingProducts)
		}
	} else {
		productsList = allProducts
	}

	finalProductsList := make([]string, 0, len(productsList))
	skipProduct := func(p string) bool {
		for _, s := range skipProducts {
			if p == s {
				return true
			}
		}
		return false
	}
	for _, product := range productsList {
		if !skipProduct(product) {
			finalProductsList = append(finalProductsList, product)
		} else {
			log.Verbose("Skipping: ", product)
		}
	}

	if *shard < 1 {
		log.Fatalf("--shard value must be >= 1, not %d\n", *shard)
	} else if *shardCount < 1 {
		log.Fatalf("--shard-count value must be >= 1, not %d\n", *shardCount)
	} else if *shard > *shardCount {
		log.Fatalf("--shard (%d) must not be greater than --shard-count (%d)\n", *shard,
			*shardCount)
	} else if *shardCount > 1 {
		finalProductsList = splitList(finalProductsList, *shardCount)[*shard-1]
	}

	log.Verbose("Got product list: ", finalProductsList)

	s := stat.StartTool()
	s.SetTotalActions(len(finalProductsList))

	mpCtx := &mpContext{
		Logger:      log,
		Status:      s,
		SoongUi:     soongUi,
		MainOutDir:  outputDir,
		MainLogsDir: logsDir,
	}

	products := make(chan string, len(productsList))
	go func() {
		defer close(products)
		for _, product := range finalProductsList {
			products <- product
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		// To smooth out the spikes in memory usage, skew the
		// initial starting time of the jobs by a small amount.
		time.Sleep(15 * time.Second)
		go func() {
			defer wg.Done()
			for {
				select {
				case product := <-products:
					if product == "" {
						return
					}
					runSoongUiForProduct(mpCtx, product)
				}
			}
		}()
	}
	wg.Wait()

	if *alternateResultDir {
		args := zip.ZipArgs{
			FileArgs: []zip.FileArg{
				{GlobDir: logsDir, SourcePrefixToStrip: logsDir},
			},
			OutputFilePath:   filepath.Join(distDir(outDirBase()), "logs.zip"),
			NumParallelJobs:  runtime.NumCPU(),
			CompressionLevel: 5,
		}
		log.Printf("Logs zip: %v\n", args.OutputFilePath)
		if err := zip.Zip(args); err != nil {
			log.Fatalf("Error zipping logs: %v", err)
		}
	}

	s.Finish()

	if failures.count == 1 {
		log.Fatal("1 failure")
	} else if failures.count > 1 {
		log.Fatalf("%d failures %q", failures.count, failures.fails)
	} else {
		fmt.Fprintln(output, "Success")
	}
}

func cleanupAfterProduct(outDir, productZip string) {
	if *keepArtifacts {
		args := zip.ZipArgs{
			FileArgs: []zip.FileArg{
				{
					GlobDir:             outDir,
					SourcePrefixToStrip: outDir,
				},
			},
			OutputFilePath:   productZip,
			NumParallelJobs:  runtime.NumCPU(),
			CompressionLevel: 5,
		}
		if err := zip.Zip(args); err != nil {
			log.Fatalf("Error zipping artifacts: %v", err)
		}
	}
	if !*incremental {
		os.RemoveAll(outDir)
	}
}

func runSoongUiForProduct(mpctx *mpContext, product string) {
	outDir := filepath.Join(mpctx.MainOutDir, product)
	logsDir := filepath.Join(mpctx.MainLogsDir, product)
	productZip := filepath.Join(mpctx.MainOutDir, product+".zip")
	consoleLogPath := filepath.Join(logsDir, "std.log")

	if err := os.MkdirAll(outDir, 0777); err != nil {
		mpctx.Logger.Fatalf("Error creating out directory: %v", err)
	}
	if err := os.MkdirAll(logsDir, 0777); err != nil {
		mpctx.Logger.Fatalf("Error creating log directory: %v", err)
	}

	consoleLogFile, err := os.Create(consoleLogPath)
	if err != nil {
		mpctx.Logger.Fatalf("Error creating console log file: %v", err)
	}
	defer consoleLogFile.Close()

	consoleLogWriter := bufio.NewWriter(consoleLogFile)
	defer consoleLogWriter.Flush()

	args := []string{"--make-mode", "--skip-soong-tests", "--skip-ninja"}

	if !*keepArtifacts {
		args = append(args, "--empty-ninja-file")
	}

	if *onlyConfig {
		args = append(args, "--config-only")
	} else if *onlySoong {
		args = append(args, "--soong-only")
	}

	bazelStr := getBazelArg()
	if bazelStr != "" {
		args = append(args, bazelStr)
	}

	cmd := exec.Command(mpctx.SoongUi, args...)
	cmd.Stdout = consoleLogWriter
	cmd.Stderr = consoleLogWriter
	cmd.Env = append(os.Environ(),
		"OUT_DIR="+outDir,
		"TARGET_PRODUCT="+product,
		"TARGET_BUILD_VARIANT="+*buildVariant,
		"TARGET_BUILD_TYPE=release",
		"TARGET_BUILD_APPS=",
		"TARGET_BUILD_UNBUNDLED=",
		"USE_RBE=false") // Disabling RBE saves ~10 secs per product

	if *alternateResultDir {
		cmd.Env = append(cmd.Env,
			"DIST_DIR="+filepath.Join(distDir(outDirBase()), "products/"+product))
	}

	action := &status.Action{
		Description: product,
		Outputs:     []string{product},
	}

	mpctx.Status.StartAction(action)
	defer cleanupAfterProduct(outDir, productZip)

	before := time.Now()
	err = cmd.Run()

	if !*onlyConfig && !*onlySoong {
		katiBuildNinjaFile := filepath.Join(outDir, "build-"+product+".ninja")
		if after, err := os.Stat(katiBuildNinjaFile); err == nil && after.ModTime().After(before) {
			err := copyFile(consoleLogPath, filepath.Join(filepath.Dir(consoleLogPath), "std_full.log"))
			if err != nil {
				log.Fatalf("Error copying log file: %s", err)
			}
		}
	}
	var errOutput string
	if err == nil {
		errOutput = ""
	} else {
		errOutput = errMsgFromLog(consoleLogPath)
	}

	mpctx.Status.FinishAction(status.ActionResult{
		Action: action,
		Error:  err,
		Output: errOutput,
	})
}

type failureCount struct {
	count int
	fails []string
}

func (f *failureCount) StartAction(action *status.Action, counts status.Counts) {}

func (f *failureCount) FinishAction(result status.ActionResult, counts status.Counts) {
	if result.Error != nil {
		f.count += 1
		f.fails = append(f.fails, result.Action.Description)
	}
}

func (f *failureCount) Message(level status.MsgLevel, message string) {
	if level >= status.ErrorLvl {
		f.count += 1
	}
}

func (f *failureCount) Flush() {}

func (f *failureCount) Write(p []byte) (int, error) {
	// discard writes
	return len(p), nil
}

func splitList(list []string, shardCount int) (ret [][]string) {
	each := len(list) / shardCount
	extra := len(list) % shardCount
	for i := 0; i < shardCount; i++ {
		count := each
		if extra > 0 {
			count += 1
			extra -= 1
		}
		ret = append(ret, list[:count])
		list = list[count:]
	}
	return
}
