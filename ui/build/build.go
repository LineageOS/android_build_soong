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

	"android/soong/ui/metrics"
)

// SetupOutDir ensures the out directory exists, and has the proper files to
// prevent kati from recursing into it.
func SetupOutDir(ctx Context, config Config) {
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "Android.mk"))
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "CleanSpec.mk"))
	if !config.SkipKati() {
		// Run soong_build with Kati for a hybrid build, e.g. running the
		// AndroidMk singleton and postinstall commands. Communicate this to
		// soong_build by writing an empty .soong.kati_enabled marker file in the
		// soong_build output directory for the soong_build primary builder to
		// know if the user wants to run Kati after.
		//
		// This does not preclude running Kati for *product configuration purposes*.
		ensureEmptyFileExists(ctx, filepath.Join(config.SoongOutDir(), ".soong.kati_enabled"))
	}
	// The ninja_build file is used by our buildbots to understand that the output
	// can be parsed as ninja output.
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "ninja_build"))
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), ".out-dir"))

	if buildDateTimeFile, ok := config.environ.Get("BUILD_DATETIME_FILE"); ok {
		err := ioutil.WriteFile(buildDateTimeFile, []byte(config.buildDateTime), 0666) // a+rw
		if err != nil {
			ctx.Fatalln("Failed to write BUILD_DATETIME to file:", err)
		}
	} else {
		ctx.Fatalln("Missing BUILD_DATETIME_FILE")
	}
}

var combinedBuildNinjaTemplate = template.Must(template.New("combined").Parse(`
builddir = {{.OutDir}}
{{if .UseRemoteBuild }}pool local_pool
 depth = {{.Parallel}}
{{end -}}
pool highmem_pool
 depth = {{.HighmemParallel}}
{{if .HasKatiSuffix}}subninja {{.KatiBuildNinjaFile}}
subninja {{.KatiPackageNinjaFile}}
{{end -}}
subninja {{.SoongNinjaFile}}
`))

func createCombinedBuildNinjaFile(ctx Context, config Config) {
	// If we're in SkipKati mode, skip creating this file if it already exists
	if config.SkipKati() {
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

// These are bitmasks which can be used to check whether various flags are set e.g. whether to use Bazel.
const (
	BuildNone          = iota
	BuildProductConfig = 1 << iota
	BuildSoong         = 1 << iota
	BuildKati          = 1 << iota
	BuildNinja         = 1 << iota
	BuildBazel         = 1 << iota
	RunBuildTests      = 1 << iota
	BuildAll           = BuildProductConfig | BuildSoong | BuildKati | BuildNinja
	BuildAllWithBazel  = BuildProductConfig | BuildSoong | BuildKati | BuildBazel
)

// checkProblematicFiles fails the build if existing Android.mk or CleanSpec.mk files are found at the root of the tree.
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

// checkCaseSensitivity issues a warning if a case-insensitive file system is being used.
func checkCaseSensitivity(ctx Context, config Config) {
	outDir := config.OutDir()
	lowerCase := filepath.Join(outDir, "casecheck.txt")
	upperCase := filepath.Join(outDir, "CaseCheck.txt")
	lowerData := "a"
	upperData := "B"

	if err := ioutil.WriteFile(lowerCase, []byte(lowerData), 0666); err != nil { // a+rw
		ctx.Fatalln("Failed to check case sensitivity:", err)
	}

	if err := ioutil.WriteFile(upperCase, []byte(upperData), 0666); err != nil { // a+rw
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

// help prints a help/usage message, via the build/make/help.sh script.
func help(ctx Context, config Config) {
	cmd := Command(ctx, config, "help.sh", "build/make/help.sh")
	cmd.Sandbox = dumpvarsSandbox
	cmd.RunAndPrintOrFatal()
}

// checkRAM warns if there probably isn't enough RAM to complete a build.
func checkRAM(ctx Context, config Config) {
	if totalRAM := config.TotalRAM(); totalRAM != 0 {
		ram := float32(totalRAM) / (1024 * 1024 * 1024)
		ctx.Verbosef("Total RAM: %.3vGB", ram)

		if ram <= 16 {
			ctx.Println("************************************************************")
			ctx.Printf("You are building on a machine with %.3vGB of RAM\n", ram)
			ctx.Println("")
			ctx.Println("The minimum required amount of free memory is around 16GB,")
			ctx.Println("and even with that, some configurations may not work.")
			ctx.Println("")
			ctx.Println("If you run into segfaults or other errors, try reducing your")
			ctx.Println("-j value.")
			ctx.Println("************************************************************")
		} else if ram <= float32(config.Parallel()) {
			// Want at least 1GB of RAM per job.
			ctx.Printf("Warning: high -j%d count compared to %.3vGB of RAM", config.Parallel(), ram)
			ctx.Println("If you run into segfaults or other errors, try a lower -j value")
		}
	}
}

// Build the tree. The 'what' argument can be used to chose which components of
// the build to run, via checking various bitmasks.
func Build(ctx Context, config Config, what int) {
	ctx.Verboseln("Starting build with args:", config.Arguments())
	ctx.Verboseln("Environment:", config.Environment().Environ())

	ctx.BeginTrace(metrics.Total, "total")
	defer ctx.EndTrace()

	if inList("help", config.Arguments()) {
		help(ctx, config)
		return
	}

	// Make sure that no other Soong process is running with the same output directory
	buildLock := BecomeSingletonOrFail(ctx, config)
	defer buildLock.Unlock()

	if inList("clean", config.Arguments()) || inList("clobber", config.Arguments()) {
		clean(ctx, config)
		return
	}

	// checkProblematicFiles aborts the build if Android.mk or CleanSpec.mk are found at the root of the tree.
	checkProblematicFiles(ctx)

	checkRAM(ctx, config)

	SetupOutDir(ctx, config)

	// checkCaseSensitivity issues a warning if a case-insensitive file system is being used.
	checkCaseSensitivity(ctx, config)

	ensureEmptyDirectoriesExist(ctx, config.TempDir())

	SetupPath(ctx, config)

	if config.SkipConfig() {
		ctx.Verboseln("Skipping Config as requested")
		what = what &^ BuildProductConfig
	}

	if config.SkipKati() {
		ctx.Verboseln("Skipping Kati as requested")
		what = what &^ BuildKati
	}

	if config.SkipNinja() {
		ctx.Verboseln("Skipping Ninja as requested")
		what = what &^ BuildNinja
	}

	if config.StartGoma() {
		// Ensure start Goma compiler_proxy
		startGoma(ctx, config)
	}

	if config.StartRBE() {
		// Ensure RBE proxy is started
		startRBE(ctx, config)
	}

	if what&BuildProductConfig != 0 {
		// Run make for product config
		runMakeProductConfig(ctx, config)
	}

	// Everything below here depends on product config.

	if inList("installclean", config.Arguments()) ||
		inList("install-clean", config.Arguments()) {
		installClean(ctx, config)
		ctx.Println("Deleted images and staging directories.")
		return
	}

	if inList("dataclean", config.Arguments()) ||
		inList("data-clean", config.Arguments()) {
		dataClean(ctx, config)
		ctx.Println("Deleted data files.")
		return
	}

	if what&BuildSoong != 0 {
		// Run Soong
		runSoong(ctx, config)

		if config.Environment().IsEnvTrue("GENERATE_BAZEL_FILES") {
			// Return early, if we're using Soong as the bp2build converter.
			return
		}
	}

	if what&BuildKati != 0 {
		// Run ckati
		genKatiSuffix(ctx, config)
		runKatiCleanSpec(ctx, config)
		runKatiBuild(ctx, config)
		runKatiPackage(ctx, config)

		ioutil.WriteFile(config.LastKatiSuffixFile(), []byte(config.KatiSuffix()), 0666) // a+rw
	} else {
		// Load last Kati Suffix if it exists
		if katiSuffix, err := ioutil.ReadFile(config.LastKatiSuffixFile()); err == nil {
			ctx.Verboseln("Loaded previous kati config:", string(katiSuffix))
			config.SetKatiSuffix(string(katiSuffix))
		}
	}

	// Write combined ninja file
	createCombinedBuildNinjaFile(ctx, config)

	distGzipFile(ctx, config, config.CombinedNinjaFile())

	if what&RunBuildTests != 0 {
		testForDanglingRules(ctx, config)
	}

	if what&BuildNinja != 0 {
		if what&BuildKati != 0 {
			installCleanIfNecessary(ctx, config)
		}

		// Run ninja
		runNinjaForBuild(ctx, config)
	}

	// Currently, using Bazel requires Kati and Soong to run first, so check whether to run Bazel last.
	if what&BuildBazel != 0 {
		runBazel(ctx, config)
	}
}

// distGzipFile writes a compressed copy of src to the distDir if dist is enabled.  Failures
// are printed but non-fatal.
func distGzipFile(ctx Context, config Config, src string, subDirs ...string) {
	if !config.Dist() {
		return
	}

	subDir := filepath.Join(subDirs...)
	destDir := filepath.Join(config.RealDistDir(), "soong_ui", subDir)

	if err := os.MkdirAll(destDir, 0777); err != nil { // a+rwx
		ctx.Printf("failed to mkdir %s: %s", destDir, err.Error())
	}

	if err := gzipFileToDir(src, destDir); err != nil {
		ctx.Printf("failed to dist %s: %s", filepath.Base(src), err.Error())
	}
}

// distFile writes a copy of src to the distDir if dist is enabled.  Failures are printed but
// non-fatal.
func distFile(ctx Context, config Config, src string, subDirs ...string) {
	if !config.Dist() {
		return
	}

	subDir := filepath.Join(subDirs...)
	destDir := filepath.Join(config.RealDistDir(), "soong_ui", subDir)

	if err := os.MkdirAll(destDir, 0777); err != nil { // a+rwx
		ctx.Printf("failed to mkdir %s: %s", destDir, err.Error())
	}

	if _, err := copyFile(src, filepath.Join(destDir, filepath.Base(src))); err != nil {
		ctx.Printf("failed to dist %s: %s", filepath.Base(src), err.Error())
	}
}
