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

package release_config_lib

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

type StringList []string

func (l *StringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func (l *StringList) String() string {
	return fmt.Sprintf("%v", *l)
}

func LoadTextproto(path string, message proto.Message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ret := prototext.Unmarshal(data, message)
	return ret
}

func WalkTextprotoFiles(root string, subdir string, Func fs.WalkDirFunc) error {
	path := filepath.Join(root, subdir)
	if _, err := os.Stat(path); err != nil {
		// Missing subdirs are not an error.
		return nil
	}
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(d.Name(), ".textproto") && d.Type().IsRegular() {
			return Func(path, d, err)
		}
		return nil
	})
}

func GetDefaultOutDir() string {
	outEnv := os.Getenv("OUT_DIR")
	if outEnv == "" {
		outEnv = "out"
	}
	return filepath.Join(outEnv, "soong", "release-config")
}

func GetDefaultMapPaths() StringList {
	var defaultMapPaths StringList
	defaultLocations := StringList{
		"build/release/release_config_map.textproto",
		"vendor/google_shared/build/release/release_config_map.textproto",
		"vendor/google/release/release_config_map.textproto",
	}
	for _, path := range defaultLocations {
		if _, err := os.Stat(path); err == nil {
			defaultMapPaths = append(defaultMapPaths, path)
		}
	}
	prodMaps := os.Getenv("PRODUCT_RELEASE_CONFIG_MAPS")
	if prodMaps != "" {
		defaultMapPaths = append(defaultMapPaths, strings.Split(prodMaps, " ")...)
	}
	return defaultMapPaths
}
