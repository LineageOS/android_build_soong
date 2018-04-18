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
	"path/filepath"

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

	bootstrap.Main(ctx.Context, configuration, configuration.ConfigFileName, configuration.ProductVariablesFileName)

	if docFile != "" {
		writeDocs(ctx, docFile)
	}
}
