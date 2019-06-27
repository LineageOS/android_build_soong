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

package terminal

import (
	"bytes"
	"io"
	"os"
	"syscall"
	"unsafe"
)

func isSmartTerminal(w io.Writer) bool {
	if term, ok := os.LookupEnv("TERM"); ok && term == "dumb" {
		return false
	}
	if f, ok := w.(*os.File); ok {
		var termios syscall.Termios
		_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
			ioctlGetTermios, uintptr(unsafe.Pointer(&termios)),
			0, 0, 0)
		return err == 0
	} else if _, ok := w.(*fakeSmartTerminal); ok {
		return true
	}
	return false
}

func termSize(w io.Writer) (width int, height int, ok bool) {
	if f, ok := w.(*os.File); ok {
		var winsize struct {
			ws_row, ws_column    uint16
			ws_xpixel, ws_ypixel uint16
		}
		_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
			syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&winsize)),
			0, 0, 0)
		return int(winsize.ws_column), int(winsize.ws_row), err == 0
	} else if f, ok := w.(*fakeSmartTerminal); ok {
		return f.termWidth, f.termHeight, true
	}
	return 0, 0, false
}

// stripAnsiEscapes strips ANSI control codes from a byte array in place.
func stripAnsiEscapes(input []byte) []byte {
	// read represents the remaining part of input that needs to be processed.
	read := input
	// write represents where we should be writing in input.
	// It will share the same backing store as input so that we make our modifications
	// in place.
	write := input

	// advance will copy count bytes from read to write and advance those slices
	advance := func(write, read []byte, count int) ([]byte, []byte) {
		copy(write, read[:count])
		return write[count:], read[count:]
	}

	for {
		// Find the next escape sequence
		i := bytes.IndexByte(read, 0x1b)
		// If it isn't found, or if there isn't room for <ESC>[, finish
		if i == -1 || i+1 >= len(read) {
			copy(write, read)
			break
		}

		// Not a CSI code, continue searching
		if read[i+1] != '[' {
			write, read = advance(write, read, i+1)
			continue
		}

		// Found a CSI code, advance up to the <ESC>
		write, read = advance(write, read, i)

		// Find the end of the CSI code
		i = bytes.IndexFunc(read, func(r rune) bool {
			return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		})
		if i == -1 {
			// We didn't find the end of the code, just remove the rest
			i = len(read) - 1
		}

		// Strip off the end marker too
		i = i + 1

		// Skip the reader forward and reduce final length by that amount
		read = read[i:]
		input = input[:len(input)-i]
	}

	return input
}

type fakeSmartTerminal struct {
	bytes.Buffer
	termWidth, termHeight int
}
