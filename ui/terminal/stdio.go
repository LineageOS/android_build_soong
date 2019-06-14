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
	"io"
	"os"
)

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
