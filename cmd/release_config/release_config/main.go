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
	"os"

	rc_lib "android/soong/cmd/release_config/release_config_lib"
)

func main() {
	var top string
	var releaseConfigMapPaths rc_lib.StringList
	var targetRelease string
	var outputDir string
	var err error
	var configs *rc_lib.ReleaseConfigs

	flag.StringVar(&top, "top", ".", "path to top of workspace")
	flag.Var(&releaseConfigMapPaths, "map", "path to a release_config_map.textproto. may be repeated")
	flag.StringVar(&targetRelease, "release", "trunk_staging", "TARGET_RELEASE for this build")
	flag.StringVar(&outputDir, "out_dir", rc_lib.GetDefaultOutDir(), "basepath for the output. Multiple formats are created")
	flag.Parse()

	if err = os.Chdir(top); err != nil {
		panic(err)
	}
	configs, err = rc_lib.ReadReleaseConfigMaps(releaseConfigMapPaths, targetRelease)
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll(outputDir, 0775)
	if err != nil {
		panic(err)
	}
	err = configs.DumpMakefile(outputDir, targetRelease)
	if err != nil {
		panic(err)
	}
	err = configs.DumpArtifact(outputDir)
	if err != nil {
		panic(err)
	}
}
