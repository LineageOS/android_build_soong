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
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/cmd/sbox/sbox_proto"
	"android/soong/makedeps"

	"github.com/golang/protobuf/proto"
)

var (
	sandboxesRoot string
	manifestFile  string
	keepOutDir    bool
)

const (
	depFilePlaceholder    = "__SBOX_DEPFILE__"
	sandboxDirPlaceholder = "__SBOX_SANDBOX_DIR__"
)

func init() {
	flag.StringVar(&sandboxesRoot, "sandbox-path", "",
		"root of temp directory to put the sandbox into")
	flag.StringVar(&manifestFile, "manifest", "",
		"textproto manifest describing the sandboxed command(s)")
	flag.BoolVar(&keepOutDir, "keep-out-dir", false,
		"whether to keep the sandbox directory when done")
}

func usageViolation(violation string) {
	if violation != "" {
		fmt.Fprintf(os.Stderr, "Usage error: %s.\n\n", violation)
	}

	fmt.Fprintf(os.Stderr,
		"Usage: sbox --manifest <manifest> --sandbox-path <sandboxPath>\n")

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
	if manifestFile == "" {
		usageViolation("--manifest <manifest> is required and must be non-empty")
	}
	if sandboxesRoot == "" {
		// In practice, the value of sandboxesRoot will mostly likely be at a fixed location relative to OUT_DIR,
		// and the sbox executable will most likely be at a fixed location relative to OUT_DIR too, so
		// the value of sandboxesRoot will most likely be at a fixed location relative to the sbox executable
		// However, Soong also needs to be able to separately remove the sandbox directory on startup (if it has anything left in it)
		// and by passing it as a parameter we don't need to duplicate its value
		usageViolation("--sandbox-path <sandboxPath> is required and must be non-empty")
	}

	manifest, err := readManifest(manifestFile)

	if len(manifest.Commands) == 0 {
		return fmt.Errorf("at least one commands entry is required in %q", manifestFile)
	}

	// setup sandbox directory
	err = os.MkdirAll(sandboxesRoot, 0777)
	if err != nil {
		return fmt.Errorf("failed to create %q: %w", sandboxesRoot, err)
	}

	// This tool assumes that there are no two concurrent runs with the same
	// manifestFile. It should therefore be safe to use the hash of the
	// manifestFile as the temporary directory name. We do this because it
	// makes the temporary directory name deterministic. There are some
	// tools that embed the name of the temporary output in the output, and
	// they otherwise cause non-determinism, which then poisons actions
	// depending on this one.
	hash := sha1.New()
	hash.Write([]byte(manifestFile))
	tempDir := filepath.Join(sandboxesRoot, "sbox", hex.EncodeToString(hash.Sum(nil)))

	err = os.RemoveAll(tempDir)
	if err != nil {
		return err
	}
	err = os.MkdirAll(tempDir, 0777)
	if err != nil {
		return fmt.Errorf("failed to create temporary dir in %q: %w", sandboxesRoot, err)
	}

	// In the common case, the following line of code is what removes the sandbox
	// If a fatal error occurs (such as if our Go process is killed unexpectedly),
	// then at the beginning of the next build, Soong will wipe the temporary
	// directory.
	defer func() {
		// in some cases we decline to remove the temp dir, to facilitate debugging
		if !keepOutDir {
			os.RemoveAll(tempDir)
		}
	}()

	// If there is more than one command in the manifest use a separate directory for each one.
	useSubDir := len(manifest.Commands) > 1
	var commandDepFiles []string

	for i, command := range manifest.Commands {
		localTempDir := tempDir
		if useSubDir {
			localTempDir = filepath.Join(localTempDir, strconv.Itoa(i))
		}
		depFile, err := runCommand(command, localTempDir)
		if err != nil {
			// Running the command failed, keep the temporary output directory around in
			// case a user wants to inspect it for debugging purposes.  Soong will delete
			// it at the beginning of the next build anyway.
			keepOutDir = true
			return err
		}
		if depFile != "" {
			commandDepFiles = append(commandDepFiles, depFile)
		}
	}

	outputDepFile := manifest.GetOutputDepfile()
	if len(commandDepFiles) > 0 && outputDepFile == "" {
		return fmt.Errorf("Sandboxed commands used %s but output depfile is not set in manifest file",
			depFilePlaceholder)
	}

	if outputDepFile != "" {
		// Merge the depfiles from each command in the manifest to a single output depfile.
		err = rewriteDepFiles(commandDepFiles, outputDepFile)
		if err != nil {
			return fmt.Errorf("failed merging depfiles: %w", err)
		}
	}

	return nil
}

// readManifest reads an sbox manifest from a textproto file.
func readManifest(file string) (*sbox_proto.Manifest, error) {
	manifestData, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest %q: %w", file, err)
	}

	manifest := sbox_proto.Manifest{}

	err = proto.UnmarshalText(string(manifestData), &manifest)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest %q: %w", file, err)
	}

	return &manifest, nil
}

// runCommand runs a single command from a manifest.  If the command references the
// __SBOX_DEPFILE__ placeholder it returns the name of the depfile that was used.
func runCommand(command *sbox_proto.Command, tempDir string) (depFile string, err error) {
	rawCommand := command.GetCommand()
	if rawCommand == "" {
		return "", fmt.Errorf("command is required")
	}

	err = os.MkdirAll(tempDir, 0777)
	if err != nil {
		return "", fmt.Errorf("failed to create %q: %w", tempDir, err)
	}

	// Copy in any files specified by the manifest.
	err = copyFiles(command.CopyBefore, "", tempDir)
	if err != nil {
		return "", err
	}

	pathToTempDirInSbox := tempDir
	if command.GetChdir() {
		pathToTempDirInSbox = "."
	}

	if strings.Contains(rawCommand, depFilePlaceholder) {
		depFile = filepath.Join(pathToTempDirInSbox, "deps.d")
		rawCommand = strings.Replace(rawCommand, depFilePlaceholder, depFile, -1)
	}

	if strings.Contains(rawCommand, sandboxDirPlaceholder) {
		rawCommand = strings.Replace(rawCommand, sandboxDirPlaceholder, pathToTempDirInSbox, -1)
	}

	// Emulate ninja's behavior of creating the directories for any output files before
	// running the command.
	err = makeOutputDirs(command.CopyAfter, tempDir)
	if err != nil {
		return "", err
	}

	commandDescription := rawCommand

	cmd := exec.Command("bash", "-c", rawCommand)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if command.GetChdir() {
		cmd.Dir = tempDir
		path := os.Getenv("PATH")
		absPath, err := makeAbsPathEnv(path)
		if err != nil {
			return "", err
		}
		err = os.Setenv("PATH", absPath)
		if err != nil {
			return "", fmt.Errorf("Failed to update PATH: %w", err)
		}
	}
	err = cmd.Run()

	if exit, ok := err.(*exec.ExitError); ok && !exit.Success() {
		return "", fmt.Errorf("sbox command failed with err:\n%s\n%w\n", commandDescription, err)
	} else if err != nil {
		return "", err
	}

	missingOutputErrors := validateOutputFiles(command.CopyAfter, tempDir)

	if len(missingOutputErrors) > 0 {
		// find all created files for making a more informative error message
		createdFiles := findAllFilesUnder(tempDir)

		// build error message
		errorMessage := "mismatch between declared and actual outputs\n"
		errorMessage += "in sbox command(" + commandDescription + ")\n\n"
		errorMessage += "in sandbox " + tempDir + ",\n"
		errorMessage += fmt.Sprintf("failed to create %v files:\n", len(missingOutputErrors))
		for _, missingOutputError := range missingOutputErrors {
			errorMessage += "  " + missingOutputError.Error() + "\n"
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

		return "", errors.New(errorMessage)
	}
	// the created files match the declared files; now move them
	err = moveFiles(command.CopyAfter, tempDir, "")

	return depFile, nil
}

// makeOutputDirs creates directories in the sandbox dir for every file that has a rule to be copied
// out of the sandbox.  This emulate's Ninja's behavior of creating directories for output files
// so that the tools don't have to.
func makeOutputDirs(copies []*sbox_proto.Copy, sandboxDir string) error {
	for _, copyPair := range copies {
		dir := joinPath(sandboxDir, filepath.Dir(copyPair.GetFrom()))
		err := os.MkdirAll(dir, 0777)
		if err != nil {
			return err
		}
	}
	return nil
}

// validateOutputFiles verifies that all files that have a rule to be copied out of the sandbox
// were created by the command.
func validateOutputFiles(copies []*sbox_proto.Copy, sandboxDir string) []error {
	var missingOutputErrors []error
	for _, copyPair := range copies {
		fromPath := joinPath(sandboxDir, copyPair.GetFrom())
		fileInfo, err := os.Stat(fromPath)
		if err != nil {
			missingOutputErrors = append(missingOutputErrors, fmt.Errorf("%s: does not exist", fromPath))
			continue
		}
		if fileInfo.IsDir() {
			missingOutputErrors = append(missingOutputErrors, fmt.Errorf("%s: not a file", fromPath))
		}
	}
	return missingOutputErrors
}

// copyFiles copies files in or out of the sandbox.
func copyFiles(copies []*sbox_proto.Copy, fromDir, toDir string) error {
	for _, copyPair := range copies {
		fromPath := joinPath(fromDir, copyPair.GetFrom())
		toPath := joinPath(toDir, copyPair.GetTo())
		err := copyOneFile(fromPath, toPath, copyPair.GetExecutable())
		if err != nil {
			return fmt.Errorf("error copying %q to %q: %w", fromPath, toPath, err)
		}
	}
	return nil
}

// copyOneFile copies a file.
func copyOneFile(from string, to string, executable bool) error {
	err := os.MkdirAll(filepath.Dir(to), 0777)
	if err != nil {
		return err
	}

	stat, err := os.Stat(from)
	if err != nil {
		return err
	}

	perm := stat.Mode()
	if executable {
		perm = perm | 0100 // u+x
	}

	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer func() {
		out.Close()
		if err != nil {
			os.Remove(to)
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	if err = out.Close(); err != nil {
		return err
	}

	if err = os.Chmod(to, perm); err != nil {
		return err
	}

	return nil
}

// moveFiles moves files specified by a set of copy rules.  It uses os.Rename, so it is restricted
// to moving files where the source and destination are in the same filesystem.  This is OK for
// sbox because the temporary directory is inside the out directory.  It updates the timestamp
// of the new file.
func moveFiles(copies []*sbox_proto.Copy, fromDir, toDir string) error {
	for _, copyPair := range copies {
		fromPath := joinPath(fromDir, copyPair.GetFrom())
		toPath := joinPath(toDir, copyPair.GetTo())
		err := os.MkdirAll(filepath.Dir(toPath), 0777)
		if err != nil {
			return err
		}

		err = os.Rename(fromPath, toPath)
		if err != nil {
			return err
		}

		// Update the timestamp of the output file in case the tool wrote an old timestamp (for example, tar can extract
		// files with old timestamps).
		now := time.Now()
		err = os.Chtimes(toPath, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

// Rewrite one or more depfiles so that it doesn't include the (randomized) sandbox directory
// to an output file.
func rewriteDepFiles(ins []string, out string) error {
	var mergedDeps []string
	for _, in := range ins {
		data, err := ioutil.ReadFile(in)
		if err != nil {
			return err
		}

		deps, err := makedeps.Parse(in, bytes.NewBuffer(data))
		if err != nil {
			return err
		}
		mergedDeps = append(mergedDeps, deps.Inputs...)
	}

	deps := makedeps.Deps{
		// Ninja doesn't care what the output file is, so we can use any string here.
		Output: "outputfile",
		Inputs: mergedDeps,
	}

	// Make the directory for the output depfile in case it is in a different directory
	// than any of the output files.
	outDir := filepath.Dir(out)
	err := os.MkdirAll(outDir, 0777)
	if err != nil {
		return fmt.Errorf("failed to create %q: %w", outDir, err)
	}

	return ioutil.WriteFile(out, deps.Print(), 0666)
}

// joinPath wraps filepath.Join but returns file without appending to dir if file is
// absolute.
func joinPath(dir, file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join(dir, file)
}

func makeAbsPathEnv(pathEnv string) (string, error) {
	pathEnvElements := filepath.SplitList(pathEnv)
	for i, p := range pathEnvElements {
		if !filepath.IsAbs(p) {
			absPath, err := filepath.Abs(p)
			if err != nil {
				return "", fmt.Errorf("failed to make PATH entry %q absolute: %w", p, err)
			}
			pathEnvElements[i] = absPath
		}
	}
	return strings.Join(pathEnvElements, string(filepath.ListSeparator)), nil
}
