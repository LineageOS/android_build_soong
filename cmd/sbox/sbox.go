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
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android/soong/cmd/sbox/sbox_proto"
	"android/soong/makedeps"
	"android/soong/response"

	"google.golang.org/protobuf/encoding/prototext"
)

var (
	sandboxesRoot  string
	outputDir      string
	manifestFile   string
	keepOutDir     bool
	writeIfChanged bool
)

const (
	depFilePlaceholder    = "__SBOX_DEPFILE__"
	sandboxDirPlaceholder = "__SBOX_SANDBOX_DIR__"
)

func init() {
	flag.StringVar(&sandboxesRoot, "sandbox-path", "",
		"root of temp directory to put the sandbox into")
	flag.StringVar(&outputDir, "output-dir", "",
		"directory which will contain all output files and only output files")
	flag.StringVar(&manifestFile, "manifest", "",
		"textproto manifest describing the sandboxed command(s)")
	flag.BoolVar(&keepOutDir, "keep-out-dir", false,
		"whether to keep the sandbox directory when done")
	flag.BoolVar(&writeIfChanged, "write-if-changed", false,
		"only write the output files if they have changed")
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
	if err != nil {
		return err
	}

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
		depFile, err := runCommand(command, localTempDir, i)
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

// createCommandScript will create and return an exec.Cmd that runs rawCommand.
//
// rawCommand is executed via a script in the sandbox.
// scriptPath is the temporary where the script is created.
// scriptPathInSandbox is the path to the script in the sbox environment.
//
// returns an exec.Cmd that can be ran from within sbox context if no error, or nil if error.
// caller must ensure script is cleaned up if function succeeds.
func createCommandScript(rawCommand, scriptPath, scriptPathInSandbox string) (*exec.Cmd, error) {
	err := os.WriteFile(scriptPath, []byte(rawCommand), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write command %s... to %s",
			rawCommand[0:40], scriptPath)
	}
	return exec.Command("bash", scriptPathInSandbox), nil
}

// readManifest reads an sbox manifest from a textproto file.
func readManifest(file string) (*sbox_proto.Manifest, error) {
	manifestData, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest %q: %w", file, err)
	}

	manifest := sbox_proto.Manifest{}

	err = prototext.Unmarshal(manifestData, &manifest)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest %q: %w", file, err)
	}

	return &manifest, nil
}

// runCommand runs a single command from a manifest.  If the command references the
// __SBOX_DEPFILE__ placeholder it returns the name of the depfile that was used.
func runCommand(command *sbox_proto.Command, tempDir string, commandIndex int) (depFile string, err error) {
	rawCommand := command.GetCommand()
	if rawCommand == "" {
		return "", fmt.Errorf("command is required")
	}

	// Remove files from the output directory
	err = clearOutputDirectory(command.CopyAfter, outputDir, writeType(writeIfChanged))
	if err != nil {
		return "", err
	}

	pathToTempDirInSbox := tempDir
	if command.GetChdir() {
		pathToTempDirInSbox = "."
	}

	err = os.MkdirAll(tempDir, 0777)
	if err != nil {
		return "", fmt.Errorf("failed to create %q: %w", tempDir, err)
	}

	// Copy in any files specified by the manifest.
	err = copyFiles(command.CopyBefore, "", tempDir, requireFromExists, alwaysWrite)
	if err != nil {
		return "", err
	}
	err = copyRspFiles(command.RspFiles, tempDir, pathToTempDirInSbox)
	if err != nil {
		return "", err
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

	scriptName := fmt.Sprintf("sbox_command.%d.bash", commandIndex)
	scriptPath := joinPath(tempDir, scriptName)
	scriptPathInSandbox := joinPath(pathToTempDirInSbox, scriptName)
	cmd, err := createCommandScript(rawCommand, scriptPath, scriptPathInSandbox)
	if err != nil {
		return "", err
	}

	buf := &bytes.Buffer{}
	cmd.Stdin = os.Stdin
	cmd.Stdout = buf
	cmd.Stderr = buf

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

	if err != nil {
		// The command failed, do a best effort copy of output files out of the sandbox.  This is
		// especially useful for linters with baselines that print an error message on failure
		// with a command to copy the output lint errors to the new baseline.  Use a copy instead of
		// a move to leave the sandbox intact for manual inspection
		copyFiles(command.CopyAfter, tempDir, "", allowFromNotExists, writeType(writeIfChanged))
	}

	// If the command  was executed but failed with an error, print a debugging message before
	// the command's output so it doesn't scroll the real error message off the screen.
	if exit, ok := err.(*exec.ExitError); ok && !exit.Success() {
		fmt.Fprintf(os.Stderr,
			"The failing command was run inside an sbox sandbox in temporary directory\n"+
				"%s\n"+
				"The failing command line can be found in\n"+
				"%s\n",
			tempDir, scriptPath)
	}

	// Write the command's combined stdout/stderr.
	os.Stdout.Write(buf.Bytes())

	if err != nil {
		return "", err
	}

	err = validateOutputFiles(command.CopyAfter, tempDir, outputDir, rawCommand)
	if err != nil {
		return "", err
	}

	// the created files match the declared files; now move them
	err = moveFiles(command.CopyAfter, tempDir, "", writeType(writeIfChanged))
	if err != nil {
		return "", err
	}

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
func validateOutputFiles(copies []*sbox_proto.Copy, sandboxDir, outputDir, rawCommand string) error {
	var missingOutputErrors []error
	var incorrectOutputDirectoryErrors []error
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

		toPath := copyPair.GetTo()
		if rel, err := filepath.Rel(outputDir, toPath); err != nil {
			return err
		} else if strings.HasPrefix(rel, "../") {
			incorrectOutputDirectoryErrors = append(incorrectOutputDirectoryErrors,
				fmt.Errorf("%s is not under %s", toPath, outputDir))
		}
	}

	const maxErrors = 25

	if len(incorrectOutputDirectoryErrors) > 0 {
		errorMessage := ""
		more := 0
		if len(incorrectOutputDirectoryErrors) > maxErrors {
			more = len(incorrectOutputDirectoryErrors) - maxErrors
			incorrectOutputDirectoryErrors = incorrectOutputDirectoryErrors[:maxErrors]
		}

		for _, err := range incorrectOutputDirectoryErrors {
			errorMessage += err.Error() + "\n"
		}
		if more > 0 {
			errorMessage += fmt.Sprintf("...%v more", more)
		}

		return errors.New(errorMessage)
	}

	if len(missingOutputErrors) > 0 {
		// find all created files for making a more informative error message
		createdFiles := findAllFilesUnder(sandboxDir)

		// build error message
		errorMessage := "mismatch between declared and actual outputs\n"
		errorMessage += "in sbox command(" + rawCommand + ")\n\n"
		errorMessage += "in sandbox " + sandboxDir + ",\n"
		errorMessage += fmt.Sprintf("failed to create %v files:\n", len(missingOutputErrors))
		for _, missingOutputError := range missingOutputErrors {
			errorMessage += "  " + missingOutputError.Error() + "\n"
		}
		if len(createdFiles) < 1 {
			errorMessage += "created 0 files."
		} else {
			errorMessage += fmt.Sprintf("did create %v files:\n", len(createdFiles))
			creationMessages := createdFiles
			if len(creationMessages) > maxErrors {
				creationMessages = creationMessages[:maxErrors]
				creationMessages = append(creationMessages, fmt.Sprintf("...%v more", len(createdFiles)-maxErrors))
			}
			for _, creationMessage := range creationMessages {
				errorMessage += "  " + creationMessage + "\n"
			}
		}

		return errors.New(errorMessage)
	}

	return nil
}

type existsType bool

const (
	requireFromExists  existsType = false
	allowFromNotExists            = true
)

type writeType bool

const (
	alwaysWrite        writeType = false
	onlyWriteIfChanged           = true
)

// copyFiles copies files in or out of the sandbox.  If exists is allowFromNotExists then errors
// caused by a from path not existing are ignored.  If write is onlyWriteIfChanged then the output
// file is compared to the input file and not written to if it is the same, avoiding updating
// the timestamp.
func copyFiles(copies []*sbox_proto.Copy, fromDir, toDir string, exists existsType, write writeType) error {
	for _, copyPair := range copies {
		fromPath := joinPath(fromDir, copyPair.GetFrom())
		toPath := joinPath(toDir, copyPair.GetTo())
		err := copyOneFile(fromPath, toPath, copyPair.GetExecutable(), exists, write)
		if err != nil {
			return fmt.Errorf("error copying %q to %q: %w", fromPath, toPath, err)
		}
	}
	return nil
}

// copyOneFile copies a file and its permissions.  If forceExecutable is true it adds u+x to the
// permissions.  If exists is allowFromNotExists it returns nil if the from path doesn't exist.
// If write is onlyWriteIfChanged then the output file is compared to the input file and not written to
// if it is the same, avoiding updating the timestamp. If from is a symlink, the symlink itself
// will be copied, instead of what it points to.
func copyOneFile(from string, to string, forceExecutable bool, exists existsType,
	write writeType) error {
	err := os.MkdirAll(filepath.Dir(to), 0777)
	if err != nil {
		return err
	}

	stat, err := os.Lstat(from)
	if err != nil {
		if os.IsNotExist(err) && exists == allowFromNotExists {
			return nil
		}
		return err
	}

	if stat.Mode()&fs.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(from)
		if err != nil {
			return err
		}
		if write == onlyWriteIfChanged {
			toLinkTarget, err := os.Readlink(to)
			if err == nil && toLinkTarget == linkTarget {
				return nil
			}
		}
		err = os.Remove(to)
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		return os.Symlink(linkTarget, to)
	}

	perm := stat.Mode()
	if forceExecutable {
		perm = perm | 0100 // u+x
	}

	if write == onlyWriteIfChanged && filesHaveSameContents(from, to) {
		return nil
	}

	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	// Remove the target before copying.  In most cases the file won't exist, but if there are
	// duplicate copy rules for a file and the source file was read-only the second copy could
	// fail.
	err = os.Remove(to)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

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

// copyRspFiles copies rsp files into the sandbox with path mappings, and also copies the files
// listed into the sandbox.
func copyRspFiles(rspFiles []*sbox_proto.RspFile, toDir, toDirInSandbox string) error {
	for _, rspFile := range rspFiles {
		err := copyOneRspFile(rspFile, toDir, toDirInSandbox)
		if err != nil {
			return err
		}
	}
	return nil
}

// copyOneRspFiles copies an rsp file into the sandbox with path mappings, and also copies the files
// listed into the sandbox.
func copyOneRspFile(rspFile *sbox_proto.RspFile, toDir, toDirInSandbox string) error {
	in, err := os.Open(rspFile.GetFile())
	if err != nil {
		return err
	}
	defer in.Close()

	files, err := response.ReadRspFile(in)
	if err != nil {
		return err
	}

	for i, from := range files {
		// Convert the real path of the input file into the path inside the sandbox using the
		// path mappings.
		to := applyPathMappings(rspFile.PathMappings, from)

		// Copy the file into the sandbox.
		err := copyOneFile(from, joinPath(toDir, to), false, requireFromExists, alwaysWrite)
		if err != nil {
			return err
		}

		// Rewrite the name in the list of files to be relative to the sandbox directory.
		files[i] = joinPath(toDirInSandbox, to)
	}

	// Convert the real path of the rsp file into the path inside the sandbox using the path
	// mappings.
	outRspFile := joinPath(toDir, applyPathMappings(rspFile.PathMappings, rspFile.GetFile()))

	err = os.MkdirAll(filepath.Dir(outRspFile), 0777)
	if err != nil {
		return err
	}

	out, err := os.Create(outRspFile)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the rsp file with converted paths into the sandbox.
	err = response.WriteRspFile(out, files)
	if err != nil {
		return err
	}

	return nil
}

// applyPathMappings takes a list of path mappings and a path, and returns the path with the first
// matching path mapping applied.  If the path does not match any of the path mappings then it is
// returned unmodified.
func applyPathMappings(pathMappings []*sbox_proto.PathMapping, path string) string {
	for _, mapping := range pathMappings {
		if strings.HasPrefix(path, mapping.GetFrom()+"/") {
			return joinPath(mapping.GetTo()+"/", strings.TrimPrefix(path, mapping.GetFrom()+"/"))
		}
	}
	return path
}

// moveFiles moves files specified by a set of copy rules.  It uses os.Rename, so it is restricted
// to moving files where the source and destination are in the same filesystem.  This is OK for
// sbox because the temporary directory is inside the out directory.  If write is onlyWriteIfChanged
// then the output file is compared to the input file and not written to if it is the same, avoiding
// updating the timestamp.  Otherwise it always updates the timestamp of the new file.
func moveFiles(copies []*sbox_proto.Copy, fromDir, toDir string, write writeType) error {
	for _, copyPair := range copies {
		fromPath := joinPath(fromDir, copyPair.GetFrom())
		toPath := joinPath(toDir, copyPair.GetTo())
		err := os.MkdirAll(filepath.Dir(toPath), 0777)
		if err != nil {
			return err
		}

		if write == onlyWriteIfChanged && filesHaveSameContents(fromPath, toPath) {
			continue
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

// clearOutputDirectory removes all files in the output directory if write is alwaysWrite, or
// any files not listed in copies if write is onlyWriteIfChanged
func clearOutputDirectory(copies []*sbox_proto.Copy, outputDir string, write writeType) error {
	if outputDir == "" {
		return fmt.Errorf("output directory must be set")
	}

	if write == alwaysWrite {
		// When writing all the output files remove the whole output directory
		return os.RemoveAll(outputDir)
	}

	outputFiles := make(map[string]bool, len(copies))
	for _, copyPair := range copies {
		outputFiles[copyPair.GetTo()] = true
	}

	existingFiles := findAllFilesUnder(outputDir)
	for _, existingFile := range existingFiles {
		fullExistingFile := filepath.Join(outputDir, existingFile)
		if !outputFiles[fullExistingFile] {
			err := os.Remove(fullExistingFile)
			if err != nil {
				return fmt.Errorf("failed to remove obsolete output file %s: %w", fullExistingFile, err)
			}
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

// filesHaveSameContents compares the contents if two files, returning true if they are the same
// and returning false if they are different or any errors occur.
func filesHaveSameContents(a, b string) bool {
	// Compare the sizes of the two files
	statA, err := os.Stat(a)
	if err != nil {
		return false
	}
	statB, err := os.Stat(b)
	if err != nil {
		return false
	}

	if statA.Size() != statB.Size() {
		return false
	}

	// Open the two files
	fileA, err := os.Open(a)
	if err != nil {
		return false
	}
	defer fileA.Close()
	fileB, err := os.Open(b)
	if err != nil {
		return false
	}
	defer fileB.Close()

	// Compare the files 1MB at a time
	const bufSize = 1 * 1024 * 1024
	bufA := make([]byte, bufSize)
	bufB := make([]byte, bufSize)

	remain := statA.Size()
	for remain > 0 {
		toRead := int64(bufSize)
		if toRead > remain {
			toRead = remain
		}

		_, err = io.ReadFull(fileA, bufA[:toRead])
		if err != nil {
			return false
		}
		_, err = io.ReadFull(fileB, bufB[:toRead])
		if err != nil {
			return false
		}

		if bytes.Compare(bufA[:toRead], bufB[:toRead]) != 0 {
			return false
		}

		remain -= toRead
	}

	return true
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
