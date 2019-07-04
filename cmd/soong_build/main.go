// Copyright 2015 Google Inc. All rights reserved.
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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/blueprint/bootstrap"

	"android/soong/android"
)

var (
	docFile string
)

func init() {
	flag.StringVar(&docFile, "soong_docs", "", "build documentation file to output")
}

func newNameResolver(config android.Config) *android.NameResolver {
	namespacePathsToExport := make(map[string]bool)

	for _, namespaceName := range config.ExportedNamespaces() {
		namespacePathsToExport[namespaceName] = true
	}

	namespacePathsToExport["."] = true // always export the root namespace

	exportFilter := func(namespace *android.Namespace) bool {
		return namespacePathsToExport[namespace.Path]
	}

	return android.NewNameResolver(exportFilter)
}

func main() {
	if android.SoongDelveListen != "" {
		if android.SoongDelvePath == "" {
			fmt.Fprintln(os.Stderr, "SOONG_DELVE is set but failed to find dlv")
			os.Exit(1)
		}
		pid := strconv.Itoa(os.Getpid())
		cmd := []string{android.SoongDelvePath,
			"attach", pid,
			"--headless",
			"-l", android.SoongDelveListen,
			"--api-version=2",
			"--accept-multiclient",
			"--log",
		}

		fmt.Println("Starting", strings.Join(cmd, " "))
		dlv := exec.Command(cmd[0], cmd[1:]...)
		dlv.Stdout = os.Stdout
		dlv.Stderr = os.Stderr
		dlv.Stdin = nil

		// Put dlv into its own process group so we can kill it and the child process it starts.
		dlv.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		err := dlv.Start()
		if err != nil {
			// Print the error starting dlv and continue.
			fmt.Println(err)
		} else {
			// Kill the process group for dlv when soong_build exits.
			defer syscall.Kill(-dlv.Process.Pid, syscall.SIGKILL)
			// Wait to give dlv a chance to connect and pause the process.
			time.Sleep(time.Second)
		}
	}

	flag.Parse()

	// The top-level Blueprints file is passed as the first argument.
	srcDir := filepath.Dir(flag.Arg(0))

	ctx := android.NewContext()
	ctx.Register()

	configuration, err := android.NewConfig(srcDir, bootstrap.BuildDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}

	if docFile != "" {
		configuration.SetStopBefore(bootstrap.StopBeforePrepareBuildActions)
	}

	ctx.SetNameInterface(newNameResolver(configuration))

	ctx.SetAllowMissingDependencies(configuration.AllowMissingDependencies())

	extraNinjaDeps := []string{configuration.ConfigFileName, configuration.ProductVariablesFileName}

	// Read the SOONG_DELVE again through configuration so that there is a dependency on the environment variable
	// and soong_build will rerun when it is set for the first time.
	if listen := configuration.Getenv("SOONG_DELVE"); listen != "" {
		// Add a non-existent file to the dependencies so that soong_build will rerun when the debugger is
		// enabled even if it completed successfully.
		extraNinjaDeps = append(extraNinjaDeps, filepath.Join(configuration.BuildDir(), "always_rerun_for_delve"))
	}

	bootstrap.Main(ctx.Context, configuration, extraNinjaDeps...)

	if docFile != "" {
		if err := writeDocs(ctx, docFile); err != nil {
			fmt.Fprintf(os.Stderr, "%s", err)
			os.Exit(1)
		}
	}
}
