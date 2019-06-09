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

// Package terminal provides a set of interfaces that can be used to interact
// with the terminal (including falling back when the terminal is detected to
// be a redirect or other dumb terminal)
package terminal

import (
	"fmt"
	"io"
	"os"
)

// Writer provides an interface to write temporary and permanent messages to
// the terminal.
//
// The terminal is considered to be a dumb terminal if TERM==dumb, or if a
// terminal isn't detected on stdout/stderr (generally because it's a pipe or
// file). Dumb terminals will strip out all ANSI escape sequences, including
// colors.
type Writer interface {
	// Print prints the string to the terminal, overwriting any current
	// status being displayed.
	//
	// On a dumb terminal, the status messages will be kept.
	Print(str string)

	// Finish ensures that the output ends with a newline (preserving any
	// current status line that is current displayed).
	//
	// This does nothing on dumb terminals.
	Finish()

	// Write implements the io.Writer interface. This is primarily so that
	// the logger can use this interface to print to stderr without
	// breaking the other semantics of this interface.
	//
	// Try to use any of the other functions if possible.
	Write(p []byte) (n int, err error)

	isSmartTerminal() bool
	termWidth() (int, bool)
}

// NewWriter creates a new Writer based on the stdio and the TERM
// environment variable.
func NewWriter(stdio StdioInterface) Writer {
	w := &writerImpl{
		stdio: stdio,
	}

	return w
}

type writerImpl struct {
	stdio StdioInterface
}

func (w *writerImpl) Print(str string) {
	fmt.Fprint(w.stdio.Stdout(), str)
	if len(str) == 0 || str[len(str)-1] != '\n' {
		fmt.Fprint(w.stdio.Stdout(), "\n")
	}
}

func (w *writerImpl) Finish() {}

func (w *writerImpl) Write(p []byte) (n int, err error) {
	return w.stdio.Stdout().Write(p)
}

func (w *writerImpl) isSmartTerminal() bool {
	return isSmartTerminal(w.stdio.Stdout())
}

func (w *writerImpl) termWidth() (int, bool) {
	return termWidth(w.stdio.Stdout())
}

// StdioInterface represents a set of stdin/stdout/stderr Reader/Writers
type StdioInterface interface {
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
}

// StdioImpl uses the OS stdin/stdout/stderr to implement StdioInterface
type StdioImpl struct{}

func (StdioImpl) Stdin() io.Reader  { return os.Stdin }
func (StdioImpl) Stdout() io.Writer { return os.Stdout }
func (StdioImpl) Stderr() io.Writer { return os.Stderr }

var _ StdioInterface = StdioImpl{}

type customStdio struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func NewCustomStdio(stdin io.Reader, stdout, stderr io.Writer) StdioInterface {
	return customStdio{stdin, stdout, stderr}
}

func (c customStdio) Stdin() io.Reader  { return c.stdin }
func (c customStdio) Stdout() io.Writer { return c.stdout }
func (c customStdio) Stderr() io.Writer { return c.stderr }

var _ StdioInterface = customStdio{}
