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
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"android/soong/jar"
	"android/soong/third_party/zip"
)

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

func (s zipsToNotStripSet) Set(zip_path string) error {
	s[zip_path] = true

	return nil
}

var (
	sortEntries     = flag.Bool("s", false, "sort entries (defaults to the order from the input zip files)")
	emulateJar      = flag.Bool("j", false, "sort zip entries using jar ordering (META-INF first)")
	stripDirs       fileList
	stripFiles      fileList
	zipsToNotStrip  = make(zipsToNotStripSet)
	stripDirEntries = flag.Bool("D", false, "strip directory entries from the output zip file")
	manifest        = flag.String("m", "", "manifest file to insert in jar")
)

func init() {
	flag.Var(&stripDirs, "stripDir", "the prefix of file path to be excluded from the output zip")
	flag.Var(&stripFiles, "stripFile", "filenames to be excluded from the output zip, accepts wildcards")
	flag.Var(&zipsToNotStrip, "zipToNotStrip", "the input zip file which is not applicable for stripping")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: merge_zips [-jsD] [-m manifest] output [inputs...]")
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
	inputs := args[1:]

	log.SetFlags(log.Lshortfile)

	// make writer
	output, err := os.Create(outputPath)
	if err != nil {
		log.Fatal(err)
	}
	defer output.Close()
	writer := zip.NewWriter(output)
	defer func() {
		err := writer.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// make readers
	readers := []namedZipReader{}
	for _, input := range inputs {
		reader, err := zip.OpenReader(input)
		if err != nil {
			log.Fatal(err)
		}
		defer reader.Close()
		namedReader := namedZipReader{path: input, reader: reader}
		readers = append(readers, namedReader)
	}

	if *manifest != "" && !*emulateJar {
		log.Fatal(errors.New("must specify -j when specifying a manifest via -m"))
	}

	// do merge
	err = mergeZips(readers, writer, *manifest, *sortEntries, *emulateJar, *stripDirEntries)
	if err != nil {
		log.Fatal(err)
	}
}

// a namedZipReader reads a .zip file and can say which file it's reading
type namedZipReader struct {
	path   string
	reader *zip.ReadCloser
}

// a zipEntryPath refers to a file contained in a zip
type zipEntryPath struct {
	zipName   string
	entryName string
}

func (p zipEntryPath) String() string {
	return p.zipName + "/" + p.entryName
}

// a zipEntry is a zipSource that pulls its content from another zip
type zipEntry struct {
	path    zipEntryPath
	content *zip.File
}

func (ze zipEntry) String() string {
	return ze.path.String()
}

func (ze zipEntry) IsDir() bool {
	return ze.content.FileInfo().IsDir()
}

func (ze zipEntry) CRC32() uint32 {
	return ze.content.FileHeader.CRC32
}

func (ze zipEntry) WriteToZip(dest string, zw *zip.Writer) error {
	return zw.CopyFrom(ze.content, dest)
}

// a bufferEntry is a zipSource that pulls its content from a []byte
type bufferEntry struct {
	fh      *zip.FileHeader
	content []byte
}

func (be bufferEntry) String() string {
	return "internal buffer"
}

func (be bufferEntry) IsDir() bool {
	return be.fh.FileInfo().IsDir()
}

func (be bufferEntry) CRC32() uint32 {
	return crc32.ChecksumIEEE(be.content)
}

func (be bufferEntry) WriteToZip(dest string, zw *zip.Writer) error {
	w, err := zw.CreateHeader(be.fh)
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

type zipSource interface {
	String() string
	IsDir() bool
	CRC32() uint32
	WriteToZip(dest string, zw *zip.Writer) error
}

// a fileMapping specifies to copy a zip entry from one place to another
type fileMapping struct {
	dest   string
	source zipSource
}

func mergeZips(readers []namedZipReader, writer *zip.Writer, manifest string,
	sortEntries, emulateJar, stripDirEntries bool) error {

	sourceByDest := make(map[string]zipSource, 0)
	orderedMappings := []fileMapping{}

	// if dest already exists returns a non-null zipSource for the existing source
	addMapping := func(dest string, source zipSource) zipSource {
		mapKey := filepath.Clean(dest)
		if existingSource, exists := sourceByDest[mapKey]; exists {
			return existingSource
		}

		sourceByDest[mapKey] = source
		orderedMappings = append(orderedMappings, fileMapping{source: source, dest: dest})
		return nil
	}

	if manifest != "" {
		if !stripDirEntries {
			dirHeader := jar.MetaDirFileHeader()
			dirSource := bufferEntry{dirHeader, nil}
			addMapping(jar.MetaDir, dirSource)
		}

		fh, buf, err := jar.ManifestFileContents(manifest)
		if err != nil {
			return err
		}

		fileSource := bufferEntry{fh, buf}
		addMapping(jar.ManifestFile, fileSource)
	}

	for _, namedReader := range readers {
		_, skipStripThisZip := zipsToNotStrip[namedReader.path]
		for _, file := range namedReader.reader.File {
			if !skipStripThisZip && shouldStripFile(emulateJar, file.Name) {
				continue
			}

			if stripDirEntries && file.FileInfo().IsDir() {
				continue
			}

			// check for other files or directories destined for the same path
			dest := file.Name

			// make a new entry to add
			source := zipEntry{path: zipEntryPath{zipName: namedReader.path, entryName: file.Name}, content: file}

			if existingSource := addMapping(dest, source); existingSource != nil {
				// handle duplicates
				if existingSource.IsDir() != source.IsDir() {
					return fmt.Errorf("Directory/file mismatch at %v from %v and %v\n",
						dest, existingSource, source)
				}
				if emulateJar &&
					file.Name == jar.ManifestFile || file.Name == jar.ModuleInfoClass {
					// Skip manifest and module info files that are not from the first input file
					continue
				}
				if !source.IsDir() {
					if emulateJar {
						if existingSource.CRC32() != source.CRC32() {
							fmt.Fprintf(os.Stdout, "WARNING: Duplicate path %v found in %v and %v\n",
								dest, existingSource, source)
						}
					} else {
						return fmt.Errorf("Duplicate path %v found in %v and %v\n",
							dest, existingSource, source)
					}
				}
			}
		}
	}

	if emulateJar {
		jarSort(orderedMappings)
	} else if sortEntries {
		alphanumericSort(orderedMappings)
	}

	for _, entry := range orderedMappings {
		if err := entry.source.WriteToZip(entry.dest, writer); err != nil {
			return err
		}
	}

	return nil
}

func shouldStripFile(emulateJar bool, name string) bool {
	for _, dir := range stripDirs {
		if strings.HasPrefix(name, dir+"/") {
			if emulateJar {
				if name != jar.MetaDir && name != jar.ManifestFile {
					return true
				}
			} else {
				return true
			}
		}
	}
	for _, pattern := range stripFiles {
		if match, err := filepath.Match(pattern, filepath.Base(name)); err != nil {
			panic(fmt.Errorf("%s: %s", err.Error(), pattern))
		} else if match {
			return true
		}
	}
	return false
}

func jarSort(files []fileMapping) {
	sort.SliceStable(files, func(i, j int) bool {
		return jar.EntryNamesLess(files[i].dest, files[j].dest)
	})
}

func alphanumericSort(files []fileMapping) {
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].dest < files[j].dest
	})
}
