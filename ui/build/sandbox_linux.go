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
	"bytes"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"
)

type Sandbox struct {
	Enabled              bool
	DisableWhenUsingGoma bool

	AllowBuildBrokenUsesNetwork bool
}

var (
	noSandbox    = Sandbox{}
	basicSandbox = Sandbox{
		Enabled: true,
	}

	dumpvarsSandbox = basicSandbox
	katiSandbox     = basicSandbox
	soongSandbox    = basicSandbox
	ninjaSandbox    = Sandbox{
		Enabled:              true,
		DisableWhenUsingGoma: true,

		AllowBuildBrokenUsesNetwork: true,
	}
)

const nsjailPath = "prebuilts/build-tools/linux-x86/bin/nsjail"

var sandboxConfig struct {
	once sync.Once

	working bool
	group   string
}

func (c *Cmd) sandboxSupported() bool {
	if !c.Sandbox.Enabled {
		return false
	}

	// Goma is incompatible with PID namespaces and Mount namespaces. b/122767582
	if c.Sandbox.DisableWhenUsingGoma && c.config.UseGoma() {
		return false
	}

	sandboxConfig.once.Do(func() {
		sandboxConfig.group = "nogroup"
		if _, err := user.LookupGroup(sandboxConfig.group); err != nil {
			sandboxConfig.group = "nobody"
		}

		cmd := exec.CommandContext(c.ctx.Context, nsjailPath,
			"-H", "android-build",
			"-e",
			"-u", "nobody",
			"-g", sandboxConfig.group,
			"-B", "/",
			"--disable_clone_newcgroup",
			"--",
			"/bin/bash", "-c", `if [ $(hostname) == "android-build" ]; then echo "Android" "Success"; else echo Failure; fi`)
		cmd.Env = c.config.Environment().Environ()

		c.ctx.Verboseln(cmd.Args)
		data, err := cmd.CombinedOutput()
		if err == nil && bytes.Contains(data, []byte("Android Success")) {
			sandboxConfig.working = true
			return
		}

		c.ctx.Println("Build sandboxing disabled due to nsjail error.")

		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			c.ctx.Verboseln(line)
		}

		if err == nil {
			c.ctx.Verboseln("nsjail exited successfully, but without the correct output")
		} else if e, ok := err.(*exec.ExitError); ok {
			c.ctx.Verbosef("nsjail failed with %v", e.ProcessState.String())
		} else {
			c.ctx.Verbosef("nsjail failed with %v", err)
		}
	})

	return sandboxConfig.working
}

func (c *Cmd) wrapSandbox() {
	wd, _ := os.Getwd()

	sandboxArgs := []string{
		// The executable to run
		"-x", c.Path,

		// Set the hostname to something consistent
		"-H", "android-build",

		// Use the current working dir
		"--cwd", wd,

		// No time limit
		"-t", "0",

		// Keep all environment variables, we already filter them out
		// in soong_ui
		"-e",

		// Mount /proc read-write, necessary to run a nested nsjail or minijail0
		"--proc_rw",

		// Use a consistent user & group.
		// Note that these are mapped back to the real UID/GID when
		// doing filesystem operations, so they're rather arbitrary.
		"-u", "nobody",
		"-g", sandboxConfig.group,

		// Set high values, as nsjail uses low defaults.
		"--rlimit_as", "soft",
		"--rlimit_core", "soft",
		"--rlimit_cpu", "soft",
		"--rlimit_fsize", "soft",
		"--rlimit_nofile", "soft",

		// For now, just map everything. Eventually we should limit this, especially to make most things readonly.
		"-B", "/",

		// Disable newcgroup for now, since it may require newer kernels
		// TODO: try out cgroups
		"--disable_clone_newcgroup",

		// Only log important warnings / errors
		"-q",
	}

	if c.Sandbox.AllowBuildBrokenUsesNetwork && c.config.BuildBrokenUsesNetwork() {
		c.ctx.Printf("AllowBuildBrokenUsesNetwork: %v", c.Sandbox.AllowBuildBrokenUsesNetwork)
		c.ctx.Printf("BuildBrokenUsesNetwork: %v", c.config.BuildBrokenUsesNetwork())
		sandboxArgs = append(sandboxArgs, "-N")
	} else if dlv, _ := c.config.Environment().Get("SOONG_DELVE"); dlv != "" {
		// The debugger is enabled and soong_build will pause until a remote delve process connects, allow
		// network connections.
		sandboxArgs = append(sandboxArgs, "-N")
	}

	// Stop nsjail from parsing arguments
	sandboxArgs = append(sandboxArgs, "--")

	c.Args = append(sandboxArgs, c.Args[1:]...)
	c.Path = nsjailPath

	env := Environment(c.Env)
	if _, hasUser := env.Get("USER"); hasUser {
		env.Set("USER", "nobody")
	}
	c.Env = []string(env)
}
