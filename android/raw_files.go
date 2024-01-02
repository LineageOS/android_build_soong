// Copyright 2023 Google Inc. All rights reserved.
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

package android

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/google/blueprint"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/blueprint/proptools"
)

// WriteFileRule creates a ninja rule to write contents to a file by immediately writing the
// contents, plus a trailing newline, to a file in out/soong/raw-${TARGET_PRODUCT}, and then creating
// a ninja rule to copy the file into place.
func WriteFileRule(ctx BuilderContext, outputFile WritablePath, content string) {
	writeFileRule(ctx, outputFile, content, true, false)
}

// WriteFileRuleVerbatim creates a ninja rule to write contents to a file by immediately writing the
// contents to a file in out/soong/raw-${TARGET_PRODUCT}, and then creating a ninja rule to copy the file into place.
func WriteFileRuleVerbatim(ctx BuilderContext, outputFile WritablePath, content string) {
	writeFileRule(ctx, outputFile, content, false, false)
}

// WriteExecutableFileRuleVerbatim is the same as WriteFileRuleVerbatim, but runs chmod +x on the result
func WriteExecutableFileRuleVerbatim(ctx BuilderContext, outputFile WritablePath, content string) {
	writeFileRule(ctx, outputFile, content, false, true)
}

// tempFile provides a testable wrapper around a file in out/soong/.temp.  It writes to a temporary file when
// not in tests, but writes to a buffer in memory when used in tests.
type tempFile struct {
	// tempFile contains wraps an io.Writer, which will be file if testMode is false, or testBuf if it is true.
	io.Writer

	file    *os.File
	testBuf *strings.Builder
}

func newTempFile(ctx BuilderContext, pattern string, testMode bool) *tempFile {
	if testMode {
		testBuf := &strings.Builder{}
		return &tempFile{
			Writer:  testBuf,
			testBuf: testBuf,
		}
	} else {
		f, err := os.CreateTemp(absolutePath(ctx.Config().tempDir()), pattern)
		if err != nil {
			panic(fmt.Errorf("failed to open temporary raw file: %w", err))
		}
		return &tempFile{
			Writer: f,
			file:   f,
		}
	}
}

func (t *tempFile) close() error {
	if t.file != nil {
		return t.file.Close()
	}
	return nil
}

func (t *tempFile) name() string {
	if t.file != nil {
		return t.file.Name()
	}
	return "temp_file_in_test"
}

func (t *tempFile) rename(to string) {
	if t.file != nil {
		os.MkdirAll(filepath.Dir(to), 0777)
		err := os.Rename(t.file.Name(), to)
		if err != nil {
			panic(fmt.Errorf("failed to rename %s to %s: %w", t.file.Name(), to, err))
		}
	}
}

func (t *tempFile) remove() error {
	if t.file != nil {
		return os.Remove(t.file.Name())
	}
	return nil
}

func writeContentToTempFileAndHash(ctx BuilderContext, content string, newline bool) (*tempFile, string) {
	tempFile := newTempFile(ctx, "raw", ctx.Config().captureBuild)
	defer tempFile.close()

	hash := sha1.New()
	w := io.MultiWriter(tempFile, hash)

	_, err := io.WriteString(w, content)
	if err == nil && newline {
		_, err = io.WriteString(w, "\n")
	}
	if err != nil {
		panic(fmt.Errorf("failed to write to temporary raw file %s: %w", tempFile.name(), err))
	}
	return tempFile, hex.EncodeToString(hash.Sum(nil))
}

func writeFileRule(ctx BuilderContext, outputFile WritablePath, content string, newline bool, executable bool) {
	// Write the contents to a temporary file while computing its hash.
	tempFile, hash := writeContentToTempFileAndHash(ctx, content, newline)

	// Shard the final location of the raw file into a subdirectory based on the first two characters of the
	// hash to avoid making the raw directory too large and slowing down accesses.
	relPath := filepath.Join(hash[0:2], hash)

	// These files are written during soong_build.  If something outside the build deleted them there would be no
	// trigger to rerun soong_build, and the build would break with dependencies on missing files.  Writing them
	// to their final locations would risk having them deleted when cleaning a module, and would also pollute the
	// output directory with files for modules that have never been built.
	// Instead, the files are written to a separate "raw" directory next to the build.ninja file, and a ninja
	// rule is created to copy the files into their final location as needed.
	// Obsolete files written by previous runs of soong_build must be cleaned up to avoid continually growing
	// disk usage as the hashes of the files change over time.  The cleanup must not remove files that were
	// created by previous runs of soong_build for other products, as the build.ninja files for those products
	// may still exist and still reference those files.  The raw files from different products are kept
	// separate by appending the Make_suffix to the directory name.
	rawPath := PathForOutput(ctx, "raw"+proptools.String(ctx.Config().productVariables.Make_suffix), relPath)

	rawFileInfo := rawFileInfo{
		relPath: relPath,
	}

	if ctx.Config().captureBuild {
		// When running tests tempFile won't write to disk, instead store the contents for later retrieval by
		// ContentFromFileRuleForTests.
		rawFileInfo.contentForTests = tempFile.testBuf.String()
	}

	rawFileSet := getRawFileSet(ctx.Config())
	if _, exists := rawFileSet.LoadOrStore(hash, rawFileInfo); exists {
		// If a raw file with this hash has already been created delete the temporary file.
		tempFile.remove()
	} else {
		// If this is the first time this hash has been seen then move it from the temporary directory
		// to the raw directory.  If the file already exists in the raw directory assume it has the correct
		// contents.
		absRawPath := absolutePath(rawPath.String())
		_, err := os.Stat(absRawPath)
		if os.IsNotExist(err) {
			tempFile.rename(absRawPath)
		} else if err != nil {
			panic(fmt.Errorf("failed to stat %q: %w", absRawPath, err))
		} else {
			tempFile.remove()
		}
	}

	// Emit a rule to copy the file from raw directory to the final requested location in the output tree.
	// Restat is used to ensure that two different products that produce identical files copied from their
	// own raw directories they don't cause everything downstream to rebuild.
	rule := rawFileCopy
	if executable {
		rule = rawFileCopyExecutable
	}
	ctx.Build(pctx, BuildParams{
		Rule:        rule,
		Input:       rawPath,
		Output:      outputFile,
		Description: "raw " + outputFile.Base(),
	})
}

var (
	rawFileCopy = pctx.AndroidStaticRule("rawFileCopy",
		blueprint.RuleParams{
			Command:     "if ! cmp -s $in $out; then cp $in $out; fi",
			Description: "copy raw file $out",
			Restat:      true,
		})
	rawFileCopyExecutable = pctx.AndroidStaticRule("rawFileCopyExecutable",
		blueprint.RuleParams{
			Command:     "if ! cmp -s $in $out; then cp $in $out; fi && chmod +x $out",
			Description: "copy raw exectuable file $out",
			Restat:      true,
		})
)

type rawFileInfo struct {
	relPath         string
	contentForTests string
}

var rawFileSetKey OnceKey = NewOnceKey("raw file set")

func getRawFileSet(config Config) *SyncMap[string, rawFileInfo] {
	return config.Once(rawFileSetKey, func() any {
		return &SyncMap[string, rawFileInfo]{}
	}).(*SyncMap[string, rawFileInfo])
}

// ContentFromFileRuleForTests returns the content that was passed to a WriteFileRule for use
// in tests.
func ContentFromFileRuleForTests(t *testing.T, ctx *TestContext, params TestingBuildParams) string {
	t.Helper()
	if params.Rule != rawFileCopy && params.Rule != rawFileCopyExecutable {
		t.Errorf("expected params.Rule to be rawFileCopy or rawFileCopyExecutable, was %q", params.Rule)
		return ""
	}

	key := filepath.Base(params.Input.String())
	rawFileSet := getRawFileSet(ctx.Config())
	rawFileInfo, _ := rawFileSet.Load(key)

	return rawFileInfo.contentForTests
}

func rawFilesSingletonFactory() Singleton {
	return &rawFilesSingleton{}
}

type rawFilesSingleton struct{}

func (rawFilesSingleton) GenerateBuildActions(ctx SingletonContext) {
	if ctx.Config().captureBuild {
		// Nothing to do when running in tests, no temporary files were created.
		return
	}
	rawFileSet := getRawFileSet(ctx.Config())
	rawFilesDir := PathForOutput(ctx, "raw"+proptools.String(ctx.Config().productVariables.Make_suffix)).String()
	absRawFilesDir := absolutePath(rawFilesDir)
	err := filepath.WalkDir(absRawFilesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Ignore obsolete directories for now.
			return nil
		}

		// Assume the basename of the file is a hash
		key := filepath.Base(path)
		relPath, err := filepath.Rel(absRawFilesDir, path)
		if err != nil {
			return err
		}

		// Check if a file with the same hash was written by this run of soong_build.  If the file was not written,
		// or if a file with the same hash was written but to a different path in the raw directory, then delete it.
		// Checking that the path matches allows changing the structure of the raw directory, for example to increase
		// the sharding.
		rawFileInfo, written := rawFileSet.Load(key)
		if !written || rawFileInfo.relPath != relPath {
			os.Remove(path)
		}
		return nil
	})
	if err != nil {
		panic(fmt.Errorf("failed to clean %q: %w", rawFilesDir, err))
	}
}
