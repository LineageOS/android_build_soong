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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"android/soong/ui/build"
	"android/soong/ui/logger"
	"android/soong/ui/status"
	"android/soong/ui/terminal"
	"android/soong/ui/tracer"
	"android/soong/zip"
)

// We default to number of cpus / 4, which seems to be the sweet spot for my
// system. I suspect this is mostly due to memory or disk bandwidth though, and
// may depend on the size ofthe source tree, so this probably isn't a great
// default.
func detectNumJobs() int {
	if runtime.NumCPU() < 4 {
		return 1
	}
	return runtime.NumCPU() / 4
}

var numJobs = flag.Int("j", detectNumJobs(), "number of parallel kati jobs")

var keepArtifacts = flag.Bool("keep", false, "keep archives of artifacts")

var outDir = flag.String("out", "", "path to store output directories (defaults to tmpdir under $OUT when empty)")
var alternateResultDir = flag.Bool("dist", false, "write select results to $DIST_DIR (or <out>/dist when empty)")

var onlyConfig = flag.Bool("only-config", false, "Only run product config (not Soong or Kati)")
var onlySoong = flag.Bool("only-soong", false, "Only run product config and Soong (not Kati)")

var buildVariant = flag.String("variant", "eng", "build variant to use")

var skipProducts = flag.String("skip-products", "", "comma-separated list of products to skip (known failures, etc)")
var includeProducts = flag.String("products", "", "comma-separated list of products to build")

const errorLeadingLines = 20
const errorTrailingLines = 20

type Product struct {
	ctx     build.Context
	config  build.Config
	logFile string
	action  *status.Action
}

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

func main() {
	writer := terminal.NewWriter(terminal.StdioImpl{})
	defer writer.Finish()

	log := logger.New(writer)
	defer log.Cleanup()

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trace := tracer.New(log)
	defer trace.Close()

	stat := &status.Status{}
	defer stat.Finish()
	stat.AddOutput(terminal.NewStatusOutput(writer, ""))

	var failures failureCount
	stat.AddOutput(&failures)

	build.SetupSignals(log, cancel, func() {
		trace.Close()
		log.Cleanup()
		stat.Finish()
	})

	buildCtx := build.Context{ContextImpl: &build.ContextImpl{
		Context: ctx,
		Logger:  log,
		Tracer:  trace,
		Writer:  writer,
		Status:  stat,
	}}

	config := build.NewConfig(buildCtx)
	if *outDir == "" {
		name := "multiproduct-" + time.Now().Format("20060102150405")

		*outDir = filepath.Join(config.OutDir(), name)

		// Ensure the empty files exist in the output directory
		// containing our output directory too. This is mostly for
		// safety, but also triggers the ninja_build file so that our
		// build servers know that they can parse the output as if it
		// was ninja output.
		build.SetupOutDir(buildCtx, config)

		if err := os.MkdirAll(*outDir, 0777); err != nil {
			log.Fatalf("Failed to create tempdir: %v", err)
		}
	}
	config.Environment().Set("OUT_DIR", *outDir)
	log.Println("Output directory:", *outDir)

	logsDir := filepath.Join(config.OutDir(), "logs")
	os.MkdirAll(logsDir, 0777)

	build.SetupOutDir(buildCtx, config)
	if *alternateResultDir {
		distLogsDir := filepath.Join(config.DistDir(), "logs")
		os.MkdirAll(distLogsDir, 0777)
		log.SetOutput(filepath.Join(distLogsDir, "soong.log"))
		trace.SetOutput(filepath.Join(distLogsDir, "build.trace"))
	} else {
		log.SetOutput(filepath.Join(config.OutDir(), "soong.log"))
		trace.SetOutput(filepath.Join(config.OutDir(), "build.trace"))
	}

	setMaxFiles(log)

	finder := build.NewSourceFinder(buildCtx, config)
	defer finder.Shutdown()

	build.FindSources(buildCtx, config, finder)

	vars, err := build.DumpMakeVars(buildCtx, config, nil, []string{"all_named_products"})
	if err != nil {
		log.Fatal(err)
	}
	var productsList []string
	allProducts := strings.Fields(vars["all_named_products"])

	if *includeProducts != "" {
		missingProducts := []string{}
		for _, product := range strings.Split(*includeProducts, ",") {
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

	products := make([]string, 0, len(productsList))
	skipList := strings.Split(*skipProducts, ",")
	skipProduct := func(p string) bool {
		for _, s := range skipList {
			if p == s {
				return true
			}
		}
		return false
	}
	for _, product := range productsList {
		if !skipProduct(product) {
			products = append(products, product)
		} else {
			log.Verbose("Skipping: ", product)
		}
	}

	log.Verbose("Got product list: ", products)

	s := buildCtx.Status.StartTool()
	s.SetTotalActions(len(products))

	var wg sync.WaitGroup
	productConfigs := make(chan Product, len(products))

	// Run the product config for every product in parallel
	for _, product := range products {
		wg.Add(1)
		go func(product string) {
			var stdLog string

			defer wg.Done()

			action := &status.Action{
				Description: product,
				Outputs:     []string{product},
			}
			s.StartAction(action)
			defer logger.Recover(func(err error) {
				s.FinishAction(status.ActionResult{
					Action: action,
					Error:  err,
					Output: errMsgFromLog(stdLog),
				})
			})

			productOutDir := filepath.Join(config.OutDir(), product)
			productLogDir := filepath.Join(logsDir, product)

			if err := os.MkdirAll(productOutDir, 0777); err != nil {
				log.Fatalf("Error creating out directory: %v", err)
			}
			if err := os.MkdirAll(productLogDir, 0777); err != nil {
				log.Fatalf("Error creating log directory: %v", err)
			}

			stdLog = filepath.Join(productLogDir, "std.log")
			f, err := os.Create(stdLog)
			if err != nil {
				log.Fatalf("Error creating std.log: %v", err)
			}

			productLog := logger.New(f)
			productLog.SetOutput(filepath.Join(productLogDir, "soong.log"))

			productCtx := build.Context{ContextImpl: &build.ContextImpl{
				Context: ctx,
				Logger:  productLog,
				Tracer:  trace,
				Writer:  terminal.NewWriter(terminal.NewCustomStdio(nil, f, f)),
				Thread:  trace.NewThread(product),
				Status:  &status.Status{},
			}}
			productCtx.Status.AddOutput(terminal.NewStatusOutput(productCtx.Writer, ""))

			productConfig := build.NewConfig(productCtx)
			productConfig.Environment().Set("OUT_DIR", productOutDir)
			build.FindSources(productCtx, productConfig, finder)
			productConfig.Lunch(productCtx, product, *buildVariant)

			build.Build(productCtx, productConfig, build.BuildProductConfig)
			productConfigs <- Product{productCtx, productConfig, stdLog, action}
		}(product)
	}
	go func() {
		defer close(productConfigs)
		wg.Wait()
	}()

	var wg2 sync.WaitGroup
	// Then run up to numJobs worth of Soong and Kati
	for i := 0; i < *numJobs; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for product := range productConfigs {
				func() {
					defer logger.Recover(func(err error) {
						s.FinishAction(status.ActionResult{
							Action: product.action,
							Error:  err,
							Output: errMsgFromLog(product.logFile),
						})
					})

					defer func() {
						if *keepArtifacts {
							args := zip.ZipArgs{
								FileArgs: []zip.FileArg{
									{
										GlobDir:             product.config.OutDir(),
										SourcePrefixToStrip: product.config.OutDir(),
									},
								},
								OutputFilePath:   filepath.Join(config.OutDir(), product.config.TargetProduct()+".zip"),
								NumParallelJobs:  runtime.NumCPU(),
								CompressionLevel: 5,
							}
							if err := zip.Run(args); err != nil {
								log.Fatalf("Error zipping artifacts: %v", err)
							}
						}
						os.RemoveAll(product.config.OutDir())
					}()

					buildWhat := 0
					if !*onlyConfig {
						buildWhat |= build.BuildSoong
						if !*onlySoong {
							buildWhat |= build.BuildKati
						}
					}
					build.Build(product.ctx, product.config, buildWhat)
					s.FinishAction(status.ActionResult{
						Action: product.action,
					})
				}()
			}
		}()
	}
	wg2.Wait()

	if *alternateResultDir {
		args := zip.ZipArgs{
			FileArgs: []zip.FileArg{
				{GlobDir: logsDir, SourcePrefixToStrip: logsDir},
			},
			OutputFilePath:   filepath.Join(config.DistDir(), "logs.zip"),
			NumParallelJobs:  runtime.NumCPU(),
			CompressionLevel: 5,
		}
		if err := zip.Run(args); err != nil {
			log.Fatalf("Error zipping logs: %v", err)
		}
	}

	s.Finish()

	if failures == 1 {
		log.Fatal("1 failure")
	} else if failures > 1 {
		log.Fatalf("%d failures", failures)
	} else {
		writer.Print("Success")
	}
}

type failureCount int

func (f *failureCount) StartAction(action *status.Action, counts status.Counts) {}

func (f *failureCount) FinishAction(result status.ActionResult, counts status.Counts) {
	if result.Error != nil {
		*f += 1
	}
}

func (f *failureCount) Message(level status.MsgLevel, message string) {
	if level >= status.ErrorLvl {
		*f += 1
	}
}

func (f *failureCount) Flush() {}
