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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"android/soong/finder/fs"
)

// This file provides a Finder struct that can quickly search for files satisfying
// certain criteria.
// This Finder gets its speed partially from parallelism and partially from caching.
// If a Stat call returns the same result as last time, then it means Finder
// can skip the ReadDir call for that dir.

// The primary data structure used by the finder is the field Finder.nodes ,
// which is a tree of nodes of type *pathMap .
// Each node represents a directory on disk, along with its stats, subdirectories,
// and contained files.

// The common use case for the Finder is that the caller creates a Finder and gives
// it the same query that was given to it in the previous execution.
// In this situation, the major events that take place are:
// 1. The Finder begins to load its db
// 2. The Finder begins to stat the directories mentioned in its db (using multiple threads)
//    Calling Stat on each of these directories is generally a large fraction of the total time
// 3. The Finder begins to construct a separate tree of nodes in each of its threads
// 4. The Finder merges the individual node trees into the main node tree
// 5. The Finder may call ReadDir a few times if there are a few directories that are out-of-date
//    These ReadDir calls might prompt additional Stat calls, etc
// 6. The Finder waits for all loading to complete
// 7. The Finder searches the cache for files matching the user's query (using multiple threads)

// These are the invariants regarding concurrency:
// 1. The public methods of Finder are threadsafe.
//      The public methods are only performance-optimized for one caller at a time, however.
//      For the moment, multiple concurrent callers shouldn't expect any better performance than
//      multiple serial callers.
// 2. While building the node tree, only one thread may ever access the <children> collection of a
//    *pathMap at once.
//    a) The thread that accesses the <children> collection is the thread that discovers the
//       children (by reading them from the cache or by having received a response to ReadDir).
//       1) Consequently, the thread that discovers the children also spawns requests to stat
//          subdirs.
//    b) Consequently, while building the node tree, no thread may do a lookup of its
//       *pathMap via filepath because another thread may be adding children to the
//       <children> collection of an ancestor node. Additionally, in rare cases, another thread
//       may be removing children from an ancestor node if the children were only discovered to
//       be irrelevant after calling ReadDir (which happens if a prune-file was just added).
// 3. No query will begin to be serviced until all loading (both reading the db
//    and scanning the filesystem) is complete.
//    Tests indicate that it only takes about 10% as long to search the in-memory cache as to
//    generate it, making this not a huge loss in performance.
// 4. The parsing of the db and the initial setup of the pathMap tree must complete before
//      beginning to call listDirSync (because listDirSync can create new entries in the pathMap)

// see cmd/finder.go or finder_test.go for usage examples

// Update versionString whenever making a backwards-incompatible change to the cache file format
const versionString = "Android finder version 1"

// a CacheParams specifies which files and directories the user wishes be scanned and
// potentially added to the cache
type CacheParams struct {
	// WorkingDirectory is used as a base for any relative file paths given to the Finder
	WorkingDirectory string

	// RootDirs are the root directories used to initiate the search
	RootDirs []string

	// ExcludeDirs are directory names that if encountered are removed from the search
	ExcludeDirs []string

	// PruneFiles are file names that if encountered prune their entire directory
	// (including siblings)
	PruneFiles []string

	// IncludeFiles are file names to include as matches
	IncludeFiles []string
}

// a cacheConfig stores the inputs that determine what should be included in the cache
type cacheConfig struct {
	CacheParams

	// FilesystemView is a unique identifier telling which parts of which file systems
	// are readable by the Finder. In practice its value is essentially username@hostname.
	// FilesystemView is set to ensure that a cache file copied to another host or
	// found by another user doesn't inadvertently get reused.
	FilesystemView string
}

func (p *cacheConfig) Dump() ([]byte, error) {
	bytes, err := json.Marshal(p)
	return bytes, err
}

// a cacheMetadata stores version information about the cache
type cacheMetadata struct {
	// The Version enables the Finder to determine whether it can even parse the file
	// If the version changes, the entire cache file must be regenerated
	Version string

	// The CacheParams enables the Finder to determine whether the parameters match
	// If the CacheParams change, the Finder can choose how much of the cache file to reuse
	// (although in practice, the Finder will probably choose to ignore the entire file anyway)
	Config cacheConfig
}

type Logger interface {
	Output(calldepth int, s string) error
}

// the Finder is the main struct that callers will want to use
type Finder struct {
	// configuration
	DbPath              string
	numDbLoadingThreads int
	numSearchingThreads int
	cacheMetadata       cacheMetadata
	logger              Logger
	filesystem          fs.FileSystem

	// temporary state
	threadPool        *threadPool
	mutex             sync.Mutex
	fsErrs            []fsErr
	errlock           sync.Mutex
	shutdownWaitgroup sync.WaitGroup

	// non-temporary state
	modifiedFlag int32
	nodes        pathMap
}

var defaultNumThreads = runtime.NumCPU() * 2

// New creates a new Finder for use
func New(cacheParams CacheParams, filesystem fs.FileSystem,
	logger Logger, dbPath string) (f *Finder, err error) {
	return newImpl(cacheParams, filesystem, logger, dbPath, defaultNumThreads)
}

// newImpl is like New but accepts more params
func newImpl(cacheParams CacheParams, filesystem fs.FileSystem,
	logger Logger, dbPath string, numThreads int) (f *Finder, err error) {
	numDbLoadingThreads := numThreads
	numSearchingThreads := numThreads

	metadata := cacheMetadata{
		Version: versionString,
		Config: cacheConfig{
			CacheParams:    cacheParams,
			FilesystemView: filesystem.ViewId(),
		},
	}

	f = &Finder{
		numDbLoadingThreads: numDbLoadingThreads,
		numSearchingThreads: numSearchingThreads,
		cacheMetadata:       metadata,
		logger:              logger,
		filesystem:          filesystem,

		nodes:  *newPathMap("/"),
		DbPath: dbPath,

		shutdownWaitgroup: sync.WaitGroup{},
	}

	f.loadFromFilesystem()

	// check for any filesystem errors
	err = f.getErr()
	if err != nil {
		return nil, err
	}

	// confirm that every path mentioned in the CacheConfig exists
	for _, path := range cacheParams.RootDirs {
		if !filepath.IsAbs(path) {
			path = filepath.Join(f.cacheMetadata.Config.WorkingDirectory, path)
		}
		node := f.nodes.GetNode(filepath.Clean(path), false)
		if node == nil || node.ModTime == 0 {
			return nil, fmt.Errorf("path %v was specified to be included in the cache but does not exist\n", path)
		}
	}

	return f, nil
}

// FindNamed searches for every cached file
func (f *Finder) FindAll() []string {
	return f.FindAt("/")
}

// FindNamed searches for every cached file under <rootDir>
func (f *Finder) FindAt(rootDir string) []string {
	filter := func(entries DirEntries) (dirNames []string, fileNames []string) {
		return entries.DirNames, entries.FileNames
	}
	return f.FindMatching(rootDir, filter)
}

// FindNamed searches for every cached file named <fileName>
func (f *Finder) FindNamed(fileName string) []string {
	return f.FindNamedAt("/", fileName)
}

// FindNamedAt searches under <rootPath> for every file named <fileName>
// The reason a caller might use FindNamedAt instead of FindNamed is if they want
// to limit their search to a subset of the cache
func (f *Finder) FindNamedAt(rootPath string, fileName string) []string {
	filter := func(entries DirEntries) (dirNames []string, fileNames []string) {
		matches := []string{}
		for _, foundName := range entries.FileNames {
			if foundName == fileName {
				matches = append(matches, foundName)
			}
		}
		return entries.DirNames, matches

	}
	return f.FindMatching(rootPath, filter)
}

// FindFirstNamed searches for every file named <fileName>
// Whenever it finds a match, it stops search subdirectories
func (f *Finder) FindFirstNamed(fileName string) []string {
	return f.FindFirstNamedAt("/", fileName)
}

// FindFirstNamedAt searches for every file named <fileName>
// Whenever it finds a match, it stops search subdirectories
func (f *Finder) FindFirstNamedAt(rootPath string, fileName string) []string {
	filter := func(entries DirEntries) (dirNames []string, fileNames []string) {
		matches := []string{}
		for _, foundName := range entries.FileNames {
			if foundName == fileName {
				matches = append(matches, foundName)
			}
		}

		if len(matches) > 0 {
			return []string{}, matches
		}
		return entries.DirNames, matches
	}
	return f.FindMatching(rootPath, filter)
}

// FindMatching is the most general exported function for searching for files in the cache
// The WalkFunc will be invoked repeatedly and is expected to modify the provided DirEntries
// in place, removing file paths and directories as desired.
// WalkFunc will be invoked potentially many times in parallel, and must be threadsafe.
func (f *Finder) FindMatching(rootPath string, filter WalkFunc) []string {
	// set up some parameters
	scanStart := time.Now()
	var isRel bool
	workingDir := f.cacheMetadata.Config.WorkingDirectory

	isRel = !filepath.IsAbs(rootPath)
	if isRel {
		rootPath = filepath.Join(workingDir, rootPath)
	}

	rootPath = filepath.Clean(rootPath)

	// ensure nothing else is using the Finder
	f.verbosef("FindMatching waiting for finder to be idle\n")
	f.lock()
	defer f.unlock()

	node := f.nodes.GetNode(rootPath, false)
	if node == nil {
		f.verbosef("No data for path %v ; apparently not included in cache params: %v\n",
			rootPath, f.cacheMetadata.Config.CacheParams)
		// path is not found; don't do a search
		return []string{}
	}

	// search for matching files
	f.verbosef("Finder finding %v using cache\n", rootPath)
	results := f.findInCacheMultithreaded(node, filter, f.numSearchingThreads)

	// format and return results
	if isRel {
		for i := 0; i < len(results); i++ {
			results[i] = strings.Replace(results[i], workingDir+"/", "", 1)
		}
	}
	sort.Strings(results)
	f.verbosef("Found %v files under %v in %v using cache\n",
		len(results), rootPath, time.Since(scanStart))
	return results
}

// Shutdown declares that the finder is no longer needed and waits for its cleanup to complete
// Currently, that only entails waiting for the database dump to complete.
func (f *Finder) Shutdown() {
	f.waitForDbDump()
}

// End of public api

func (f *Finder) goDumpDb() {
	if f.wasModified() {
		f.shutdownWaitgroup.Add(1)
		go func() {
			err := f.dumpDb()
			if err != nil {
				f.verbosef("%v\n", err)
			}
			f.shutdownWaitgroup.Done()
		}()
	} else {
		f.verbosef("Skipping dumping unmodified db\n")
	}
}

func (f *Finder) waitForDbDump() {
	f.shutdownWaitgroup.Wait()
}

// joinCleanPaths is like filepath.Join but is faster because
// joinCleanPaths doesn't have to support paths ending in "/" or containing ".."
func joinCleanPaths(base string, leaf string) string {
	if base == "" {
		return leaf
	}
	if base == "/" {
		return base + leaf
	}
	if leaf == "" {
		return base
	}
	return base + "/" + leaf
}

func (f *Finder) verbosef(format string, args ...interface{}) {
	f.logger.Output(2, fmt.Sprintf(format, args...))
}

// loadFromFilesystem populates the in-memory cache based on the contents of the filesystem
func (f *Finder) loadFromFilesystem() {
	f.threadPool = newThreadPool(f.numDbLoadingThreads)

	err := f.startFromExternalCache()
	if err != nil {
		f.startWithoutExternalCache()
	}

	f.goDumpDb()

	f.threadPool = nil
}

func (f *Finder) startFind(path string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(f.cacheMetadata.Config.WorkingDirectory, path)
	}
	node := f.nodes.GetNode(path, true)
	f.statDirAsync(node)
}

func (f *Finder) lock() {
	f.mutex.Lock()
}

func (f *Finder) unlock() {
	f.mutex.Unlock()
}

// a statResponse is the relevant portion of the response from the filesystem to a Stat call
type statResponse struct {
	ModTime int64
	Inode   uint64
	Device  uint64
}

// a pathAndStats stores a path and its stats
type pathAndStats struct {
	statResponse

	Path string
}

// a dirFullInfo stores all of the relevant information we know about a directory
type dirFullInfo struct {
	pathAndStats

	FileNames []string
}

// a PersistedDirInfo is the information about a dir that we save to our cache on disk
type PersistedDirInfo struct {
	// These field names are short because they are repeated many times in the output json file
	P string   // path
	T int64    // modification time
	I uint64   // inode number
	F []string // relevant filenames contained
}

// a PersistedDirs is the information that we persist for a group of dirs
type PersistedDirs struct {
	// the device on which each directory is stored
	Device uint64
	// the common root path to which all contained dirs are relative
	Root string
	// the directories themselves
	Dirs []PersistedDirInfo
}

// a CacheEntry is the smallest unit that can be read and parsed from the cache (on disk) at a time
type CacheEntry []PersistedDirs

// a DirEntries lists the files and directories contained directly within a specific directory
type DirEntries struct {
	Path string

	// elements of DirNames are just the dir names; they don't include any '/' character
	DirNames []string
	// elements of FileNames are just the file names; they don't include '/' character
	FileNames []string
}

// a WalkFunc is the type that is passed into various Find functions for determining which
// directories the caller wishes be walked. The WalkFunc is expected to decide which
// directories to walk and which files to consider as matches to the original query.
type WalkFunc func(DirEntries) (dirs []string, files []string)

// a mapNode stores the relevant stats about a directory to be stored in a pathMap
type mapNode struct {
	statResponse
	FileNames []string
}

// a pathMap implements the directory tree structure of nodes
type pathMap struct {
	mapNode

	path string

	children map[string]*pathMap

	// number of descendent nodes, including self
	approximateNumDescendents int
}

func newPathMap(path string) *pathMap {
	result := &pathMap{path: path, children: make(map[string]*pathMap, 4),
		approximateNumDescendents: 1}
	return result
}

// GetNode returns the node at <path>
func (m *pathMap) GetNode(path string, createIfNotFound bool) *pathMap {
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	node := m
	for {
		if path == "" {
			return node
		}

		index := strings.Index(path, "/")
		var firstComponent string
		if index >= 0 {
			firstComponent = path[:index]
			path = path[index+1:]
		} else {
			firstComponent = path
			path = ""
		}

		child, found := node.children[firstComponent]

		if !found {
			if createIfNotFound {
				child = node.newChild(firstComponent)
			} else {
				return nil
			}
		}

		node = child
	}
}

func (m *pathMap) newChild(name string) (child *pathMap) {
	path := joinCleanPaths(m.path, name)
	newChild := newPathMap(path)
	m.children[name] = newChild

	return m.children[name]
}

func (m *pathMap) UpdateNumDescendents() int {
	count := 1
	for _, child := range m.children {
		count += child.approximateNumDescendents
	}
	m.approximateNumDescendents = count
	return count
}

func (m *pathMap) UpdateNumDescendentsRecursive() {
	for _, child := range m.children {
		child.UpdateNumDescendentsRecursive()
	}
	m.UpdateNumDescendents()
}

func (m *pathMap) MergeIn(other *pathMap) {
	for key, theirs := range other.children {
		ours, found := m.children[key]
		if found {
			ours.MergeIn(theirs)
		} else {
			m.children[key] = theirs
		}
	}
	if other.ModTime != 0 {
		m.mapNode = other.mapNode
	}
	m.UpdateNumDescendents()
}

func (m *pathMap) DumpAll() []dirFullInfo {
	results := []dirFullInfo{}
	m.dumpInto("", &results)
	return results
}

func (m *pathMap) dumpInto(path string, results *[]dirFullInfo) {
	*results = append(*results,
		dirFullInfo{
			pathAndStats{statResponse: m.statResponse, Path: path},
			m.FileNames},
	)
	for key, child := range m.children {
		childPath := joinCleanPaths(path, key)
		if len(childPath) == 0 || childPath[0] != '/' {
			childPath = "/" + childPath
		}
		child.dumpInto(childPath, results)
	}
}

// a semaphore can be locked by up to <capacity> callers at once
type semaphore struct {
	pool chan bool
}

func newSemaphore(capacity int) *semaphore {
	return &semaphore{pool: make(chan bool, capacity)}
}

func (l *semaphore) Lock() {
	l.pool <- true
}

func (l *semaphore) Unlock() {
	<-l.pool
}

// A threadPool runs goroutines and supports throttling and waiting.
// Without throttling, Go may exhaust the maximum number of various resources, such as
// threads or file descriptors, and crash the program.
type threadPool struct {
	receivedRequests sync.WaitGroup
	activeRequests   semaphore
}

func newThreadPool(maxNumConcurrentThreads int) *threadPool {
	return &threadPool{
		receivedRequests: sync.WaitGroup{},
		activeRequests:   *newSemaphore(maxNumConcurrentThreads),
	}
}

// Run requests to run the given function in its own goroutine
func (p *threadPool) Run(function func()) {
	p.receivedRequests.Add(1)
	// If Run() was called from within a goroutine spawned by this threadPool,
	// then we may need to return from Run() before having capacity to actually
	// run <function>.
	//
	// It's possible that the body of <function> contains a statement (such as a syscall)
	// that will cause Go to pin it to a thread, or will contain a statement that uses
	// another resource that is in short supply (such as a file descriptor), so we can't
	// actually run <function> until we have capacity.
	//
	// However, the semaphore used for synchronization is implemented via a channel and
	// shouldn't require a new thread for each access.
	go func() {
		p.activeRequests.Lock()
		function()
		p.activeRequests.Unlock()
		p.receivedRequests.Done()
	}()
}

// Wait waits until all goroutines are done, just like sync.WaitGroup's Wait
func (p *threadPool) Wait() {
	p.receivedRequests.Wait()
}

type fsErr struct {
	path string
	err  error
}

func (e fsErr) String() string {
	return e.path + ": " + e.err.Error()
}

func (f *Finder) serializeCacheEntry(dirInfos []dirFullInfo) ([]byte, error) {
	// group each dirFullInfo by its Device, to avoid having to repeat it in the output
	dirsByDevice := map[uint64][]PersistedDirInfo{}
	for _, entry := range dirInfos {
		_, found := dirsByDevice[entry.Device]
		if !found {
			dirsByDevice[entry.Device] = []PersistedDirInfo{}
		}
		dirsByDevice[entry.Device] = append(dirsByDevice[entry.Device],
			PersistedDirInfo{P: entry.Path, T: entry.ModTime, I: entry.Inode, F: entry.FileNames})
	}

	cacheEntry := CacheEntry{}

	for device, infos := range dirsByDevice {
		// find common prefix
		prefix := ""
		if len(infos) > 0 {
			prefix = infos[0].P
		}
		for _, info := range infos {
			for !strings.HasPrefix(info.P+"/", prefix+"/") {
				prefix = filepath.Dir(prefix)
				if prefix == "/" {
					break
				}
			}
		}
		// remove common prefix
		for i := range infos {
			suffix := strings.Replace(infos[i].P, prefix, "", 1)
			if len(suffix) > 0 && suffix[0] == '/' {
				suffix = suffix[1:]
			}
			infos[i].P = suffix
		}

		// turn the map (keyed by device) into a list of structs with labeled fields
		// this is to improve readability of the output
		cacheEntry = append(cacheEntry, PersistedDirs{Device: device, Root: prefix, Dirs: infos})
	}

	// convert to json.
	// it would save some space to use a different format than json for the db file,
	// but the space and time savings are small, and json is easy for humans to read
	bytes, err := json.Marshal(cacheEntry)
	return bytes, err
}

func (f *Finder) parseCacheEntry(bytes []byte) ([]dirFullInfo, error) {
	var cacheEntry CacheEntry
	err := json.Unmarshal(bytes, &cacheEntry)
	if err != nil {
		return nil, err
	}

	// convert from a CacheEntry to a []dirFullInfo (by copying a few fields)
	capacity := 0
	for _, element := range cacheEntry {
		capacity += len(element.Dirs)
	}
	nodes := make([]dirFullInfo, capacity)
	count := 0
	for _, element := range cacheEntry {
		for _, dir := range element.Dirs {
			path := joinCleanPaths(element.Root, dir.P)

			nodes[count] = dirFullInfo{
				pathAndStats: pathAndStats{
					statResponse: statResponse{
						ModTime: dir.T, Inode: dir.I, Device: element.Device,
					},
					Path: path},
				FileNames: dir.F}
			count++
		}
	}
	return nodes, nil
}

// We use the following separator byte to distinguish individually parseable blocks of json
// because we know this separator won't appear in the json that we're parsing.
//
// The newline byte can only appear in a UTF-8 stream if the newline character appears, because:
// - The newline character is encoded as "0000 1010" in binary ("0a" in hex)
// - UTF-8 dictates that bytes beginning with a "0" bit are never emitted as part of a multibyte
//   character.
//
// We know that the newline character will never appear in our json string, because:
// - If a newline character appears as part of a data string, then json encoding will
//   emit two characters instead: '\' and 'n'.
// - The json encoder that we use doesn't emit the optional newlines between any of its
//   other outputs.
const lineSeparator = byte('\n')

func (f *Finder) readLine(reader *bufio.Reader) ([]byte, error) {
	return reader.ReadBytes(lineSeparator)
}

// validateCacheHeader reads the cache header from cacheReader and tells whether the cache is compatible with this Finder
func (f *Finder) validateCacheHeader(cacheReader *bufio.Reader) bool {
	cacheVersionBytes, err := f.readLine(cacheReader)
	if err != nil {
		f.verbosef("Failed to read database header; database is invalid\n")
		return false
	}
	if len(cacheVersionBytes) > 0 && cacheVersionBytes[len(cacheVersionBytes)-1] == lineSeparator {
		cacheVersionBytes = cacheVersionBytes[:len(cacheVersionBytes)-1]
	}
	cacheVersionString := string(cacheVersionBytes)
	currentVersion := f.cacheMetadata.Version
	if cacheVersionString != currentVersion {
		f.verbosef("Version changed from %q to %q, database is not applicable\n", cacheVersionString, currentVersion)
		return false
	}

	cacheParamBytes, err := f.readLine(cacheReader)
	if err != nil {
		f.verbosef("Failed to read database search params; database is invalid\n")
		return false
	}

	if len(cacheParamBytes) > 0 && cacheParamBytes[len(cacheParamBytes)-1] == lineSeparator {
		cacheParamBytes = cacheParamBytes[:len(cacheParamBytes)-1]
	}

	currentParamBytes, err := f.cacheMetadata.Config.Dump()
	if err != nil {
		panic("Finder failed to serialize its parameters")
	}
	cacheParamString := string(cacheParamBytes)
	currentParamString := string(currentParamBytes)
	if cacheParamString != currentParamString {
		f.verbosef("Params changed from %q to %q, database is not applicable\n", cacheParamString, currentParamString)
		return false
	}
	return true
}

// loadBytes compares the cache info in <data> to the state of the filesystem
// loadBytes returns a map representing <data> and also a slice of dirs that need to be re-walked
func (f *Finder) loadBytes(id int, data []byte) (m *pathMap, dirsToWalk []string, err error) {

	helperStartTime := time.Now()

	cachedNodes, err := f.parseCacheEntry(data)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to parse block %v: %v\n", id, err.Error())
	}

	unmarshalDate := time.Now()
	f.verbosef("Unmarshaled %v objects for %v in %v\n",
		len(cachedNodes), id, unmarshalDate.Sub(helperStartTime))

	tempMap := newPathMap("/")
	stats := make([]statResponse, len(cachedNodes))

	for i, node := range cachedNodes {
		// check the file system for an updated timestamp
		stats[i] = f.statDirSync(node.Path)
	}

	dirsToWalk = []string{}
	for i, cachedNode := range cachedNodes {
		updated := stats[i]
		// save the cached value
		container := tempMap.GetNode(cachedNode.Path, true)
		container.mapNode = mapNode{statResponse: updated}

		// if the metadata changed and the directory still exists, then
		// make a note to walk it later
		if !f.isInfoUpToDate(cachedNode.statResponse, updated) && updated.ModTime != 0 {
			f.setModified()
			// make a note that the directory needs to be walked
			dirsToWalk = append(dirsToWalk, cachedNode.Path)
		} else {
			container.mapNode.FileNames = cachedNode.FileNames
		}
	}
	// count the number of nodes to improve our understanding of the shape of the tree,
	// thereby improving parallelism of subsequent searches
	tempMap.UpdateNumDescendentsRecursive()

	f.verbosef("Statted inodes of block %v in %v\n", id, time.Now().Sub(unmarshalDate))
	return tempMap, dirsToWalk, nil
}

// startFromExternalCache loads the cache database from disk
// startFromExternalCache waits to return until the load of the cache db is complete, but
// startFromExternalCache does not wait for all every listDir() or statDir() request to complete
func (f *Finder) startFromExternalCache() (err error) {
	startTime := time.Now()
	dbPath := f.DbPath

	// open cache file and validate its header
	reader, err := f.filesystem.Open(dbPath)
	if err != nil {
		return errors.New("No data to load from database\n")
	}
	bufferedReader := bufio.NewReader(reader)
	if !f.validateCacheHeader(bufferedReader) {
		return errors.New("Cache header does not match")
	}
	f.verbosef("Database header matches, will attempt to use database %v\n", f.DbPath)

	// read the file and spawn threads to process it
	nodesToWalk := [][]*pathMap{}
	mainTree := newPathMap("/")

	// read the blocks and stream them into <blockChannel>
	type dataBlock struct {
		id   int
		err  error
		data []byte
	}
	blockChannel := make(chan dataBlock, f.numDbLoadingThreads)
	readBlocks := func() {
		index := 0
		for {
			// It takes some time to unmarshal the input from json, so we want
			// to unmarshal it in parallel. In order to find valid places to
			// break the input, we scan for the line separators that we inserted
			// (for this purpose) when we dumped the database.
			data, err := f.readLine(bufferedReader)
			var response dataBlock
			done := false
			if err != nil && err != io.EOF {
				response = dataBlock{id: index, err: err, data: nil}
				done = true
			} else {
				done = (err == io.EOF)
				response = dataBlock{id: index, err: nil, data: data}
			}
			blockChannel <- response
			index++
			duration := time.Since(startTime)
			f.verbosef("Read block %v after %v\n", index, duration)
			if done {
				f.verbosef("Read %v blocks in %v\n", index, duration)
				close(blockChannel)
				return
			}
		}
	}
	go readBlocks()

	// Read from <blockChannel> and stream the responses into <resultChannel>.
	type workResponse struct {
		id          int
		err         error
		tree        *pathMap
		updatedDirs []string
	}
	resultChannel := make(chan workResponse)
	processBlocks := func() {
		numProcessed := 0
		threadPool := newThreadPool(f.numDbLoadingThreads)
		for {
			// get a block to process
			block, received := <-blockChannel
			if !received {
				break
			}

			if block.err != nil {
				resultChannel <- workResponse{err: block.err}
				break
			}
			numProcessed++
			// wait until there is CPU available to process it
			threadPool.Run(
				func() {
					processStartTime := time.Now()
					f.verbosef("Starting to process block %v after %v\n",
						block.id, processStartTime.Sub(startTime))
					tempMap, updatedDirs, err := f.loadBytes(block.id, block.data)
					var response workResponse
					if err != nil {
						f.verbosef(
							"Block %v failed to parse with error %v\n",
							block.id, err)
						response = workResponse{err: err}
					} else {
						response = workResponse{
							id:          block.id,
							err:         nil,
							tree:        tempMap,
							updatedDirs: updatedDirs,
						}
					}
					f.verbosef("Processed block %v in %v\n",
						block.id, time.Since(processStartTime),
					)
					resultChannel <- response
				},
			)
		}
		threadPool.Wait()
		f.verbosef("Finished processing %v blocks in %v\n",
			numProcessed, time.Since(startTime))
		close(resultChannel)
	}
	go processBlocks()

	// Read from <resultChannel> and use the results
	combineResults := func() (err error) {
		for {
			result, received := <-resultChannel
			if !received {
				break
			}
			if err != nil {
				// In case of an error, wait for work to complete before
				// returning the error. This ensures that any subsequent
				// work doesn't need to compete for resources (and possibly
				// fail due to, for example, a filesystem limit on the number of
				// concurrently open files) with past work.
				continue
			}
			if result.err != nil {
				err = result.err
				continue
			}
			// update main tree
			mainTree.MergeIn(result.tree)
			// record any new directories that we will need to Stat()
			updatedNodes := make([]*pathMap, len(result.updatedDirs))
			for j, dir := range result.updatedDirs {
				node := mainTree.GetNode(dir, false)
				updatedNodes[j] = node
			}
			nodesToWalk = append(nodesToWalk, updatedNodes)
		}
		return err
	}
	err = combineResults()
	if err != nil {
		return err
	}

	f.nodes = *mainTree

	// after having loaded the entire db and therefore created entries for
	// the directories we know of, now it's safe to start calling ReadDir on
	// any updated directories
	for i := range nodesToWalk {
		f.listDirsAsync(nodesToWalk[i])
	}
	f.verbosef("Loaded db and statted known dirs in %v\n", time.Since(startTime))
	f.threadPool.Wait()
	f.verbosef("Loaded db and statted all dirs in %v\n", time.Now().Sub(startTime))

	return err
}

// startWithoutExternalCache starts scanning the filesystem according to the cache config
// startWithoutExternalCache should be called if startFromExternalCache is not applicable
func (f *Finder) startWithoutExternalCache() {
	startTime := time.Now()
	configDirs := f.cacheMetadata.Config.RootDirs

	// clean paths
	candidates := make([]string, len(configDirs))
	for i, dir := range configDirs {
		candidates[i] = filepath.Clean(dir)
	}
	// remove duplicates
	dirsToScan := make([]string, 0, len(configDirs))
	for _, candidate := range candidates {
		include := true
		for _, included := range dirsToScan {
			if included == "/" || strings.HasPrefix(candidate+"/", included+"/") {
				include = false
				break
			}
		}
		if include {
			dirsToScan = append(dirsToScan, candidate)
		}
	}

	// start searching finally
	for _, path := range dirsToScan {
		f.verbosef("Starting find of %v\n", path)
		f.startFind(path)
	}

	f.threadPool.Wait()

	f.verbosef("Scanned filesystem (not using cache) in %v\n", time.Now().Sub(startTime))
}

// isInfoUpToDate tells whether <new> can confirm that results computed at <old> are still valid
func (f *Finder) isInfoUpToDate(old statResponse, new statResponse) (equal bool) {
	if old.Inode != new.Inode {
		return false
	}
	if old.ModTime != new.ModTime {
		return false
	}
	if old.Device != new.Device {
		return false
	}
	return true
}

func (f *Finder) wasModified() bool {
	return atomic.LoadInt32(&f.modifiedFlag) > 0
}

func (f *Finder) setModified() {
	var newVal int32
	newVal = 1
	atomic.StoreInt32(&f.modifiedFlag, newVal)
}

// sortedDirEntries exports directory entries to facilitate dumping them to the external cache
func (f *Finder) sortedDirEntries() []dirFullInfo {
	startTime := time.Now()
	nodes := make([]dirFullInfo, 0)
	for _, node := range f.nodes.DumpAll() {
		if node.ModTime != 0 {
			nodes = append(nodes, node)
		}
	}
	discoveryDate := time.Now()
	f.verbosef("Generated %v cache entries in %v\n", len(nodes), discoveryDate.Sub(startTime))
	less := func(i int, j int) bool {
		return nodes[i].Path < nodes[j].Path
	}
	sort.Slice(nodes, less)
	sortDate := time.Now()
	f.verbosef("Sorted %v cache entries in %v\n", len(nodes), sortDate.Sub(discoveryDate))

	return nodes
}

// serializeDb converts the cache database into a form to save to disk
func (f *Finder) serializeDb() ([]byte, error) {
	// sort dir entries
	var entryList = f.sortedDirEntries()

	// Generate an output file that can be conveniently loaded using the same number of threads
	// as were used in this execution (because presumably that will be the number of threads
	// used in the next execution too)

	// generate header
	header := []byte{}
	header = append(header, []byte(f.cacheMetadata.Version)...)
	header = append(header, lineSeparator)
	configDump, err := f.cacheMetadata.Config.Dump()
	if err != nil {
		return nil, err
	}
	header = append(header, configDump...)

	// serialize individual blocks in parallel
	numBlocks := f.numDbLoadingThreads
	if numBlocks > len(entryList) {
		numBlocks = len(entryList)
	}
	blocks := make([][]byte, 1+numBlocks)
	blocks[0] = header
	blockMin := 0
	wg := sync.WaitGroup{}
	var errLock sync.Mutex

	for i := 1; i <= numBlocks; i++ {
		// identify next block
		blockMax := len(entryList) * i / numBlocks
		block := entryList[blockMin:blockMax]

		// process block
		wg.Add(1)
		go func(index int, block []dirFullInfo) {
			byteBlock, subErr := f.serializeCacheEntry(block)
			f.verbosef("Serialized block %v into %v bytes\n", index, len(byteBlock))
			if subErr != nil {
				f.verbosef("%v\n", subErr.Error())
				errLock.Lock()
				err = subErr
				errLock.Unlock()
			} else {
				blocks[index] = byteBlock
			}
			wg.Done()
		}(i, block)

		blockMin = blockMax
	}

	wg.Wait()

	if err != nil {
		return nil, err
	}

	content := bytes.Join(blocks, []byte{lineSeparator})

	return content, nil
}

// dumpDb saves the cache database to disk
func (f *Finder) dumpDb() error {
	startTime := time.Now()
	f.verbosef("Dumping db\n")

	tempPath := f.DbPath + ".tmp"

	bytes, err := f.serializeDb()
	if err != nil {
		return err
	}
	serializeDate := time.Now()
	f.verbosef("Serialized db in %v\n", serializeDate.Sub(startTime))
	// dump file and atomically move
	err = f.filesystem.WriteFile(tempPath, bytes, 0777)
	if err != nil {
		return err
	}
	err = f.filesystem.Rename(tempPath, f.DbPath)
	if err != nil {
		return err
	}

	f.verbosef("Wrote db in %v\n", time.Now().Sub(serializeDate))
	return nil

}

// canIgnoreFsErr checks for certain classes of filesystem errors that are safe to ignore
func (f *Finder) canIgnoreFsErr(err error) bool {
	pathErr, isPathErr := err.(*os.PathError)
	if !isPathErr {
		// Don't recognize this error
		return false
	}
	if os.IsPermission(pathErr) {
		// Permission errors are ignored:
		// https://issuetracker.google.com/37553659
		// https://github.com/google/kati/pull/116
		return true
	}
	if pathErr.Err == os.ErrNotExist {
		// If a directory doesn't exist, that generally means the cache is out-of-date
		return true
	}
	// Don't recognize this error
	return false
}

// onFsError should be called whenever a potentially fatal error is returned from a filesystem call
func (f *Finder) onFsError(path string, err error) {
	if !f.canIgnoreFsErr(err) {
		// We could send the errors through a channel instead, although that would cause this call
		// to block unless we preallocated a sufficient buffer or spawned a reader thread.
		// Although it wouldn't be too complicated to spawn a reader thread, it's still slightly
		// more convenient to use a lock. Only in an unusual situation should this code be
		// invoked anyway.
		f.errlock.Lock()
		f.fsErrs = append(f.fsErrs, fsErr{path: path, err: err})
		f.errlock.Unlock()
	}
}

// discardErrsForPrunedPaths removes any errors for paths that are no longer included in the cache
func (f *Finder) discardErrsForPrunedPaths() {
	// This function could be somewhat inefficient due to being single-threaded,
	// but the length of f.fsErrs should be approximately 0, so it shouldn't take long anyway.
	relevantErrs := make([]fsErr, 0, len(f.fsErrs))
	for _, fsErr := range f.fsErrs {
		path := fsErr.path
		node := f.nodes.GetNode(path, false)
		if node != nil {
			// The path in question wasn't pruned due to a failure to process a parent directory.
			// So, the failure to process this path is important
			relevantErrs = append(relevantErrs, fsErr)
		}
	}
	f.fsErrs = relevantErrs
}

// getErr returns an error based on previous calls to onFsErr, if any
func (f *Finder) getErr() error {
	f.discardErrsForPrunedPaths()

	numErrs := len(f.fsErrs)
	if numErrs < 1 {
		return nil
	}

	maxNumErrsToInclude := 10
	message := ""
	if numErrs > maxNumErrsToInclude {
		message = fmt.Sprintf("finder encountered %v errors: %v...", numErrs, f.fsErrs[:maxNumErrsToInclude])
	} else {
		message = fmt.Sprintf("finder encountered %v errors: %v", numErrs, f.fsErrs)
	}

	return errors.New(message)
}

func (f *Finder) statDirAsync(dir *pathMap) {
	node := dir
	path := dir.path
	f.threadPool.Run(
		func() {
			updatedStats := f.statDirSync(path)

			if !f.isInfoUpToDate(node.statResponse, updatedStats) {
				node.mapNode = mapNode{
					statResponse: updatedStats,
					FileNames:    []string{},
				}
				f.setModified()
				if node.statResponse.ModTime != 0 {
					// modification time was updated, so re-scan for
					// child directories
					f.listDirAsync(dir)
				}
			}
		},
	)
}

func (f *Finder) statDirSync(path string) statResponse {

	fileInfo, err := f.filesystem.Lstat(path)

	var stats statResponse
	if err != nil {
		// possibly record this error
		f.onFsError(path, err)
		// in case of a failure to stat the directory, treat the directory as missing (modTime = 0)
		return stats
	}
	modTime := fileInfo.ModTime()
	stats = statResponse{}
	inode, err := f.filesystem.InodeNumber(fileInfo)
	if err != nil {
		panic(fmt.Sprintf("Could not get inode number of %v: %v\n", path, err.Error()))
	}
	stats.Inode = inode
	device, err := f.filesystem.DeviceNumber(fileInfo)
	if err != nil {
		panic(fmt.Sprintf("Could not get device number of %v: %v\n", path, err.Error()))
	}
	stats.Device = device
	permissionsChangeTime, err := f.filesystem.PermTime(fileInfo)

	if err != nil {
		panic(fmt.Sprintf("Could not get permissions modification time (CTime) of %v: %v\n", path, err.Error()))
	}
	// We're only interested in knowing whether anything about the directory
	// has changed since last check, so we use the latest of the two
	// modification times (content modification (mtime) and
	// permission modification (ctime))
	if permissionsChangeTime.After(modTime) {
		modTime = permissionsChangeTime
	}
	stats.ModTime = modTime.UnixNano()

	return stats
}

// pruneCacheCandidates removes the items that we don't want to include in our persistent cache
func (f *Finder) pruneCacheCandidates(items *DirEntries) {

	for _, fileName := range items.FileNames {
		for _, abortedName := range f.cacheMetadata.Config.PruneFiles {
			if fileName == abortedName {
				items.FileNames = []string{}
				items.DirNames = []string{}
				return
			}
		}
	}

	// remove any files that aren't the ones we want to include
	writeIndex := 0
	for _, fileName := range items.FileNames {
		// include only these files
		for _, includedName := range f.cacheMetadata.Config.IncludeFiles {
			if fileName == includedName {
				items.FileNames[writeIndex] = fileName
				writeIndex++
				break
			}
		}
	}
	// resize
	items.FileNames = items.FileNames[:writeIndex]

	writeIndex = 0
	for _, dirName := range items.DirNames {
		items.DirNames[writeIndex] = dirName
		// ignore other dirs that are known to not be inputs to the build process
		include := true
		for _, excludedName := range f.cacheMetadata.Config.ExcludeDirs {
			if dirName == excludedName {
				// don't include
				include = false
				break
			}
		}
		if include {
			writeIndex++
		}
	}
	// resize
	items.DirNames = items.DirNames[:writeIndex]
}

func (f *Finder) listDirsAsync(nodes []*pathMap) {
	f.threadPool.Run(
		func() {
			for i := range nodes {
				f.listDirSync(nodes[i])
			}
		},
	)
}

func (f *Finder) listDirAsync(node *pathMap) {
	f.threadPool.Run(
		func() {
			f.listDirSync(node)
		},
	)
}

func (f *Finder) listDirSync(dir *pathMap) {
	path := dir.path
	children, err := f.filesystem.ReadDir(path)

	if err != nil {
		// possibly record this error
		f.onFsError(path, err)
		// if listing the contents of the directory fails (presumably due to
		// permission denied), then treat the directory as empty
		children = nil
	}

	var subdirs []string
	var subfiles []string

	for _, child := range children {
		linkBits := child.Mode() & os.ModeSymlink
		isLink := linkBits != 0
		if child.IsDir() {
			if !isLink {
				// Skip symlink dirs.
				// We don't have to support symlink dirs because
				// that would cause duplicates.
				subdirs = append(subdirs, child.Name())
			}
		} else {
			// We do have to support symlink files because the link name might be
			// different than the target name
			// (for example, Android.bp -> build/soong/root.bp)
			subfiles = append(subfiles, child.Name())
		}

	}
	parentNode := dir

	entry := &DirEntries{Path: path, DirNames: subdirs, FileNames: subfiles}
	f.pruneCacheCandidates(entry)

	// create a pathMap node for each relevant subdirectory
	relevantChildren := map[string]*pathMap{}
	for _, subdirName := range entry.DirNames {
		childNode, found := parentNode.children[subdirName]
		// if we already knew of this directory, then we already have a request pending to Stat it
		// if we didn't already know of this directory, then we must Stat it now
		if !found {
			childNode = parentNode.newChild(subdirName)
			f.statDirAsync(childNode)
		}
		relevantChildren[subdirName] = childNode
	}
	// Note that in rare cases, it's possible that we're reducing the set of
	// children via this statement, if these are all true:
	// 1. we previously had a cache that knew about subdirectories of parentNode
	// 2. the user created a prune-file (described in pruneCacheCandidates)
	//    inside <parentNode>, which specifies that the contents of parentNode
	//    are to be ignored.
	// The fact that it's possible to remove children here means that *pathMap structs
	// must not be looked up from f.nodes by filepath (and instead must be accessed by
	// direct pointer) until after every listDirSync completes
	parentNode.FileNames = entry.FileNames
	parentNode.children = relevantChildren

}

// listMatches takes a node and a function that specifies which subdirectories and
// files to include, and listMatches returns the matches
func (f *Finder) listMatches(node *pathMap,
	filter WalkFunc) (subDirs []*pathMap, filePaths []string) {
	entries := DirEntries{
		FileNames: node.FileNames,
	}
	entries.DirNames = make([]string, 0, len(node.children))
	for childName := range node.children {
		entries.DirNames = append(entries.DirNames, childName)
	}

	dirNames, fileNames := filter(entries)

	subDirs = []*pathMap{}
	filePaths = make([]string, 0, len(fileNames))
	for _, fileName := range fileNames {
		filePaths = append(filePaths, joinCleanPaths(node.path, fileName))
	}
	subDirs = make([]*pathMap, 0, len(dirNames))
	for _, childName := range dirNames {
		child, ok := node.children[childName]
		if ok {
			subDirs = append(subDirs, child)
		}
	}

	return subDirs, filePaths
}

// findInCacheMultithreaded spawns potentially multiple goroutines with which to search the cache.
func (f *Finder) findInCacheMultithreaded(node *pathMap, filter WalkFunc,
	approxNumThreads int) []string {

	if approxNumThreads < 2 {
		// Done spawning threads; process remaining directories
		return f.findInCacheSinglethreaded(node, filter)
	}

	totalWork := 0
	for _, child := range node.children {
		totalWork += child.approximateNumDescendents
	}
	childrenResults := make(chan []string, len(node.children))

	subDirs, filePaths := f.listMatches(node, filter)

	// process child directories
	for _, child := range subDirs {
		numChildThreads := approxNumThreads * child.approximateNumDescendents / totalWork
		childProcessor := func(child *pathMap) {
			childResults := f.findInCacheMultithreaded(child, filter, numChildThreads)
			childrenResults <- childResults
		}
		// If we're allowed to use more than 1 thread to process this directory,
		// then instead we use 1 thread for each subdirectory.
		// It would be strange to spawn threads for only some subdirectories.
		go childProcessor(child)
	}

	// collect results
	for i := 0; i < len(subDirs); i++ {
		childResults := <-childrenResults
		filePaths = append(filePaths, childResults...)
	}
	close(childrenResults)

	return filePaths
}

// findInCacheSinglethreaded synchronously searches the cache for all matching file paths
// note findInCacheSinglethreaded runs 2X to 4X as fast by being iterative rather than recursive
func (f *Finder) findInCacheSinglethreaded(node *pathMap, filter WalkFunc) []string {
	if node == nil {
		return []string{}
	}

	nodes := []*pathMap{node}
	matches := []string{}

	for len(nodes) > 0 {
		currentNode := nodes[0]
		nodes = nodes[1:]

		subDirs, filePaths := f.listMatches(currentNode, filter)

		nodes = append(nodes, subDirs...)

		matches = append(matches, filePaths...)
	}
	return matches
}
