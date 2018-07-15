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
	"strings"
	"sync"
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

	// Status prints the first line of the string to the terminal,
	// overwriting any previous status line. Strings longer than the width
	// of the terminal will be cut off.
	//
	// On a dumb terminal, previous status messages will remain, and the
	// entire first line of the string will be printed.
	StatusLine(str string)

	// StatusAndMessage prints the first line of status to the terminal,
	// similarly to StatusLine(), then prints the full msg below that. The
	// status line is retained.
	//
	// There is guaranteed to be no other output in between the status and
	// message.
	StatusAndMessage(status, msg string)

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
}

// NewWriter creates a new Writer based on the stdio and the TERM
// environment variable.
func NewWriter(stdio StdioInterface) Writer {
	w := &writerImpl{
		stdio: stdio,

		haveBlankLine: true,
	}

	if term, ok := os.LookupEnv("TERM"); ok && term != "dumb" {
		w.smartTerminal = isTerminal(stdio.Stdout())
		w.stripEscapes = !w.smartTerminal
	}

	return w
}

type writerImpl struct {
	stdio StdioInterface

	haveBlankLine bool

	// Protecting the above, we assume that smartTerminal and stripEscapes
	// does not change after initial setup.
	lock sync.Mutex

	smartTerminal bool
	stripEscapes  bool
}

func (w *writerImpl) isSmartTerminal() bool {
	return w.smartTerminal
}

func (w *writerImpl) requestLine() {
	if !w.haveBlankLine {
		fmt.Fprintln(w.stdio.Stdout())
		w.haveBlankLine = true
	}
}

func (w *writerImpl) Print(str string) {
	if w.stripEscapes {
		str = string(stripAnsiEscapes([]byte(str)))
	}

	w.lock.Lock()
	defer w.lock.Unlock()
	w.print(str)
}

func (w *writerImpl) print(str string) {
	if !w.haveBlankLine {
		fmt.Fprint(w.stdio.Stdout(), "\r", "\x1b[K")
		w.haveBlankLine = true
	}
	fmt.Fprint(w.stdio.Stdout(), str)
	if len(str) == 0 || str[len(str)-1] != '\n' {
		fmt.Fprint(w.stdio.Stdout(), "\n")
	}
}

func (w *writerImpl) StatusLine(str string) {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.statusLine(str)
}

func (w *writerImpl) statusLine(str string) {
	if !w.smartTerminal {
		fmt.Fprintln(w.stdio.Stdout(), str)
		return
	}

	idx := strings.IndexRune(str, '\n')
	if idx != -1 {
		str = str[0:idx]
	}

	// Limit line width to the terminal width, otherwise we'll wrap onto
	// another line and we won't delete the previous line.
	//
	// Run this on every line in case the window has been resized while
	// we're printing. This could be optimized to only re-run when we get
	// SIGWINCH if it ever becomes too time consuming.
	if max, ok := termWidth(w.stdio.Stdout()); ok {
		if len(str) > max {
			// TODO: Just do a max. Ninja elides the middle, but that's
			// more complicated and these lines aren't that important.
			str = str[:max]
		}
	}

	// Move to the beginning on the line, print the output, then clear
	// the rest of the line.
	fmt.Fprint(w.stdio.Stdout(), "\r", str, "\x1b[K")
	w.haveBlankLine = false
}

func (w *writerImpl) StatusAndMessage(status, msg string) {
	if w.stripEscapes {
		msg = string(stripAnsiEscapes([]byte(msg)))
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	w.statusLine(status)
	w.requestLine()
	w.print(msg)
}

func (w *writerImpl) Finish() {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.requestLine()
}

func (w *writerImpl) Write(p []byte) (n int, err error) {
	w.Print(string(p))
	return len(p), nil
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
