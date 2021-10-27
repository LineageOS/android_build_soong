// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The application to convert product configuration makefiles to Starlark.
// Converts either given list of files (and optionally the dependent files
// of the same kind), or all all product configuration makefiles in the
// given source tree.
// Previous version of a converted file can be backed up.
// Optionally prints detailed statistics at the end.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"android/soong/androidmk/parser"
	"android/soong/mk2rbc"
)

var (
	rootDir = flag.String("root", ".", "the value of // for load paths")
	// TODO(asmundak): remove this option once there is a consensus on suffix
	suffix   = flag.String("suffix", ".rbc", "generated files' suffix")
	dryRun   = flag.Bool("dry_run", false, "dry run")
	recurse  = flag.Bool("convert_dependents", false, "convert all dependent files")
	mode     = flag.String("mode", "", `"backup" to back up existing files, "write" to overwrite them`)
	warn     = flag.Bool("warnings", false, "warn about partially failed conversions")
	verbose  = flag.Bool("v", false, "print summary")
	errstat  = flag.Bool("error_stat", false, "print error statistics")
	traceVar = flag.String("trace", "", "comma-separated list of variables to trace")
	// TODO(asmundak): this option is for debugging
	allInSource           = flag.Bool("all", false, "convert all product config makefiles in the tree under //")
	outputTop             = flag.String("outdir", "", "write output files into this directory hierarchy")
	launcher              = flag.String("launcher", "", "generated launcher path. If set, the non-flag argument is _product_name_")
	printProductConfigMap = flag.Bool("print_product_config_map", false, "print product config map and exit")
	cpuProfile            = flag.String("cpu_profile", "", "write cpu profile to file")
	traceCalls            = flag.Bool("trace_calls", false, "trace function calls")
)

func init() {
	// Simplistic flag aliasing: works, but the usage string is ugly and
	// both flag and its alias can be present on the command line
	flagAlias := func(target string, alias string) {
		if f := flag.Lookup(target); f != nil {
			flag.Var(f.Value, alias, "alias for --"+f.Name)
			return
		}
		quit("cannot alias unknown flag " + target)
	}
	flagAlias("suffix", "s")
	flagAlias("root", "d")
	flagAlias("dry_run", "n")
	flagAlias("convert_dependents", "r")
	flagAlias("warnings", "w")
	flagAlias("error_stat", "e")
}

var backupSuffix string
var tracedVariables []string
var errorLogger = errorsByType{data: make(map[string]datum)}
var makefileFinder = &LinuxMakefileFinder{}
var versionDefaultsMk = filepath.Join("build", "make", "core", "version_defaults.mk")

func main() {
	flag.Usage = func() {
		cmd := filepath.Base(os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %[1]s flags file...\n"+
				"or:    %[1]s flags --launcher=PATH PRODUCT\n", cmd)
		flag.PrintDefaults()
	}
	flag.Parse()

	// Delouse
	if *suffix == ".mk" {
		quit("cannot use .mk as generated file suffix")
	}
	if *suffix == "" {
		quit("suffix cannot be empty")
	}
	if *outputTop != "" {
		if err := os.MkdirAll(*outputTop, os.ModeDir+os.ModePerm); err != nil {
			quit(err)
		}
		s, err := filepath.Abs(*outputTop)
		if err != nil {
			quit(err)
		}
		*outputTop = s
	}
	if *allInSource && len(flag.Args()) > 0 {
		quit("file list cannot be specified when -all is present")
	}
	if *allInSource && *launcher != "" {
		quit("--all and --launcher are mutually exclusive")
	}

	// Flag-driven adjustments
	if (*suffix)[0] != '.' {
		*suffix = "." + *suffix
	}
	if *mode == "backup" {
		backupSuffix = time.Now().Format("20060102150405")
	}
	if *traceVar != "" {
		tracedVariables = strings.Split(*traceVar, ",")
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			quit(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	// Find out global variables
	getConfigVariables()
	getSoongVariables()

	if *printProductConfigMap {
		productConfigMap := buildProductConfigMap()
		var products []string
		for p := range productConfigMap {
			products = append(products, p)
		}
		sort.Strings(products)
		for _, p := range products {
			fmt.Println(p, productConfigMap[p])
		}
		os.Exit(0)
	}

	// Convert!
	ok := true
	if *launcher != "" {
		if len(flag.Args()) != 1 {
			quit(fmt.Errorf("a launcher can be generated only for a single product"))
		}
		product := flag.Args()[0]
		productConfigMap := buildProductConfigMap()
		path, found := productConfigMap[product]
		if !found {
			quit(fmt.Errorf("cannot generate configuration launcher for %s, it is not a known product",
				product))
		}
		versionDefaults, err := generateVersionDefaults()
		if err != nil {
			quit(err)
		}
		ok = convertOne(path) && ok
		versionDefaultsPath := outputFilePath(versionDefaultsMk)
		err = writeGenerated(versionDefaultsPath, versionDefaults)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s:%s", path, err)
			ok = false
		}

		err = writeGenerated(*launcher, mk2rbc.Launcher(outputFilePath(path), versionDefaultsPath,
			mk2rbc.MakePath2ModuleName(path)))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s:%s", path, err)
			ok = false
		}
	} else {
		files := flag.Args()
		if *allInSource {
			productConfigMap := buildProductConfigMap()
			for _, path := range productConfigMap {
				files = append(files, path)
			}
		}
		for _, mkFile := range files {
			ok = convertOne(mkFile) && ok
		}
	}

	printStats()
	if *errstat {
		errorLogger.printStatistics()
	}
	if !ok {
		os.Exit(1)
	}
}

func generateVersionDefaults() (string, error) {
	versionSettings, err := mk2rbc.ParseVersionDefaults(filepath.Join(*rootDir, versionDefaultsMk))
	if err != nil {
		return "", err
	}
	return mk2rbc.VersionDefaults(versionSettings), nil

}

func quit(s interface{}) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(2)
}

func buildProductConfigMap() map[string]string {
	const androidProductsMk = "AndroidProducts.mk"
	// Build the list of AndroidProducts.mk files: it's
	// build/make/target/product/AndroidProducts.mk + device/**/AndroidProducts.mk plus + vendor/**/AndroidProducts.mk
	targetAndroidProductsFile := filepath.Join(*rootDir, "build", "make", "target", "product", androidProductsMk)
	if _, err := os.Stat(targetAndroidProductsFile); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n(hint: %s is not a source tree root)\n",
			targetAndroidProductsFile, err, *rootDir)
	}
	productConfigMap := make(map[string]string)
	if err := mk2rbc.UpdateProductConfigMap(productConfigMap, targetAndroidProductsFile); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", targetAndroidProductsFile, err)
	}
	for _, t := range []string{"device", "vendor"} {
		_ = filepath.WalkDir(filepath.Join(*rootDir, t),
			func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || filepath.Base(path) != androidProductsMk {
					return nil
				}
				if err2 := mk2rbc.UpdateProductConfigMap(productConfigMap, path); err2 != nil {
					fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
					// Keep going, we want to find all such errors in a single run
				}
				return nil
			})
	}
	return productConfigMap
}

func getConfigVariables() {
	path := filepath.Join(*rootDir, "build", "make", "core", "product.mk")
	if err := mk2rbc.FindConfigVariables(path, mk2rbc.KnownVariables); err != nil {
		quit(fmt.Errorf("%s\n(check --root[=%s], it should point to the source root)",
			err, *rootDir))
	}
}

// Implements mkparser.Scope, to be used by mkparser.Value.Value()
type fileNameScope struct {
	mk2rbc.ScopeBase
}

func (s fileNameScope) Get(name string) string {
	if name != "BUILD_SYSTEM" {
		return fmt.Sprintf("$(%s)", name)
	}
	return filepath.Join(*rootDir, "build", "make", "core")
}

func getSoongVariables() {
	path := filepath.Join(*rootDir, "build", "make", "core", "soong_config.mk")
	err := mk2rbc.FindSoongVariables(path, fileNameScope{}, mk2rbc.KnownVariables)
	if err != nil {
		quit(err)
	}
}

var converted = make(map[string]*mk2rbc.StarlarkScript)

//goland:noinspection RegExpRepeatedSpace
var cpNormalizer = regexp.MustCompile(
	"#  Copyright \\(C\\) 20.. The Android Open Source Project")

const cpNormalizedCopyright = "#  Copyright (C) 20xx The Android Open Source Project"
const copyright = `#
#  Copyright (C) 20xx The Android Open Source Project
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#       http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
`

// Convert a single file.
// Write the result either to the same directory, to the same place in
// the output hierarchy, or to the stdout.
// Optionally, recursively convert the files this one includes by
// $(call inherit-product) or an include statement.
func convertOne(mkFile string) (ok bool) {
	if v, ok := converted[mkFile]; ok {
		return v != nil
	}
	converted[mkFile] = nil
	defer func() {
		if r := recover(); r != nil {
			ok = false
			fmt.Fprintf(os.Stderr, "%s: panic while converting: %s\n%s\n", mkFile, r, debug.Stack())
		}
	}()

	mk2starRequest := mk2rbc.Request{
		MkFile:             mkFile,
		Reader:             nil,
		RootDir:            *rootDir,
		OutputDir:          *outputTop,
		OutputSuffix:       *suffix,
		TracedVariables:    tracedVariables,
		TraceCalls:         *traceCalls,
		WarnPartialSuccess: *warn,
		SourceFS:           os.DirFS(*rootDir),
		MakefileFinder:     makefileFinder,
	}
	if *errstat {
		mk2starRequest.ErrorLogger = errorLogger
	}
	ss, err := mk2rbc.Convert(mk2starRequest)
	if err != nil {
		fmt.Fprintln(os.Stderr, mkFile, ": ", err)
		return false
	}
	script := ss.String()
	outputPath := outputFilePath(mkFile)

	if *dryRun {
		fmt.Printf("==== %s ====\n", outputPath)
		// Print generated script after removing the copyright header
		outText := cpNormalizer.ReplaceAllString(script, cpNormalizedCopyright)
		fmt.Println(strings.TrimPrefix(outText, copyright))
	} else {
		if err := maybeBackup(outputPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return false
		}
		if err := writeGenerated(outputPath, script); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return false
		}
	}
	ok = true
	if *recurse {
		for _, sub := range ss.SubConfigFiles() {
			// File may be absent if it is a conditional load
			if _, err := os.Stat(sub); os.IsNotExist(err) {
				continue
			}
			ok = convertOne(sub) && ok
		}
	}
	converted[mkFile] = ss
	return ok
}

// Optionally saves the previous version of the generated file
func maybeBackup(filename string) error {
	stat, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return nil
	}
	if !stat.Mode().IsRegular() {
		return fmt.Errorf("%s exists and is not a regular file", filename)
	}
	switch *mode {
	case "backup":
		return os.Rename(filename, filename+backupSuffix)
	case "write":
		return os.Remove(filename)
	default:
		return fmt.Errorf("%s already exists, use --mode option", filename)
	}
}

func outputFilePath(mkFile string) string {
	path := strings.TrimSuffix(mkFile, filepath.Ext(mkFile)) + *suffix
	if *outputTop != "" {
		path = filepath.Join(*outputTop, path)
	}
	return path
}

func writeGenerated(path string, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), os.ModeDir|os.ModePerm); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path, []byte(contents), 0644); err != nil {
		return err
	}
	return nil
}

func printStats() {
	var sortedFiles []string
	if !*warn && !*verbose {
		return
	}
	for p := range converted {
		sortedFiles = append(sortedFiles, p)
	}
	sort.Strings(sortedFiles)

	nOk, nPartial, nFailed := 0, 0, 0
	for _, f := range sortedFiles {
		if converted[f] == nil {
			nFailed++
		} else if converted[f].HasErrors() {
			nPartial++
		} else {
			nOk++
		}
	}
	if *warn {
		if nPartial > 0 {
			fmt.Fprintf(os.Stderr, "Conversion was partially successful for:\n")
			for _, f := range sortedFiles {
				if ss := converted[f]; ss != nil && ss.HasErrors() {
					fmt.Fprintln(os.Stderr, "  ", f)
				}
			}
		}

		if nFailed > 0 {
			fmt.Fprintf(os.Stderr, "Conversion failed for files:\n")
			for _, f := range sortedFiles {
				if converted[f] == nil {
					fmt.Fprintln(os.Stderr, "  ", f)
				}
			}
		}
	}
	if *verbose {
		fmt.Fprintf(os.Stderr, "%-16s%5d\n", "Succeeded:", nOk)
		fmt.Fprintf(os.Stderr, "%-16s%5d\n", "Partial:", nPartial)
		fmt.Fprintf(os.Stderr, "%-16s%5d\n", "Failed:", nFailed)
	}
}

type datum struct {
	count          int
	formattingArgs []string
}

type errorsByType struct {
	data map[string]datum
}

func (ebt errorsByType) NewError(message string, node parser.Node, args ...interface{}) {
	v, exists := ebt.data[message]
	if exists {
		v.count++
	} else {
		v = datum{1, nil}
	}
	if strings.Contains(message, "%s") {
		var newArg1 string
		if len(args) == 0 {
			panic(fmt.Errorf(`%s has %%s but args are missing`, message))
		}
		newArg1 = fmt.Sprint(args[0])
		if message == "unsupported line" {
			newArg1 = node.Dump()
		} else if message == "unsupported directive %s" {
			if newArg1 == "include" || newArg1 == "-include" {
				newArg1 = node.Dump()
			}
		}
		v.formattingArgs = append(v.formattingArgs, newArg1)
	}
	ebt.data[message] = v
}

func (ebt errorsByType) printStatistics() {
	if len(ebt.data) > 0 {
		fmt.Fprintln(os.Stderr, "Error counts:")
	}
	for message, data := range ebt.data {
		if len(data.formattingArgs) == 0 {
			fmt.Fprintf(os.Stderr, "%4d %s\n", data.count, message)
			continue
		}
		itemsByFreq, count := stringsWithFreq(data.formattingArgs, 30)
		fmt.Fprintf(os.Stderr, "%4d %s [%d unique items]:\n", data.count, message, count)
		fmt.Fprintln(os.Stderr, "      ", itemsByFreq)
	}
}

func stringsWithFreq(items []string, topN int) (string, int) {
	freq := make(map[string]int)
	for _, item := range items {
		freq[strings.TrimPrefix(strings.TrimSuffix(item, "]"), "[")]++
	}
	var sorted []string
	for item := range freq {
		sorted = append(sorted, item)
	}
	sort.Slice(sorted, func(i int, j int) bool {
		return freq[sorted[i]] > freq[sorted[j]]
	})
	sep := ""
	res := ""
	for i, item := range sorted {
		if i >= topN {
			res += " ..."
			break
		}
		count := freq[item]
		if count > 1 {
			res += fmt.Sprintf("%s%s(%d)", sep, item, count)
		} else {
			res += fmt.Sprintf("%s%s", sep, item)
		}
		sep = ", "
	}
	return res, len(sorted)
}

type LinuxMakefileFinder struct {
	cachedRoot      string
	cachedMakefiles []string
}

func (l *LinuxMakefileFinder) Find(root string) []string {
	if l.cachedMakefiles != nil && l.cachedRoot == root {
		return l.cachedMakefiles
	}
	l.cachedRoot = root
	l.cachedMakefiles = make([]string, 0)

	// Return all *.mk files but not in hidden directories.

	// NOTE(asmundak): as it turns out, even the WalkDir (which is an _optimized_ directory tree walker)
	// is about twice slower than running `find` command (14s vs 6s on the internal Android source tree).
	common_args := []string{"!", "-type", "d", "-name", "*.mk", "!", "-path", "*/.*/*"}
	if root != "" {
		common_args = append([]string{root}, common_args...)
	}
	cmd := exec.Command("/usr/bin/find", common_args...)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		panic(fmt.Errorf("cannot get the output from %s: %s", cmd, err))
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		l.cachedMakefiles = append(l.cachedMakefiles, strings.TrimPrefix(scanner.Text(), "./"))
	}
	stdout.Close()
	return l.cachedMakefiles
}
