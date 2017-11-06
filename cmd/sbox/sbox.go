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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

var (
	sandboxesRoot string
	rawCommand    string
	outputRoot    string
	keepOutDir    bool
	depfileOut    string
)

func init() {
	flag.StringVar(&sandboxesRoot, "sandbox-path", "",
		"root of temp directory to put the sandbox into")
	flag.StringVar(&rawCommand, "c", "",
		"command to run")
	flag.StringVar(&outputRoot, "output-root", "",
		"root of directory to copy outputs into")
	flag.BoolVar(&keepOutDir, "keep-out-dir", false,
		"whether to keep the sandbox directory when done")

	flag.StringVar(&depfileOut, "depfile-out", "",
		"file path of the depfile to generate. This value will replace '__SBOX_DEPFILE__' in the command and will be treated as an output but won't be added to __SBOX_OUT_FILES__")
}

func usageViolation(violation string) {
	if violation != "" {
		fmt.Fprintf(os.Stderr, "Usage error: %s.\n\n", violation)
	}

	fmt.Fprintf(os.Stderr,
		"Usage: sbox -c <commandToRun> --sandbox-path <sandboxPath> --output-root <outputRoot> [--depfile-out depFile] <outputFile> [<outputFile>...]\n"+
			"\n"+
			"Runs <commandToRun> and moves each <outputFile> out of <sandboxPath>\n"+
			"and into <outputRoot>\n")

	flag.PrintDefaults()

	os.Exit(1)
}

func main() {
	flag.Usage = func() {
		usageViolation("")
	}
	flag.Parse()

	error := run()
	if error != nil {
		fmt.Fprintln(os.Stderr, error)
		os.Exit(1)
	}
}

func run() error {
	if rawCommand == "" {
		usageViolation("-c <commandToRun> is required and must be non-empty")
	}
	if sandboxesRoot == "" {
		// In practice, the value of sandboxesRoot will mostly likely be at a fixed location relative to OUT_DIR,
		// and the sbox executable will most likely be at a fixed location relative to OUT_DIR too, so
		// the value of sandboxesRoot will most likely be at a fixed location relative to the sbox executable
		// However, Soong also needs to be able to separately remove the sandbox directory on startup (if it has anything left in it)
		// and by passing it as a parameter we don't need to duplicate its value
		usageViolation("--sandbox-path <sandboxPath> is required and must be non-empty")
	}
	if len(outputRoot) == 0 {
		usageViolation("--output-root <outputRoot> is required and must be non-empty")
	}

	// the contents of the __SBOX_OUT_FILES__ variable
	outputsVarEntries := flag.Args()
	if len(outputsVarEntries) == 0 {
		usageViolation("at least one output file must be given")
	}

	// all outputs
	var allOutputs []string

	os.MkdirAll(sandboxesRoot, 0777)

	tempDir, err := ioutil.TempDir(sandboxesRoot, "sbox")

	// Rewrite output file paths to be relative to output root
	// This facilitates matching them up against the corresponding paths in the temporary directory in case they're absolute
	for i, filePath := range outputsVarEntries {
		relativePath, err := filepath.Rel(outputRoot, filePath)
		if err != nil {
			return err
		}
		outputsVarEntries[i] = relativePath
	}

	allOutputs = append([]string(nil), outputsVarEntries...)

	if depfileOut != "" {
		sandboxedDepfile, err := filepath.Rel(outputRoot, depfileOut)
		if err != nil {
			return err
		}
		allOutputs = append(allOutputs, sandboxedDepfile)
		if !strings.Contains(rawCommand, "__SBOX_DEPFILE__") {
			return fmt.Errorf("the --depfile-out argument only makes sense if the command contains the text __SBOX_DEPFILE__")
		}
		rawCommand = strings.Replace(rawCommand, "__SBOX_DEPFILE__", filepath.Join(tempDir, sandboxedDepfile), -1)

	}

	if err != nil {
		return fmt.Errorf("Failed to create temp dir: %s", err)
	}

	// In the common case, the following line of code is what removes the sandbox
	// If a fatal error occurs (such as if our Go process is killed unexpectedly),
	// then at the beginning of the next build, Soong will retry the cleanup
	defer func() {
		// in some cases we decline to remove the temp dir, to facilitate debugging
		if !keepOutDir {
			os.RemoveAll(tempDir)
		}
	}()

	if strings.Contains(rawCommand, "__SBOX_OUT_DIR__") {
		rawCommand = strings.Replace(rawCommand, "__SBOX_OUT_DIR__", tempDir, -1)
	}

	if strings.Contains(rawCommand, "__SBOX_OUT_FILES__") {
		// expands into a space-separated list of output files to be generated into the sandbox directory
		tempOutPaths := []string{}
		for _, outputPath := range outputsVarEntries {
			tempOutPath := path.Join(tempDir, outputPath)
			tempOutPaths = append(tempOutPaths, tempOutPath)
		}
		pathsText := strings.Join(tempOutPaths, " ")
		rawCommand = strings.Replace(rawCommand, "__SBOX_OUT_FILES__", pathsText, -1)
	}

	for _, filePath := range allOutputs {
		dir := path.Join(tempDir, filepath.Dir(filePath))
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			return err
		}
	}

	commandDescription := rawCommand

	cmd := exec.Command("bash", "-c", rawCommand)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if exit, ok := err.(*exec.ExitError); ok && !exit.Success() {
		return fmt.Errorf("sbox command (%s) failed with err %#v\n", commandDescription, err.Error())
	} else if err != nil {
		return err
	}

	// validate that all files are created properly
	var outputErrors []error
	for _, filePath := range allOutputs {
		tempPath := filepath.Join(tempDir, filePath)
		fileInfo, err := os.Stat(tempPath)
		if err != nil {
			outputErrors = append(outputErrors, fmt.Errorf("failed to create expected output file: %s\n", tempPath))
			continue
		}
		if fileInfo.IsDir() {
			outputErrors = append(outputErrors, fmt.Errorf("Output path %s refers to a directory, not a file. This is not permitted because it prevents robust up-to-date checks\n", filePath))
		}
	}
	if len(outputErrors) > 0 {
		// Keep the temporary output directory around in case a user wants to inspect it for debugging purposes.
		// Soong will delete it later anyway.
		keepOutDir = true
		return fmt.Errorf("mismatch between declared and actual outputs in sbox command (%s):\n%v", commandDescription, outputErrors)
	}
	// the created files match the declared files; now move them
	for _, filePath := range allOutputs {
		tempPath := filepath.Join(tempDir, filePath)
		destPath := filePath
		if len(outputRoot) != 0 {
			destPath = filepath.Join(outputRoot, filePath)
		}
		err := os.Rename(tempPath, destPath)
		if err != nil {
			return err
		}
	}

	// TODO(jeffrygaston) if a process creates more output files than it declares, should there be a warning?
	return nil
}
