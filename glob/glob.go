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

package glob

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"blueprint/deptools"
)

func IsGlob(glob string) bool {
	return strings.IndexAny(glob, "*?[") >= 0
}

// GlobWithDepFile finds all files that match glob.  It compares the list of files
// against the contents of fileListFile, and rewrites fileListFile if it has changed.  It also
// writes all of the the directories it traversed as a depenencies on fileListFile to depFile.
//
// The format of glob is either path/*.ext for a single directory glob, or path/**/*.ext
// for a recursive glob.
//
// Returns a list of file paths, and an error.
func GlobWithDepFile(glob, fileListFile, depFile string) (files []string, err error) {
	globPattern := filepath.Base(glob)
	globDir := filepath.Dir(glob)
	recursive := false

	if filepath.Base(globDir) == "**" {
		recursive = true
		globDir = filepath.Dir(globDir)
	}

	var dirs []string

	err = filepath.Walk(globDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.Mode().IsDir() {
				dirs = append(dirs, path)
				if !recursive && path != globDir {
					return filepath.SkipDir
				}
			} else if info.Mode().IsRegular() {
				match, err := filepath.Match(globPattern, info.Name())
				if err != nil {
					return err
				}
				if match {
					files = append(files, path)
				}
			}

			return nil
		})

	fileList := strings.Join(files, "\n")

	writeFileIfChanged(fileListFile, []byte(fileList), 0666)
	deptools.WriteDepFile(depFile, fileListFile, dirs)

	return
}

func writeFileIfChanged(filename string, data []byte, perm os.FileMode) error {
	var isChanged bool

	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}

	info, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// The file does not exist yet.
			isChanged = true
		} else {
			return err
		}
	} else {
		if info.Size() != int64(len(data)) {
			isChanged = true
		} else {
			oldData, err := ioutil.ReadFile(filename)
			if err != nil {
				return err
			}

			if len(oldData) != len(data) {
				isChanged = true
			} else {
				for i := range data {
					if oldData[i] != data[i] {
						isChanged = true
						break
					}
				}
			}
		}
	}

	if isChanged {
		err = ioutil.WriteFile(filename, data, perm)
		if err != nil {
			return err
		}
	}

	return nil
}
