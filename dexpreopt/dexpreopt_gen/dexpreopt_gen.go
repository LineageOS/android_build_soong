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
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

	"github.com/google/blueprint/pathtools"
)

var (
	dexpreoptScriptPath = flag.String("dexpreopt_script", "", "path to output dexpreopt script")
	stripScriptPath     = flag.String("strip_script", "", "path to output strip script")
	globalConfigPath    = flag.String("global", "", "path to global configuration file")
	moduleConfigPath    = flag.String("module", "", "path to module configuration file")
	outDir              = flag.String("out_dir", "", "path to output directory")
)

type pathContext struct {
	config android.Config
}

func (x *pathContext) Fs() pathtools.FileSystem   { return pathtools.OsFs }
func (x *pathContext) Config() android.Config     { return x.config }
func (x *pathContext) AddNinjaFileDeps(...string) {}

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

	ctx := &pathContext{android.TestConfig(*outDir, nil)}

	globalConfig, err := dexpreopt.LoadGlobalConfig(ctx, *globalConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading global config %q: %s\n", *globalConfigPath, err)
		os.Exit(2)
	}

	moduleConfig, err := dexpreopt.LoadModuleConfig(ctx, *moduleConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading module config %q: %s\n", *moduleConfigPath, err)
		os.Exit(2)
	}

	// This shouldn't be using *PathForTesting, but it's outside of soong_build so its OK for now.
	moduleConfig.StripInputPath = android.PathForTesting("$1")
	moduleConfig.StripOutputPath = android.WritablePathForTesting("$2")

	moduleConfig.DexPath = android.PathForTesting("$1")

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

	writeScripts(ctx, globalConfig, moduleConfig, *dexpreoptScriptPath, *stripScriptPath)
}

func writeScripts(ctx android.PathContext, global dexpreopt.GlobalConfig, module dexpreopt.ModuleConfig,
	dexpreoptScriptPath, stripScriptPath string) {
	dexpreoptRule, err := dexpreopt.GenerateDexpreoptRule(ctx, global, module)
	if err != nil {
		panic(err)
	}

	installDir := module.BuildPath.InSameDir(ctx, "dexpreopt_install")

	dexpreoptRule.Command().FlagWithArg("rm -rf ", installDir.String())
	dexpreoptRule.Command().FlagWithArg("mkdir -p ", installDir.String())

	for _, install := range dexpreoptRule.Installs() {
		installPath := installDir.Join(ctx, strings.TrimPrefix(install.To, "/"))
		dexpreoptRule.Command().Text("mkdir -p").Flag(filepath.Dir(installPath.String()))
		dexpreoptRule.Command().Text("cp -f").Input(install.From).Output(installPath)
	}
	dexpreoptRule.Command().Tool(global.Tools.SoongZip).
		FlagWithArg("-o ", "$2").
		FlagWithArg("-C ", installDir.String()).
		FlagWithArg("-D ", installDir.String())

	stripRule, err := dexpreopt.GenerateStripRule(global, module)
	if err != nil {
		panic(err)
	}

	write := func(rule *android.RuleBuilder, file string) {
		script := &bytes.Buffer{}
		script.WriteString(scriptHeader)
		for _, c := range rule.Commands() {
			script.WriteString(c)
			script.WriteString("\n\n")
		}

		depFile := &bytes.Buffer{}

		fmt.Fprint(depFile, `: \`+"\n")
		for _, tool := range rule.Tools() {
			fmt.Fprintf(depFile, `    %s \`+"\n", tool)
		}
		for _, input := range rule.Inputs() {
			// Assume the rule that ran the script already has a dependency on the input file passed on the
			// command line.
			if input.String() != "$1" {
				fmt.Fprintf(depFile, `    %s \`+"\n", input)
			}
		}
		depFile.WriteString("\n")

		fmt.Fprintln(script, "rm -f $2.d")
		// Write the output path unescaped so the $2 gets expanded
		fmt.Fprintln(script, `echo -n $2 > $2.d`)
		// Write the rest of the depsfile using cat <<'EOF', which will not do any shell expansion on
		// the contents to preserve backslashes and special characters in filenames.
		fmt.Fprintf(script, "cat >> $2.d <<'EOF'\n%sEOF\n", depFile.String())

		err := pathtools.WriteFileIfChanged(file, script.Bytes(), 0755)
		if err != nil {
			panic(err)
		}
	}

	// The written scripts will assume the input is $1 and the output is $2
	if module.DexPath.String() != "$1" {
		panic(fmt.Errorf("module.DexPath must be '$1', was %q", module.DexPath))
	}
	if module.StripInputPath.String() != "$1" {
		panic(fmt.Errorf("module.StripInputPath must be '$1', was %q", module.StripInputPath))
	}
	if module.StripOutputPath.String() != "$2" {
		panic(fmt.Errorf("module.StripOutputPath must be '$2', was %q", module.StripOutputPath))
	}

	write(dexpreoptRule, dexpreoptScriptPath)
	write(stripRule, stripScriptPath)
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
