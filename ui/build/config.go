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
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"android/soong/shared"

	"github.com/golang/protobuf/proto"

	smpb "android/soong/ui/metrics/metrics_proto"
)

type Config struct{ *configImpl }

type configImpl struct {
	// From the environment
	arguments     []string
	goma          bool
	environ       *Environment
	distDir       string
	buildDateTime string

	// From the arguments
	parallel   int
	keepGoing  int
	verbose    bool
	checkbuild bool
	dist       bool
	skipMake   bool

	// From the product config
	katiArgs        []string
	ninjaArgs       []string
	katiSuffix      string
	targetDevice    string
	targetDeviceDir string

	// Autodetected
	totalRAM uint64

	pdkBuild bool

	brokenDupRules     bool
	brokenUsesNetwork  bool
	brokenNinjaEnvVars []string

	pathReplaced bool
}

const srcDirFileCheck = "build/soong/root.bp"

var buildFiles = []string{"Android.mk", "Android.bp"}

type BuildAction uint

const (
	// Builds all of the modules and their dependencies of a specified directory, relative to the root
	// directory of the source tree.
	BUILD_MODULES_IN_A_DIRECTORY BuildAction = iota

	// Builds all of the modules and their dependencies of a list of specified directories. All specified
	// directories are relative to the root directory of the source tree.
	BUILD_MODULES_IN_DIRECTORIES

	// Build a list of specified modules. If none was specified, simply build the whole source tree.
	BUILD_MODULES
)

// checkTopDir validates that the current directory is at the root directory of the source tree.
func checkTopDir(ctx Context) {
	if _, err := os.Stat(srcDirFileCheck); err != nil {
		if os.IsNotExist(err) {
			ctx.Fatalf("Current working directory must be the source tree. %q not found.", srcDirFileCheck)
		}
		ctx.Fatalln("Error verifying tree state:", err)
	}
}

func NewConfig(ctx Context, args ...string) Config {
	ret := &configImpl{
		environ: OsEnvironment(),
	}

	// Sane default matching ninja
	ret.parallel = runtime.NumCPU() + 2
	ret.keepGoing = 1

	ret.totalRAM = detectTotalRAM(ctx)

	ret.parseArgs(ctx, args)

	// Make sure OUT_DIR is set appropriately
	if outDir, ok := ret.environ.Get("OUT_DIR"); ok {
		ret.environ.Set("OUT_DIR", filepath.Clean(outDir))
	} else {
		outDir := "out"
		if baseDir, ok := ret.environ.Get("OUT_DIR_COMMON_BASE"); ok {
			if wd, err := os.Getwd(); err != nil {
				ctx.Fatalln("Failed to get working directory:", err)
			} else {
				outDir = filepath.Join(baseDir, filepath.Base(wd))
			}
		}
		ret.environ.Set("OUT_DIR", outDir)
	}

	if distDir, ok := ret.environ.Get("DIST_DIR"); ok {
		ret.distDir = filepath.Clean(distDir)
	} else {
		ret.distDir = filepath.Join(ret.OutDir(), "dist")
	}

	ret.environ.Unset(
		// We're already using it
		"USE_SOONG_UI",

		// We should never use GOROOT/GOPATH from the shell environment
		"GOROOT",
		"GOPATH",

		// These should only come from Soong, not the environment.
		"CLANG",
		"CLANG_CXX",
		"CCC_CC",
		"CCC_CXX",

		// Used by the goma compiler wrapper, but should only be set by
		// gomacc
		"GOMACC_PATH",

		// We handle this above
		"OUT_DIR_COMMON_BASE",

		// This is handled above too, and set for individual commands later
		"DIST_DIR",

		// Variables that have caused problems in the past
		"BASH_ENV",
		"CDPATH",
		"DISPLAY",
		"GREP_OPTIONS",
		"NDK_ROOT",
		"POSIXLY_CORRECT",

		// Drop make flags
		"MAKEFLAGS",
		"MAKELEVEL",
		"MFLAGS",

		// Set in envsetup.sh, reset in makefiles
		"ANDROID_JAVA_TOOLCHAIN",

		// Set by envsetup.sh, but shouldn't be used inside the build because envsetup.sh is optional
		"ANDROID_BUILD_TOP",
		"ANDROID_HOST_OUT",
		"ANDROID_PRODUCT_OUT",
		"ANDROID_HOST_OUT_TESTCASES",
		"ANDROID_TARGET_OUT_TESTCASES",
		"ANDROID_TOOLCHAIN",
		"ANDROID_TOOLCHAIN_2ND_ARCH",
		"ANDROID_DEV_SCRIPTS",
		"ANDROID_EMULATOR_PREBUILTS",
		"ANDROID_PRE_BUILD_PATHS",

		// Only set in multiproduct_kati after config generation
		"EMPTY_NINJA_FILE",
	)

	if ret.UseGoma() || ret.ForceUseGoma() {
		ctx.Println("Goma for Android has been deprecated and replaced with RBE. See go/rbe_for_android for instructions on how to use RBE.")
		ctx.Fatalln("USE_GOMA / FORCE_USE_GOMA flag is no longer supported.")
	}

	// Tell python not to spam the source tree with .pyc files.
	ret.environ.Set("PYTHONDONTWRITEBYTECODE", "1")

	tmpDir := absPath(ctx, ret.TempDir())
	ret.environ.Set("TMPDIR", tmpDir)

	// Always set ASAN_SYMBOLIZER_PATH so that ASAN-based tools can symbolize any crashes
	symbolizerPath := filepath.Join("prebuilts/clang/host", ret.HostPrebuiltTag(),
		"llvm-binutils-stable/llvm-symbolizer")
	ret.environ.Set("ASAN_SYMBOLIZER_PATH", absPath(ctx, symbolizerPath))

	// Precondition: the current directory is the top of the source tree
	checkTopDir(ctx)

	if srcDir := absPath(ctx, "."); strings.ContainsRune(srcDir, ' ') {
		ctx.Println("You are building in a directory whose absolute path contains a space character:")
		ctx.Println()
		ctx.Printf("%q\n", srcDir)
		ctx.Println()
		ctx.Fatalln("Directory names containing spaces are not supported")
	}

	if outDir := ret.OutDir(); strings.ContainsRune(outDir, ' ') {
		ctx.Println("The absolute path of your output directory ($OUT_DIR) contains a space character:")
		ctx.Println()
		ctx.Printf("%q\n", outDir)
		ctx.Println()
		ctx.Fatalln("Directory names containing spaces are not supported")
	}

	if distDir := ret.DistDir(); strings.ContainsRune(distDir, ' ') {
		ctx.Println("The absolute path of your dist directory ($DIST_DIR) contains a space character:")
		ctx.Println()
		ctx.Printf("%q\n", distDir)
		ctx.Println()
		ctx.Fatalln("Directory names containing spaces are not supported")
	}

	// Configure Java-related variables, including adding it to $PATH
	java8Home := filepath.Join("prebuilts/jdk/jdk8", ret.HostPrebuiltTag())
	java9Home := filepath.Join("prebuilts/jdk/jdk9", ret.HostPrebuiltTag())
	java11Home := filepath.Join("prebuilts/jdk/jdk11", ret.HostPrebuiltTag())
	javaHome := func() string {
		if override, ok := ret.environ.Get("OVERRIDE_ANDROID_JAVA_HOME"); ok {
			return override
		}
		if toolchain11, ok := ret.environ.Get("EXPERIMENTAL_USE_OPENJDK11_TOOLCHAIN"); ok && toolchain11 != "true" {
			ctx.Fatalln("The environment variable EXPERIMENTAL_USE_OPENJDK11_TOOLCHAIN is no longer supported. An OpenJDK 11 toolchain is now the global default.")
		}
		return java11Home
	}()
	absJavaHome := absPath(ctx, javaHome)

	ret.configureLocale(ctx)

	newPath := []string{filepath.Join(absJavaHome, "bin")}
	if path, ok := ret.environ.Get("PATH"); ok && path != "" {
		newPath = append(newPath, path)
	}

	ret.environ.Unset("OVERRIDE_ANDROID_JAVA_HOME")
	ret.environ.Set("JAVA_HOME", absJavaHome)
	ret.environ.Set("ANDROID_JAVA_HOME", javaHome)
	ret.environ.Set("ANDROID_JAVA8_HOME", java8Home)
	ret.environ.Set("ANDROID_JAVA9_HOME", java9Home)
	ret.environ.Set("ANDROID_JAVA11_HOME", java11Home)
	ret.environ.Set("PATH", strings.Join(newPath, string(filepath.ListSeparator)))

	outDir := ret.OutDir()
	buildDateTimeFile := filepath.Join(outDir, "build_date.txt")
	if buildDateTime, ok := ret.environ.Get("BUILD_DATETIME"); ok && buildDateTime != "" {
		ret.buildDateTime = buildDateTime
	} else {
		ret.buildDateTime = strconv.FormatInt(time.Now().Unix(), 10)
	}

	ret.environ.Set("BUILD_DATETIME_FILE", buildDateTimeFile)

	if ret.UseRBE() {
		for k, v := range getRBEVars(ctx, Config{ret}) {
			ret.environ.Set(k, v)
		}
	}

	c := Config{ret}
	storeConfigMetrics(ctx, c)
	return c
}

// NewBuildActionConfig returns a build configuration based on the build action. The arguments are
// processed based on the build action and extracts any arguments that belongs to the build action.
func NewBuildActionConfig(action BuildAction, dir string, ctx Context, args ...string) Config {
	return NewConfig(ctx, getConfigArgs(action, dir, ctx, args)...)
}

// storeConfigMetrics selects a set of configuration information and store in
// the metrics system for further analysis.
func storeConfigMetrics(ctx Context, config Config) {
	if ctx.Metrics == nil {
		return
	}

	b := &smpb.BuildConfig{
		ForceUseGoma: proto.Bool(config.ForceUseGoma()),
		UseGoma:      proto.Bool(config.UseGoma()),
		UseRbe:       proto.Bool(config.UseRBE()),
	}
	ctx.Metrics.BuildConfig(b)
}

// getConfigArgs processes the command arguments based on the build action and creates a set of new
// arguments to be accepted by Config.
func getConfigArgs(action BuildAction, dir string, ctx Context, args []string) []string {
	// The next block of code verifies that the current directory is the root directory of the source
	// tree. It then finds the relative path of dir based on the root directory of the source tree
	// and verify that dir is inside of the source tree.
	checkTopDir(ctx)
	topDir, err := os.Getwd()
	if err != nil {
		ctx.Fatalf("Error retrieving top directory: %v", err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		ctx.Fatalf("Unable to evaluate symlink of %s: %v", dir, err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		ctx.Fatalf("Unable to find absolute path %s: %v", dir, err)
	}
	relDir, err := filepath.Rel(topDir, dir)
	if err != nil {
		ctx.Fatalf("Unable to find relative path %s of %s: %v", relDir, topDir, err)
	}
	// If there are ".." in the path, it's not in the source tree.
	if strings.Contains(relDir, "..") {
		ctx.Fatalf("Directory %s is not under the source tree %s", dir, topDir)
	}

	configArgs := args[:]

	// If the arguments contains GET-INSTALL-PATH, change the target name prefix from MODULES-IN- to
	// GET-INSTALL-PATH-IN- to extract the installation path instead of building the modules.
	targetNamePrefix := "MODULES-IN-"
	if inList("GET-INSTALL-PATH", configArgs) {
		targetNamePrefix = "GET-INSTALL-PATH-IN-"
		configArgs = removeFromList("GET-INSTALL-PATH", configArgs)
	}

	var targets []string

	switch action {
	case BUILD_MODULES:
		// No additional processing is required when building a list of specific modules or all modules.
	case BUILD_MODULES_IN_A_DIRECTORY:
		// If dir is the root source tree, all the modules are built of the source tree are built so
		// no need to find the build file.
		if topDir == dir {
			break
		}

		buildFile := findBuildFile(ctx, relDir)
		if buildFile == "" {
			ctx.Fatalf("Build file not found for %s directory", relDir)
		}
		targets = []string{convertToTarget(filepath.Dir(buildFile), targetNamePrefix)}
	case BUILD_MODULES_IN_DIRECTORIES:
		newConfigArgs, dirs := splitArgs(configArgs)
		configArgs = newConfigArgs
		targets = getTargetsFromDirs(ctx, relDir, dirs, targetNamePrefix)
	}

	// Tidy only override all other specified targets.
	tidyOnly := os.Getenv("WITH_TIDY_ONLY")
	if tidyOnly == "true" || tidyOnly == "1" {
		configArgs = append(configArgs, "tidy_only")
	} else {
		configArgs = append(configArgs, targets...)
	}

	return configArgs
}

// convertToTarget replaces "/" to "-" in dir and pre-append the targetNamePrefix to the target name.
func convertToTarget(dir string, targetNamePrefix string) string {
	return targetNamePrefix + strings.ReplaceAll(dir, "/", "-")
}

// hasBuildFile returns true if dir contains an Android build file.
func hasBuildFile(ctx Context, dir string) bool {
	for _, buildFile := range buildFiles {
		_, err := os.Stat(filepath.Join(dir, buildFile))
		if err == nil {
			return true
		}
		if !os.IsNotExist(err) {
			ctx.Fatalf("Error retrieving the build file stats: %v", err)
		}
	}
	return false
}

// findBuildFile finds a build file (makefile or blueprint file) by looking if there is a build file
// in the current and any sub directory of dir. If a build file is not found, traverse the path
// up by one directory and repeat again until either a build file is found or reached to the root
// source tree. The returned filename of build file is "Android.mk". If one was not found, a blank
// string is returned.
func findBuildFile(ctx Context, dir string) string {
	// If the string is empty or ".", assume it is top directory of the source tree.
	if dir == "" || dir == "." {
		return ""
	}

	found := false
	for buildDir := dir; buildDir != "."; buildDir = filepath.Dir(buildDir) {
		err := filepath.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if found {
				return filepath.SkipDir
			}
			if info.IsDir() {
				return nil
			}
			for _, buildFile := range buildFiles {
				if info.Name() == buildFile {
					found = true
					return filepath.SkipDir
				}
			}
			return nil
		})
		if err != nil {
			ctx.Fatalf("Error finding Android build file: %v", err)
		}

		if found {
			return filepath.Join(buildDir, "Android.mk")
		}
	}

	return ""
}

// splitArgs iterates over the arguments list and splits into two lists: arguments and directories.
func splitArgs(args []string) (newArgs []string, dirs []string) {
	specialArgs := map[string]bool{
		"showcommands": true,
		"snod":         true,
		"dist":         true,
		"checkbuild":   true,
	}

	newArgs = []string{}
	dirs = []string{}

	for _, arg := range args {
		// It's a dash argument if it starts with "-" or it's a key=value pair, it's not a directory.
		if strings.IndexRune(arg, '-') == 0 || strings.IndexRune(arg, '=') != -1 {
			newArgs = append(newArgs, arg)
			continue
		}

		if _, ok := specialArgs[arg]; ok {
			newArgs = append(newArgs, arg)
			continue
		}

		dirs = append(dirs, arg)
	}

	return newArgs, dirs
}

// getTargetsFromDirs iterates over the dirs list and creates a list of targets to build. If a
// directory from the dirs list does not exist, a fatal error is raised. relDir is related to the
// source root tree where the build action command was invoked. Each directory is validated if the
// build file can be found and follows the format "dir1:target1,target2,...". Target is optional.
func getTargetsFromDirs(ctx Context, relDir string, dirs []string, targetNamePrefix string) (targets []string) {
	for _, dir := range dirs {
		// The directory may have specified specific modules to build. ":" is the separator to separate
		// the directory and the list of modules.
		s := strings.Split(dir, ":")
		l := len(s)
		if l > 2 { // more than one ":" was specified.
			ctx.Fatalf("%s not in proper directory:target1,target2,... format (\":\" was specified more than once)", dir)
		}

		dir = filepath.Join(relDir, s[0])
		if _, err := os.Stat(dir); err != nil {
			ctx.Fatalf("couldn't find directory %s", dir)
		}

		// Verify that if there are any targets specified after ":". Each target is separated by ",".
		var newTargets []string
		if l == 2 && s[1] != "" {
			newTargets = strings.Split(s[1], ",")
			if inList("", newTargets) {
				ctx.Fatalf("%s not in proper directory:target1,target2,... format", dir)
			}
		}

		// If there are specified targets to build in dir, an android build file must exist for the one
		// shot build. For the non-targets case, find the appropriate build file and build all the
		// modules in dir (or the closest one in the dir path).
		if len(newTargets) > 0 {
			if !hasBuildFile(ctx, dir) {
				ctx.Fatalf("Couldn't locate a build file from %s directory", dir)
			}
		} else {
			buildFile := findBuildFile(ctx, dir)
			if buildFile == "" {
				ctx.Fatalf("Build file not found for %s directory", dir)
			}
			newTargets = []string{convertToTarget(filepath.Dir(buildFile), targetNamePrefix)}
		}

		targets = append(targets, newTargets...)
	}

	return targets
}

func (c *configImpl) parseArgs(ctx Context, args []string) {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "--make-mode" {
		} else if arg == "showcommands" {
			c.verbose = true
		} else if arg == "--skip-make" {
			c.skipMake = true
		} else if len(arg) > 0 && arg[0] == '-' {
			parseArgNum := func(def int) int {
				if len(arg) > 2 {
					p, err := strconv.ParseUint(arg[2:], 10, 31)
					if err != nil {
						ctx.Fatalf("Failed to parse %q: %v", arg, err)
					}
					return int(p)
				} else if i+1 < len(args) {
					p, err := strconv.ParseUint(args[i+1], 10, 31)
					if err == nil {
						i++
						return int(p)
					}
				}
				return def
			}

			if len(arg) > 1 && arg[1] == 'j' {
				c.parallel = parseArgNum(c.parallel)
			} else if len(arg) > 1 && arg[1] == 'k' {
				c.keepGoing = parseArgNum(0)
			} else {
				ctx.Fatalln("Unknown option:", arg)
			}
		} else if k, v, ok := decodeKeyValue(arg); ok && len(k) > 0 {
			c.environ.Set(k, v)
		} else if arg == "dist" {
			c.dist = true
		} else {
			if arg == "checkbuild" {
				c.checkbuild = true
			}
			c.arguments = append(c.arguments, arg)
		}
	}
}

func (c *configImpl) configureLocale(ctx Context) {
	cmd := Command(ctx, Config{c}, "locale", "locale", "-a")
	output, err := cmd.Output()

	var locales []string
	if err == nil {
		locales = strings.Split(string(output), "\n")
	} else {
		// If we're unable to list the locales, let's assume en_US.UTF-8
		locales = []string{"en_US.UTF-8"}
		ctx.Verbosef("Failed to list locales (%q), falling back to %q", err, locales)
	}

	// gettext uses LANGUAGE, which is passed directly through

	// For LANG and LC_*, only preserve the evaluated version of
	// LC_MESSAGES
	user_lang := ""
	if lc_all, ok := c.environ.Get("LC_ALL"); ok {
		user_lang = lc_all
	} else if lc_messages, ok := c.environ.Get("LC_MESSAGES"); ok {
		user_lang = lc_messages
	} else if lang, ok := c.environ.Get("LANG"); ok {
		user_lang = lang
	}

	c.environ.UnsetWithPrefix("LC_")

	if user_lang != "" {
		c.environ.Set("LC_MESSAGES", user_lang)
	}

	// The for LANG, use C.UTF-8 if it exists (Debian currently, proposed
	// for others)
	if inList("C.UTF-8", locales) {
		c.environ.Set("LANG", "C.UTF-8")
	} else if inList("C.utf8", locales) {
		// These normalize to the same thing
		c.environ.Set("LANG", "C.UTF-8")
	} else if inList("en_US.UTF-8", locales) {
		c.environ.Set("LANG", "en_US.UTF-8")
	} else if inList("en_US.utf8", locales) {
		// These normalize to the same thing
		c.environ.Set("LANG", "en_US.UTF-8")
	} else {
		ctx.Fatalln("System doesn't support either C.UTF-8 or en_US.UTF-8")
	}
}

// Lunch configures the environment for a specific product similarly to the
// `lunch` bash function.
func (c *configImpl) Lunch(ctx Context, product, variant string) {
	if variant != "eng" && variant != "userdebug" && variant != "user" {
		ctx.Fatalf("Invalid variant %q. Must be one of 'user', 'userdebug' or 'eng'", variant)
	}

	c.environ.Set("TARGET_PRODUCT", product)
	c.environ.Set("TARGET_BUILD_VARIANT", variant)
	c.environ.Set("TARGET_BUILD_TYPE", "release")
	c.environ.Unset("TARGET_BUILD_APPS")
}

// Tapas configures the environment to build one or more unbundled apps,
// similarly to the `tapas` bash function.
func (c *configImpl) Tapas(ctx Context, apps []string, arch, variant string) {
	if len(apps) == 0 {
		apps = []string{"all"}
	}
	if variant == "" {
		variant = "eng"
	}

	if variant != "eng" && variant != "userdebug" && variant != "user" {
		ctx.Fatalf("Invalid variant %q. Must be one of 'user', 'userdebug' or 'eng'", variant)
	}

	var product string
	switch arch {
	case "arm", "":
		product = "aosp_arm"
	case "arm64":
		product = "aosm_arm64"
	case "mips":
		product = "aosp_mips"
	case "mips64":
		product = "aosp_mips64"
	case "x86":
		product = "aosp_x86"
	case "x86_64":
		product = "aosp_x86_64"
	default:
		ctx.Fatalf("Invalid architecture: %q", arch)
	}

	c.environ.Set("TARGET_PRODUCT", product)
	c.environ.Set("TARGET_BUILD_VARIANT", variant)
	c.environ.Set("TARGET_BUILD_TYPE", "release")
	c.environ.Set("TARGET_BUILD_APPS", strings.Join(apps, " "))
}

func (c *configImpl) Environment() *Environment {
	return c.environ
}

func (c *configImpl) Arguments() []string {
	return c.arguments
}

func (c *configImpl) OutDir() string {
	if outDir, ok := c.environ.Get("OUT_DIR"); ok {
		return outDir
	}
	return "out"
}

func (c *configImpl) DistDir() string {
	return c.distDir
}

func (c *configImpl) NinjaArgs() []string {
	if c.skipMake {
		return c.arguments
	}
	return c.ninjaArgs
}

func (c *configImpl) SoongOutDir() string {
	return filepath.Join(c.OutDir(), "soong")
}

func (c *configImpl) TempDir() string {
	return shared.TempDirForOutDir(c.SoongOutDir())
}

func (c *configImpl) FileListDir() string {
	return filepath.Join(c.OutDir(), ".module_paths")
}

func (c *configImpl) KatiSuffix() string {
	if c.katiSuffix != "" {
		return c.katiSuffix
	}
	panic("SetKatiSuffix has not been called")
}

// Checkbuild returns true if "checkbuild" was one of the build goals, which means that the
// user is interested in additional checks at the expense of build time.
func (c *configImpl) Checkbuild() bool {
	return c.checkbuild
}

func (c *configImpl) Dist() bool {
	return c.dist
}

func (c *configImpl) IsVerbose() bool {
	return c.verbose
}

func (c *configImpl) SkipMake() bool {
	return c.skipMake
}

func (c *configImpl) TargetProduct() string {
	if v, ok := c.environ.Get("TARGET_PRODUCT"); ok {
		return v
	}
	panic("TARGET_PRODUCT is not defined")
}

func (c *configImpl) TargetDevice() string {
	return c.targetDevice
}

func (c *configImpl) SetTargetDevice(device string) {
	c.targetDevice = device
}

func (c *configImpl) TargetBuildVariant() string {
	if v, ok := c.environ.Get("TARGET_BUILD_VARIANT"); ok {
		return v
	}
	panic("TARGET_BUILD_VARIANT is not defined")
}

func (c *configImpl) KatiArgs() []string {
	return c.katiArgs
}

func (c *configImpl) Parallel() int {
	return c.parallel
}

func (c *configImpl) HighmemParallel() int {
	if i, ok := c.environ.GetInt("NINJA_HIGHMEM_NUM_JOBS"); ok {
		return i
	}

	const minMemPerHighmemProcess = 8 * 1024 * 1024 * 1024
	parallel := c.Parallel()
	if c.UseRemoteBuild() {
		// Ninja doesn't support nested pools, and when remote builds are enabled the total ninja parallelism
		// is set very high (i.e. 500).  Using a large value here would cause the total number of running jobs
		// to be the sum of the sizes of the local and highmem pools, which will cause extra CPU contention.
		// Return 1/16th of the size of the local pool, rounding up.
		return (parallel + 15) / 16
	} else if c.totalRAM == 0 {
		// Couldn't detect the total RAM, don't restrict highmem processes.
		return parallel
	} else if c.totalRAM <= 16*1024*1024*1024 {
		// Less than 16GB of ram, restrict to 1 highmem processes
		return 1
	} else if c.totalRAM <= 32*1024*1024*1024 {
		// Less than 32GB of ram, restrict to 2 highmem processes
		return 2
	} else if p := int(c.totalRAM / minMemPerHighmemProcess); p < parallel {
		// If less than 8GB total RAM per process, reduce the number of highmem processes
		return p
	}
	// No restriction on highmem processes
	return parallel
}

func (c *configImpl) TotalRAM() uint64 {
	return c.totalRAM
}

// ForceUseGoma determines whether we should override Goma deprecation
// and use Goma for the current build or not.
func (c *configImpl) ForceUseGoma() bool {
	if v, ok := c.environ.Get("FORCE_USE_GOMA"); ok {
		v = strings.TrimSpace(v)
		if v != "" && v != "false" {
			return true
		}
	}
	return false
}

func (c *configImpl) UseGoma() bool {
	if v, ok := c.environ.Get("USE_GOMA"); ok {
		v = strings.TrimSpace(v)
		if v != "" && v != "false" {
			return true
		}
	}
	return false
}

func (c *configImpl) StartGoma() bool {
	if !c.UseGoma() {
		return false
	}

	if v, ok := c.environ.Get("NOSTART_GOMA"); ok {
		v = strings.TrimSpace(v)
		if v != "" && v != "false" {
			return false
		}
	}
	return true
}

func (c *configImpl) UseRBE() bool {
	if v, ok := c.environ.Get("USE_RBE"); ok {
		v = strings.TrimSpace(v)
		if v != "" && v != "false" {
			return true
		}
	}
	return false
}

func (c *configImpl) StartRBE() bool {
	if !c.UseRBE() {
		return false
	}

	if v, ok := c.environ.Get("NOSTART_RBE"); ok {
		v = strings.TrimSpace(v)
		if v != "" && v != "false" {
			return false
		}
	}
	return true
}

func (c *configImpl) logDir() string {
	if c.Dist() {
		return filepath.Join(c.DistDir(), "logs")
	}
	return c.OutDir()
}

func (c *configImpl) rbeStatsOutputDir() string {
	for _, f := range []string{"RBE_output_dir", "FLAG_output_dir"} {
		if v, ok := c.environ.Get(f); ok {
			return v
		}
	}
	return c.logDir()
}

func (c *configImpl) rbeLogPath() string {
	for _, f := range []string{"RBE_log_path", "FLAG_log_path"} {
		if v, ok := c.environ.Get(f); ok {
			return v
		}
	}
	return fmt.Sprintf("text://%v/reproxy_log.txt", c.logDir())
}

func (c *configImpl) rbeExecRoot() string {
	for _, f := range []string{"RBE_exec_root", "FLAG_exec_root"} {
		if v, ok := c.environ.Get(f); ok {
			return v
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func (c *configImpl) rbeDir() string {
	if v, ok := c.environ.Get("RBE_DIR"); ok {
		return v
	}
	return "prebuilts/remoteexecution-client/live/"
}

func (c *configImpl) rbeReproxy() string {
	for _, f := range []string{"RBE_re_proxy", "FLAG_re_proxy"} {
		if v, ok := c.environ.Get(f); ok {
			return v
		}
	}
	return filepath.Join(c.rbeDir(), "reproxy")
}

func (c *configImpl) rbeAuth() (string, string) {
	credFlags := []string{"use_application_default_credentials", "use_gce_credentials", "credential_file"}
	for _, cf := range credFlags {
		for _, f := range []string{"RBE_" + cf, "FLAG_" + cf} {
			if v, ok := c.environ.Get(f); ok {
				v = strings.TrimSpace(v)
				if v != "" && v != "false" && v != "0" {
					return "RBE_" + cf, v
				}
			}
		}
	}
	return "RBE_use_application_default_credentials", "true"
}

func (c *configImpl) UseRemoteBuild() bool {
	return c.UseGoma() || c.UseRBE()
}

// RemoteParallel controls how many remote jobs (i.e., commands which contain
// gomacc) are run in parallel.  Note the parallelism of all other jobs is
// still limited by Parallel()
func (c *configImpl) RemoteParallel() int {
	if !c.UseRemoteBuild() {
		return 0
	}
	if i, ok := c.environ.GetInt("NINJA_REMOTE_NUM_JOBS"); ok {
		return i
	}
	return 500
}

func (c *configImpl) SetKatiArgs(args []string) {
	c.katiArgs = args
}

func (c *configImpl) SetNinjaArgs(args []string) {
	c.ninjaArgs = args
}

func (c *configImpl) SetKatiSuffix(suffix string) {
	c.katiSuffix = suffix
}

func (c *configImpl) LastKatiSuffixFile() string {
	return filepath.Join(c.OutDir(), "last_kati_suffix")
}

func (c *configImpl) HasKatiSuffix() bool {
	return c.katiSuffix != ""
}

func (c *configImpl) KatiEnvFile() string {
	return filepath.Join(c.OutDir(), "env"+c.KatiSuffix()+".sh")
}

func (c *configImpl) KatiBuildNinjaFile() string {
	return filepath.Join(c.OutDir(), "build"+c.KatiSuffix()+katiBuildSuffix+".ninja")
}

func (c *configImpl) KatiPackageNinjaFile() string {
	return filepath.Join(c.OutDir(), "build"+c.KatiSuffix()+katiPackageSuffix+".ninja")
}

func (c *configImpl) SoongNinjaFile() string {
	return filepath.Join(c.SoongOutDir(), "build.ninja")
}

func (c *configImpl) CombinedNinjaFile() string {
	if c.katiSuffix == "" {
		return filepath.Join(c.OutDir(), "combined.ninja")
	}
	return filepath.Join(c.OutDir(), "combined"+c.KatiSuffix()+".ninja")
}

func (c *configImpl) SoongAndroidMk() string {
	return filepath.Join(c.SoongOutDir(), "Android-"+c.TargetProduct()+".mk")
}

func (c *configImpl) SoongMakeVarsMk() string {
	return filepath.Join(c.SoongOutDir(), "make_vars-"+c.TargetProduct()+".mk")
}

func (c *configImpl) ProductOut() string {
	return filepath.Join(c.OutDir(), "target", "product", c.TargetDevice())
}

func (c *configImpl) DevicePreviousProductConfig() string {
	return filepath.Join(c.ProductOut(), "previous_build_config.mk")
}

func (c *configImpl) KatiPackageMkDir() string {
	return filepath.Join(c.ProductOut(), "obj", "CONFIG", "kati_packaging")
}

func (c *configImpl) hostOutRoot() string {
	return filepath.Join(c.OutDir(), "host")
}

func (c *configImpl) HostOut() string {
	return filepath.Join(c.hostOutRoot(), c.HostPrebuiltTag())
}

// This probably needs to be multi-valued, so not exporting it for now
func (c *configImpl) hostCrossOut() string {
	if runtime.GOOS == "linux" {
		return filepath.Join(c.hostOutRoot(), "windows-x86")
	} else {
		return ""
	}
}

func (c *configImpl) HostPrebuiltTag() string {
	if runtime.GOOS == "linux" {
		return "linux-x86"
	} else if runtime.GOOS == "darwin" {
		return "darwin-x86"
	} else {
		panic("Unsupported OS")
	}
}

func (c *configImpl) PrebuiltBuildTool(name string) string {
	if v, ok := c.environ.Get("SANITIZE_HOST"); ok {
		if sanitize := strings.Fields(v); inList("address", sanitize) {
			asan := filepath.Join("prebuilts/build-tools", c.HostPrebuiltTag(), "asan/bin", name)
			if _, err := os.Stat(asan); err == nil {
				return asan
			}
		}
	}
	return filepath.Join("prebuilts/build-tools", c.HostPrebuiltTag(), "bin", name)
}

func (c *configImpl) SetBuildBrokenDupRules(val bool) {
	c.brokenDupRules = val
}

func (c *configImpl) BuildBrokenDupRules() bool {
	return c.brokenDupRules
}

func (c *configImpl) SetBuildBrokenUsesNetwork(val bool) {
	c.brokenUsesNetwork = val
}

func (c *configImpl) BuildBrokenUsesNetwork() bool {
	return c.brokenUsesNetwork
}

func (c *configImpl) SetBuildBrokenNinjaUsesEnvVars(val []string) {
	c.brokenNinjaEnvVars = val
}

func (c *configImpl) BuildBrokenNinjaUsesEnvVars() []string {
	return c.brokenNinjaEnvVars
}

func (c *configImpl) SetTargetDeviceDir(dir string) {
	c.targetDeviceDir = dir
}

func (c *configImpl) TargetDeviceDir() string {
	return c.targetDeviceDir
}

func (c *configImpl) SetPdkBuild(pdk bool) {
	c.pdkBuild = pdk
}

func (c *configImpl) IsPdkBuild() bool {
	return c.pdkBuild
}

func (c *configImpl) BuildDateTime() string {
	return c.buildDateTime
}

func (c *configImpl) MetricsUploaderApp() string {
	if p, ok := c.environ.Get("ANDROID_ENABLE_METRICS_UPLOAD"); ok {
		return p
	}
	return ""
}
