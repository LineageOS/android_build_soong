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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

var (
	dexpreoptScriptPath   = flag.String("dexpreopt_script", "", "path to output dexpreopt script")
	globalSoongConfigPath = flag.String("global_soong", "", "path to global configuration file for settings originating from Soong")
	globalConfigPath      = flag.String("global", "", "path to global configuration file")
	moduleConfigPath      = flag.String("module", "", "path to module configuration file")
	outDir                = flag.String("out_dir", "", "path to output directory")
	// If uses_target_files is true, dexpreopt_gen will be running on extracted target_files.zip files.
	// In this case, the tool replace output file path with $(basePath)/$(on-device file path).
	// The flag is useful when running dex2oat on system image and vendor image which are built separately.
	usesTargetFiles = flag.Bool("uses_target_files", false, "whether or not dexpreopt is running on target_files")
	// basePath indicates the path where target_files.zip is extracted.
	basePath            = flag.String("base_path", ".", "base path where images and tools are extracted")
	productPackagesPath = flag.String("product_packages", "", "path to product_packages.txt")
)

type builderContext struct {
	config android.Config
}

func (x *builderContext) Config() android.Config                            { return x.config }
func (x *builderContext) AddNinjaFileDeps(...string)                        {}
func (x *builderContext) Build(android.PackageContext, android.BuildParams) {}
func (x *builderContext) Rule(android.PackageContext, string, blueprint.RuleParams, ...string) blueprint.Rule {
	return nil
}

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

	if *globalSoongConfigPath == "" {
		usage("--global_soong configuration file is required")
	}

	if *globalConfigPath == "" {
		usage("--global configuration file is required")
	}

	if *moduleConfigPath == "" {
		usage("--module configuration file is required")
	}

	if *productPackagesPath == "" {
		usage("--product_packages configuration file is required")
	}

	// NOTE: duplicating --out_dir here is incorrect (one should be the another
	// plus "/soong" but doing so apparently breaks dexpreopt
	ctx := &builderContext{android.NullConfig(*outDir, *outDir)}

	globalSoongConfigData, err := ioutil.ReadFile(*globalSoongConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading global Soong config %q: %s\n", *globalSoongConfigPath, err)
		os.Exit(2)
	}

	globalSoongConfig, err := dexpreopt.ParseGlobalSoongConfig(ctx, globalSoongConfigData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing global Soong config %q: %s\n", *globalSoongConfigPath, err)
		os.Exit(2)
	}

	globalConfigData, err := ioutil.ReadFile(*globalConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading global config %q: %s\n", *globalConfigPath, err)
		os.Exit(2)
	}

	globalConfig, err := dexpreopt.ParseGlobalConfig(ctx, globalConfigData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing global config %q: %s\n", *globalConfigPath, err)
		os.Exit(2)
	}

	moduleConfigData, err := ioutil.ReadFile(*moduleConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading module config %q: %s\n", *moduleConfigPath, err)
		os.Exit(2)
	}

	moduleConfig, err := dexpreopt.ParseModuleConfig(ctx, moduleConfigData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing module config %q: %s\n", *moduleConfigPath, err)
		os.Exit(2)
	}

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
	if *usesTargetFiles {
		moduleConfig.ManifestPath = android.OptionalPath{}
		prefix := "dex2oat_result"
		moduleConfig.BuildPath = android.PathForOutput(ctx, filepath.Join(prefix, moduleConfig.DexLocation))
		for i, location := range moduleConfig.PreoptBootClassPathDexLocations {
			moduleConfig.PreoptBootClassPathDexFiles[i] = android.PathForSource(ctx, *basePath+location)
		}
		for i := range moduleConfig.ClassLoaderContexts {
			for _, v := range moduleConfig.ClassLoaderContexts[i] {
				v.Host = android.PathForSource(ctx, *basePath+v.Device)
			}
		}
		moduleConfig.EnforceUsesLibraries = false
		for i, location := range moduleConfig.DexPreoptImageLocationsOnDevice {
			moduleConfig.DexPreoptImageLocationsOnHost[i] = *basePath + location
		}
	}
	writeScripts(ctx, globalSoongConfig, globalConfig, moduleConfig, *dexpreoptScriptPath, *productPackagesPath)
}

func writeScripts(ctx android.BuilderContext, globalSoong *dexpreopt.GlobalSoongConfig,
	global *dexpreopt.GlobalConfig, module *dexpreopt.ModuleConfig, dexpreoptScriptPath string,
	productPackagesPath string) {
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
	dexpreoptRule, err := dexpreopt.GenerateDexpreoptRule(
		ctx, globalSoong, global, module, android.PathForTesting(productPackagesPath))
	if err != nil {
		panic(err)
	}
	// When usesTargetFiles is true, only odex/vdex files are necessary.
	// So skip redunant processes(such as copying the result to the artifact path, and zipping, and so on.)
	if *usesTargetFiles {
		write(dexpreoptRule, dexpreoptScriptPath)
		return
	}
	installDir := module.BuildPath.InSameDir(ctx, "dexpreopt_install")

	dexpreoptRule.Command().FlagWithArg("rm -rf ", installDir.String())
	dexpreoptRule.Command().FlagWithArg("mkdir -p ", installDir.String())

	for _, install := range dexpreoptRule.Installs() {
		installPath := installDir.Join(ctx, strings.TrimPrefix(install.To, "/"))
		dexpreoptRule.Command().Text("mkdir -p").Flag(filepath.Dir(installPath.String()))
		dexpreoptRule.Command().Text("cp -f").Input(install.From).Output(installPath)
	}
	dexpreoptRule.Command().Tool(globalSoong.SoongZip).
		FlagWithArg("-o ", "$2").
		FlagWithArg("-C ", installDir.String()).
		FlagWithArg("-D ", installDir.String())

	// The written scripts will assume the input is $1 and the output is $2
	if module.DexPath.String() != "$1" {
		panic(fmt.Errorf("module.DexPath must be '$1', was %q", module.DexPath))
	}

	write(dexpreoptRule, dexpreoptScriptPath)
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
