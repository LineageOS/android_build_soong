// Copyright 2019 Google Inc. All rights reserved.
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
	"archive/zip"
	"context"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// ZipArtifact represents a zip file that may be local or remote.
type ZipArtifact interface {
	// Files returns the list of files contained in the zip file.
	Files() ([]*ZipArtifactFile, error)

	// Close closes the zip file artifact.
	Close()
}

// localZipArtifact is a handle to a local zip file artifact.
type localZipArtifact struct {
	zr    *zip.ReadCloser
	files []*ZipArtifactFile
}

// NewLocalZipArtifact returns a ZipArtifact for a local zip file..
func NewLocalZipArtifact(name string) (ZipArtifact, error) {
	zr, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}

	var files []*ZipArtifactFile
	for _, zf := range zr.File {
		files = append(files, &ZipArtifactFile{zf})
	}

	return &localZipArtifact{
		zr:    zr,
		files: files,
	}, nil
}

// Files returns the list of files contained in the local zip file artifact.
func (z *localZipArtifact) Files() ([]*ZipArtifactFile, error) {
	return z.files, nil
}

// Close closes the buffered reader of the local zip file artifact.
func (z *localZipArtifact) Close() {
	z.zr.Close()
}

// ZipArtifactFile contains a zip.File handle to the data inside the remote *-target_files-*.zip
// build artifact.
type ZipArtifactFile struct {
	*zip.File
}

// Extract begins extract a file from inside a ZipArtifact.  It returns an
// ExtractedZipArtifactFile handle.
func (zf *ZipArtifactFile) Extract(ctx context.Context, dir string,
	limiter chan bool) *ExtractedZipArtifactFile {

	d := &ExtractedZipArtifactFile{
		initCh: make(chan struct{}),
	}

	go func() {
		defer close(d.initCh)
		limiter <- true
		defer func() { <-limiter }()

		zr, err := zf.Open()
		if err != nil {
			d.err = err
			return
		}
		defer zr.Close()

		crc := crc32.NewIEEE()
		r := io.TeeReader(zr, crc)

		if filepath.Clean(zf.Name) != zf.Name {
			d.err = fmt.Errorf("invalid filename %q", zf.Name)
			return
		}
		path := filepath.Join(dir, zf.Name)

		err = os.MkdirAll(filepath.Dir(path), 0777)
		if err != nil {
			d.err = err
			return
		}

		err = os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			d.err = err
			return
		}

		if zf.Mode().IsRegular() {
			w, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, zf.Mode())
			if err != nil {
				d.err = err
				return
			}
			defer w.Close()

			_, err = io.Copy(w, r)
			if err != nil {
				d.err = err
				return
			}
		} else if zf.Mode()&os.ModeSymlink != 0 {
			target, err := ioutil.ReadAll(r)
			if err != nil {
				d.err = err
				return
			}

			err = os.Symlink(string(target), path)
			if err != nil {
				d.err = err
				return
			}
		} else {
			d.err = fmt.Errorf("unknown mode %q", zf.Mode())
			return
		}

		if crc.Sum32() != zf.CRC32 {
			d.err = fmt.Errorf("crc mismatch for %v", zf.Name)
			return
		}

		d.path = path
	}()

	return d
}

// ExtractedZipArtifactFile is a handle to a downloaded file from a remoteZipArtifact.  The download
// may still be in progress, and will be complete with Path() returns.
type ExtractedZipArtifactFile struct {
	initCh chan struct{}
	err    error

	path string
}

// Path returns the path to the downloaded file and any errors that occurred during the download.
// It will block until the download is complete.
func (d *ExtractedZipArtifactFile) Path() (string, error) {
	<-d.initCh
	return d.path, d.err
}
