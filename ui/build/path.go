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

package build

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/blueprint/microfactory"

	"android/soong/ui/build/paths"
	"android/soong/ui/metrics"
)

func parsePathDir(dir string) []string {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()

	if s, err := f.Stat(); err != nil || !s.IsDir() {
		return nil
	}

	infos, err := f.Readdir(-1)
	if err != nil {
		return nil
	}

	ret := make([]string, 0, len(infos))
	for _, info := range infos {
		if m := info.Mode(); !m.IsDir() && m&0111 != 0 {
			ret = append(ret, info.Name())
		}
	}
	return ret
}

// A "lite" version of SetupPath used for dumpvars, or other places that need
// minimal overhead (but at the expense of logging). If tmpDir is empty, the
// default TMPDIR is used from config.
func SetupLitePath(ctx Context, config Config, tmpDir string) {
	if config.pathReplaced {
		return
	}

	ctx.BeginTrace(metrics.RunSetupTool, "litepath")
	defer ctx.EndTrace()

	origPath, _ := config.Environment().Get("PATH")

	if tmpDir == "" {
		tmpDir, _ = config.Environment().Get("TMPDIR")
	}
	myPath := filepath.Join(tmpDir, "path")
	ensureEmptyDirectoriesExist(ctx, myPath)

	os.Setenv("PATH", origPath)
	for name, pathConfig := range paths.Configuration {
		if !pathConfig.Symlink {
			continue
		}

		origExec, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		origExec, err = filepath.Abs(origExec)
		if err != nil {
			continue
		}

		err = os.Symlink(origExec, filepath.Join(myPath, name))
		if err != nil {
			ctx.Fatalln("Failed to create symlink:", err)
		}
	}

	myPath, _ = filepath.Abs(myPath)

	prebuiltsPath, _ := filepath.Abs("prebuilts/build-tools/path/" + runtime.GOOS + "-x86")
	myPath = prebuiltsPath + string(os.PathListSeparator) + myPath

	config.Environment().Set("PATH", myPath)
	config.pathReplaced = true
}

func SetupPath(ctx Context, config Config) {
	if config.pathReplaced {
		return
	}

	ctx.BeginTrace(metrics.RunSetupTool, "path")
	defer ctx.EndTrace()

	origPath, _ := config.Environment().Get("PATH")
	myPath := filepath.Join(config.OutDir(), ".path")
	interposer := myPath + "_interposer"

	var cfg microfactory.Config
	cfg.Map("android/soong", "build/soong")
	cfg.TrimPath, _ = filepath.Abs(".")
	if _, err := microfactory.Build(&cfg, interposer, "android/soong/cmd/path_interposer"); err != nil {
		ctx.Fatalln("Failed to build path interposer:", err)
	}

	if err := ioutil.WriteFile(interposer+"_origpath", []byte(origPath), 0777); err != nil {
		ctx.Fatalln("Failed to write original path:", err)
	}

	entries, err := paths.LogListener(ctx.Context, interposer+"_log")
	if err != nil {
		ctx.Fatalln("Failed to listen for path logs:", err)
	}

	go func() {
		for log := range entries {
			curPid := os.Getpid()
			for i, proc := range log.Parents {
				if proc.Pid == curPid {
					log.Parents = log.Parents[i:]
					break
				}
			}
			procPrints := []string{
				"See https://android.googlesource.com/platform/build/+/master/Changes.md#PATH_Tools for more information.",
			}
			if len(log.Parents) > 0 {
				procPrints = append(procPrints, "Process tree:")
				for i, proc := range log.Parents {
					procPrints = append(procPrints, fmt.Sprintf("%s→ %s", strings.Repeat(" ", i), proc.Command))
				}
			}

			config := paths.GetConfig(log.Basename)
			if config.Error {
				ctx.Printf("Disallowed PATH tool %q used: %#v", log.Basename, log.Args)
				for _, line := range procPrints {
					ctx.Println(line)
				}
			} else {
				ctx.Verbosef("Unknown PATH tool %q used: %#v", log.Basename, log.Args)
				for _, line := range procPrints {
					ctx.Verboseln(line)
				}
			}
		}
	}()

	ensureEmptyDirectoriesExist(ctx, myPath)

	var execs []string
	for _, pathEntry := range filepath.SplitList(origPath) {
		if pathEntry == "" {
			// Ignore the current directory
			continue
		}
		// TODO(dwillemsen): remove path entries under TOP? or anything
		// that looks like an android source dir? They won't exist on
		// the build servers, since they're added by envsetup.sh.
		// (Except for the JDK, which is configured in ui/build/config.go)

		execs = append(execs, parsePathDir(pathEntry)...)
	}

	allowAllSymlinks := config.Environment().IsEnvTrue("TEMPORARY_DISABLE_PATH_RESTRICTIONS")
	for _, name := range execs {
		if !paths.GetConfig(name).Symlink && !allowAllSymlinks {
			continue
		}

		err := os.Symlink("../.path_interposer", filepath.Join(myPath, name))
		// Intentionally ignore existing files -- that means that we
		// just created it, and the first one should win.
		if err != nil && !os.IsExist(err) {
			ctx.Fatalln("Failed to create symlink:", err)
		}
	}

	myPath, _ = filepath.Abs(myPath)

	// We put some prebuilts in $PATH, since it's infeasible to add dependencies for all of
	// them.
	prebuiltsPath, _ := filepath.Abs("prebuilts/build-tools/path/" + runtime.GOOS + "-x86")
	myPath = prebuiltsPath + string(os.PathListSeparator) + myPath

	config.Environment().Set("PATH", myPath)
	config.pathReplaced = true
}
