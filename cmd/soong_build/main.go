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

	"blueprint"
	"blueprint/bootstrap"

	"android/soong/cc"
	"android/soong/common"
	"android/soong/config"
	"android/soong/genrule"
)

func main() {
	flag.Parse()

	// The top-level Blueprints file is passed as the first argument.
	srcDir := filepath.Dir(flag.Arg(0))

	ctx := blueprint.NewContext()

	// Module types
	ctx.RegisterModuleType("cc_library_static", cc.NewCCLibraryStatic)
	ctx.RegisterModuleType("cc_library_shared", cc.NewCCLibraryShared)
	ctx.RegisterModuleType("cc_library", cc.NewCCLibrary)
	ctx.RegisterModuleType("cc_object", cc.NewCCObject)
	ctx.RegisterModuleType("cc_binary", cc.NewCCBinary)
	ctx.RegisterModuleType("cc_test", cc.NewCCTest)

	ctx.RegisterModuleType("toolchain_library", cc.NewToolchainLibrary)

	ctx.RegisterModuleType("cc_library_host_static", cc.NewCCLibraryHostStatic)
	ctx.RegisterModuleType("cc_library_host_shared", cc.NewCCLibraryHostShared)
	ctx.RegisterModuleType("cc_binary_host", cc.NewCCBinaryHost)

	ctx.RegisterModuleType("gensrcs", genrule.NewGenSrcs)

	// Mutators
	ctx.RegisterEarlyMutator("arch", common.ArchMutator)
	ctx.RegisterEarlyMutator("link", cc.LinkageMutator)

	// Singletons
	ctx.RegisterSingletonType("checkbuild", common.CheckbuildSingleton)

	configuration, err := config.New(srcDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}

	// Temporary hack
	//ctx.SetIgnoreUnknownModuleTypes(true)

	bootstrap.Main(ctx, configuration, config.ConfigFileName)
}
