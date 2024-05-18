// Copyright 2024 Google Inc. All rights reserved.
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

	rc_lib "android/soong/cmd/release_config/release_config_lib"
)

func main() {
	var top string
	var quiet bool
	var releaseConfigMapPaths rc_lib.StringList
	var targetRelease string
	var outputDir string
	var err error
	var configs *rc_lib.ReleaseConfigs
	var json, pb, textproto bool
	var product string
	var allMake bool
	var useBuildVar bool
	var guard bool

	defaultRelease := os.Getenv("TARGET_RELEASE")
	if defaultRelease == "" {
		defaultRelease = "trunk_staging"
	}

	flag.StringVar(&top, "top", ".", "path to top of workspace")
	flag.StringVar(&product, "product", os.Getenv("TARGET_PRODUCT"), "TARGET_PRODUCT for the build")
	flag.BoolVar(&quiet, "quiet", false, "disable warning messages")
	flag.Var(&releaseConfigMapPaths, "map", "path to a release_config_map.textproto. may be repeated")
	flag.StringVar(&targetRelease, "release", defaultRelease, "TARGET_RELEASE for this build")
	flag.StringVar(&outputDir, "out_dir", rc_lib.GetDefaultOutDir(), "basepath for the output. Multiple formats are created")
	flag.BoolVar(&textproto, "textproto", true, "write artifacts as text protobuf")
	flag.BoolVar(&json, "json", true, "write artifacts as json")
	flag.BoolVar(&pb, "pb", true, "write artifacts as binary protobuf")
	flag.BoolVar(&allMake, "all_make", false, "write makefiles for all release configs")
	flag.BoolVar(&useBuildVar, "use_get_build_var", false, "use get_build_var PRODUCT_RELEASE_CONFIG_MAPS")
	flag.BoolVar(&guard, "guard", true, "whether to guard with RELEASE_BUILD_FLAGS_IN_PROTOBUF")

	flag.Parse()

	if quiet {
		rc_lib.DisableWarnings()
	}

	if err = os.Chdir(top); err != nil {
		panic(err)
	}
	configs, err = rc_lib.ReadReleaseConfigMaps(releaseConfigMapPaths, targetRelease, useBuildVar)
	if err != nil {
		panic(err)
	}
	config, err := configs.GetReleaseConfig(targetRelease)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(outputDir, 0775)
	if err != nil {
		panic(err)
	}

	makefilePath := filepath.Join(outputDir, fmt.Sprintf("release_config-%s-%s.varmk", product, targetRelease))
	useProto, ok := config.FlagArtifacts["RELEASE_BUILD_FLAGS_IN_PROTOBUF"]
	if guard && (!ok || rc_lib.MarshalValue(useProto.Value) == "") {
		// We were told to guard operation and either we have no build flag, or it is False.
		// Write an empty file so that release_config.mk will use the old process.
		os.WriteFile(makefilePath, []byte{}, 0644)
		return
	}
	// Write the makefile where release_config.mk is going to look for it.
	err = configs.WriteMakefile(makefilePath, targetRelease)
	if err != nil {
		panic(err)
	}
	if allMake {
		// Write one makefile per release config, using the canonical release name.
		for _, c := range configs.GetSortedReleaseConfigs() {
			if c.Name != targetRelease {
				makefilePath = filepath.Join(outputDir, fmt.Sprintf("release_config-%s-%s.varmk", product, c.Name))
				err = configs.WriteMakefile(makefilePath, c.Name)
				if err != nil {
					panic(err)
				}
			}
		}
	}
	if json {
		err = configs.WriteArtifact(outputDir, product, "json")
		if err != nil {
			panic(err)
		}
	}
	if pb {
		err = configs.WriteArtifact(outputDir, product, "pb")
		if err != nil {
			panic(err)
		}
	}
	if textproto {
		err = configs.WriteArtifact(outputDir, product, "textproto")
		if err != nil {
			panic(err)
		}
	}
	if err = config.WritePartitionBuildFlags(outputDir, product, targetRelease); err != nil {
		panic(err)
	}

}
