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
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"android/soong/makedeps"
)

var (
	sandboxesRoot string
	rawCommand    string
	outputRoot    string
	keepOutDir    bool
	copyAllOutput bool
	depfileOut    string
	inputHash     string
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
	flag.BoolVar(&copyAllOutput, "copy-all-output", false,
		"whether to copy all output files")

	flag.StringVar(&depfileOut, "depfile-out", "",
		"file path of the depfile to generate. This value will replace '__SBOX_DEPFILE__' in the command and will be treated as an output but won't be added to __SBOX_OUT_FILES__")

	flag.StringVar(&inputHash, "input-hash", "",
		"This option is ignored. Typical usage is to supply a hash of the list of input names so that the module will be rebuilt if the list (and thus the hash) changes.")
}

func usageViolation(violation string) {
	if violation != "" {
		fmt.Fprintf(os.Stderr, "Usage error: %s.\n\n", violation)
	}

	fmt.Fprintf(os.Stderr,
		"Usage: sbox -c <commandToRun> --sandbox-path <sandboxPath> --output-root <outputRoot> [--depfile-out depFile] [--input-hash hash] <outputFile> [<outputFile>...]\n"+
			"\n"+
			"Deletes <outputRoot>,"+
			"runs <commandToRun>,"+
			"and moves each <outputFile> out of <sandboxPath> and into <outputRoot>\n")

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

func findAllFilesUnder(root string) (paths []string) {
	paths = []string{}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				// couldn't find relative path from ancestor?
				panic(err)
			}
			paths = append(paths, relPath)
		}
		return nil
	})
	return paths
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
	if !copyAllOutput && len(outputsVarEntries) == 0 {
		usageViolation("at least one output file must be given")
	}

	// all outputs
	var allOutputs []string

	// setup directories
	err := os.MkdirAll(sandboxesRoot, 0777)
	if err != nil {
		return err
	}
	err = os.RemoveAll(outputRoot)
	if err != nil {
		return err
	}
	err = os.MkdirAll(outputRoot, 0777)
	if err != nil {
		return err
	}

	tempDir, err := ioutil.TempDir(sandboxesRoot, "sbox")

	for i, filePath := range outputsVarEntries {
		if !strings.HasPrefix(filePath, "__SBOX_OUT_DIR__/") {
			return fmt.Errorf("output files must start with `__SBOX_OUT_DIR__/`")
		}
		outputsVarEntries[i] = strings.TrimPrefix(filePath, "__SBOX_OUT_DIR__/")
	}

	allOutputs = append([]string(nil), outputsVarEntries...)

	if depfileOut != "" {
		sandboxedDepfile, err := filepath.Rel(outputRoot, depfileOut)
		if err != nil {
			return err
		}
		allOutputs = append(allOutputs, sandboxedDepfile)
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
	var missingOutputErrors []string
	for _, filePath := range allOutputs {
		tempPath := filepath.Join(tempDir, filePath)
		fileInfo, err := os.Stat(tempPath)
		if err != nil {
			missingOutputErrors = append(missingOutputErrors, fmt.Sprintf("%s: does not exist", filePath))
			continue
		}
		if fileInfo.IsDir() {
			missingOutputErrors = append(missingOutputErrors, fmt.Sprintf("%s: not a file", filePath))
		}
	}
	if !copyAllOutput && len(missingOutputErrors) > 0 {
		// find all created files for making a more informative error message
		createdFiles := findAllFilesUnder(tempDir)

		// build error message
		errorMessage := "mismatch between declared and actual outputs\n"
		errorMessage += "in sbox command(" + commandDescription + ")\n\n"
		errorMessage += "in sandbox " + tempDir + ",\n"
		errorMessage += fmt.Sprintf("failed to create %v files:\n", len(missingOutputErrors))
		for _, missingOutputError := range missingOutputErrors {
			errorMessage += "  " + missingOutputError + "\n"
		}
		if len(createdFiles) < 1 {
			errorMessage += "created 0 files."
		} else {
			errorMessage += fmt.Sprintf("did create %v files:\n", len(createdFiles))
			creationMessages := createdFiles
			maxNumCreationLines := 10
			if len(creationMessages) > maxNumCreationLines {
				creationMessages = creationMessages[:maxNumCreationLines]
				creationMessages = append(creationMessages, fmt.Sprintf("...%v more", len(createdFiles)-maxNumCreationLines))
			}
			for _, creationMessage := range creationMessages {
				errorMessage += "  " + creationMessage + "\n"
			}
		}

		// Keep the temporary output directory around in case a user wants to inspect it for debugging purposes.
		// Soong will delete it later anyway.
		keepOutDir = true
		return errors.New(errorMessage)
	}
	var filePathList []string
	if copyAllOutput {
		filePathList = findAllFilesUnder(tempDir)
	} else {
		filePathList = allOutputs
	}
	// the created files match the declared files; now move them
	for _, filePath := range filePathList {
		tempPath := filepath.Join(tempDir, filePath)
		destPath := filePath
		if len(outputRoot) != 0 {
			destPath = filepath.Join(outputRoot, filePath)
		}
		err := os.MkdirAll(filepath.Dir(destPath), 0777)
		if err != nil {
			return err
		}

		// Update the timestamp of the output file in case the tool wrote an old timestamp (for example, tar can extract
		// files with old timestamps).
		now := time.Now()
		err = os.Chtimes(tempPath, now, now)
		if err != nil {
			return err
		}

		err = os.Rename(tempPath, destPath)
		if err != nil {
			return err
		}
	}

	// Rewrite the depfile so that it doesn't include the (randomized) sandbox directory
	if depfileOut != "" {
		in, err := ioutil.ReadFile(depfileOut)
		if err != nil {
			return err
		}

		deps, err := makedeps.Parse(depfileOut, bytes.NewBuffer(in))
		if err != nil {
			return err
		}

		deps.Output = "outputfile"

		err = ioutil.WriteFile(depfileOut, deps.Print(), 0666)
		if err != nil {
			return err
		}
	}

	// TODO(jeffrygaston) if a process creates more output files than it declares, should there be a warning?
	return nil
}
