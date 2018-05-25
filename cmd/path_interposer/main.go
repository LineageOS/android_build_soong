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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"android/soong/ui/build/paths"
)

func main() {
	interposer, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to locate interposer executable:", err)
		os.Exit(1)
	}

	if fi, err := os.Lstat(interposer); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(interposer)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Unable to read link to interposer executable:", err)
				os.Exit(1)
			}
			if filepath.IsAbs(link) {
				interposer = link
			} else {
				interposer = filepath.Join(filepath.Dir(interposer), link)
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "Unable to stat interposer executable:", err)
		os.Exit(1)
	}

	disableError := false
	if e, ok := os.LookupEnv("TEMPORARY_DISABLE_PATH_RESTRICTIONS"); ok {
		disableError = e == "1" || e == "y" || e == "yes" || e == "on" || e == "true"
	}

	exitCode, err := Main(os.Stdout, os.Stderr, interposer, os.Args, mainOpts{
		disableError: disableError,

		sendLog:       paths.SendLog,
		config:        paths.GetConfig,
		lookupParents: lookupParents,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	os.Exit(exitCode)
}

var usage = fmt.Errorf(`To use the PATH interposer:
 * Write the original PATH variable to <interposer>_origpath
 * Set up a directory of symlinks to the PATH interposer, and use that in PATH

If a tool isn't in the allowed list, a log will be posted to the unix domain
socket at <interposer>_log.`)

type mainOpts struct {
	disableError bool

	sendLog       func(logSocket string, entry *paths.LogEntry, done chan interface{})
	config        func(name string) paths.PathConfig
	lookupParents func() []paths.LogProcess
}

func Main(stdout, stderr io.Writer, interposer string, args []string, opts mainOpts) (int, error) {
	base := filepath.Base(args[0])

	origPathFile := interposer + "_origpath"
	if base == filepath.Base(interposer) {
		return 1, usage
	}

	origPath, err := ioutil.ReadFile(origPathFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, usage
		} else {
			return 1, fmt.Errorf("Failed to read original PATH: %v", err)
		}
	}

	cmd := &exec.Cmd{
		Args: args,
		Env:  os.Environ(),

		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	if err := os.Setenv("PATH", string(origPath)); err != nil {
		return 1, fmt.Errorf("Failed to set PATH env: %v", err)
	}

	if config := opts.config(base); config.Log || config.Error {
		var procs []paths.LogProcess
		if opts.lookupParents != nil {
			procs = opts.lookupParents()
		}

		if opts.sendLog != nil {
			waitForLog := make(chan interface{})
			opts.sendLog(interposer+"_log", &paths.LogEntry{
				Basename: base,
				Args:     args,
				Parents:  procs,
			}, waitForLog)
			defer func() { <-waitForLog }()
		}
		if config.Error && !opts.disableError {
			return 1, fmt.Errorf("%q is not allowed to be used. See https://android.googlesource.com/platform/build/+/master/Changes.md#PATH_Tools for more information.", base)
		}
	}

	cmd.Path, err = exec.LookPath(base)
	if err != nil {
		return 1, err
	}

	if err = cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.Exited() {
					return status.ExitStatus(), nil
				} else if status.Signaled() {
					exitCode := 128 + int(status.Signal())
					return exitCode, nil
				} else {
					return 1, exitErr
				}
			} else {
				return 1, nil
			}
		}
	}

	return 0, nil
}

type procEntry struct {
	Pid     int
	Ppid    int
	Command string
}

func readProcs() map[int]procEntry {
	cmd := exec.Command("ps", "-o", "pid,ppid,command")
	data, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseProcs(data)
}

func parseProcs(data []byte) map[int]procEntry {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 2 {
		return nil
	}
	// Remove the header
	lines = lines[1:]

	ret := make(map[int]procEntry, len(lines))
	for _, line := range lines {
		fields := bytes.SplitN(line, []byte(" "), 2)
		if len(fields) != 2 {
			continue
		}

		pid, err := strconv.Atoi(string(fields[0]))
		if err != nil {
			continue
		}

		line = bytes.TrimLeft(fields[1], " ")

		fields = bytes.SplitN(line, []byte(" "), 2)
		if len(fields) != 2 {
			continue
		}

		ppid, err := strconv.Atoi(string(fields[0]))
		if err != nil {
			continue
		}

		ret[pid] = procEntry{
			Pid:     pid,
			Ppid:    ppid,
			Command: string(bytes.TrimLeft(fields[1], " ")),
		}
	}

	return ret
}

func lookupParents() []paths.LogProcess {
	procs := readProcs()
	if procs == nil {
		return nil
	}

	list := []paths.LogProcess{}
	pid := os.Getpid()
	for {
		entry, ok := procs[pid]
		if !ok {
			break
		}

		list = append([]paths.LogProcess{
			{
				Pid:     pid,
				Command: entry.Command,
			},
		}, list...)

		pid = entry.Ppid
	}

	return list
}
