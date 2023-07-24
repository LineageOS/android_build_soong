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
	"hash/crc32"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"android/soong/response"

	"github.com/google/blueprint/pathtools"

	"android/soong/jar"
	"android/soong/third_party/zip"
)

// Input zip: we can open it, close it, and obtain an array of entries
type InputZip interface {
	Name() string
	Open() error
	Close() error
	Entries() []*zip.File
	IsOpen() bool
}

// An entry that can be written to the output zip
type ZipEntryContents interface {
	String() string
	IsDir() bool
	CRC32() uint32
	Size() uint64
	WriteToZip(dest string, zw *zip.Writer) error
}

// a ZipEntryFromZip is a ZipEntryContents that pulls its content from another zip
// identified by the input zip and the index of the entry in its entries array
type ZipEntryFromZip struct {
	inputZip InputZip
	index    int
	name     string
	isDir    bool
	crc32    uint32
	size     uint64
}

func NewZipEntryFromZip(inputZip InputZip, entryIndex int) *ZipEntryFromZip {
	fi := inputZip.Entries()[entryIndex]
	newEntry := ZipEntryFromZip{inputZip: inputZip,
		index: entryIndex,
		name:  fi.Name,
		isDir: fi.FileInfo().IsDir(),
		crc32: fi.CRC32,
		size:  fi.UncompressedSize64,
	}
	return &newEntry
}

func (ze ZipEntryFromZip) String() string {
	return fmt.Sprintf("%s!%s", ze.inputZip.Name(), ze.name)
}

func (ze ZipEntryFromZip) IsDir() bool {
	return ze.isDir
}

func (ze ZipEntryFromZip) CRC32() uint32 {
	return ze.crc32
}

func (ze ZipEntryFromZip) Size() uint64 {
	return ze.size
}

func (ze ZipEntryFromZip) WriteToZip(dest string, zw *zip.Writer) error {
	if err := ze.inputZip.Open(); err != nil {
		return err
	}
	return zw.CopyFrom(ze.inputZip.Entries()[ze.index], dest)
}

// a ZipEntryFromBuffer is a ZipEntryContents that pulls its content from a []byte
type ZipEntryFromBuffer struct {
	fh      *zip.FileHeader
	content []byte
}

func (be ZipEntryFromBuffer) String() string {
	return "internal buffer"
}

func (be ZipEntryFromBuffer) IsDir() bool {
	return be.fh.FileInfo().IsDir()
}

func (be ZipEntryFromBuffer) CRC32() uint32 {
	return crc32.ChecksumIEEE(be.content)
}

func (be ZipEntryFromBuffer) Size() uint64 {
	return uint64(len(be.content))
}

func (be ZipEntryFromBuffer) WriteToZip(dest string, zw *zip.Writer) error {
	w, err := zw.CreateHeaderAndroid(be.fh)
	if err != nil {
		return err
	}

	if !be.IsDir() {
		_, err = w.Write(be.content)
		if err != nil {
			return err
		}
	}

	return nil
}

// Processing state.
type OutputZip struct {
	outputWriter     *zip.Writer
	stripDirEntries  bool
	emulateJar       bool
	sortEntries      bool
	ignoreDuplicates bool
	excludeDirs      []string
	excludeFiles     []string
	sourceByDest     map[string]ZipEntryContents
}

func NewOutputZip(outputWriter *zip.Writer, sortEntries, emulateJar, stripDirEntries, ignoreDuplicates bool) *OutputZip {
	return &OutputZip{
		outputWriter:     outputWriter,
		stripDirEntries:  stripDirEntries,
		emulateJar:       emulateJar,
		sortEntries:      sortEntries,
		sourceByDest:     make(map[string]ZipEntryContents, 0),
		ignoreDuplicates: ignoreDuplicates,
	}
}

func (oz *OutputZip) setExcludeDirs(excludeDirs []string) {
	oz.excludeDirs = make([]string, len(excludeDirs))
	for i, dir := range excludeDirs {
		oz.excludeDirs[i] = filepath.Clean(dir)
	}
}

func (oz *OutputZip) setExcludeFiles(excludeFiles []string) {
	oz.excludeFiles = excludeFiles
}

// Adds an entry with given name whose source is given ZipEntryContents. Returns old ZipEntryContents
// if entry with given name already exists.
func (oz *OutputZip) addZipEntry(name string, source ZipEntryContents) (ZipEntryContents, error) {
	if existingSource, exists := oz.sourceByDest[name]; exists {
		return existingSource, nil
	}
	oz.sourceByDest[name] = source
	// Delay writing an entry if entries need to be rearranged.
	if oz.emulateJar || oz.sortEntries {
		return nil, nil
	}
	return nil, source.WriteToZip(name, oz.outputWriter)
}

// Adds an entry for the manifest (META-INF/MANIFEST.MF from the given file
func (oz *OutputZip) addManifest(manifestPath string) error {
	if !oz.stripDirEntries {
		if _, err := oz.addZipEntry(jar.MetaDir, ZipEntryFromBuffer{jar.MetaDirFileHeader(), nil}); err != nil {
			return err
		}
	}
	contents, err := ioutil.ReadFile(manifestPath)
	if err == nil {
		fh, buf, err := jar.ManifestFileContents(contents)
		if err == nil {
			_, err = oz.addZipEntry(jar.ManifestFile, ZipEntryFromBuffer{fh, buf})
		}
	}
	return err
}

// Adds an entry with given name and contents read from given file
func (oz *OutputZip) addZipEntryFromFile(name string, path string) error {
	buf, err := ioutil.ReadFile(path)
	if err == nil {
		fh := &zip.FileHeader{
			Name:               name,
			Method:             zip.Store,
			UncompressedSize64: uint64(len(buf)),
		}
		fh.SetMode(0700)
		fh.SetModTime(jar.DefaultTime)
		_, err = oz.addZipEntry(name, ZipEntryFromBuffer{fh, buf})
	}
	return err
}

func (oz *OutputZip) addEmptyEntry(entry string) error {
	var emptyBuf []byte
	fh := &zip.FileHeader{
		Name:               entry,
		Method:             zip.Store,
		UncompressedSize64: uint64(len(emptyBuf)),
	}
	fh.SetMode(0700)
	fh.SetModTime(jar.DefaultTime)
	_, err := oz.addZipEntry(entry, ZipEntryFromBuffer{fh, emptyBuf})
	return err
}

// Returns true if given entry is to be excluded
func (oz *OutputZip) isEntryExcluded(name string) bool {
	for _, dir := range oz.excludeDirs {
		dir = filepath.Clean(dir)
		patterns := []string{
			dir + "/",      // the directory itself
			dir + "/**/*",  // files recursively in the directory
			dir + "/**/*/", // directories recursively in the directory
		}

		for _, pattern := range patterns {
			match, err := pathtools.Match(pattern, name)
			if err != nil {
				panic(fmt.Errorf("%s: %s", err.Error(), pattern))
			}
			if match {
				if oz.emulateJar {
					// When merging jar files, don't strip META-INF/MANIFEST.MF even if stripping META-INF is
					// requested.
					// TODO(ccross): which files does this affect?
					if name != jar.MetaDir && name != jar.ManifestFile {
						return true
					}
				}
				return true
			}
		}
	}

	for _, pattern := range oz.excludeFiles {
		match, err := pathtools.Match(pattern, name)
		if err != nil {
			panic(fmt.Errorf("%s: %s", err.Error(), pattern))
		}
		if match {
			return true
		}
	}
	return false
}

// Creates a zip entry whose contents is an entry from the given input zip.
func (oz *OutputZip) copyEntry(inputZip InputZip, index int) error {
	entry := NewZipEntryFromZip(inputZip, index)
	if oz.stripDirEntries && entry.IsDir() {
		return nil
	}
	existingEntry, err := oz.addZipEntry(entry.name, entry)
	if err != nil {
		return err
	}
	if existingEntry == nil {
		return nil
	}

	// File types should match
	if existingEntry.IsDir() != entry.IsDir() {
		return fmt.Errorf("Directory/file mismatch at %v from %v and %v\n",
			entry.name, existingEntry, entry)
	}

	if oz.ignoreDuplicates ||
		// Skip manifest and module info files that are not from the first input file
		(oz.emulateJar && entry.name == jar.ManifestFile || entry.name == jar.ModuleInfoClass) ||
		// Identical entries
		(existingEntry.CRC32() == entry.CRC32() && existingEntry.Size() == entry.Size()) ||
		// Directory entries
		entry.IsDir() {
		return nil
	}

	return fmt.Errorf("Duplicate path %v found in %v and %v\n", entry.name, existingEntry, inputZip.Name())
}

func (oz *OutputZip) entriesArray() []string {
	entries := make([]string, len(oz.sourceByDest))
	i := 0
	for entry := range oz.sourceByDest {
		entries[i] = entry
		i++
	}
	return entries
}

func (oz *OutputZip) jarSorted() []string {
	entries := oz.entriesArray()
	sort.SliceStable(entries, func(i, j int) bool { return jar.EntryNamesLess(entries[i], entries[j]) })
	return entries
}

func (oz *OutputZip) alphanumericSorted() []string {
	entries := oz.entriesArray()
	sort.Strings(entries)
	return entries
}

func (oz *OutputZip) writeEntries(entries []string) error {
	for _, entry := range entries {
		source, _ := oz.sourceByDest[entry]
		if err := source.WriteToZip(entry, oz.outputWriter); err != nil {
			return err
		}
	}
	return nil
}

func (oz *OutputZip) getUninitializedPythonPackages(inputZips []InputZip) ([]string, error) {
	// the runfiles packages needs to be populated with "__init__.py".
	// the runfiles dirs have been treated as packages.
	allPackages := make(map[string]bool)
	initedPackages := make(map[string]bool)
	getPackage := func(path string) string {
		ret := filepath.Dir(path)
		// filepath.Dir("abc") -> "." and filepath.Dir("/abc") -> "/".
		if ret == "." || ret == "/" {
			return ""
		}
		return ret
	}

	// put existing __init__.py files to a set first. This set is used for preventing
	// generated __init__.py files from overwriting existing ones.
	for _, inputZip := range inputZips {
		if err := inputZip.Open(); err != nil {
			return nil, err
		}
		for _, file := range inputZip.Entries() {
			pyPkg := getPackage(file.Name)
			baseName := filepath.Base(file.Name)
			if baseName == "__init__.py" || baseName == "__init__.pyc" {
				if _, found := initedPackages[pyPkg]; found {
					panic(fmt.Errorf("found __init__.py path duplicates during pars merging: %q", file.Name))
				}
				initedPackages[pyPkg] = true
			}
			for pyPkg != "" {
				if _, found := allPackages[pyPkg]; found {
					break
				}
				allPackages[pyPkg] = true
				pyPkg = getPackage(pyPkg)
			}
		}
	}
	noInitPackages := make([]string, 0)
	for pyPkg := range allPackages {
		if _, found := initedPackages[pyPkg]; !found {
			noInitPackages = append(noInitPackages, pyPkg)
		}
	}
	return noInitPackages, nil
}

// An InputZip owned by the InputZipsManager. Opened ManagedInputZip's are chained in the open order.
type ManagedInputZip struct {
	owner        *InputZipsManager
	realInputZip InputZip
	older        *ManagedInputZip
	newer        *ManagedInputZip
}

// Maintains the array of ManagedInputZips, keeping track of open input ones. When an InputZip is opened,
// may close some other InputZip to limit the number of open ones.
type InputZipsManager struct {
	inputZips     []*ManagedInputZip
	nOpenZips     int
	maxOpenZips   int
	openInputZips *ManagedInputZip
}

func (miz *ManagedInputZip) unlink() {
	olderMiz := miz.older
	newerMiz := miz.newer
	if newerMiz.older != miz || olderMiz.newer != miz {
		panic(fmt.Errorf("removing %p:%#v: broken list between %p:%#v and %p:%#v",
			miz, miz, newerMiz, newerMiz, olderMiz, olderMiz))
	}
	olderMiz.newer = newerMiz
	newerMiz.older = olderMiz
	miz.newer = nil
	miz.older = nil
}

func (miz *ManagedInputZip) link(olderMiz *ManagedInputZip) {
	if olderMiz.newer != nil || olderMiz.older != nil {
		panic(fmt.Errorf("inputZip is already open"))
	}
	oldOlderMiz := miz.older
	if oldOlderMiz.newer != miz {
		panic(fmt.Errorf("broken list between %p:%#v and %p:%#v", miz, miz, oldOlderMiz, oldOlderMiz))
	}
	miz.older = olderMiz
	olderMiz.older = oldOlderMiz
	oldOlderMiz.newer = olderMiz
	olderMiz.newer = miz
}

func NewInputZipsManager(nInputZips, maxOpenZips int) *InputZipsManager {
	if maxOpenZips < 3 {
		panic(fmt.Errorf("open zips limit should be above 3"))
	}
	// In the fake element .older points to the most recently opened InputZip, and .newer points to the oldest.
	head := new(ManagedInputZip)
	head.older = head
	head.newer = head
	return &InputZipsManager{
		inputZips:     make([]*ManagedInputZip, 0, nInputZips),
		maxOpenZips:   maxOpenZips,
		openInputZips: head,
	}
}

// InputZip factory
func (izm *InputZipsManager) Manage(inz InputZip) InputZip {
	iz := &ManagedInputZip{owner: izm, realInputZip: inz}
	izm.inputZips = append(izm.inputZips, iz)
	return iz
}

// Opens or reopens ManagedInputZip.
func (izm *InputZipsManager) reopen(miz *ManagedInputZip) error {
	if miz.realInputZip.IsOpen() {
		if miz != izm.openInputZips {
			miz.unlink()
			izm.openInputZips.link(miz)
		}
		return nil
	}
	if izm.nOpenZips >= izm.maxOpenZips {
		if err := izm.close(izm.openInputZips.older); err != nil {
			return err
		}
	}
	if err := miz.realInputZip.Open(); err != nil {
		return err
	}
	izm.openInputZips.link(miz)
	izm.nOpenZips++
	return nil
}

func (izm *InputZipsManager) close(miz *ManagedInputZip) error {
	if miz.IsOpen() {
		err := miz.realInputZip.Close()
		izm.nOpenZips--
		miz.unlink()
		return err
	}
	return nil
}

// Checks that openInputZips deque is valid
func (izm *InputZipsManager) checkOpenZipsDeque() {
	nReallyOpen := 0
	el := izm.openInputZips
	for {
		elNext := el.older
		if elNext.newer != el {
			panic(fmt.Errorf("Element:\n  %p: %v\nNext:\n  %p %v", el, el, elNext, elNext))
		}
		if elNext == izm.openInputZips {
			break
		}
		el = elNext
		if !el.IsOpen() {
			panic(fmt.Errorf("Found unopened element"))
		}
		nReallyOpen++
		if nReallyOpen > izm.nOpenZips {
			panic(fmt.Errorf("found %d open zips, should be %d", nReallyOpen, izm.nOpenZips))
		}
	}
	if nReallyOpen > izm.nOpenZips {
		panic(fmt.Errorf("found %d open zips, should be %d", nReallyOpen, izm.nOpenZips))
	}
}

func (miz *ManagedInputZip) Name() string {
	return miz.realInputZip.Name()
}

func (miz *ManagedInputZip) Open() error {
	return miz.owner.reopen(miz)
}

func (miz *ManagedInputZip) Close() error {
	return miz.owner.close(miz)
}

func (miz *ManagedInputZip) IsOpen() bool {
	return miz.realInputZip.IsOpen()
}

func (miz *ManagedInputZip) Entries() []*zip.File {
	if !miz.IsOpen() {
		panic(fmt.Errorf("%s: is not open", miz.Name()))
	}
	return miz.realInputZip.Entries()
}

// Actual processing.
func mergeZips(inputZips []InputZip, writer *zip.Writer, manifest, pyMain string,
	sortEntries, emulateJar, emulatePar, stripDirEntries, ignoreDuplicates bool,
	excludeFiles, excludeDirs []string, zipsToNotStrip map[string]bool) error {

	out := NewOutputZip(writer, sortEntries, emulateJar, stripDirEntries, ignoreDuplicates)
	out.setExcludeFiles(excludeFiles)
	out.setExcludeDirs(excludeDirs)
	if manifest != "" {
		if err := out.addManifest(manifest); err != nil {
			return err
		}
	}
	if pyMain != "" {
		if err := out.addZipEntryFromFile("__main__.py", pyMain); err != nil {
			return err
		}
	}

	if emulatePar {
		noInitPackages, err := out.getUninitializedPythonPackages(inputZips)
		if err != nil {
			return err
		}
		for _, uninitializedPyPackage := range noInitPackages {
			if err = out.addEmptyEntry(filepath.Join(uninitializedPyPackage, "__init__.py")); err != nil {
				return err
			}
		}
	}

	var jarServices jar.Services

	// Finally, add entries from all the input zips.
	for _, inputZip := range inputZips {
		_, copyFully := zipsToNotStrip[inputZip.Name()]
		if err := inputZip.Open(); err != nil {
			return err
		}

		for i, entry := range inputZip.Entries() {
			if emulateJar && jarServices.IsServiceFile(entry) {
				// If this is a jar, collect service files to combine  instead of adding them to the zip.
				err := jarServices.AddServiceFile(entry)
				if err != nil {
					return err
				}
				continue
			}
			if copyFully || !out.isEntryExcluded(entry.Name) {
				if err := out.copyEntry(inputZip, i); err != nil {
					return err
				}
			}
		}
		// Unless we need to rearrange the entries, the input zip can now be closed.
		if !(emulateJar || sortEntries) {
			if err := inputZip.Close(); err != nil {
				return err
			}
		}
	}

	if emulateJar {
		// Combine all the service files into a single list of combined service files and add them to the zip.
		for _, serviceFile := range jarServices.ServiceFiles() {
			_, err := out.addZipEntry(serviceFile.Name, ZipEntryFromBuffer{
				fh:      serviceFile.FileHeader,
				content: serviceFile.Contents,
			})
			if err != nil {
				return err
			}
		}
		return out.writeEntries(out.jarSorted())
	} else if sortEntries {
		return out.writeEntries(out.alphanumericSorted())
	}
	return nil
}

// Process command line
type fileList []string

func (f *fileList) String() string {
	return `""`
}

func (f *fileList) Set(name string) error {
	*f = append(*f, filepath.Clean(name))

	return nil
}

type zipsToNotStripSet map[string]bool

func (s zipsToNotStripSet) String() string {
	return `""`
}

func (s zipsToNotStripSet) Set(path string) error {
	s[path] = true
	return nil
}

var (
	sortEntries      = flag.Bool("s", false, "sort entries (defaults to the order from the input zip files)")
	emulateJar       = flag.Bool("j", false, "sort zip entries using jar ordering (META-INF first)")
	emulatePar       = flag.Bool("p", false, "merge zip entries based on par format")
	excludeDirs      fileList
	excludeFiles     fileList
	zipsToNotStrip   = make(zipsToNotStripSet)
	stripDirEntries  = flag.Bool("D", false, "strip directory entries from the output zip file")
	manifest         = flag.String("m", "", "manifest file to insert in jar")
	pyMain           = flag.String("pm", "", "__main__.py file to insert in par")
	prefix           = flag.String("prefix", "", "A file to prefix to the zip file")
	ignoreDuplicates = flag.Bool("ignore-duplicates", false, "take each entry from the first zip it exists in and don't warn")
)

func init() {
	flag.Var(&excludeDirs, "stripDir", "directories to be excluded from the output zip, accepts wildcards")
	flag.Var(&excludeFiles, "stripFile", "files to be excluded from the output zip, accepts wildcards")
	flag.Var(&zipsToNotStrip, "zipToNotStrip", "the input zip file which is not applicable for stripping")
}

type FileInputZip struct {
	name   string
	reader *zip.ReadCloser
}

func (fiz *FileInputZip) Name() string {
	return fiz.name
}

func (fiz *FileInputZip) Close() error {
	if fiz.IsOpen() {
		reader := fiz.reader
		fiz.reader = nil
		return reader.Close()
	}
	return nil
}

func (fiz *FileInputZip) Entries() []*zip.File {
	if !fiz.IsOpen() {
		panic(fmt.Errorf("%s: is not open", fiz.Name()))
	}
	return fiz.reader.File
}

func (fiz *FileInputZip) IsOpen() bool {
	return fiz.reader != nil
}

func (fiz *FileInputZip) Open() error {
	if fiz.IsOpen() {
		return nil
	}
	var err error
	if fiz.reader, err = zip.OpenReader(fiz.Name()); err != nil {
		return fmt.Errorf("%s: %s", fiz.Name(), err.Error())
	}
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: merge_zips [-jpsD] [-m manifest] [--prefix script] [-pm __main__.py] OutputZip [inputs...]")
		flag.PrintDefaults()
	}

	// parse args
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		os.Exit(1)
	}
	outputPath := args[0]
	inputs := make([]string, 0)
	for _, input := range args[1:] {
		if input[0] == '@' {
			f, err := os.Open(strings.TrimPrefix(input[1:], "@"))
			if err != nil {
				log.Fatal(err)
			}

			rspInputs, err := response.ReadRspFile(f)
			f.Close()
			if err != nil {
				log.Fatal(err)
			}
			inputs = append(inputs, rspInputs...)
		} else {
			inputs = append(inputs, input)
		}
	}

	log.SetFlags(log.Lshortfile)

	// make writer
	outputZip, err := os.Create(outputPath)
	if err != nil {
		log.Fatal(err)
	}
	defer outputZip.Close()

	var offset int64
	if *prefix != "" {
		prefixFile, err := os.Open(*prefix)
		if err != nil {
			log.Fatal(err)
		}
		offset, err = io.Copy(outputZip, prefixFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	writer := zip.NewWriter(outputZip)
	defer func() {
		err := writer.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	writer.SetOffset(offset)

	if *manifest != "" && !*emulateJar {
		log.Fatal(errors.New("must specify -j when specifying a manifest via -m"))
	}

	if *pyMain != "" && !*emulatePar {
		log.Fatal(errors.New("must specify -p when specifying a Python __main__.py via -pm"))
	}

	// do merge
	inputZipsManager := NewInputZipsManager(len(inputs), 1000)
	inputZips := make([]InputZip, len(inputs))
	for i, input := range inputs {
		inputZips[i] = inputZipsManager.Manage(&FileInputZip{name: input})
	}
	err = mergeZips(inputZips, writer, *manifest, *pyMain, *sortEntries, *emulateJar, *emulatePar,
		*stripDirEntries, *ignoreDuplicates, []string(excludeFiles), []string(excludeDirs),
		map[string]bool(zipsToNotStrip))
	if err != nil {
		log.Fatal(err)
	}
}
