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

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"

	"android/soong/art"
	"android/soong/cc"
	"android/soong/common"
	"android/soong/genrule"
	"android/soong/java"
)

func main() {
	flag.Parse()

	// The top-level Blueprints file is passed as the first argument.
	srcDir := filepath.Dir(flag.Arg(0))

	ctx := blueprint.NewContext()

	// Module types
	ctx.RegisterModuleType("cc_library_static", cc.CCLibraryStaticFactory)
	ctx.RegisterModuleType("cc_library_shared", cc.CCLibrarySharedFactory)
	ctx.RegisterModuleType("cc_library", cc.CCLibraryFactory)
	ctx.RegisterModuleType("cc_object", cc.CCObjectFactory)
	ctx.RegisterModuleType("cc_binary", cc.CCBinaryFactory)
	ctx.RegisterModuleType("cc_test", cc.CCTestFactory)
	ctx.RegisterModuleType("cc_benchmark", cc.CCBenchmarkFactory)

	ctx.RegisterModuleType("toolchain_library", cc.ToolchainLibraryFactory)
	ctx.RegisterModuleType("ndk_prebuilt_library", cc.NdkPrebuiltLibraryFactory)
	ctx.RegisterModuleType("ndk_prebuilt_object", cc.NdkPrebuiltObjectFactory)
	ctx.RegisterModuleType("ndk_prebuilt_static_stl", cc.NdkPrebuiltStaticStlFactory)
	ctx.RegisterModuleType("ndk_prebuilt_shared_stl", cc.NdkPrebuiltSharedStlFactory)

	ctx.RegisterModuleType("cc_library_host_static", cc.CCLibraryHostStaticFactory)
	ctx.RegisterModuleType("cc_library_host_shared", cc.CCLibraryHostSharedFactory)
	ctx.RegisterModuleType("cc_binary_host", cc.CCBinaryHostFactory)
	ctx.RegisterModuleType("cc_test_host", cc.CCTestHostFactory)
	ctx.RegisterModuleType("cc_benchmark_host", cc.CCBenchmarkHostFactory)

	ctx.RegisterModuleType("gensrcs", genrule.GenSrcsFactory)
	ctx.RegisterModuleType("genrule", genrule.GenRuleFactory)

	ctx.RegisterModuleType("art_cc_library", art.ArtCCLibraryFactory)
	ctx.RegisterModuleType("art_cc_binary", art.ArtCCBinaryFactory)

	ctx.RegisterModuleType("java_library", java.JavaLibraryFactory)
	ctx.RegisterModuleType("java_library_static", java.JavaLibraryFactory)
	ctx.RegisterModuleType("java_library_host", java.JavaLibraryHostFactory)
	ctx.RegisterModuleType("java_binary", java.JavaBinaryFactory)
	ctx.RegisterModuleType("java_binary_host", java.JavaBinaryHostFactory)
	ctx.RegisterModuleType("prebuilt_java_library", java.JavaPrebuiltFactory)
	ctx.RegisterModuleType("prebuilt_sdk", java.SdkPrebuiltFactory)
	ctx.RegisterModuleType("android_app", java.AndroidAppFactory)

	// Mutators
	ctx.RegisterEarlyMutator("host_or_device", common.HostOrDeviceMutator)
	ctx.RegisterEarlyMutator("arch", common.ArchMutator)
	ctx.RegisterEarlyMutator("link", cc.LinkageMutator)
	ctx.RegisterEarlyMutator("test_per_src", cc.TestPerSrcMutator)

	// Singletons
	ctx.RegisterSingletonType("buildtarget", common.BuildTargetSingleton)
	ctx.RegisterSingletonType("env", common.EnvSingleton)
	ctx.RegisterSingletonType("logtags", java.LogtagsSingleton)

	configuration, err := common.NewConfig(srcDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}

	// Temporary hack
	//ctx.SetIgnoreUnknownModuleTypes(true)

	bootstrap.Main(ctx, configuration, common.ConfigFileName)
}
