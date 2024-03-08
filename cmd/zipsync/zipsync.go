// Copyright 2018 Google Inc. All rights reserved.
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
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	outputDir  = flag.String("d", "", "output dir")
	outputFile = flag.String("l", "", "output list file")
	zipPrefix  = flag.String("zip-prefix", "", "optional prefix within the zip file to extract, stripping the prefix")
	filter     multiFlag
)

func init() {
	flag.Var(&filter, "f", "optional filter pattern")
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func writeFile(filename string, in io.Reader, perm os.FileMode) error {
	out, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err != nil {
		out.Close()
		return err
	}

	return out.Close()
}

func writeSymlink(filename string, in io.Reader) error {
	b, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}
	dest := string(b)
	err = os.Symlink(dest, filename)
	return err
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zipsync -d <output dir> [-l <output file>] [-f <pattern>] [zip]...")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *outputDir == "" {
		flag.Usage()
		os.Exit(1)
	}

	inputs := flag.Args()

	// For now, just wipe the output directory and replace its contents with the zip files
	// Eventually this could only modify the directory contents as necessary to bring it up
	// to date with the zip files.
	must(os.RemoveAll(*outputDir))

	must(os.MkdirAll(*outputDir, 0777))

	var files []string
	seen := make(map[string]string)

	if *zipPrefix != "" {
		*zipPrefix = filepath.Clean(*zipPrefix) + "/"
	}

	for _, input := range inputs {
		reader, err := zip.OpenReader(input)
		if err != nil {
			log.Fatal(err)
		}
		defer reader.Close()

		for _, f := range reader.File {
			name := f.Name
			if *zipPrefix != "" {
				if !strings.HasPrefix(name, *zipPrefix) {
					continue
				}
				name = strings.TrimPrefix(name, *zipPrefix)
			}

			if filter != nil {
				if match, err := filter.Match(filepath.Base(name)); err != nil {
					log.Fatal(err)
				} else if !match {
					continue
				}
			}

			if filepath.IsAbs(name) {
				log.Fatalf("%q in %q is an absolute path", name, input)
			}

			if prev, exists := seen[name]; exists {
				log.Fatalf("%q found in both %q and %q", name, prev, input)
			}
			seen[name] = input

			filename := filepath.Join(*outputDir, name)
			if f.FileInfo().IsDir() {
				must(os.MkdirAll(filename, 0777))
			} else {
				must(os.MkdirAll(filepath.Dir(filename), 0777))
				in, err := f.Open()
				if err != nil {
					log.Fatal(err)
				}
				if f.FileInfo().Mode()&os.ModeSymlink != 0 {
					must(writeSymlink(filename, in))
				} else {
					must(writeFile(filename, in, f.FileInfo().Mode()))
				}
				in.Close()
				files = append(files, filename)
			}
		}
	}

	if *outputFile != "" {
		data := strings.Join(files, "\n")
		if len(files) > 0 {
			data += "\n"
		}
		must(ioutil.WriteFile(*outputFile, []byte(data), 0666))
	}
}

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, " ")
}

func (m *multiFlag) Set(s string) error {
	*m = append(*m, s)
	return nil
}

func (m *multiFlag) Match(s string) (bool, error) {
	if m == nil {
		return false, nil
	}
	for _, f := range *m {
		if match, err := filepath.Match(f, s); err != nil {
			return false, err
		} else if match {
			return true, nil
		}
	}
	return false, nil
}
