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

// soong_zip is a utility used during the build to create a zip archive by pulling the entries from
// various sources:
//  * explicitly specified files
//  * files whose paths are read from a file
//  * directories traversed recursively
// It can optionally change the recorded path of an entry.

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"

	"android/soong/response"
	"android/soong/zip"
)

type uniqueSet map[string]bool

func (u *uniqueSet) String() string {
	return `""`
}

func (u *uniqueSet) Set(s string) error {
	if _, found := (*u)[s]; found {
		return fmt.Errorf("File %q was specified twice as a file to not deflate", s)
	} else {
		(*u)[s] = true
	}

	return nil
}

type file struct{}

func (file) String() string { return `""` }

func (file) Set(s string) error {
	fileArgsBuilder.File(s)
	return nil
}

type listFiles struct{}

func (listFiles) String() string { return `""` }

func (listFiles) Set(s string) error {
	fileArgsBuilder.List(s)
	return nil
}

type rspFiles struct{}

func (rspFiles) String() string { return `""` }

func (rspFiles) Set(s string) error {
	fileArgsBuilder.RspFile(s)
	return nil
}

type explicitFile struct{}

func (explicitFile) String() string { return `""` }

func (explicitFile) Set(s string) error {
	fileArgsBuilder.ExplicitPathInZip(s)
	return nil
}

type dir struct{}

func (dir) String() string { return `""` }

func (dir) Set(s string) error {
	fileArgsBuilder.Dir(s)
	return nil
}

type relativeRoot struct{}

func (relativeRoot) String() string { return "" }

func (relativeRoot) Set(s string) error {
	fileArgsBuilder.SourcePrefixToStrip(s)
	return nil
}

type junkPaths struct{}

func (junkPaths) IsBoolFlag() bool { return true }
func (junkPaths) String() string   { return "" }

func (junkPaths) Set(s string) error {
	v, err := strconv.ParseBool(s)
	fileArgsBuilder.JunkPaths(v)
	return err
}

type rootPrefix struct{}

func (rootPrefix) String() string { return "" }

func (rootPrefix) Set(s string) error {
	fileArgsBuilder.PathPrefixInZip(s)
	return nil
}

var (
	fileArgsBuilder  = zip.NewFileArgsBuilder()
	nonDeflatedFiles = make(uniqueSet)
)

func main() {
	var expandedArgs []string
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "@") {
			f, err := os.Open(strings.TrimPrefix(arg, "@"))
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}

			respArgs, err := response.ReadRspFile(f)
			f.Close()
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
			expandedArgs = append(expandedArgs, respArgs...)
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	flags := flag.NewFlagSet("flags", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: soong_zip -o zipfile [-m manifest] [-C dir] [-f|-l file] [-D dir]...\n")
		flags.PrintDefaults()
		os.Exit(2)
	}

	out := flags.String("o", "", "file to write zip file to")
	manifest := flags.String("m", "", "input jar manifest file name")
	directories := flags.Bool("d", false, "include directories in zip")
	compLevel := flags.Int("L", 5, "deflate compression level (0-9)")
	emulateJar := flags.Bool("jar", false, "modify the resultant .zip to emulate the output of 'jar'")
	writeIfChanged := flags.Bool("write_if_changed", false, "only update resultant .zip if it has changed")
	ignoreMissingFiles := flags.Bool("ignore_missing_files", false, "continue if a requested file does not exist")
	symlinks := flags.Bool("symlinks", true, "store symbolic links in zip instead of following them")
	srcJar := flags.Bool("srcjar", false, "move .java files to locations that match their package statement")

	parallelJobs := flags.Int("parallel", runtime.NumCPU(), "number of parallel threads to use")
	cpuProfile := flags.String("cpuprofile", "", "write cpu profile to file")
	traceFile := flags.String("trace", "", "write trace to file")
	sha256Checksum := flags.Bool("sha256", false, "add a zip header to each file containing its SHA256 digest")
	doNotWrite := flags.Bool("n", false, "Nothing is written to disk -- all other work happens")
	quiet := flags.Bool("quiet", false, "do not print warnings to console")

	flags.Var(&rootPrefix{}, "P", "path prefix within the zip at which to place files")
	flags.Var(&listFiles{}, "l", "file containing list of files to zip")
	flags.Var(&rspFiles{}, "r", "file containing list of files to zip with Ninja rsp file escaping")
	flags.Var(&dir{}, "D", "directory to include in zip")
	flags.Var(&file{}, "f", "file to include in zip")
	flags.Var(&nonDeflatedFiles, "s", "file path to be stored within the zip without compression")
	flags.Var(&relativeRoot{}, "C", "path to use as relative root of files in following -f, -l, or -D arguments")
	flags.Var(&junkPaths{}, "j", "junk paths, zip files without directory names")
	flags.Var(&explicitFile{}, "e", "filename to use in the zip file for the next -f argument")

	flags.Parse(expandedArgs[1:])

	if flags.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments %s\n", strings.Join(flags.Args(), " "))
		flags.Usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer f.Close()
		err = trace.Start(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		defer trace.Stop()
	}

	if fileArgsBuilder.Error() != nil {
		fmt.Fprintln(os.Stderr, fileArgsBuilder.Error())
		os.Exit(1)
	}

	err := zip.Zip(zip.ZipArgs{
		FileArgs:                 fileArgsBuilder.FileArgs(),
		OutputFilePath:           *out,
		EmulateJar:               *emulateJar,
		SrcJar:                   *srcJar,
		AddDirectoryEntriesToZip: *directories,
		CompressionLevel:         *compLevel,
		ManifestSourcePath:       *manifest,
		NumParallelJobs:          *parallelJobs,
		NonDeflatedFiles:         nonDeflatedFiles,
		WriteIfChanged:           *writeIfChanged,
		StoreSymlinks:            *symlinks,
		IgnoreMissingFiles:       *ignoreMissingFiles,
		Sha256Checksum:           *sha256Checksum,
		DoNotWrite:               *doNotWrite,
		Quiet:                    *quiet,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
		os.Exit(1)
	}
}
