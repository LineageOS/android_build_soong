// Copyright 2015 Google Inc. All rights reserved.
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

package zip

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"android/soong/response"

	"github.com/google/blueprint/pathtools"

	"android/soong/jar"
	"android/soong/third_party/zip"
)

// Sha256HeaderID is a custom Header ID for the `extra` field in
// the file header to store the SHA checksum.
const Sha256HeaderID = 0x4967

// Sha256HeaderSignature is the signature to verify that the extra
// data block is used to store the SHA checksum.
const Sha256HeaderSignature = 0x9514

// Block size used during parallel compression of a single file.
const parallelBlockSize = 1 * 1024 * 1024 // 1MB

// Minimum file size to use parallel compression. It requires more
// flate.Writer allocations, since we can't change the dictionary
// during Reset
const minParallelFileSize = parallelBlockSize * 6

// Size of the ZIP compression window (32KB)
const windowSize = 32 * 1024

type nopCloser struct {
	io.Writer
}

func (nopCloser) Close() error {
	return nil
}

type byteReaderCloser struct {
	*bytes.Reader
	io.Closer
}

type pathMapping struct {
	dest, src string
	zipMethod uint16
}

type FileArg struct {
	PathPrefixInZip, SourcePrefixToStrip string
	ExplicitPathInZip                    string
	SourceFiles                          []string
	JunkPaths                            bool
	GlobDir                              string
}

type FileArgsBuilder struct {
	state FileArg
	err   error
	fs    pathtools.FileSystem

	fileArgs []FileArg
}

func NewFileArgsBuilder() *FileArgsBuilder {
	return &FileArgsBuilder{
		fs: pathtools.OsFs,
	}
}

func (b *FileArgsBuilder) JunkPaths(v bool) *FileArgsBuilder {
	b.state.JunkPaths = v
	b.state.SourcePrefixToStrip = ""
	return b
}

func (b *FileArgsBuilder) SourcePrefixToStrip(prefixToStrip string) *FileArgsBuilder {
	b.state.JunkPaths = false
	b.state.SourcePrefixToStrip = prefixToStrip
	return b
}

func (b *FileArgsBuilder) PathPrefixInZip(rootPrefix string) *FileArgsBuilder {
	b.state.PathPrefixInZip = rootPrefix
	return b
}

func (b *FileArgsBuilder) File(name string) *FileArgsBuilder {
	if b.err != nil {
		return b
	}

	arg := b.state
	arg.SourceFiles = []string{name}
	b.fileArgs = append(b.fileArgs, arg)

	if b.state.ExplicitPathInZip != "" {
		b.state.ExplicitPathInZip = ""
	}
	return b
}

func (b *FileArgsBuilder) Dir(name string) *FileArgsBuilder {
	if b.err != nil {
		return b
	}

	arg := b.state
	arg.GlobDir = name
	b.fileArgs = append(b.fileArgs, arg)
	return b
}

// List reads the file names from the given file and adds them to the source files list.
func (b *FileArgsBuilder) List(name string) *FileArgsBuilder {
	if b.err != nil {
		return b
	}

	f, err := b.fs.Open(name)
	if err != nil {
		b.err = err
		return b
	}
	defer f.Close()

	list, err := ioutil.ReadAll(f)
	if err != nil {
		b.err = err
		return b
	}

	arg := b.state
	arg.SourceFiles = strings.Fields(string(list))
	b.fileArgs = append(b.fileArgs, arg)
	return b
}

// RspFile reads the file names from given .rsp file and adds them to the source files list.
func (b *FileArgsBuilder) RspFile(name string) *FileArgsBuilder {
	if b.err != nil {
		return b
	}

	f, err := b.fs.Open(name)
	if err != nil {
		b.err = err
		return b
	}
	defer f.Close()

	arg := b.state
	arg.SourceFiles, err = response.ReadRspFile(f)
	if err != nil {
		b.err = err
		return b
	}
	for i := range arg.SourceFiles {
		arg.SourceFiles[i] = pathtools.MatchEscape(arg.SourceFiles[i])
	}
	b.fileArgs = append(b.fileArgs, arg)
	return b
}

// ExplicitPathInZip sets the path in the zip file for the next File call.
func (b *FileArgsBuilder) ExplicitPathInZip(s string) *FileArgsBuilder {
	b.state.ExplicitPathInZip = s
	return b
}

func (b *FileArgsBuilder) Error() error {
	if b == nil {
		return nil
	}
	return b.err
}

func (b *FileArgsBuilder) FileArgs() []FileArg {
	if b == nil {
		return nil
	}
	return b.fileArgs
}

type IncorrectRelativeRootError struct {
	RelativeRoot string
	Path         string
}

func (x IncorrectRelativeRootError) Error() string {
	return fmt.Sprintf("path %q is outside relative root %q", x.Path, x.RelativeRoot)
}

type ConflictingFileError struct {
	Dest string
	Prev string
	Src  string
}

func (x ConflictingFileError) Error() string {
	return fmt.Sprintf("destination %q has two files %q and %q", x.Dest, x.Prev, x.Src)
}

type ZipWriter struct {
	time         time.Time
	createdFiles map[string]string
	createdDirs  map[string]string
	directories  bool

	errors   chan error
	writeOps chan chan *zipEntry

	cpuRateLimiter    *CPURateLimiter
	memoryRateLimiter *MemoryRateLimiter

	compressorPool sync.Pool
	compLevel      int

	followSymlinks     pathtools.ShouldFollowSymlinks
	ignoreMissingFiles bool

	stderr io.Writer
	fs     pathtools.FileSystem

	sha256Checksum bool
}

type zipEntry struct {
	fh *zip.FileHeader

	// List of delayed io.Reader
	futureReaders chan chan io.Reader

	// Only used for passing into the MemoryRateLimiter to ensure we
	// release as much memory as much as we request
	allocatedSize int64
}

type ZipArgs struct {
	FileArgs                 []FileArg
	OutputFilePath           string
	EmulateJar               bool
	SrcJar                   bool
	AddDirectoryEntriesToZip bool
	CompressionLevel         int
	ManifestSourcePath       string
	NumParallelJobs          int
	NonDeflatedFiles         map[string]bool
	WriteIfChanged           bool
	StoreSymlinks            bool
	IgnoreMissingFiles       bool
	Sha256Checksum           bool
	DoNotWrite               bool
	Quiet                    bool

	Stderr     io.Writer
	Filesystem pathtools.FileSystem
}

func zipTo(args ZipArgs, w io.Writer) error {
	if args.EmulateJar {
		args.AddDirectoryEntriesToZip = true
	}

	// Have Glob follow symlinks if they are not being stored as symlinks in the zip file.
	followSymlinks := pathtools.ShouldFollowSymlinks(!args.StoreSymlinks)

	z := &ZipWriter{
		time:               jar.DefaultTime,
		createdDirs:        make(map[string]string),
		createdFiles:       make(map[string]string),
		directories:        args.AddDirectoryEntriesToZip,
		compLevel:          args.CompressionLevel,
		followSymlinks:     followSymlinks,
		ignoreMissingFiles: args.IgnoreMissingFiles,
		stderr:             args.Stderr,
		fs:                 args.Filesystem,
		sha256Checksum:     args.Sha256Checksum,
	}

	if z.fs == nil {
		z.fs = pathtools.OsFs
	}

	if z.stderr == nil {
		z.stderr = os.Stderr
	}

	pathMappings := []pathMapping{}

	noCompression := args.CompressionLevel == 0

	for _, fa := range args.FileArgs {
		var srcs []string
		for _, s := range fa.SourceFiles {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}

			result, err := z.fs.Glob(s, nil, followSymlinks)
			if err != nil {
				return err
			}
			if len(result.Matches) == 0 {
				err := &os.PathError{
					Op:   "lstat",
					Path: s,
					Err:  os.ErrNotExist,
				}
				if args.IgnoreMissingFiles {
					if !args.Quiet {
						fmt.Fprintln(z.stderr, "warning:", err)
					}
				} else {
					return err
				}
			}
			srcs = append(srcs, result.Matches...)
		}
		if fa.GlobDir != "" {
			if exists, isDir, err := z.fs.Exists(fa.GlobDir); err != nil {
				return err
			} else if !exists && !args.IgnoreMissingFiles {
				err := &os.PathError{
					Op:   "lstat",
					Path: fa.GlobDir,
					Err:  os.ErrNotExist,
				}
				if args.IgnoreMissingFiles {
					if !args.Quiet {
						fmt.Fprintln(z.stderr, "warning:", err)
					}
				} else {
					return err
				}
			} else if !isDir && !args.IgnoreMissingFiles {
				err := &os.PathError{
					Op:   "lstat",
					Path: fa.GlobDir,
					Err:  syscall.ENOTDIR,
				}
				if args.IgnoreMissingFiles {
					if !args.Quiet {
						fmt.Fprintln(z.stderr, "warning:", err)
					}
				} else {
					return err
				}
			}
			result, err := z.fs.Glob(filepath.Join(fa.GlobDir, "**/*"), nil, followSymlinks)
			if err != nil {
				return err
			}
			srcs = append(srcs, result.Matches...)
		}
		for _, src := range srcs {
			err := fillPathPairs(fa, src, &pathMappings, args.NonDeflatedFiles, noCompression)
			if err != nil {
				return err
			}
		}
	}

	return z.write(w, pathMappings, args.ManifestSourcePath, args.EmulateJar, args.SrcJar, args.NumParallelJobs)
}

// Zip creates an output zip archive from given sources.
func Zip(args ZipArgs) error {
	if args.OutputFilePath == "" {
		return fmt.Errorf("output file path must be nonempty")
	}

	buf := &bytes.Buffer{}
	var out io.Writer = buf

	var zipErr error

	if args.DoNotWrite {
		out = io.Discard
	} else if !args.WriteIfChanged {
		f, err := os.Create(args.OutputFilePath)
		if err != nil {
			return err
		}

		defer f.Close()
		defer func() {
			if zipErr != nil {
				os.Remove(args.OutputFilePath)
			}
		}()

		out = f
	}

	zipErr = zipTo(args, out)
	if zipErr != nil {
		return zipErr
	}

	if args.WriteIfChanged && !args.DoNotWrite {
		err := pathtools.WriteFileIfChanged(args.OutputFilePath, buf.Bytes(), 0666)
		if err != nil {
			return err
		}
	}

	return nil
}

func fillPathPairs(fa FileArg, src string, pathMappings *[]pathMapping,
	nonDeflatedFiles map[string]bool, noCompression bool) error {

	var dest string

	if fa.ExplicitPathInZip != "" {
		dest = fa.ExplicitPathInZip
	} else if fa.JunkPaths {
		dest = filepath.Base(src)
	} else {
		var err error
		dest, err = filepath.Rel(fa.SourcePrefixToStrip, src)
		if err != nil {
			return err
		}
		if strings.HasPrefix(dest, "../") {
			return IncorrectRelativeRootError{
				Path:         src,
				RelativeRoot: fa.SourcePrefixToStrip,
			}
		}
	}
	dest = filepath.Join(fa.PathPrefixInZip, dest)

	zipMethod := zip.Deflate
	if _, found := nonDeflatedFiles[dest]; found || noCompression {
		zipMethod = zip.Store
	}
	*pathMappings = append(*pathMappings,
		pathMapping{dest: dest, src: src, zipMethod: zipMethod})

	return nil
}

func jarSort(mappings []pathMapping) {
	sort.SliceStable(mappings, func(i int, j int) bool {
		return jar.EntryNamesLess(mappings[i].dest, mappings[j].dest)
	})
}

func (z *ZipWriter) write(f io.Writer, pathMappings []pathMapping, manifest string, emulateJar, srcJar bool,
	parallelJobs int) error {

	z.errors = make(chan error)
	defer close(z.errors)

	// This channel size can be essentially unlimited -- it's used as a fifo
	// queue decouple the CPU and IO loads. Directories don't require any
	// compression time, but still cost some IO. Similar with small files that
	// can be very fast to compress. Some files that are more difficult to
	// compress won't take a corresponding longer time writing out.
	//
	// The optimum size here depends on your CPU and IO characteristics, and
	// the the layout of your zip file. 1000 was chosen mostly at random as
	// something that worked reasonably well for a test file.
	//
	// The RateLimit object will put the upper bounds on the number of
	// parallel compressions and outstanding buffers.
	z.writeOps = make(chan chan *zipEntry, 1000)
	z.cpuRateLimiter = NewCPURateLimiter(int64(parallelJobs))
	z.memoryRateLimiter = NewMemoryRateLimiter(0)
	defer func() {
		z.cpuRateLimiter.Stop()
		z.memoryRateLimiter.Stop()
	}()

	if manifest != "" && !emulateJar {
		return errors.New("must specify --jar when specifying a manifest via -m")
	}

	if emulateJar {
		// manifest may be empty, in which case addManifest will fill in a default
		pathMappings = append(pathMappings, pathMapping{jar.ManifestFile, manifest, zip.Deflate})

		jarSort(pathMappings)
	}

	go func() {
		var err error
		defer close(z.writeOps)

		for _, ele := range pathMappings {
			if emulateJar && ele.dest == jar.ManifestFile {
				err = z.addManifest(ele.dest, ele.src, ele.zipMethod)
			} else {
				err = z.addFile(ele.dest, ele.src, ele.zipMethod, emulateJar, srcJar)
			}
			if err != nil {
				z.errors <- err
				return
			}
		}
	}()

	zipw := zip.NewWriter(f)

	var currentWriteOpChan chan *zipEntry
	var currentWriter io.WriteCloser
	var currentReaders chan chan io.Reader
	var currentReader chan io.Reader
	var done bool

	for !done {
		var writeOpsChan chan chan *zipEntry
		var writeOpChan chan *zipEntry
		var readersChan chan chan io.Reader

		if currentReader != nil {
			// Only read and process errors
		} else if currentReaders != nil {
			readersChan = currentReaders
		} else if currentWriteOpChan != nil {
			writeOpChan = currentWriteOpChan
		} else {
			writeOpsChan = z.writeOps
		}

		select {
		case writeOp, ok := <-writeOpsChan:
			if !ok {
				done = true
			}

			currentWriteOpChan = writeOp

		case op := <-writeOpChan:
			currentWriteOpChan = nil

			var err error
			if op.fh.Method == zip.Deflate {
				currentWriter, err = zipw.CreateCompressedHeader(op.fh)
			} else {
				var zw io.Writer

				op.fh.CompressedSize64 = op.fh.UncompressedSize64

				zw, err = zipw.CreateHeaderAndroid(op.fh)
				currentWriter = nopCloser{zw}
			}
			if err != nil {
				return err
			}

			currentReaders = op.futureReaders
			if op.futureReaders == nil {
				currentWriter.Close()
				currentWriter = nil
			}
			z.memoryRateLimiter.Finish(op.allocatedSize)

		case futureReader, ok := <-readersChan:
			if !ok {
				// Done with reading
				currentWriter.Close()
				currentWriter = nil
				currentReaders = nil
			}

			currentReader = futureReader

		case reader := <-currentReader:
			_, err := io.Copy(currentWriter, reader)
			if err != nil {
				return err
			}

			currentReader = nil

		case err := <-z.errors:
			return err
		}
	}

	// One last chance to catch an error
	select {
	case err := <-z.errors:
		return err
	default:
		zipw.Close()
		return nil
	}
}

// imports (possibly with compression) <src> into the zip at sub-path <dest>
func (z *ZipWriter) addFile(dest, src string, method uint16, emulateJar, srcJar bool) error {
	var fileSize int64
	var executable bool

	var s os.FileInfo
	var err error
	if z.followSymlinks {
		s, err = z.fs.Stat(src)
	} else {
		s, err = z.fs.Lstat(src)
	}

	if err != nil {
		if os.IsNotExist(err) && z.ignoreMissingFiles {
			fmt.Fprintln(z.stderr, "warning:", err)
			return nil
		}
		return err
	}

	createParentDirs := func(dest, src string) error {
		if err := z.writeDirectory(filepath.Dir(dest), src, emulateJar); err != nil {
			return err
		}

		if prev, exists := z.createdDirs[dest]; exists {
			return fmt.Errorf("destination %q is both a directory %q and a file %q", dest, prev, src)
		}

		return nil
	}

	checkDuplicateFiles := func(dest, src string) (bool, error) {
		if prev, exists := z.createdFiles[dest]; exists {
			if prev != src {
				return true, ConflictingFileError{
					Dest: dest,
					Prev: prev,
					Src:  src,
				}
			}
			return true, nil
		}

		z.createdFiles[dest] = src
		return false, nil
	}

	if s.IsDir() {
		if z.directories {
			return z.writeDirectory(dest, src, emulateJar)
		}
		return nil
	} else if s.Mode()&os.ModeSymlink != 0 {
		err = createParentDirs(dest, src)
		if err != nil {
			return err
		}

		duplicate, err := checkDuplicateFiles(dest, src)
		if err != nil {
			return err
		}
		if duplicate {
			return nil
		}

		return z.writeSymlink(dest, src)
	} else if s.Mode().IsRegular() {
		r, err := z.fs.Open(src)
		if err != nil {
			return err
		}

		if srcJar && filepath.Ext(src) == ".java" {
			// rewrite the destination using the package path if it can be determined
			pkg, err := jar.JavaPackage(r, src)
			if err != nil {
				// ignore errors for now, leaving the file at in its original location in the zip
			} else {
				dest = filepath.Join(filepath.Join(strings.Split(pkg, ".")...), filepath.Base(src))
			}

			_, err = r.Seek(0, io.SeekStart)
			if err != nil {
				return err
			}
		}

		fileSize = s.Size()
		executable = s.Mode()&0100 != 0

		header := &zip.FileHeader{
			Name:               dest,
			Method:             method,
			UncompressedSize64: uint64(fileSize),
		}

		mode := os.FileMode(0644)
		if executable {
			mode = 0755
		}
		header.SetMode(mode)

		err = createParentDirs(dest, src)
		if err != nil {
			return err
		}

		duplicate, err := checkDuplicateFiles(dest, src)
		if err != nil {
			return err
		}
		if duplicate {
			return nil
		}

		return z.writeFileContents(header, r)
	} else {
		return fmt.Errorf("%s is not a file, directory, or symlink", src)
	}
}

func (z *ZipWriter) addManifest(dest string, src string, _ uint16) error {
	if prev, exists := z.createdDirs[dest]; exists {
		return fmt.Errorf("destination %q is both a directory %q and a file %q", dest, prev, src)
	}
	if prev, exists := z.createdFiles[dest]; exists {
		if prev != src {
			return ConflictingFileError{
				Dest: dest,
				Prev: prev,
				Src:  src,
			}
		}
		return nil
	}

	if err := z.writeDirectory(filepath.Dir(dest), src, true); err != nil {
		return err
	}

	var contents []byte
	if src != "" {
		f, err := z.fs.Open(src)
		if err != nil {
			return err
		}

		contents, err = ioutil.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
	}

	fh, buf, err := jar.ManifestFileContents(contents)
	if err != nil {
		return err
	}

	reader := &byteReaderCloser{bytes.NewReader(buf), ioutil.NopCloser(nil)}

	return z.writeFileContents(fh, reader)
}

func (z *ZipWriter) writeFileContents(header *zip.FileHeader, r pathtools.ReaderAtSeekerCloser) (err error) {

	header.SetModTime(z.time)

	compressChan := make(chan *zipEntry, 1)
	z.writeOps <- compressChan

	// Pre-fill a zipEntry, it will be sent in the compressChan once
	// we're sure about the Method and CRC.
	ze := &zipEntry{
		fh: header,
	}

	ze.allocatedSize = int64(header.UncompressedSize64)
	z.cpuRateLimiter.Request()
	z.memoryRateLimiter.Request(ze.allocatedSize)

	fileSize := int64(header.UncompressedSize64)
	if fileSize == 0 {
		fileSize = int64(header.UncompressedSize)
	}

	if header.Method == zip.Deflate && fileSize >= minParallelFileSize {
		wg := new(sync.WaitGroup)

		// Allocate enough buffer to hold all readers. We'll limit
		// this based on actual buffer sizes in RateLimit.
		ze.futureReaders = make(chan chan io.Reader, (fileSize/parallelBlockSize)+1)

		// Calculate the CRC and SHA256 in the background, since reading
		// the entire file could take a while.
		//
		// We could split this up into chunks as well, but it's faster
		// than the compression. Due to the Go Zip API, we also need to
		// know the result before we can begin writing the compressed
		// data out to the zipfile.
		//
		// We calculate SHA256 only if `-sha256` is set.
		wg.Add(1)
		go z.checksumFileAsync(r, ze, compressChan, wg)

		for start := int64(0); start < fileSize; start += parallelBlockSize {
			sr := io.NewSectionReader(r, start, parallelBlockSize)
			resultChan := make(chan io.Reader, 1)
			ze.futureReaders <- resultChan

			z.cpuRateLimiter.Request()

			last := !(start+parallelBlockSize < fileSize)
			var dict []byte
			if start >= windowSize {
				dict, err = ioutil.ReadAll(io.NewSectionReader(r, start-windowSize, windowSize))
				if err != nil {
					return err
				}
			}

			wg.Add(1)
			go z.compressPartialFile(sr, dict, last, resultChan, wg)
		}

		close(ze.futureReaders)

		// Close the file handle after all readers are done
		go func(wg *sync.WaitGroup, closer io.Closer) {
			wg.Wait()
			closer.Close()
		}(wg, r)
	} else {
		go func() {
			z.compressWholeFile(ze, r, compressChan)
			r.Close()
		}()
	}

	return nil
}

func (z *ZipWriter) checksumFileAsync(r io.ReadSeeker, ze *zipEntry, resultChan chan *zipEntry, wg *sync.WaitGroup) {
	defer wg.Done()
	defer z.cpuRateLimiter.Finish()

	z.checksumFile(r, ze)

	resultChan <- ze
	close(resultChan)
}

func (z *ZipWriter) checksumFile(r io.ReadSeeker, ze *zipEntry) {
	crc := crc32.NewIEEE()
	writers := []io.Writer{crc}

	var shaHasher hash.Hash
	if z.sha256Checksum && !ze.fh.Mode().IsDir() {
		shaHasher = sha256.New()
		writers = append(writers, shaHasher)
	}

	w := io.MultiWriter(writers...)

	_, err := io.Copy(w, r)
	if err != nil {
		z.errors <- err
		return
	}

	ze.fh.CRC32 = crc.Sum32()
	if shaHasher != nil {
		z.appendSHAToExtra(ze, shaHasher.Sum(nil))
	}
}

func (z *ZipWriter) appendSHAToExtra(ze *zipEntry, checksum []byte) {
	// The block of SHA256 checksum consist of:
	// - Header ID, equals to Sha256HeaderID (2 bytes)
	// - Data size (2 bytes)
	// - Data block:
	//   - Signature, equals to Sha256HeaderSignature (2 bytes)
	//   - Data, SHA checksum value
	var buf []byte
	buf = binary.LittleEndian.AppendUint16(buf, Sha256HeaderID)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(checksum)+2))
	buf = binary.LittleEndian.AppendUint16(buf, Sha256HeaderSignature)
	buf = append(buf, checksum...)
	ze.fh.Extra = append(ze.fh.Extra, buf...)
}

func (z *ZipWriter) compressPartialFile(r io.Reader, dict []byte, last bool, resultChan chan io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()

	result, err := z.compressBlock(r, dict, last)
	if err != nil {
		z.errors <- err
		return
	}

	z.cpuRateLimiter.Finish()

	resultChan <- result
}

func (z *ZipWriter) compressBlock(r io.Reader, dict []byte, last bool) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	var fw *flate.Writer
	var err error
	if len(dict) > 0 {
		// There's no way to Reset a Writer with a new dictionary, so
		// don't use the Pool
		fw, err = flate.NewWriterDict(buf, z.compLevel, dict)
	} else {
		var ok bool
		if fw, ok = z.compressorPool.Get().(*flate.Writer); ok {
			fw.Reset(buf)
		} else {
			fw, err = flate.NewWriter(buf, z.compLevel)
		}
		defer z.compressorPool.Put(fw)
	}
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(fw, r)
	if err != nil {
		return nil, err
	}
	if last {
		fw.Close()
	} else {
		fw.Flush()
	}

	return buf, nil
}

func (z *ZipWriter) compressWholeFile(ze *zipEntry, r io.ReadSeeker, compressChan chan *zipEntry) {
	z.checksumFile(r, ze)

	_, err := r.Seek(0, 0)
	if err != nil {
		z.errors <- err
		return
	}

	readFile := func(reader io.ReadSeeker) ([]byte, error) {
		_, err := reader.Seek(0, 0)
		if err != nil {
			return nil, err
		}

		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			return nil, err
		}

		return buf, nil
	}

	ze.futureReaders = make(chan chan io.Reader, 1)
	futureReader := make(chan io.Reader, 1)
	ze.futureReaders <- futureReader
	close(ze.futureReaders)

	if ze.fh.Method == zip.Deflate {
		compressed, err := z.compressBlock(r, nil, true)
		if err != nil {
			z.errors <- err
			return
		}
		if uint64(compressed.Len()) < ze.fh.UncompressedSize64 {
			futureReader <- compressed
		} else {
			buf, err := readFile(r)
			if err != nil {
				z.errors <- err
				return
			}
			ze.fh.Method = zip.Store
			futureReader <- bytes.NewReader(buf)
		}
	} else {
		buf, err := readFile(r)
		if err != nil {
			z.errors <- err
			return
		}
		ze.fh.Method = zip.Store
		futureReader <- bytes.NewReader(buf)
	}

	z.cpuRateLimiter.Finish()

	close(futureReader)

	compressChan <- ze
	close(compressChan)
}

// writeDirectory annotates that dir is a directory created for the src file or directory, and adds
// the directory entry to the zip file if directories are enabled.
func (z *ZipWriter) writeDirectory(dir string, src string, emulateJar bool) error {
	// clean the input
	dir = filepath.Clean(dir)

	// discover any uncreated directories in the path
	var zipDirs []string
	for dir != "" && dir != "." {
		if _, exists := z.createdDirs[dir]; exists {
			break
		}

		if prev, exists := z.createdFiles[dir]; exists {
			return fmt.Errorf("destination %q is both a directory %q and a file %q", dir, src, prev)
		}

		z.createdDirs[dir] = src
		// parent directories precede their children
		zipDirs = append([]string{dir}, zipDirs...)

		dir = filepath.Dir(dir)
	}

	if z.directories {
		// make a directory entry for each uncreated directory
		for _, cleanDir := range zipDirs {
			var dirHeader *zip.FileHeader

			if emulateJar && cleanDir+"/" == jar.MetaDir {
				dirHeader = jar.MetaDirFileHeader()
			} else {
				dirHeader = &zip.FileHeader{
					Name: cleanDir + "/",
				}
				dirHeader.SetMode(0755 | os.ModeDir)
			}

			dirHeader.SetModTime(z.time)

			ze := make(chan *zipEntry, 1)
			ze <- &zipEntry{
				fh: dirHeader,
			}
			close(ze)
			z.writeOps <- ze
		}
	}

	return nil
}

func (z *ZipWriter) writeSymlink(rel, file string) error {
	fileHeader := &zip.FileHeader{
		Name: rel,
	}
	fileHeader.SetModTime(z.time)
	fileHeader.SetMode(0777 | os.ModeSymlink)

	dest, err := z.fs.Readlink(file)
	if err != nil {
		return err
	}

	fileHeader.UncompressedSize64 = uint64(len(dest))
	fileHeader.CRC32 = crc32.ChecksumIEEE([]byte(dest))

	ze := make(chan *zipEntry, 1)
	futureReaders := make(chan chan io.Reader, 1)
	futureReader := make(chan io.Reader, 1)
	futureReaders <- futureReader
	close(futureReaders)
	futureReader <- bytes.NewBufferString(dest)
	close(futureReader)

	ze <- &zipEntry{
		fh:            fileHeader,
		futureReaders: futureReaders,
	}
	close(ze)
	z.writeOps <- ze

	return nil
}
