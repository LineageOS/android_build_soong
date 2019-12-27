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

package build

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
)

// Ensures the out directory exists, and has the proper files to prevent kati
// from recursing into it.
func SetupOutDir(ctx Context, config Config) {
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "Android.mk"))
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "CleanSpec.mk"))
	if !config.SkipMake() {
		ensureEmptyFileExists(ctx, filepath.Join(config.SoongOutDir(), ".soong.in_make"))
	}
	// The ninja_build file is used by our buildbots to understand that the output
	// can be parsed as ninja output.
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "ninja_build"))
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), ".out-dir"))
}

var combinedBuildNinjaTemplate = template.Must(template.New("combined").Parse(`
builddir = {{.OutDir}}
pool local_pool
 depth = {{.Parallel}}
build _kati_always_build_: phony
{{if .HasKatiSuffix}}subninja {{.KatiBuildNinjaFile}}
subninja {{.KatiPackageNinjaFile}}
{{end -}}
subninja {{.SoongNinjaFile}}
`))

func createCombinedBuildNinjaFile(ctx Context, config Config) {
	// If we're in SkipMake mode, skip creating this file if it already exists
	if config.SkipMake() {
		if _, err := os.Stat(config.CombinedNinjaFile()); err == nil || !os.IsNotExist(err) {
			return
		}
	}

	file, err := os.Create(config.CombinedNinjaFile())
	if err != nil {
		ctx.Fatalln("Failed to create combined ninja file:", err)
	}
	defer file.Close()

	if err := combinedBuildNinjaTemplate.Execute(file, config); err != nil {
		ctx.Fatalln("Failed to write combined ninja file:", err)
	}
}

const (
	BuildNone          = iota
	BuildProductConfig = 1 << iota
	BuildSoong         = 1 << iota
	BuildKati          = 1 << iota
	BuildNinja         = 1 << iota
	RunBuildTests      = 1 << iota
	BuildAll           = BuildProductConfig | BuildSoong | BuildKati | BuildNinja
)

func checkProblematicFiles(ctx Context) {
	files := []string{"Android.mk", "CleanSpec.mk"}
	for _, file := range files {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			absolute := absPath(ctx, file)
			ctx.Printf("Found %s in tree root. This file needs to be removed to build.\n", file)
			ctx.Fatalf("    rm %s\n", absolute)
		}
	}
}

func checkCaseSensitivity(ctx Context, config Config) {
	outDir := config.OutDir()
	lowerCase := filepath.Join(outDir, "casecheck.txt")
	upperCase := filepath.Join(outDir, "CaseCheck.txt")
	lowerData := "a"
	upperData := "B"

	err := ioutil.WriteFile(lowerCase, []byte(lowerData), 0777)
	if err != nil {
		ctx.Fatalln("Failed to check case sensitivity:", err)
	}

	err = ioutil.WriteFile(upperCase, []byte(upperData), 0777)
	if err != nil {
		ctx.Fatalln("Failed to check case sensitivity:", err)
	}

	res, err := ioutil.ReadFile(lowerCase)
	if err != nil {
		ctx.Fatalln("Failed to check case sensitivity:", err)
	}

	if string(res) != lowerData {
		ctx.Println("************************************************************")
		ctx.Println("You are building on a case-insensitive filesystem.")
		ctx.Println("Please move your source tree to a case-sensitive filesystem.")
		ctx.Println("************************************************************")
		ctx.Fatalln("Case-insensitive filesystems not supported")
	}
}

func help(ctx Context, config Config, what int) {
	cmd := Command(ctx, config, "help.sh", "build/make/help.sh")
	cmd.Sandbox = dumpvarsSandbox
	cmd.RunAndPrintOrFatal()
}

// Build the tree. The 'what' argument can be used to chose which components of
// the build to run.
func Build(ctx Context, config Config, what int) {
	ctx.Verboseln("Starting build with args:", config.Arguments())
	ctx.Verboseln("Environment:", config.Environment().Environ())
	ctx.Verbosef("Total RAM: %dGB", config.TotalRAM()/1024/1024/1024)

	if config.SkipMake() {
		ctx.Verboseln("Skipping Make/Kati as requested")
		what = what & (BuildSoong | BuildNinja)
	}

	if inList("help", config.Arguments()) {
		help(ctx, config, what)
		return
	} else if inList("clean", config.Arguments()) || inList("clobber", config.Arguments()) {
		clean(ctx, config, what)
		return
	}

	// Make sure that no other Soong process is running with the same output directory
	buildLock := BecomeSingletonOrFail(ctx, config)
	defer buildLock.Unlock()

	checkProblematicFiles(ctx)

	SetupOutDir(ctx, config)

	checkCaseSensitivity(ctx, config)

	ensureEmptyDirectoriesExist(ctx, config.TempDir())

	SetupPath(ctx, config)

	if config.StartGoma() {
		// Ensure start Goma compiler_proxy
		startGoma(ctx, config)
	}

	if what&BuildProductConfig != 0 {
		// Run make for product config
		runMakeProductConfig(ctx, config)
	}

	if inList("installclean", config.Arguments()) {
		installClean(ctx, config, what)
		ctx.Println("Deleted images and staging directories.")
		return
	} else if inList("dataclean", config.Arguments()) {
		dataClean(ctx, config, what)
		ctx.Println("Deleted data files.")
		return
	}

	if what&BuildSoong != 0 {
		// Run Soong
		runSoong(ctx, config)
	}

	if what&BuildKati != 0 {
		// Run ckati
		genKatiSuffix(ctx, config)
		runKatiCleanSpec(ctx, config)
		runKatiBuild(ctx, config)
		runKatiPackage(ctx, config)

		ioutil.WriteFile(config.LastKatiSuffixFile(), []byte(config.KatiSuffix()), 0777)
	} else {
		// Load last Kati Suffix if it exists
		if katiSuffix, err := ioutil.ReadFile(config.LastKatiSuffixFile()); err == nil {
			ctx.Verboseln("Loaded previous kati config:", string(katiSuffix))
			config.SetKatiSuffix(string(katiSuffix))
		}
	}

	// Write combined ninja file
	createCombinedBuildNinjaFile(ctx, config)

	if what&RunBuildTests != 0 {
		testForDanglingRules(ctx, config)
	}

	if what&BuildNinja != 0 {
		if !config.SkipMake() {
			installCleanIfNecessary(ctx, config)
		}

		// Run ninja
		runNinja(ctx, config)
	}
}
