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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"android/soong/dexpreopt"

	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
)

var (
	extrasOutputPath    = flag.String("extras_zip", "", "path to output zip file of extra files")
	dexpreoptScriptPath = flag.String("dexpreopt_script", "", "path to output dexpreopt script")
	stripScriptPath     = flag.String("strip_script", "", "path to output strip script")
	globalConfigPath    = flag.String("global", "", "path to global configuration file")
	moduleConfigPath    = flag.String("module", "", "path to module configuration file")
)

func main() {
	flag.Parse()

	usage := func(err string) {
		if err != "" {
			fmt.Println(err)
			flag.Usage()
			os.Exit(1)
		}
	}

	if flag.NArg() > 0 {
		usage("unrecognized argument " + flag.Arg(0))
	}

	if *dexpreoptScriptPath == "" {
		usage("path to output dexpreopt script is required")
	}

	if *stripScriptPath == "" {
		usage("path to output strip script is required")
	}

	if *globalConfigPath == "" {
		usage("path to global configuration file is required")
	}

	if *moduleConfigPath == "" {
		usage("path to module configuration file is required")
	}

	globalConfig, err := dexpreopt.LoadGlobalConfig(*globalConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading global config %q: %s\n", *globalConfigPath, err)
		os.Exit(2)
	}

	moduleConfig, err := dexpreopt.LoadModuleConfig(*moduleConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading module config %q: %s\n", *moduleConfigPath, err)
		os.Exit(2)
	}

	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case runtime.Error:
				panic(x)
			case error:
				fmt.Fprintln(os.Stderr, "error:", r)
				os.Exit(3)
			default:
				panic(x)
			}
		}
	}()

	writeScripts(globalConfig, moduleConfig, *dexpreoptScriptPath, *stripScriptPath, *extrasOutputPath)
}

func writeScripts(global dexpreopt.GlobalConfig, module dexpreopt.ModuleConfig,
	dexpreoptScriptPath, stripScriptPath, extrasOutputPath string) {
	dexpreoptRule, err := dexpreopt.GenerateDexpreoptRule(global, module)
	if err != nil {
		panic(err)
	}

	installDir := filepath.Join(filepath.Dir(module.BuildPath), "dexpreopt_install")

	dexpreoptRule.Command().FlagWithArg("rm -rf ", installDir)

	for _, install := range dexpreoptRule.Installs() {
		installPath := filepath.Join(installDir, install.To)
		dexpreoptRule.Command().Text("mkdir -p").Flag(filepath.Dir(installPath))
		dexpreoptRule.Command().Text("cp -f").Input(install.From).Output(installPath)
	}
	dexpreoptRule.Command().Tool(global.Tools.SoongZip).
		FlagWithOutput("-o ", extrasOutputPath).
		FlagWithArg("-C ", installDir).
		FlagWithArg("-D ", installDir)

	stripRule, err := dexpreopt.GenerateStripRule(global, module)
	if err != nil {
		panic(err)
	}

	write := func(rule *dexpreopt.Rule, file, output string) {
		script := &bytes.Buffer{}
		script.WriteString(scriptHeader)
		for _, c := range rule.Commands() {
			script.WriteString(c)
			script.WriteString("\n\n")
		}

		depFile := &bytes.Buffer{}
		fmt.Fprintf(depFile, "%s: \\\n", output)
		for _, tool := range dexpreoptRule.Tools() {
			fmt.Fprintf(depFile, "    %s \\\n", tool)
		}
		for _, input := range dexpreoptRule.Inputs() {
			fmt.Fprintf(depFile, "    %s \\\n", input)
		}
		depFile.WriteString("\n")

		fmt.Fprintf(script, `/bin/bash -c 'echo -e $0 > %s' %s\n`,
			output+".d", proptools.ShellEscape([]string{depFile.String()})[0])

		err := pathtools.WriteFileIfChanged(file, script.Bytes(), 0755)
		if err != nil {
			panic(err)
		}
	}

	write(dexpreoptRule, dexpreoptScriptPath, extrasOutputPath)
	write(stripRule, stripScriptPath, module.StripOutputPath)
}

const scriptHeader = `#!/bin/bash

err() {
  errno=$?
  echo "error: $0:$1 exited with status $errno" >&2
  echo "error in command:" >&2
  sed -n -e "$1p" $0 >&2
  if [ "$errno" -ne 0 ]; then
    exit $errno
  else
    exit 1
  fi
}

trap 'err $LINENO' ERR

`
