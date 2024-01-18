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
	"sort"
	"strings"
	"testing"

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
		t.Fatal(err.Error())
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
		t.Fatal(err.Error())
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

// runSimpleTests creates a few files, searches for findme.txt, and checks for the expected matches
func runSimpleTest(t *testing.T, existentPaths []string, expectedMatches []string) {
	filesystem := newFs()
	root := "/tmp"
	filesystem.MkDirs(root)
	for _, path := range existentPaths {
		fs.Create(t, filepath.Join(root, path), filesystem)
	}

	finder := newFinder(t,
		filesystem,
		CacheParams{
			"/cwd",
			[]string{root},
			false,
			nil,
			nil,
			[]string{"findme.txt", "skipme.txt"},
			nil,
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindNamedAt(root, "findme.txt")
	absoluteMatches := []string{}
	for i := range expectedMatches {
		absoluteMatches = append(absoluteMatches, filepath.Join(root, expectedMatches[i]))
	}
	fs.AssertSameResponse(t, foundPaths, absoluteMatches)
}

// runTestWithSuffixes creates a few files, searches for findme.txt or any file
// with suffix `.findme_ext` and checks for the expected matches
func runTestWithSuffixes(t *testing.T, existentPaths []string, expectedMatches []string) {
	filesystem := newFs()
	root := "/tmp"
	filesystem.MkDirs(root)
	for _, path := range existentPaths {
		fs.Create(t, filepath.Join(root, path), filesystem)
	}

	finder := newFinder(t,
		filesystem,
		CacheParams{
			"/cwd",
			[]string{root},
			false,
			nil,
			nil,
			[]string{"findme.txt", "skipme.txt"},
			[]string{".findme_ext"},
		},
	)
	defer finder.Shutdown()

	foundPaths := finder.FindMatching(root,
		func(entries DirEntries) (dirs []string, files []string) {
			matches := []string{}
			for _, foundName := range entries.FileNames {
				if foundName == "findme.txt" || strings.HasSuffix(foundName, ".findme_ext") {
					matches = append(matches, foundName)
				}
			}
			return entries.DirNames, matches
		})
	absoluteMatches := []string{}
	for i := range expectedMatches {
		absoluteMatches = append(absoluteMatches, filepath.Join(root, expectedMatches[i]))
	}
	fs.AssertSameResponse(t, foundPaths, absoluteMatches)
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

func TestIncludeFilesAndSuffixes(t *testing.T) {
	runTestWithSuffixes(t,
		[]string{"findme.txt", "skipme.txt", "alsome.findme_ext"},
		[]string{"findme.txt", "alsome.findme_ext"},
	)
}

func TestNestedDirectories(t *testing.T) {
	runSimpleTest(t,
		[]string{"findme.txt", "skipme.txt", "subdir/findme.txt", "subdir/skipme.txt"},
		[]string{"findme.txt", "subdir/findme.txt"},
	)
}

func TestNestedDirectoriesWithSuffixes(t *testing.T) {
	runTestWithSuffixes(t,
		[]string{"findme.txt", "skipme.txt", "subdir/findme.txt", "subdir/skipme.txt", "subdir/alsome.findme_ext"},
		[]string{"findme.txt", "subdir/findme.txt", "subdir/alsome.findme_ext"},
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
	fs.Create(t, filepath.Join(root, "findme.txt"), filesystem)

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

	fs.AssertSameResponse(t, foundPaths, []string{})
}

func TestFilesystemRoot(t *testing.T) {

	testWithNumThreads := func(t *testing.T, numThreads int) {
		filesystem := newFs()
		root := "/"
		createdPath := "/findme.txt"
		fs.Create(t, createdPath, filesystem)

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

		fs.AssertSameResponse(t, foundPaths, []string{createdPath})
	}

	testAgainstSeveralThreadcounts(t, testWithNumThreads)
}

func TestNonexistentDir(t *testing.T) {
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)

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
		t.Fatal("Did not fail when given a nonexistent root directory")
	}
}

func TestExcludeDirs(t *testing.T) {
	filesystem := newFs()
	fs.Create(t, "/tmp/exclude/findme.txt", filesystem)
	fs.Create(t, "/tmp/exclude/subdir/findme.txt", filesystem)
	fs.Create(t, "/tmp/subdir/exclude/findme.txt", filesystem)
	fs.Create(t, "/tmp/subdir/subdir/findme.txt", filesystem)
	fs.Create(t, "/tmp/subdir/findme.txt", filesystem)
	fs.Create(t, "/tmp/findme.txt", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/subdir/findme.txt",
			"/tmp/subdir/subdir/findme.txt"})
}

func TestPruneFiles(t *testing.T) {
	filesystem := newFs()
	fs.Create(t, "/tmp/out/findme.txt", filesystem)
	fs.Create(t, "/tmp/out/.ignore-out-dir", filesystem)
	fs.Create(t, "/tmp/out/child/findme.txt", filesystem)

	fs.Create(t, "/tmp/out2/.ignore-out-dir", filesystem)
	fs.Create(t, "/tmp/out2/sub/findme.txt", filesystem)

	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/include/findme.txt", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/include/findme.txt"})
}

// TestRootDir tests that the value of RootDirs is used
// tests of the filesystem root are in TestFilesystemRoot
func TestRootDir(t *testing.T) {
	filesystem := newFs()
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/subdir/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/subdir/findme.txt", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt",
			"/tmp/a/subdir/findme.txt"})
}

func TestUncachedDir(t *testing.T) {
	filesystem := newFs()
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/subdir/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/subdir/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{})

	finder.Shutdown()
}

func TestSearchingForFilesExcludedFromCache(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/misc.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{})

	finder.Shutdown()
}

func TestRelativeFilePaths(t *testing.T) {
	filesystem := newFs()

	fs.Create(t, "/tmp/ignore/hi.txt", filesystem)
	fs.Create(t, "/tmp/include/hi.txt", filesystem)
	fs.Create(t, "/cwd/hi.txt", filesystem)
	fs.Create(t, "/cwd/a/hi.txt", filesystem)
	fs.Create(t, "/cwd/a/a/hi.txt", filesystem)
	fs.Create(t, "/rel/a/hi.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"a/hi.txt",
			"a/a/hi.txt"})

	foundPaths = finder.FindNamedAt("/tmp/include", "hi.txt")
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/include/hi.txt"})

	foundPaths = finder.FindNamedAt(".", "hi.txt")
	fs.AssertSameResponse(t, foundPaths,
		[]string{"hi.txt",
			"a/hi.txt",
			"a/a/hi.txt"})

	foundPaths = finder.FindNamedAt("/rel", "hi.txt")
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/rel/a/hi.txt"})

	foundPaths = finder.FindNamedAt("/tmp/include", "hi.txt")
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/include/hi.txt"})
}

// have to run this test with the race-detector (`go test -race src/android/soong/finder/*.go`)
// for there to be much chance of the test actually detecting any error that may be present
func TestRootDirsContainedInOtherRootDirs(t *testing.T) {
	filesystem := newFs()

	fs.Create(t, "/tmp/a/b/c/d/e/f/g/h/i/j/findme.txt", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/a/b/c/d/e/f/g/h/i/j/findme.txt"})
}

func TestFindFirst(t *testing.T) {
	filesystem := newFs()
	fs.Create(t, "/tmp/a/hi.txt", filesystem)
	fs.Create(t, "/tmp/b/hi.txt", filesystem)
	fs.Create(t, "/tmp/b/a/hi.txt", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
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
			fs.Create(t, path, filesystem)
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
			fs.AssertSameResponse(t, foundPaths, paths)
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
		fs.Create(t, path, filesystem)
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
		fs.AssertSameResponse(t, testRun.foundMatches, testRun.correctMatches)
	}
}

func TestStrangelyFormattedPaths(t *testing.T) {
	filesystem := newFs()

	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt",
			"/tmp/b/findme.txt",
			"/tmp/findme.txt"})
}

func TestCorruptedCacheHeader(t *testing.T) {
	filesystem := newFs()

	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Write(t, "/finder/finder-db", "sample header", filesystem)

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

	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt",
			"/tmp/findme.txt"})
}

func TestCanUseCache(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, correctResponse)
	finder.Shutdown()

	// check results
	cacheText := fs.Read(t, finder.DbPath, filesystem)
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
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})
	fs.AssertSameReadDirCalls(t, filesystem.StatCalls, statCalls)

	finder2.Shutdown()
}

func TestCorruptedCacheBody(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, correctResponse)
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
	fs.AssertSameResponse(t, foundPaths, correctResponse)
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
	fs.Create(t, "/tmp/a/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})
	fs.AssertSameStatCalls(t, filesystem.StatCalls, []string{"/tmp", "/tmp/a"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/a"})
}

func TestFileAdded(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/ignoreme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/ignore.txt", filesystem)
	fs.Create(t, "/tmp/b/c/nope.txt", filesystem)
	fs.Create(t, "/tmp/b/c/d/irrelevant.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"findme.txt"},
		},
	)
	finder.WaitForDbDump()
	filesystem.Clock.Tick()
	foundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	finder.Shutdown()
	// check the response of the first finder
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Create(t, "/tmp/b/c/findme.txt", filesystem)
	filesystem.Clock.Tick()
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt", "/tmp/b/c/findme.txt"})
	fs.AssertSameStatCalls(t, filesystem.StatCalls, []string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/c", "/tmp/b/c/d"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b/c"})
	finder2.Shutdown()

}

func TestDirectoriesAdded(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/ignoreme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/ignore.txt", filesystem)
	fs.Create(t, "/tmp/b/c/nope.txt", filesystem)
	fs.Create(t, "/tmp/b/c/d/irrelevant.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Create(t, "/tmp/b/c/new/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/c/new/new2/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/c/new/new2/ignoreme.txt", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/a/findme.txt", "/tmp/b/c/new/findme.txt", "/tmp/b/c/new/new2/findme.txt"})
	fs.AssertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/c", "/tmp/b/c/d", "/tmp/b/c/new", "/tmp/b/c/new/new2"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b/c", "/tmp/b/c/new", "/tmp/b/c/new/new2"})

	finder2.Shutdown()
}

func TestDirectoryAndSubdirectoryBothUpdated(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/hi1.txt", filesystem)
	fs.Create(t, "/tmp/a/hi1.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/hi1.txt", "/tmp/a/hi1.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Create(t, "/tmp/hi2.txt", filesystem)
	fs.Create(t, "/tmp/a/hi2.txt", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindAll()

	// check results
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/hi1.txt", "/tmp/hi2.txt", "/tmp/a/hi1.txt", "/tmp/a/hi2.txt"})
	fs.AssertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/a"})

	finder2.Shutdown()
}

func TestFileDeleted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/ignoreme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/c/nope.txt", filesystem)
	fs.Create(t, "/tmp/b/c/d/irrelevant.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt", "/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Delete(t, "/tmp/b/findme.txt", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/findme.txt"})
	fs.AssertSameStatCalls(t, filesystem.StatCalls, []string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/c", "/tmp/b/c/d"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b"})

	finder2.Shutdown()
}

func TestDirectoriesDeleted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/1/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/1/2/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/a/1/findme.txt",
			"/tmp/a/1/2/findme.txt",
			"/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.RemoveAll(t, "/tmp/a/1", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt", "/tmp/a/findme.txt", "/tmp/b/findme.txt"})
	// Technically, we don't care whether /tmp/a/1/2 gets Statted or gets skipped
	// if the Finder detects the nonexistence of /tmp/a/1
	// However, when resuming from cache, we don't want the Finder to necessarily wait
	// to stat a directory until after statting its parent.
	// So here we just include /tmp/a/1/2 in the list.
	// The Finder is currently implemented to always restat every dir and
	// to not short-circuit due to nonexistence of parents (but it will remove
	// missing dirs from the cache for next time)
	fs.AssertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/1", "/tmp/a/1/2", "/tmp/b"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/a"})

	finder2.Shutdown()
}

func TestDirectoriesMoved(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/1/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/1/2/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/a/1/findme.txt",
			"/tmp/a/1/2/findme.txt",
			"/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Move(t, "/tmp/a", "/tmp/c", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	fs.AssertSameResponse(t, foundPaths,
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
	fs.AssertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/1", "/tmp/a/1/2", "/tmp/b", "/tmp/c", "/tmp/c/1", "/tmp/c/1/2"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/c", "/tmp/c/1", "/tmp/c/1/2"})
	finder2.Shutdown()
}

func TestDirectoriesSwapped(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/1/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/1/2/findme.txt", filesystem)
	fs.Create(t, "/tmp/b/findme.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt",
			"/tmp/a/findme.txt",
			"/tmp/a/1/findme.txt",
			"/tmp/a/1/2/findme.txt",
			"/tmp/b/findme.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Move(t, "/tmp/a", "/tmp/temp", filesystem)
	fs.Move(t, "/tmp/b", "/tmp/a", filesystem)
	fs.Move(t, "/tmp/temp", "/tmp/b", filesystem)
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "findme.txt")

	// check results
	fs.AssertSameResponse(t, foundPaths,
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
	fs.AssertSameStatCalls(t, filesystem.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/1", "/tmp/a/1/2", "/tmp/b", "/tmp/b/1", "/tmp/b/1/2"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp", "/tmp/a", "/tmp/b", "/tmp/b/1", "/tmp/b/1/2"})
	finder2.Shutdown()
}

// runFsReplacementTest tests a change modifying properties of the filesystem itself:
// runFsReplacementTest tests changing the user, the hostname, or the device number
// runFsReplacementTest is a helper method called by other tests
func runFsReplacementTest(t *testing.T, fs1 *fs.MockFs, fs2 *fs.MockFs) {
	// setup fs1
	fs.Create(t, "/tmp/findme.txt", fs1)
	fs.Create(t, "/tmp/a/findme.txt", fs1)
	fs.Create(t, "/tmp/a/a/findme.txt", fs1)

	// setup fs2 to have the same directories but different files
	fs.Create(t, "/tmp/findme.txt", fs2)
	fs.Create(t, "/tmp/a/findme.txt", fs2)
	fs.Create(t, "/tmp/a/a/ignoreme.txt", fs2)
	fs.Create(t, "/tmp/a/b/findme.txt", fs2)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt", "/tmp/a/findme.txt", "/tmp/a/a/findme.txt"})

	// copy the cache data from the first filesystem to the second
	cacheContent := fs.Read(t, finder.DbPath, fs1)
	fs.Write(t, finder.DbPath, cacheContent, fs2)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/findme.txt", "/tmp/a/findme.txt", "/tmp/a/b/findme.txt"})
	fs.AssertSameStatCalls(t, fs2.StatCalls,
		[]string{"/tmp", "/tmp/a", "/tmp/a/a", "/tmp/a/b"})
	fs.AssertSameReadDirCalls(t, fs2.ReadDirCalls,
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
		fs.Create(t, fmt.Sprintf("/tmp/%v/findme.txt", i), filesystem)
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
	string1 := fs.Read(t, finder.DbPath, filesystem)

	err := filesystem.Remove(finder.DbPath)
	if err != nil {
		t.Fatal(err)
	}

	// run another finder
	finder2 := finderWithSameParams(t, finder)
	finder2.FindNamedAt("/tmp", "findme.txt")
	finder2.Shutdown()

	string2 := fs.Read(t, finder.DbPath, filesystem)

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
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/misc.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/findme.txt", "/tmp/a/findme.txt"})

	filesystem.ClearMetrics()

	// run the finder again and confirm it doesn't check the filesystem
	refoundPaths := finder.FindNamedAt("/tmp", "findme.txt")
	fs.AssertSameResponse(t, refoundPaths, foundPaths)
	fs.AssertSameStatCalls(t, filesystem.StatCalls, []string{})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})

	finder.Shutdown()
}

func TestChangingParamsOfSecondFind(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/findme.txt", filesystem)
	fs.Create(t, "/tmp/a/metoo.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/findme.txt", "/tmp/a/findme.txt"})

	filesystem.ClearMetrics()

	// run the finder again and confirm it gets the right answer without asking the filesystem
	refoundPaths := finder.FindNamedAt("/tmp", "metoo.txt")
	fs.AssertSameResponse(t, refoundPaths, []string{"/tmp/a/metoo.txt"})
	fs.AssertSameStatCalls(t, filesystem.StatCalls, []string{})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})

	finder.Shutdown()
}

func TestSymlinkPointingToFile(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/a/hi.txt", filesystem)
	fs.Create(t, "/tmp/a/ignoreme.txt", filesystem)
	fs.Link(t, "/tmp/hi.txt", "a/hi.txt", filesystem)
	fs.Link(t, "/tmp/b/hi.txt", "../a/hi.txt", filesystem)
	fs.Link(t, "/tmp/c/hi.txt", "/tmp/hi.txt", filesystem)
	fs.Link(t, "/tmp/d/hi.txt", "../a/bye.txt", filesystem)
	fs.Link(t, "/tmp/d/bye.txt", "../a/hi.txt", filesystem)
	fs.Link(t, "/tmp/e/bye.txt", "../a/bye.txt", filesystem)
	fs.Link(t, "/tmp/f/hi.txt", "somethingThatDoesntExist", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, correctResponse)

}

func TestSymlinkPointingToDirectory(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/dir/hi.txt", filesystem)
	fs.Create(t, "/tmp/dir/ignoreme.txt", filesystem)

	fs.Link(t, "/tmp/links/dir", "../dir", filesystem)
	fs.Link(t, "/tmp/links/link", "../dir", filesystem)
	fs.Link(t, "/tmp/links/hi.txt", "../dir", filesystem)
	fs.Link(t, "/tmp/links/broken", "nothingHere", filesystem)
	fs.Link(t, "/tmp/links/recursive", "recursive", filesystem)

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
	fs.AssertSameResponse(t, foundPaths, correctResponse)

}

// TestAddPruneFile confirms that adding a prune-file (into a directory for which we
// already had a cache) causes the directory to be ignored
func TestAddPruneFile(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/out/hi.txt", filesystem)
	fs.Create(t, "/tmp/out/a/hi.txt", filesystem)
	fs.Create(t, "/tmp/hi.txt", filesystem)

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
	fs.AssertSameResponse(t, foundPaths,
		[]string{"/tmp/hi.txt",
			"/tmp/out/hi.txt",
			"/tmp/out/a/hi.txt"},
	)
	finder.Shutdown()

	// modify filesystem
	filesystem.Clock.Tick()
	fs.Create(t, "/tmp/out/.ignore-out-dir", filesystem)
	// run another find and check its result
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindNamedAt("/tmp", "hi.txt")
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/hi.txt"})
	finder2.Shutdown()
}

func TestUpdatingDbIffChanged(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/a/hi.txt", filesystem)
	fs.Create(t, "/tmp/b/bye.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	finder.WaitForDbDump()
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt"})

	// modify the filesystem
	filesystem.Clock.Tick()
	fs.Create(t, "/tmp/b/hi.txt", filesystem)
	filesystem.Clock.Tick()
	filesystem.ClearMetrics()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindAll()
	finder2.Shutdown()
	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt", "/tmp/b/hi.txt"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{"/tmp/b"})
	expectedDbWriteTime := filesystem.Clock.Time()
	actualDbWriteTime := fs.ModTime(t, finder2.DbPath, filesystem)
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
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt", "/tmp/b/hi.txt"})
	fs.AssertSameReadDirCalls(t, filesystem.ReadDirCalls, []string{})
	finder3.Shutdown()
	actualDbWriteTime = fs.ModTime(t, finder3.DbPath, filesystem)
	if actualDbWriteTime != expectedDbWriteTime {
		t.Fatalf("Re-wrote db even when contents did not change")
	}

}

func TestDirectoryNotPermitted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/hi.txt", filesystem)
	fs.Create(t, "/tmp/a/hi.txt", filesystem)
	fs.Create(t, "/tmp/a/a/hi.txt", filesystem)
	fs.Create(t, "/tmp/b/hi.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	finder.WaitForDbDump()
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	allPaths := []string{"/tmp/hi.txt", "/tmp/a/hi.txt", "/tmp/a/a/hi.txt", "/tmp/b/hi.txt"}
	// check results
	fs.AssertSameResponse(t, foundPaths, allPaths)

	// modify the filesystem
	filesystem.Clock.Tick()

	fs.SetReadable(t, "/tmp/a", false, filesystem)
	filesystem.Clock.Tick()

	// run the second finder
	finder2 := finderWithSameParams(t, finder)
	foundPaths = finder2.FindAll()
	finder2.Shutdown()
	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/hi.txt", "/tmp/b/hi.txt"})

	// modify the filesystem back
	fs.SetReadable(t, "/tmp/a", true, filesystem)

	// run the third finder
	finder3 := finderWithSameParams(t, finder2)
	foundPaths = finder3.FindAll()
	finder3.Shutdown()
	// check results
	fs.AssertSameResponse(t, foundPaths, allPaths)
}

func TestFileNotPermitted(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/hi.txt", filesystem)
	fs.SetReadable(t, "/tmp/hi.txt", false, filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	finder.WaitForDbDump()
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/hi.txt"})
}

func TestCacheEntryPathUnexpectedError(t *testing.T) {
	// setup filesystem
	filesystem := newFs()
	fs.Create(t, "/tmp/a/hi.txt", filesystem)

	// run the first finder
	finder := newFinder(
		t,
		filesystem,
		CacheParams{
			RootDirs:     []string{"/tmp"},
			IncludeFiles: []string{"hi.txt"},
		},
	)
	finder.WaitForDbDump()
	filesystem.Clock.Tick()
	foundPaths := finder.FindAll()
	finder.Shutdown()
	// check results
	fs.AssertSameResponse(t, foundPaths, []string{"/tmp/a/hi.txt"})

	// make the directory not readable
	fs.SetReadErr(t, "/tmp/a", os.ErrInvalid, filesystem)

	// run the second finder
	_, err := finderAndErrorWithSameParams(t, finder)
	if err == nil {
		t.Fatal("Failed to detect unexpected filesystem error")
	}
}
