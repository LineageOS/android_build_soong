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
	"os/exec"
)

type Sandbox string

const (
	noSandbox       = ""
	globalSandbox   = "build/soong/ui/build/sandbox/darwin/global.sb"
	dumpvarsSandbox = globalSandbox
	soongSandbox    = globalSandbox
	katiSandbox     = globalSandbox
	ninjaSandbox    = noSandbox
)

var sandboxExecPath string

func init() {
	if p, err := exec.LookPath("sandbox-exec"); err == nil {
		sandboxExecPath = p
	}
}

func (c *Cmd) sandboxSupported() bool {
	if c.Sandbox == "" {
		return false
	} else if sandboxExecPath == "" {
		c.ctx.Verboseln("sandbox-exec not found, disabling sandboxing")
		return false
	}
	return true
}

func (c *Cmd) wrapSandbox() {
	homeDir, _ := c.Environment.Get("HOME")
	outDir := absPath(c.ctx, c.config.OutDir())
	distDir := absPath(c.ctx, c.config.DistDir())

	c.Args[0] = c.Path
	c.Path = sandboxExecPath
	c.Args = append([]string{
		"sandbox-exec", "-f", string(c.Sandbox),
		"-D", "HOME=" + homeDir,
		"-D", "OUT_DIR=" + outDir,
		"-D", "DIST_DIR=" + distDir,
	}, c.Args...)
}
