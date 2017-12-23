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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"android/soong/finder"
	"android/soong/finder/fs"
)

var (
	// configuration of what to find
	excludeDirs     string
	filenamesToFind string
	pruneFiles      string

	// other configuration
	cpuprofile    string
	verbose       bool
	dbPath        string
	numIterations int
)

func init() {
	flag.StringVar(&cpuprofile, "cpuprofile", "",
		"filepath of profile file to write (optional)")
	flag.BoolVar(&verbose, "v", false, "log additional information")
	flag.StringVar(&dbPath, "db", "", "filepath of cache db")

	flag.StringVar(&excludeDirs, "exclude-dirs", "",
		"comma-separated list of directory names to exclude from search")
	flag.StringVar(&filenamesToFind, "names", "",
		"comma-separated list of filenames to find")
	flag.StringVar(&pruneFiles, "prune-files", "",
		"filenames that if discovered will exclude their entire directory "+
			"(including sibling files and directories)")
	flag.IntVar(&numIterations, "count", 1,
		"number of times to run. This is intended for use with --cpuprofile"+
			" , to increase profile accuracy")
}

var usage = func() {
	fmt.Printf("usage: finder -name <fileName> --db <dbPath> <searchDirectory> [<searchDirectory>...]\n")
	flag.PrintDefaults()
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err.Error())
		os.Exit(1)
	}
}

func stringToList(input string) []string {
	return strings.Split(input, ",")
}

func run() error {
	startTime := time.Now()
	flag.Parse()

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			return fmt.Errorf("Error opening cpuprofile: %s", err)
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	var writer io.Writer
	if verbose {
		writer = os.Stderr
	} else {
		writer = ioutil.Discard
	}

	// TODO: replace Lshortfile with Llongfile when bug 63821638 is done
	logger := log.New(writer, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

	logger.Printf("Finder starting at %v\n", startTime)

	rootPaths := flag.Args()
	if len(rootPaths) < 1 {
		usage()
		return fmt.Errorf(
			"Must give at least one <searchDirectory>")
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	params := finder.CacheParams{
		WorkingDirectory: workingDir,
		RootDirs:         rootPaths,
		ExcludeDirs:      stringToList(excludeDirs),
		PruneFiles:       stringToList(pruneFiles),
		IncludeFiles:     stringToList(filenamesToFind),
	}
	if dbPath == "" {
		usage()
		return errors.New("Param 'db' must be nonempty")
	}

	matches := []string{}
	for i := 0; i < numIterations; i++ {
		matches, err = runFind(params, logger)
		if err != nil {
			return err
		}
	}
	findDuration := time.Since(startTime)
	logger.Printf("Found these %v inodes in %v :\n", len(matches), findDuration)
	sort.Strings(matches)
	for _, match := range matches {
		fmt.Println(match)
	}
	logger.Printf("End of %v inodes\n", len(matches))
	logger.Printf("Finder completed in %v\n", time.Since(startTime))
	return nil
}

func runFind(params finder.CacheParams, logger *log.Logger) (paths []string, err error) {
	service, err := finder.New(params, fs.OsFs, logger, dbPath)
	if err != nil {
		return []string{}, err
	}
	defer service.Shutdown()
	return service.FindAll(), nil
}
