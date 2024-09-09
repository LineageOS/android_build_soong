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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	"android/soong/ui/metrics"
)

// SetupOutDir ensures the out directory exists, and has the proper files to
// prevent kati from recursing into it.
func SetupOutDir(ctx Context, config Config) {
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "Android.mk"))
	ensureEmptyFileExists(ctx, filepath.Join(config.OutDir(), "CleanSpec.mk"))
	ensureEmptyDirectoriesExist(ctx, config.TempDir())

	// Potentially write a marker file for whether kati is enabled. This is used by soong_build to
	// potentially run the AndroidMk singleton and postinstall commands.
	// Note that the absence of the  file does not not preclude running Kati for product
	// configuration purposes.
	katiEnabledMarker := filepath.Join(config.SoongOutDir(), ".soong.kati_enabled")
	if config.SkipKatiNinja() {
		os.Remove(katiEnabledMarker)
		// Note that we can not remove the file for SkipKati builds yet -- some continuous builds
		// --skip-make builds rely on kati targets being defined.
	} else if !config.SkipKati() {
		ensureEmptyFileExists(ctx, katiEnabledMarker)
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

	// BUILD_NUMBER should be set to the source control value that
	// represents the current state of the source code.  E.g., a
	// perforce changelist number or a git hash.  Can be an arbitrary string
	// (to allow for source control that uses something other than numbers),
	// but must be a single word and a valid file name.
	//
	// If no BUILD_NUMBER is set, create a useful "I am an engineering build"
	// value.  Make it start with a non-digit so that anyone trying to parse
	// it as an integer will probably get "0". This value used to contain
	// a timestamp, but now that more dependencies are tracked in order to
	// reduce the importance of `m installclean`, changing it every build
	// causes unnecessary rebuilds for local development.
	buildNumber, ok := config.environ.Get("BUILD_NUMBER")
	if ok {
		writeValueIfChanged(ctx, config, config.OutDir(), "file_name_tag.txt", buildNumber)
	} else {
		var username string
		if username, ok = config.environ.Get("BUILD_USERNAME"); !ok {
			ctx.Fatalln("Missing BUILD_USERNAME")
		}
		buildNumber = fmt.Sprintf("eng.%.6s", username)
		writeValueIfChanged(ctx, config, config.OutDir(), "file_name_tag.txt", username)
	}
	// Write the build number to a file so it can be read back in
	// without changing the command line every time.  Avoids rebuilds
	// when using ninja.
	writeValueIfChanged(ctx, config, config.SoongOutDir(), "build_number.txt", buildNumber)
}

var combinedBuildNinjaTemplate = template.Must(template.New("combined").Parse(`
builddir = {{.OutDir}}
{{if .UseRemoteBuild }}pool local_pool
 depth = {{.Parallel}}
{{end -}}
pool highmem_pool
 depth = {{.HighmemParallel}}
{{if and (not .SkipKatiNinja) .HasKatiSuffix}}subninja {{.KatiBuildNinjaFile}}
subninja {{.KatiPackageNinjaFile}}
{{end -}}
subninja {{.SoongNinjaFile}}
`))

func createCombinedBuildNinjaFile(ctx Context, config Config) {
	// If we're in SkipKati mode but want to run kati ninja, skip creating this file if it already exists
	if config.SkipKati() && !config.SkipKatiNinja() {
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

// These are bitmasks which can be used to check whether various flags are set
const (
	_ = iota
	// Whether to run the kati config step.
	RunProductConfig = 1 << iota
	// Whether to run soong to generate a ninja file.
	RunSoong = 1 << iota
	// Whether to run kati to generate a ninja file.
	RunKati = 1 << iota
	// Whether to include the kati-generated ninja file in the combined ninja.
	RunKatiNinja = 1 << iota
	// Whether to run ninja on the combined ninja.
	RunNinja       = 1 << iota
	RunDistActions = 1 << iota
	RunBuildTests  = 1 << iota
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

// Build the tree. Various flags in `config` govern which components of
// the build to run.
func Build(ctx Context, config Config) {
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

	logArgsOtherThan := func(specialTargets ...string) {
		var ignored []string
		for _, a := range config.Arguments() {
			if !inList(a, specialTargets) {
				ignored = append(ignored, a)
			}
		}
		if len(ignored) > 0 {
			ctx.Printf("ignoring arguments %q", ignored)
		}
	}

	if inList("clean", config.Arguments()) || inList("clobber", config.Arguments()) {
		logArgsOtherThan("clean", "clobber")
		clean(ctx, config)
		return
	}

	defer waitForDist(ctx)

	// checkProblematicFiles aborts the build if Android.mk or CleanSpec.mk are found at the root of the tree.
	checkProblematicFiles(ctx)

	checkRAM(ctx, config)

	SetupOutDir(ctx, config)

	// checkCaseSensitivity issues a warning if a case-insensitive file system is being used.
	checkCaseSensitivity(ctx, config)

	SetupPath(ctx, config)

	what := evaluateWhatToRun(config, ctx.Verboseln)

	if config.StartGoma() {
		startGoma(ctx, config)
	}

	rbeCh := make(chan bool)
	var rbePanic any
	if config.StartRBE() {
		cleanupRBELogsDir(ctx, config)
		checkRBERequirements(ctx, config)
		go func() {
			defer func() {
				rbePanic = recover()
				close(rbeCh)
			}()
			startRBE(ctx, config)
		}()
		defer DumpRBEMetrics(ctx, config, filepath.Join(config.LogsDir(), "rbe_metrics.pb"))
	} else {
		close(rbeCh)
	}

	if what&RunProductConfig != 0 {
		runMakeProductConfig(ctx, config)
	}

	// Everything below here depends on product config.

	if inList("installclean", config.Arguments()) ||
		inList("install-clean", config.Arguments()) {
		logArgsOtherThan("installclean", "install-clean")
		installClean(ctx, config)
		ctx.Println("Deleted images and staging directories.")
		return
	}

	if inList("dataclean", config.Arguments()) ||
		inList("data-clean", config.Arguments()) {
		logArgsOtherThan("dataclean", "data-clean")
		dataClean(ctx, config)
		ctx.Println("Deleted data files.")
		return
	}

	if what&RunSoong != 0 {
		runSoong(ctx, config)
	}

	if what&RunKati != 0 {
		genKatiSuffix(ctx, config)
		runKatiCleanSpec(ctx, config)
		runKatiBuild(ctx, config)
		runKatiPackage(ctx, config)

		ioutil.WriteFile(config.LastKatiSuffixFile(), []byte(config.KatiSuffix()), 0666) // a+rw
	} else if what&RunKatiNinja != 0 {
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

	<-rbeCh
	if rbePanic != nil {
		// If there was a ctx.Fatal in startRBE, rethrow it.
		panic(rbePanic)
	}

	if what&RunNinja != 0 {
		if what&RunKati != 0 {
			installCleanIfNecessary(ctx, config)
		}
		runNinjaForBuild(ctx, config)
	}

	if what&RunDistActions != 0 {
		runDistActions(ctx, config)
	}
}

func evaluateWhatToRun(config Config, verboseln func(v ...interface{})) int {
	//evaluate what to run
	what := 0
	if config.Checkbuild() {
		what |= RunBuildTests
	}
	if !config.SkipConfig() {
		what |= RunProductConfig
	} else {
		verboseln("Skipping Config as requested")
	}
	if !config.SkipSoong() {
		what |= RunSoong
	} else {
		verboseln("Skipping use of Soong as requested")
	}
	if !config.SkipKati() {
		what |= RunKati
	} else {
		verboseln("Skipping Kati as requested")
	}
	if !config.SkipKatiNinja() {
		what |= RunKatiNinja
	} else {
		verboseln("Skipping use of Kati ninja as requested")
	}
	if !config.SkipNinja() {
		what |= RunNinja
	} else {
		verboseln("Skipping Ninja as requested")
	}

	if !config.SoongBuildInvocationNeeded() {
		// This means that the output of soong_build is not needed and thus it would
		// run unnecessarily. In addition, if this code wasn't there invocations
		// with only special-cased target names like "m bp2build" would result in
		// passing Ninja the empty target list and it would then build the default
		// targets which is not what the user asked for.
		what = what &^ RunNinja
		what = what &^ RunKati
	}

	if config.Dist() {
		what |= RunDistActions
	}

	return what
}

var distWaitGroup sync.WaitGroup

// waitForDist waits for all backgrounded distGzipFile and distFile writes to finish
func waitForDist(ctx Context) {
	ctx.BeginTrace("soong_ui", "dist")
	defer ctx.EndTrace()

	distWaitGroup.Wait()
}

// distGzipFile writes a compressed copy of src to the distDir if dist is enabled.  Failures
// are printed but non-fatal. Uses the distWaitGroup func for backgrounding (optimization).
func distGzipFile(ctx Context, config Config, src string, subDirs ...string) {
	if !config.Dist() {
		return
	}

	subDir := filepath.Join(subDirs...)
	destDir := filepath.Join(config.RealDistDir(), "soong_ui", subDir)

	if err := os.MkdirAll(destDir, 0777); err != nil { // a+rwx
		ctx.Printf("failed to mkdir %s: %s", destDir, err.Error())
	}

	distWaitGroup.Add(1)
	go func() {
		defer distWaitGroup.Done()
		if err := gzipFileToDir(src, destDir); err != nil {
			ctx.Printf("failed to dist %s: %s", filepath.Base(src), err.Error())
		}
	}()
}

// distFile writes a copy of src to the distDir if dist is enabled.  Failures are printed but
// non-fatal. Uses the distWaitGroup func for backgrounding (optimization).
func distFile(ctx Context, config Config, src string, subDirs ...string) {
	if !config.Dist() {
		return
	}

	subDir := filepath.Join(subDirs...)
	destDir := filepath.Join(config.RealDistDir(), "soong_ui", subDir)

	if err := os.MkdirAll(destDir, 0777); err != nil { // a+rwx
		ctx.Printf("failed to mkdir %s: %s", destDir, err.Error())
	}

	distWaitGroup.Add(1)
	go func() {
		defer distWaitGroup.Done()
		if _, err := copyFile(src, filepath.Join(destDir, filepath.Base(src))); err != nil {
			ctx.Printf("failed to dist %s: %s", filepath.Base(src), err.Error())
		}
	}()
}

// Actions to run on every build where 'dist' is in the actions.
// Be careful, anything added here slows down EVERY CI build
func runDistActions(ctx Context, config Config) {
	runStagingSnapshot(ctx, config)
}
