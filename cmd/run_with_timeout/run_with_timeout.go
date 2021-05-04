// Copyright 2021 Google Inc. All rights reserved.
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

// run_with_timeout is a utility that can kill a wrapped command after a configurable timeout,
// optionally running a command to collect debugging information first.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

var (
	timeout      = flag.Duration("timeout", 0, "time after which to kill command (example: 60s)")
	onTimeoutCmd = flag.String("on_timeout", "", "command to run with `PID=<pid> sh -c` after timeout.")
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [--timeout N] [--on_timeout CMD] -- command [args...]\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "run_with_timeout is a utility that can kill a wrapped command after a configurable timeout,")
	fmt.Fprintln(os.Stderr, "optionally running a command to collect debugging information first.")

	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "command is required")
		usage()
	}

	err := runWithTimeout(flag.Arg(0), flag.Args()[1:], *timeout, *onTimeoutCmd,
		os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Fprintln(os.Stderr, "process exited with error:", exitErr.Error())
		} else {
			fmt.Fprintln(os.Stderr, "error:", err.Error())
		}
		os.Exit(1)
	}
}

func runWithTimeout(command string, args []string, timeout time.Duration, onTimeoutCmdStr string,
	stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(command, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	err := cmd.Start()
	if err != nil {
		return err
	}

	// waitCh will signal the subprocess exited.
	waitCh := make(chan error)
	go func() {
		waitCh <- cmd.Wait()
	}()

	// timeoutCh will signal the subprocess timed out if timeout was set.
	var timeoutCh <-chan time.Time = make(chan time.Time)
	if timeout > 0 {
		timeoutCh = time.After(timeout)
	}

	select {
	case err := <-waitCh:
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("process exited with error: %w", exitErr)
		}
		return err
	case <-timeoutCh:
		// Continue below.
	}

	// Process timed out before exiting.
	defer cmd.Process.Signal(syscall.SIGKILL)

	if onTimeoutCmdStr != "" {
		onTimeoutCmd := exec.Command("sh", "-c", onTimeoutCmdStr)
		onTimeoutCmd.Stdin, onTimeoutCmd.Stdout, onTimeoutCmd.Stderr = stdin, stdout, stderr
		onTimeoutCmd.Env = append(os.Environ(), fmt.Sprintf("PID=%d", cmd.Process.Pid))
		err := onTimeoutCmd.Run()
		if err != nil {
			return fmt.Errorf("on_timeout command %q exited with error: %w", onTimeoutCmdStr, err)
		}
	}

	return fmt.Errorf("timed out after %s", timeout.String())
}
