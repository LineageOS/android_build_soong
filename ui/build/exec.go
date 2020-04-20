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

package build

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Cmd is a wrapper of os/exec.Cmd that integrates with the build context for
// logging, the config's Environment for simpler environment modification, and
// implements hooks for sandboxing
type Cmd struct {
	*exec.Cmd

	Environment *Environment
	Sandbox     Sandbox

	ctx    Context
	config Config
	name   string

	started time.Time
}

func Command(ctx Context, config Config, name string, executable string, args ...string) *Cmd {
	ret := &Cmd{
		Cmd:         exec.CommandContext(ctx.Context, executable, args...),
		Environment: config.Environment().Copy(),
		Sandbox:     noSandbox,

		ctx:    ctx,
		config: config,
		name:   name,
	}

	return ret
}

func (c *Cmd) prepare() {
	if c.Env == nil {
		c.Env = c.Environment.Environ()
	}
	if c.sandboxSupported() {
		c.wrapSandbox()
	}

	c.ctx.Verbosef("%q executing %q %v\n", c.name, c.Path, c.Args)
	c.started = time.Now()
}

func (c *Cmd) report() {
	if c.Cmd.ProcessState != nil {
		rusage := c.Cmd.ProcessState.SysUsage().(*syscall.Rusage)
		c.ctx.Verbosef("%q finished with exit code %d (%s real, %s user, %s system, %dMB maxrss)",
			c.name, c.Cmd.ProcessState.ExitCode(),
			time.Since(c.started).Round(time.Millisecond),
			c.Cmd.ProcessState.UserTime().Round(time.Millisecond),
			c.Cmd.ProcessState.SystemTime().Round(time.Millisecond),
			rusage.Maxrss/1024)
	}
}

func (c *Cmd) Start() error {
	c.prepare()
	return c.Cmd.Start()
}

func (c *Cmd) Run() error {
	c.prepare()
	err := c.Cmd.Run()
	c.report()
	return err
}

func (c *Cmd) Output() ([]byte, error) {
	c.prepare()
	bytes, err := c.Cmd.Output()
	c.report()
	return bytes, err
}

func (c *Cmd) CombinedOutput() ([]byte, error) {
	c.prepare()
	bytes, err := c.Cmd.CombinedOutput()
	c.report()
	return bytes, err
}

func (c *Cmd) Wait() error {
	err := c.Cmd.Wait()
	c.report()
	return err
}

// StartOrFatal is equivalent to Start, but handles the error with a call to ctx.Fatal
func (c *Cmd) StartOrFatal() {
	if err := c.Start(); err != nil {
		c.ctx.Fatalf("Failed to run %s: %v", c.name, err)
	}
}

func (c *Cmd) reportError(err error) {
	if err == nil {
		return
	}
	if e, ok := err.(*exec.ExitError); ok {
		c.ctx.Fatalf("%s failed with: %v", c.name, e.ProcessState.String())
	} else {
		c.ctx.Fatalf("Failed to run %s: %v", c.name, err)
	}
}

// RunOrFatal is equivalent to Run, but handles the error with a call to ctx.Fatal
func (c *Cmd) RunOrFatal() {
	c.reportError(c.Run())
}

// WaitOrFatal is equivalent to Wait, but handles the error with a call to ctx.Fatal
func (c *Cmd) WaitOrFatal() {
	c.reportError(c.Wait())
}

// OutputOrFatal is equivalent to Output, but handles the error with a call to ctx.Fatal
func (c *Cmd) OutputOrFatal() []byte {
	ret, err := c.Output()
	c.reportError(err)
	return ret
}

// CombinedOutputOrFatal is equivalent to CombinedOutput, but handles the error with
// a call to ctx.Fatal
func (c *Cmd) CombinedOutputOrFatal() []byte {
	ret, err := c.CombinedOutput()
	c.reportError(err)
	return ret
}

// RunAndPrintOrFatal will run the command, then after finishing
// print any output, then handling any errors with a call to
// ctx.Fatal
func (c *Cmd) RunAndPrintOrFatal() {
	ret, err := c.CombinedOutput()
	st := c.ctx.Status.StartTool()
	if len(ret) > 0 {
		if err != nil {
			st.Error(string(ret))
		} else {
			st.Print(string(ret))
		}
	}
	st.Finish()
	c.reportError(err)
}

// RunAndStreamOrFatal will run the command, while running print
// any output, then handle any errors with a call to ctx.Fatal
func (c *Cmd) RunAndStreamOrFatal() {
	out, err := c.StdoutPipe()
	if err != nil {
		c.ctx.Fatal(err)
	}
	c.Stderr = c.Stdout

	st := c.ctx.Status.StartTool()

	c.StartOrFatal()

	buf := bufio.NewReaderSize(out, 2*1024*1024)
	for {
		// Attempt to read whole lines, but write partial lines that are too long to fit in the buffer or hit EOF
		line, err := buf.ReadString('\n')
		if line != "" {
			st.Print(strings.TrimSuffix(line, "\n"))
		} else if err == io.EOF {
			break
		} else if err != nil {
			c.ctx.Fatal(err)
		}
	}

	err = c.Wait()
	st.Finish()
	c.reportError(err)
}
