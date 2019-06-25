// Copyright 2019 Google Inc. All rights reserved.
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
	"fmt"
	"strings"
)

const targetFilesPattern = "*-target_files-*.zip"

var targetZipPartitions = []string{
	"BOOT/RAMDISK/",
	"BOOT/",
	"DATA/",
	"ODM/",
	"OEM/",
	"PRODUCT/",
	"SYSTEM_EXT/",
	"ROOT/",
	"SYSTEM/",
	"SYSTEM_OTHER/",
	"VENDOR/",
}

var targetZipFilter = []string{
	"IMAGES/",
	"OTA/",
	"META/",
	"PREBUILT_IMAGES/",
	"RADIO/",
}

func filterTargetZipFiles(files []*ZipArtifactFile, artifact string, patterns []string) ([]*ZipArtifactFile, error) {
	var ret []*ZipArtifactFile
outer:
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}

		if artifact == targetFilesPattern {
			found := false
			for _, p := range targetZipPartitions {
				if strings.HasPrefix(f.Name, p) {
					f.Name = strings.ToLower(p) + strings.TrimPrefix(f.Name, p)
					found = true
				}
			}
			for _, filter := range targetZipFilter {
				if strings.HasPrefix(f.Name, filter) {
					continue outer
				}
			}

			if !found {
				return nil, fmt.Errorf("unmatched prefix for %s", f.Name)
			}
		}

		if patterns != nil {
			for _, pattern := range patterns {
				match, _ := Match(pattern, f.Name)
				if match {
					ret = append(ret, f)
				}
			}
		} else {
			ret = append(ret, f)
		}
	}

	return ret, nil
}
