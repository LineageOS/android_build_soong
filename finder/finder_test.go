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

package finder

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sort"
	"testing"
	"time"

	"android/soong/finder/fs"
)

// some utils for tests to use
func newFs() *fs.MockFs {
	return fs.NewMockFs(map[string][]byte{})
}

func newFinder(t *testing.T, filesystem *fs.MockFs, cacheParams CacheParams) *Finder {
	return newFinderWithNumThreads(t, filesystem, cacheParams, 2)
}

func newFinderWithNumThreads(t *testing.T, filesystem *fs.MockFs, cacheParams CacheParams, numThreads int) *Finder {
	f, err := newFinderAndErr(t, filesystem, cacheParams, numThreads)
	if err != nil {
		fatal(t, err.Error())
	}
	return f
}

func newFinderAndErr(t *testing.T, filesystem *fs.MockFs, cacheParams CacheParams, numThreads int) (*Finder, error) {
	cachePath := "/finder/finder-db"
	cacheDir := filepath.Dir(cachePath)
	filesystem.MkDirs(cacheDir)
	if cacheParams.WorkingDirectory == "" {
		cacheParams.WorkingDirectory = "/cwd"
	}

	logger := log.New(ioutil.Discard, "", 0)
	f, err := newImpl(cacheParams, filesystem, logger, cachePath, numThreads)
	return f, err
}

func finderWithSameParams(t *testing.T, original *Finder) *Finder {
	f, err := finderAndErrorWithSameParams(t, original)
	if err != nil {
		fatal(t, err.Error())
	}
	return f
}

func finderAndErrorWithSameParams(t *testing.T, original *Finder) (*Finder, error) {
	f, err := newImpl(
		original.cacheMetadata.Config.CacheParams,
		original.filesystem,
		original.logger,
		original.DbPath,
		original.numDbLoadingThreads,
	)
	return f, err
}

func write(t *testing.T, path string, content string, filesystem *fs.MockFs) {
	parent := filepath.Dir(path)
	filesystem.MkDirs(parent)
	err := filesystem.WriteFile(path, []byte(content), 0777)
	if err != nil {
		fatal(t, err.Error())
	}
}

func create(t *testing.T, path string, filesystem *fs.MockFs) {
	write(t, path, "hi", filesystem)
}

func delete(t *testing.T, path string, filesystem *fs.MockFs) {
	err := filesystem.Remove(path)
	if err != nil {
		fatal(t, err.Error())
	}
}

func removeAll(t *testing.T, path string, filesystem *fs.MockFs) {
	err := filesystem.RemoveAll(path)
	if err != nil {
		fatal(t, err.Error())
	}
}

func move(t *testing.T, oldPath string, newPath string, filesystem *fs.MockFs) {
	err := filesystem.Rename(oldPath, newPath)
	if err != nil {
		fatal(t, err.Error())
	}
}

func link(t *testing.T, newPath string, oldPath string, filesystem *fs.MockFs) {
	parentPath := filepath.Dir(newPath)
	err := filesystem.MkDirs(parentPath)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = filesystem.Symlink(oldPath, newPath)
	if err != nil {
		fatal(t, err.Error())
	}
}
func read(t *testing.T, path string, filesystem *fs.MockFs) string {
	reader, err := filesystem.Open(path)
	if err != nil {
		t.Fatalf(err.Error())
	}
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err.Error())
	}
	return string(bytes)
}
func modTime(t *testing.T, path string, filesystem *fs.MockFs) time.Time {
	stats, err := filesystem.Lstat(path)
	if err != nil {
		t.Fatal(err.Error())
	}
	return stats.ModTime()
}
func setReadable(t *testing.T, path string, readable bool, filesystem *fs.MockFs) {
	err := filesystem.SetReadable(path, readable)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func setReadErr(t *testing.T, path string, readErr error, filesystem *fs.MockFs) {
	err := filesystem.SetReadErr(path, readErr)
	if err != nil {
		t.Fatal(err.Error())
	}
}

func fatal(t *testing.T, message string) {
	t.Error(message)
	debug.PrintStack()
	t.FailNow()
}

func assertSameResponse(t *testing.T, actual []string, expected []string) {
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		fatal(
			t,
			fmt.Sprintf(
				"Expected Finder to return these %v paths:\n  %v,\ninstead returned these %v paths:  %v\n",
				len(expected), expected, len(actual), actual),
		)
	}
}

func assertSameStatCalls(t *testing.T, actual []string, expected []string) {
	sort.Strings(actual)
	sort.Strings(expected)

	if !reflect.DeepEqual(actual, expected) {
		fatal(
			t,
			fmt.Sprintf(
				"Finder made incorrect Stat calls.\n"+
					"Actual:\n"+
					"%v\n"+
					"Expected:\n"+
					"%v\n"+
					"\n",
				actual, expected),
		)
	}
}
func assertSameReadDirCalls(t *testing.T, actual []string, expected []string) {
	sort.Strings(actual)
	sort.Strings(expected)

	if !reflect.DeepEqual(actual, expected) {
		fatal(
			t,
			fmt.Sprintf(
				"Finder made incorrect ReadDir calls.\n"+
					"Actual:\n"+
					"%v\n"+
					"Expected:\n"+
					"%v\n"+
					"\n",
				actual, expected),
		)
	}
}

// runSimpleTests creates a few files, searches for findme.txt, and checks for the expected matches
func runSimpleTest(t *testing.T, existentPaths []string, expectedMatches []string) {
	filesystem := newFs()
	root := "/tmp"
	filesystem.MkDirs(root)
	for _, path := range existentPaths {
		create(t, filepath.Join(root, path), filesystem)
	}

	finder := newFinder(t,
		filesystem,
		CacheParams{
			"/cwd",
			[]string{root},
			nil,
			nil,
			[]string{"findme.txt", "skipme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt(root, "findme.txt")
	absoluteMatches := []string{}
	for i := range expectedMatches {
		absoluteMatches = append(absoluteMatches, filepath.Join(root, expectedMatches[i]))
	}
	assertSameResponse(t, foundPaths, absoluteMatches)
}

// testAgainstSeveralThreadcounts runs the given test for each threadcount that we care to test
func testAgainstSeveralThreadcounts(t *testing.T, tester func(t *testing.T, numThreads int)) {
	// test singlethreaded, multithreaded, and also using the same number of threads as
	// will be used on the current system
	threadCounts := []int{1, 2, defaultNumThreads}
	for _, numThreads := range threadCounts {
		testName := fmt.Sprintf("%v threads", numThreads)
		// store numThreads in a new variable to prevent numThreads from changing in each loop
		localNumThreads := numThreads
		t.Run(testName, func(t *testing.T) {
			tester(t, localNumThreads)
		})
	}
}

// end of utils, start of individual tests

func TestSingleFile(t *testing.T) {
	runSimpleTest(t,
		[]string{"findme.txt"},
		[]string{"findme.txt"},
	)
}

func TestIncludeFiles(t *testing.T) {
	runSimpleTest(t,
		[]string{"findme.txt", "skipme.txt"},
		[]string{"findme.txt"},
	)
}

func TestNestedDirectories(t *testing.T) {
	runSimpleTest(t,
		[]string{"findme.txt", "skipme.txt", "subdir/findme.txt", "subdir/skipme.txt"},
		[]string{"findme.txt", "subdir/findme.txt"},
	)
}

func TestEmptyDirectory(t *testing.T) {
	runSimpleTest(t,
		[]string{},
		[]string{},
	)
}

func TestEmptyPath(t *testing.T) {
	filesystem := newFs()
	root := "/tmp"
	create(t, filepath.Join(root, "findme.txt"), filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{root},
			IncludeFiles: []string{"findme.txt", "skipme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("", "findme.txt")

	assertSameResponse(t, foundPaths, []string{})
}

func TestFilesystemRoot(t *testing.T) {

	testWithNumThreads := func(t *testing.T, numThreads int) {
		filesystem := newFs()
		root := "/"
		createdPath := "/findme.txt"
		create(t, createdPath, filesystem)

		finder := newFinderWithNumThreads(
			t,
			filesystem,
			CacheParams{
				RootDirs:     []string{root},
				IncludeFiles: []string{"findme.txt", "skipme.txt"},
			},
			numThreads,
		)
		defer finder.Shutdown()

		foundPaths := finder.FindNamedAt(root, "findme.txt")

		assertSameResponse(t, foundPaths, []string{createdPath})
	}

	testAgainstSeveralThreadcounts(t, testWithNumThreads)
}

func TestNonexistentDir(t *testing.T) {
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)

	_, err := newFinderAndErr(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp/IDontExist"},
			IncludeFiles: []string{"findme.txt", "skipme.txt"},
		},
		1,
	)
	if err == nil {
		fatal(t, "Did not fail when given a nonexistent root directory")
	}
}

func TestExcludeDirs(t *testing.T) {
	filesystem := newFs()
	create(t, "/tmp/exclude/findme.txt", filesystem)
	create(t, "/tmp/exclude/subdir/findme.txt", filesystem)
	create(t, "/tmp/subdir/exclude/findme.txt", filesystem)
	create(t, "/tmp/subdir/subdir/findme.txt", filesystem)
	create(t, "/tmp/subdir/findme.txt", filesystem)
	create(t, "/tmp/findme.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			ExcludeDirs:  []string{"exclude"},
			IncludeFiles: []string{"findme.txt", "skipme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/subdir/findme.txt",
			"/tmp/subdir/subdir/findme.txt"})
}

func TestPruneFiles(t *testing.T) {
	filesystem := newFs()
	create(t, "/tmp/out/findme.txt", filesystem)
	create(t, "/tmp/out/.ignore-out-dir", filesystem)
	create(t, "/tmp/out/child/findme.txt", filesystem)

	create(t, "/tmp/out2/.ignore-out-dir", filesystem)
	create(t, "/tmp/out2/sub/findme.txt", filesystem)

	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/include/findme.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			PruneFiles:   []string{".ignore-out-dir"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/include/findme.txt"})
}

// TestRootDir tests that the value of RootDirs is used
// tests of the filesystem root are in TestFilesystemRoot
func TestRootDir(t *testing.T) {
	filesystem := newFs()
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/subdir/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)
	create(t, "/tmp/b/subdir/findme.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp/a"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("/tmp/a", "findme.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt",
			"/tmp/a/subdir/findme.txt"})
}

func TestUncachedDir(t *testing.T) {
	filesystem := newFs()
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/subdir/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)
	create(t, "/tmp/b/subdir/findme.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp/b"},
			IncludeFiles: []string{"findme.txt"},
		},
	)

	foundPaths := finder.FindNamedAt("/tmp/a", "findme.txt")
	// If the caller queries for a file that is in the cache, then computing the
	// correct answer won't be fast, and it would be easy for the caller to
	// fail to notice its slowness. Instead, we only ever search the cache for files
	// to return, which enforces that we can determine which files will be
	// interesting upfront.
	assertSameResponse(t, foundPaths, []string{})

	finder.Shutdown()
}

func TestSearchingForFilesExcludedFromCache(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/misc.txt", filesystem)

	// set up the finder and run it
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "misc.txt")
	// If the caller queries for a file that is in the cache, then computing the
	// correct answer won't be fast, and it would be easy for the caller to
	// fail to notice its slowness. Instead, we only ever search the cache for files
	// to return, which enforces that we can determine which files will be
	// interesting upfront.
	assertSameResponse(t, foundPaths, []string{})

	finder.Shutdown()
}

func TestRelativeFilePaths(t *testing.T) {
	filesystem := newFs()

	create(t, "/tmp/ignore/hi.txt", filesystem)
	create(t, "/tmp/include/hi.txt", filesystem)
	create(t, "/cwd/hi.txt", filesystem)
	create(t, "/cwd/a/hi.txt", filesystem)
	create(t, "/cwd/a/a/hi.txt", filesystem)
	create(t, "/rel/a/hi.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/cwd", "../rel", "/tmp/include"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("a", "hi.txt")
	assertSameResponse(t, foundPaths,
		[]string{"a/hi.txt",
			"a/a/hi.txt"})

	foundPaths = finder.FindNamedAt("/tmp/include", "hi.txt")
	assertSameResponse(t, foundPaths, []string{"/tmp/include/hi.txt"})

	foundPaths = finder.FindNamedAt(".", "hi.txt")
	assertSameResponse(t, foundPaths,
		[]string{"hi.txt",
			"a/hi.txt",
			"a/a/hi.txt"})

	foundPaths = finder.FindNamedAt("/rel", "hi.txt")
	assertSameResponse(t, foundPaths,
		[]string{"/rel/a/hi.txt"})

	foundPaths = finder.FindNamedAt("/tmp/include", "hi.txt")
	assertSameResponse(t, foundPaths, []string{"/tmp/include/hi.txt"})
}

// have to run this test with the race-detector (`go test -race src/android/soong/finder/*.go`)
// for there to be much chance of the test actually detecting any error that may be present
func TestRootDirsContainedInOtherRootDirs(t *testing.T) {
	filesystem := newFs()

	create(t, "/tmp/a/b/c/d/e/f/g/h/i/j/findme.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/", "/tmp/a/b/c", "/tmp/a/b/c/d/e/f", "/tmp/a/b/c/d/e/f/g/h/i"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("/tmp/a", "findme.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/a/b/c/d/e/f/g/h/i/j/findme.txt"})
}

func TestFindFirst(t *testing.T) {
	filesystem := newFs()
	create(t, "/tmp/a/hi.txt", filesystem)
	create(t, "/tmp/b/hi.txt", filesystem)
	create(t, "/tmp/b/a/hi.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindFirstNamed("hi.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/a/hi.txt",
			"/tmp/b/hi.txt"},
	)
}

func TestConcurrentFindSameDirectory(t *testing.T) {

	testWithNumThreads := func(t *testing.T, numThreads int) {
		filesystem := newFs()

		// create a bunch of files and directories
		paths := []string{}
		for i := 0; i < 10; i++ {
			parentDir := fmt.Sprintf("/tmp/%v", i)
			for j := 0; j < 10; j++ {
				filePath := filepath.Join(parentDir, fmt.Sprintf("%v/findme.txt", j))
				paths = append(paths, filePath)
			}
		}
		sort.Strings(paths)
		for _, path := range paths {
			create(t, path, filesystem)
		}

		// set up a finder
		finder := newFinderWithNumThreads(
			t,
			filesystem,
			CacheParams{
				RootDirs:     []string{"/tmp"},
				IncludeFiles: []string{"findme.txt"},
			},
			numThreads,
		)
		defer finder.Shutdown()

		numTests := 20
		results := make(chan []string, numTests)
		// make several parallel calls to the finder
		for i := 0; i < numTests; i++ {
			go func() {
				foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
				results <- foundPaths
			}()
		}

		// check that each response was correct
		for i := 0; i < numTests; i++ {
			foundPaths := <-results
			assertSameResponse(t, foundPaths, paths)
		}
	}

	testAgainstSeveralThreadcounts(t, testWithNumThreads)
}

func TestConcurrentFindDifferentDirectories(t *testing.T) {
	filesystem := newFs()

	// create a bunch of files and directories
	allFiles := []string{}
	numSubdirs := 10
	rootPaths := []string{}
	queryAnswers := [][]string{}
	for i := 0; i < numSubdirs; i++ {
		parentDir := fmt.Sprintf("/tmp/%v", i)
		rootPaths = append(rootPaths, parentDir)
		queryAnswers = append(queryAnswers, []string{})
		for j := 0; j < 10; j++ {
			filePath := filepath.Join(parentDir, fmt.Sprintf("%v/findme.txt", j))
			queryAnswers[i] = append(queryAnswers[i], filePath)
			allFiles = append(allFiles, filePath)
		}
		sort.Strings(queryAnswers[i])
	}
	sort.Strings(allFiles)
	for _, path := range allFiles {
		create(t, path, filesystem)
	}

	// set up a finder
	finder := newFinder(
		t,
		filesystem,

		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	defer finder.Shutdown()

	type testRun struct {
		path           string
		foundMatches   []string
		correctMatches []string
	}

	numTests := numSubdirs + 1
	testRuns := make(chan testRun, numTests)

	searchAt := func(path string, correctMatches []string) {
		foundPaths := finder.FindNamedAt(path, "findme.txt")
		testRuns <- testRun{path, foundPaths, correctMatches}
	}

	// make several parallel calls to the finder
	go searchAt("/tmp", allFiles)
	for i := 0; i < len(rootPaths); i++ {
		go searchAt(rootPaths[i], queryAnswers[i])
	}

	// check that each response was correct
	for i := 0; i < numTests; i++ {
		testRun := <-testRuns
		assertSameResponse(t, testRun.foundMatches, testRun.correctMatches)
	}
}

func TestStrangelyFormattedPaths(t *testing.T) {
	filesystem := newFs()

	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"//tmp//a//.."},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("//tmp//a//..", "findme.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt",
			"/tmp/b/findme.txt",
			"/tmp/findme.txt"})
}

func TestCorruptedCacheHeader(t *testing.T) {
	filesystem := newFs()

	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	write(t, "/finder/finder-db", "sample header", filesystem)

	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")

	assertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt",
			"/tmp/findme.txt"})
}

func TestCanUseCache(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	// check the response of the first finder
	correctResponse := []string{"/tmp/a/findme.txt",
		"/tmp/findme.txt"}
	assertSameResponse(t, foundPaths, correctResponse)
	finder.Shutdown()

	// check results
	cacheText := read(t, finder.DbPath, filesystem)
	if len(cacheText) < 1 {
		t.Fatalf("saved cache db is empty\n")
	}
	if len(filesystem.StatCalls) == 0 {
		t.Fatal("No Stat calls recorded by mock filesystem")
	}
	if len(filesystem.ReadDirCalls) == 0 {
		t.Fatal("No ReadDir calls recorded by filesystem")
	}
	statCalls := filesystem.StatCalls
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")
	// check results
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})
	assertSameReadDirCalls(t, filesystem.StatCalls, statCalls)

	finder2.Shutdown()
}

func TestCorruptedCacheBody(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()

	// check the response of the first finder
	correctResponse := []string{"/tmp/a/findme.txt",
		"/tmp/findme.txt"}
	assertSameResponse(t, foundPaths, correctResponse)
	numStatCalls := len(filesystem.StatCalls)
	numReadDirCalls := len(filesystem.ReadDirCalls)

	// load the cache file, corrupt it, and save it
	cacheReader, err := filesystem.Open(finder.DbPath)
	if err != nil {
		t.Fatal(err)
	}
	cacheData, err := ioutil.ReadAll(cacheReader)
	if err != nil {
		t.Fatal(err)
	}
	cacheData = append(cacheData, []byte("DontMindMe")...)
	filesystem.WriteFile(finder.DbPath, cacheData, 0777)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")
	// check results
	assertSameResponse(t, foundPaths, correctResponse)
	numNewStatCalls := len(filesystem.StatCalls)
	numNewReadDirCalls := len(filesystem.ReadDirCalls)
	// It's permissable to make more Stat calls with a corrupted cache because
	// the Finder may restart once it detects corruption.
	// However, it may have already issued many Stat calls.
	// Because a corrupted db is not expected to be a common (or even a supported case),
	// we don't care to optimize it and don't cache the already-issued Stat calls
	if numNewReadDirCalls < numReadDirCalls {
		t.Fatalf(
			"Finder made fewer ReadDir calls with a corrupted cache (%v calls) than with no cache"+
				" (%v calls)",
			numNewReadDirCalls, numReadDirCalls)
	}
	if numNewStatCalls < numStatCalls {
		t.Fatalf(
			"Finder made fewer Stat calls with a corrupted cache (%v calls) than with no cache (%v calls)",
			numNewStatCalls, numStatCalls)
	}
	finder2.Shutdown()
}

func TestStatCalls(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/a/findme.txt", filesystem)

	// run finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()

	// check response
	assertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})
	assertSameStatCalls(t, filesystem.StatCalls, []string{"/tmp", "/tmp/a"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/a"})
}

func TestFileAdded(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/ignoreme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/b/ignore.txt", filesystem)
	create(t, "/tmp/b/c/nope.txt", filesystem)
	create(t, "/tmp/b/c/d/irrelevant.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	filesystem.Clock.Tick()
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	create(t, "/tmp/b/c/findme.txt", filesystem)
	filesystem.Clock.Tick()
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt", "/tmp/b/c/findme.txt"})
	assertSameStatCalls(t, filesystem.StatCalls, []string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/c", "/tmp/b/c/d"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b/c"})
	finder2.Shutdown()

}

func TestDirectoriesAdded(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/ignoreme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/b/ignore.txt", filesystem)
	create(t, "/tmp/b/c/nope.txt", filesystem)
	create(t, "/tmp/b/c/d/irrelevant.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	create(t, "/tmp/b/c/new/findme.txt", filesystem)
	create(t, "/tmp/b/c/new/new2/findme.txt", filesystem)
	create(t, "/tmp/b/c/new/new2/ignoreme.txt", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt", "/tmp/b/c/new/findme.txt", "/tmp/b/c/new/new2/findme.txt"})
	assertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/c", "/tmp/b/c/d", "/tmp/b/c/new", "/tmp/b/c/new/new2"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b/c", "/tmp/b/c/new", "/tmp/b/c/new/new2"})

	finder2.Shutdown()
}

func TestDirectoryAndSubdirectoryBothUpdated(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/hi1.txt", filesystem)
	create(t, "/tmp/a/hi1.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi1.txt", "hi2.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "hi1.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths, []string{"/tmp/hi1.txt", "/tmp/a/hi1.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	create(t, "/tmp/hi2.txt", filesystem)
	create(t, "/tmp/a/hi2.txt", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindAll()

	// check results
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/hi1.txt", "/tmp/hi2.txt", "/tmp/a/hi1.txt", "/tmp/a/hi2.txt"})
	assertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/a"})

	finder2.Shutdown()
}

func TestFileDeleted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/ignoreme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)
	create(t, "/tmp/b/c/nope.txt", filesystem)
	create(t, "/tmp/b/c/d/irrelevant.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt", "/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	delete(t, "/tmp/b/findme.txt", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})
	assertSameStatCalls(t, filesystem.StatCalls, []string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/c", "/tmp/b/c/d"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b"})

	finder2.Shutdown()
}

func TestDirectoriesDeleted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/1/findme.txt", filesystem)
	create(t, "/tmp/a/1/2/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/a/1/findme.txt",
			"/tmp/a/1/2/findme.txt",
			"/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	removeAll(t, "/tmp/a/1", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt", "/tmp/a/findme.txt", "/tmp/b/findme.txt"})
	// Technically, we don't care whether /tmp/a/1/2 gets Statted or gets skipped
	// if the Finder detects the nonexistence of /tmp/a/1
	// However, when resuming from cache, we don't want the Finder to necessarily wait
	// to stat a directory until after statting its parent.
	// So here we just include /tmp/a/1/2 in the list.
	// The Finder is currently implemented to always restat every dir and
	// to not short-circuit due to nonexistence of parents (but it will remove
	// missing dirs from the cache for next time)
	assertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/1", "/tmp/a/1/2", "/tmp/b"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/a"})

	finder2.Shutdown()
}

func TestDirectoriesMoved(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/1/findme.txt", filesystem)
	create(t, "/tmp/a/1/2/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/a/1/findme.txt",
			"/tmp/a/1/2/findme.txt",
			"/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	move(t, "/tmp/a", "/tmp/c", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/b/findme.txt",
			"/tmp/c/findme.txt",
			"/tmp/c/1/findme.txt",
			"/tmp/c/1/2/findme.txt"})
	// Technically, we don't care whether /tmp/a/1/2 gets Statted or gets skipped
	// if the Finder detects the nonexistence of /tmp/a/1
	// However, when resuming from cache, we don't want the Finder to necessarily wait
	// to stat a directory until after statting its parent.
	// So here we just include /tmp/a/1/2 in the list.
	// The Finder is currently implemented to always restat every dir and
	// to not short-circuit due to nonexistence of parents (but it will remove
	// missing dirs from the cache for next time)
	assertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/1", "/tmp/a/1/2", "/tmp/b", "/tmp/c", "/tmp/c/1", "/tmp/c/1/2"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/c", "/tmp/c/1", "/tmp/c/1/2"})
	finder2.Shutdown()
}

func TestDirectoriesSwapped(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/1/findme.txt", filesystem)
	create(t, "/tmp/a/1/2/findme.txt", filesystem)
	create(t, "/tmp/b/findme.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/a/1/findme.txt",
			"/tmp/a/1/2/findme.txt",
			"/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	move(t, "/tmp/a", "/tmp/temp", filesystem)
	move(t, "/tmp/b", "/tmp/a", filesystem)
	move(t, "/tmp/temp", "/tmp/b", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/b/findme.txt",
			"/tmp/b/1/findme.txt",
			"/tmp/b/1/2/findme.txt"})
	// Technically, we don't care whether /tmp/a/1/2 gets Statted or gets skipped
	// if the Finder detects the nonexistence of /tmp/a/1
	// However, when resuming from cache, we don't want the Finder to necessarily wait
	// to stat a directory until after statting its parent.
	// So here we just include /tmp/a/1/2 in the list.
	// The Finder is currently implemented to always restat every dir and
	// to not short-circuit due to nonexistence of parents (but it will remove
	// missing dirs from the cache for next time)
	assertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/1", "/tmp/a/1/2", "/tmp/b", "/tmp/b/1", "/tmp/b/1/2"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/1", "/tmp/b/1/2"})
	finder2.Shutdown()
}

// runFsReplacementTest tests a change modifying properties of the filesystem itself:
// runFsReplacementTest tests changing the user, the hostname, or the device number
// runFsReplacementTest is a helper method called by other tests
func runFsReplacementTest(t *testing.T, fs1 *fs.MockFs, fs2 *fs.MockFs) {
	// setup fs1
	create(t, "/tmp/findme.txt", fs1)
	create(t, "/tmp/a/findme.txt", fs1)
	create(t, "/tmp/a/a/findme.txt", fs1)

	// setup fs2 to have the same directories but different files
	create(t, "/tmp/findme.txt", fs2)
	create(t, "/tmp/a/findme.txt", fs2)
	create(t, "/tmp/a/a/ignoreme.txt", fs2)
	create(t, "/tmp/a/b/findme.txt", fs2)

	// run the first finder
	finder := newFinder(
		t,
		fs1,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt", "/tmp/a/findme.txt", "/tmp/a/a/findme.txt"})

	// copy the cache data from the first filesystem to the second
	cacheContent := read(t, finder.DbPath, fs1)
	write(t, finder.DbPath, cacheContent, fs2)

	// run the second finder, with the same config and same cache contents but a different filesystem
	finder2 := newFinder(
		t,
		fs2,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt", "/tmp/a/findme.txt", "/tmp/a/b/findme.txt"})
	assertSameStatCalls(t, fs2.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/a", "/tmp/a/b"})
	assertSameReadDirCalls(t, fs2.ReadDirCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/a", "/tmp/a/b"})
	finder2.Shutdown()
}

func TestChangeOfDevice(t *testing.T) {
	fs1 := newFs()
	// not as fine-grained mounting controls as a real filesystem, but should be adequate
	fs1.SetDeviceNumber(0)

	fs2 := newFs()
	fs2.SetDeviceNumber(1)

	runFsReplacementTest(t, fs1, fs2)
}

func TestChangeOfUserOrHost(t *testing.T) {
	fs1 := newFs()
	fs1.SetViewId("me@here")

	fs2 := newFs()
	fs2.SetViewId("you@there")

	runFsReplacementTest(t, fs1, fs2)
}

func TestConsistentCacheOrdering(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	for i := 0; i < 5; i++ {
		create(t, fmt.Sprintf("/tmp/%v/findme.txt", i), filesystem)
	}

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()

	// read db file
	string1 := read(t, finder.DbPath, filesystem)

	err := filesystem.Remove(finder.DbPath)
	if err != nil {
		t.Fatal(err)
	}

	// run another finder
	finder2 := finderWithSameParams(t, finder)
	finder2.FindNamedAt("/tmp", "findme.txt")
	finder2.Shutdown()

	string2 := read(t, finder.DbPath, filesystem)

	if string1 != string2 {
		t.Errorf("Running Finder twice generated two dbs not having identical contents.\n"+
			"Content of first file:\n"+
			"\n"+
			"%v"+
			"\n"+
			"\n"+
			"Content of second file:\n"+
			"\n"+
			"%v\n"+
			"\n",
			string1,
			string2,
		)
	}

}

func TestNumSyscallsOfSecondFind(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/misc.txt", filesystem)

	// set up the finder and run it once
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	assertSameResponse(t, foundPaths, []string{"/tmp/findme.txt", "/tmp/a/findme.txt"})

	filesystem.ClearMetrics()

	// run the finder again and confirm it doesn't check the filesystem
	refoundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	assertSameResponse(t, refoundPaths, foundPaths)
	assertSameStatCalls(t, filesystem.StatCalls, []string{})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})

	finder.Shutdown()
}

func TestChangingParamsOfSecondFind(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/findme.txt", filesystem)
	create(t, "/tmp/a/findme.txt", filesystem)
	create(t, "/tmp/a/metoo.txt", filesystem)

	// set up the finder and run it once
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt", "metoo.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	assertSameResponse(t, foundPaths, []string{"/tmp/findme.txt", "/tmp/a/findme.txt"})

	filesystem.ClearMetrics()

	// run the finder again and confirm it gets the right answer without asking the filesystem
	refoundPaths := finder.FindNamedAt("/tmp", "metoo.txt")
	assertSameResponse(t, refoundPaths, []string{"/tmp/a/metoo.txt"})
	assertSameStatCalls(t, filesystem.StatCalls, []string{})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})

	finder.Shutdown()
}

func TestSymlinkPointingToFile(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/a/hi.txt", filesystem)
	create(t, "/tmp/a/ignoreme.txt", filesystem)
	link(t, "/tmp/hi.txt", "a/hi.txt", filesystem)
	link(t, "/tmp/b/hi.txt", "../a/hi.txt", filesystem)
	link(t, "/tmp/c/hi.txt", "/tmp/hi.txt", filesystem)
	link(t, "/tmp/d/hi.txt", "../a/bye.txt", filesystem)
	link(t, "/tmp/d/bye.txt", "../a/hi.txt", filesystem)
	link(t, "/tmp/e/bye.txt", "../a/bye.txt", filesystem)
	link(t, "/tmp/f/hi.txt", "somethingThatDoesntExist", filesystem)

	// set up the finder and run it once
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	foundPaths := finder.FindNamedAt("/tmp", "hi.txt")
	// should search based on the name of the link rather than the destination or validity of the link
	correctResponse := []string{
		"/tmp/a/hi.txt",
		"/tmp/hi.txt",
		"/tmp/b/hi.txt",
		"/tmp/c/hi.txt",
		"/tmp/d/hi.txt",
		"/tmp/f/hi.txt",
	}
	assertSameResponse(t, foundPaths, correctResponse)

}

func TestSymlinkPointingToDirectory(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/dir/hi.txt", filesystem)
	create(t, "/tmp/dir/ignoreme.txt", filesystem)

	link(t, "/tmp/links/dir", "../dir", filesystem)
	link(t, "/tmp/links/link", "../dir", filesystem)
	link(t, "/tmp/links/broken", "nothingHere", filesystem)
	link(t, "/tmp/links/recursive", "recursive", filesystem)

	// set up the finder and run it once
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)

	foundPaths := finder.FindNamedAt("/tmp", "hi.txt")

	// should completely ignore symlinks that point to directories
	correctResponse := []string{
		"/tmp/dir/hi.txt",
	}
	assertSameResponse(t, foundPaths, correctResponse)

}

// TestAddPruneFile confirms that adding a prune-file (into a directory for which we
// already had a cache) causes the directory to be ignored
func TestAddPruneFile(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/out/hi.txt", filesystem)
	create(t, "/tmp/out/a/hi.txt", filesystem)
	create(t, "/tmp/hi.txt", filesystem)

	// do find
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			PruneFiles:   []string{".ignore-out-dir"},
			IncludeFiles: []string{"hi.txt"},
		},
	)

	foundPaths := finder.FindNamedAt("/tmp", "hi.txt")

	// check result
	assertSameResponse(t, foundPaths,
		[]string{"/tmp/hi.txt",
			"/tmp/out/hi.txt",
			"/tmp/out/a/hi.txt"},
	)
	finder.Shutdown()

	// modify filesystem
	filesystem.Clock.Tick()
	create(t, "/tmp/out/.ignore-out-dir", filesystem)
	// run another find and check its result
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "hi.txt")
	assertSameResponse(t, foundPaths, []string{"/tmp/hi.txt"})
	finder2.Shutdown()
}

func TestUpdatingDbIffChanged(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/a/hi.txt", filesystem)
	create(t, "/tmp/b/bye.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	create(t, "/tmp/b/hi.txt", filesystem)
	filesystem.Clock.Tick()
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindAll()
	finder2.Shutdown()
	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt", "/tmp/b/hi.txt"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b"})
	expectedDbWriteTime := filesystem.Clock.Time()
	actualDbWriteTime := modTime(t, finder2.DbPath, filesystem)
	if actualDbWriteTime != expectedDbWriteTime {
		t.Fatalf("Expected to write db at %v, actually wrote db at %v\n",
			expectedDbWriteTime, actualDbWriteTime)
	}

	// reset metrics
	filesystem.ClearMetrics()

	// run the third finder
	finder3 := finderWithSameParams(t, finder2)
	foundPaths = finder3.FindAll()

	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt", "/tmp/b/hi.txt"})
	assertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})
	finder3.Shutdown()
	actualDbWriteTime = modTime(t, finder3.DbPath, filesystem)
	if actualDbWriteTime != expectedDbWriteTime {
		t.Fatalf("Re-wrote db even when contents did not change")
	}

}

func TestDirectoryNotPermitted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/hi.txt", filesystem)
	create(t, "/tmp/a/hi.txt", filesystem)
	create(t, "/tmp/a/a/hi.txt", filesystem)
	create(t, "/tmp/b/hi.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	allPaths := []string{"/tmp/hi.txt", "/tmp/a/hi.txt", "/tmp/a/a/hi.txt", "/tmp/b/hi.txt"}
	// check results
	assertSameResponse(t, foundPaths, allPaths)

	// modify the filesystem
	filesystem.Clock.Tick()

	setReadable(t, "/tmp/a", false, filesystem)
	filesystem.Clock.Tick()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindAll()
	finder2.Shutdown()
	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/hi.txt", "/tmp/b/hi.txt"})

	// modify the filesystem back
	setReadable(t, "/tmp/a", true, filesystem)

	// run the third finder
	finder3 := finderWithSameParams(t, finder2)
	foundPaths = finder3.FindAll()
	finder3.Shutdown()
	// check results
	assertSameResponse(t, foundPaths, allPaths)
}

func TestFileNotPermitted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/hi.txt", filesystem)
	setReadable(t, "/tmp/hi.txt", false, filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/hi.txt"})
}

func TestCacheEntryPathUnexpectedError(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	create(t, "/tmp/a/hi.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	// check results
	assertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt"})

	// make the directory not readable
	setReadErr(t, "/tmp/a", os.ErrInvalid, filesystem)

	// run the second finder
	_, err := finderAndErrorWithSameParams(t, finder)
	if err == nil {
		fatal(t, "Failed to detect unexpected filesystem error")
	}
}
