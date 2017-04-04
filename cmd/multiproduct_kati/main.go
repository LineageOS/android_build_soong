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
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"android/soong/ui/build"
	"android/soong/ui/logger"
	"android/soong/ui/tracer"
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

var keep = flag.Bool("keep", false, "keep successful output files")

var outDir = flag.String("out", "", "path to store output directories (defaults to tmpdir under $OUT when empty)")

var onlyConfig = flag.Bool("only-config", false, "Only run product config (not Soong or Kati)")
var onlySoong = flag.Bool("only-soong", false, "Only run product config and Soong (not Kati)")

type Product struct {
	ctx    build.Context
	config build.Config
}

func main() {
	log := logger.New(os.Stderr)
	defer log.Cleanup()

	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trace := tracer.New(log)
	defer trace.Close()

	build.SetupSignals(log, cancel, func() {
		trace.Close()
		log.Cleanup()
	})

	buildCtx := build.Context{&build.ContextImpl{
		Context:        ctx,
		Logger:         log,
		Tracer:         trace,
		StdioInterface: build.StdioImpl{},
	}}

	failed := false

	config := build.NewConfig(buildCtx)
	if *outDir == "" {
		name := "multiproduct-" + time.Now().Format("20060102150405")

		*outDir = filepath.Join(config.OutDir(), name)

		if err := os.MkdirAll(*outDir, 0777); err != nil {
			log.Fatalf("Failed to create tempdir: %v", err)
		}

		if !*keep {
			defer func() {
				if !failed {
					os.RemoveAll(*outDir)
				}
			}()
		}
	}
	config.Environment().Set("OUT_DIR", *outDir)
	log.Println("Output directory:", *outDir)

	build.SetupOutDir(buildCtx, config)
	log.SetOutput(filepath.Join(config.OutDir(), "soong.log"))
	trace.SetOutput(filepath.Join(config.OutDir(), "build.trace"))

	vars, err := build.DumpMakeVars(buildCtx, config, nil, nil, []string{"all_named_products"})
	if err != nil {
		log.Fatal(err)
	}
	products := strings.Fields(vars["all_named_products"])
	log.Verbose("Got product list:", products)

	var wg sync.WaitGroup
	errs := make(chan error, len(products))
	productConfigs := make(chan Product, len(products))

	// Run the product config for every product in parallel
	for _, product := range products {
		wg.Add(1)
		go func(product string) {
			defer wg.Done()
			defer logger.Recover(func(err error) {
				errs <- fmt.Errorf("Error building %s: %v", product, err)
			})

			productOutDir := filepath.Join(config.OutDir(), product)

			if err := os.MkdirAll(productOutDir, 0777); err != nil {
				log.Fatalf("Error creating out directory: %v", err)
			}

			f, err := os.Create(filepath.Join(productOutDir, "std.log"))
			if err != nil {
				log.Fatalf("Error creating std.log: %v", err)
			}

			productLog := logger.New(&bytes.Buffer{})
			productLog.SetOutput(filepath.Join(productOutDir, "soong.log"))

			productCtx := build.Context{&build.ContextImpl{
				Context:        ctx,
				Logger:         productLog,
				Tracer:         trace,
				StdioInterface: build.NewCustomStdio(nil, f, f),
				Thread:         trace.NewThread(product),
			}}

			productConfig := build.NewConfig(productCtx)
			productConfig.Environment().Set("OUT_DIR", productOutDir)
			productConfig.Lunch(productCtx, product, "eng")

			build.Build(productCtx, productConfig, build.BuildProductConfig)
			productConfigs <- Product{productCtx, productConfig}
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
						errs <- fmt.Errorf("Error building %s: %v", product.config.TargetProduct(), err)
					})

					buildWhat := 0
					if !*onlyConfig {
						buildWhat |= build.BuildSoong
						if !*onlySoong {
							buildWhat |= build.BuildKati
						}
					}
					build.Build(product.ctx, product.config, buildWhat)
					if !*keep {
						os.RemoveAll(product.config.OutDir())
					}
					log.Println("Finished running for", product.config.TargetProduct())
				}()
			}
		}()
	}
	go func() {
		wg2.Wait()
		close(errs)
	}()

	for err := range errs {
		failed = true
		log.Print(err)
	}

	if failed {
		log.Fatalln("Failed")
	}
}
