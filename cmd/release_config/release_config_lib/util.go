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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

var (
	disableWarnings    bool
	containerRegexp, _ = regexp.Compile("^[a-z][a-z0-9]*([._][a-z][a-z0-9]*)*$")
)

type StringList []string

func (l *StringList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func (l *StringList) String() string {
	return fmt.Sprintf("%v", *l)
}

// Write a marshalled message to a file.
//
// Marshal the message based on the extension of the path we are writing it to.
//
// Args:
//
//	path string: the path of the file to write to.  Directories are not created.
//	  Supported extensions are: ".json", ".pb", and ".textproto".
//	message proto.Message: the message to write.
//
// Returns:
//
//	error: any error encountered.
func WriteMessage(path string, message proto.Message) (err error) {
	format := filepath.Ext(path)
	if len(format) > 1 {
		// Strip any leading dot.
		format = format[1:]
	}
	return WriteFormattedMessage(path, format, message)
}

// Write a marshalled message to a file.
//
// Marshal the message using the given format.
//
// Args:
//
//	path string: the path of the file to write to.  Directories are not created.
//	  Supported extensions are: ".json", ".pb", and ".textproto".
//	format string: one of "json", "pb", or "textproto".
//	message proto.Message: the message to write.
//
// Returns:
//
//	error: any error encountered.
func WriteFormattedMessage(path, format string, message proto.Message) (err error) {
	var data []byte
	switch format {
	case "json":
		data, err = json.MarshalIndent(message, "", "  ")
	case "pb", "binaryproto", "protobuf":
		data, err = proto.Marshal(message)
	case "textproto":
		data, err = prototext.MarshalOptions{Multiline: true}.Marshal(message)
	default:
		return fmt.Errorf("Unknown message format for %s", path)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Read a message from a file.
//
// The message is unmarshalled based on the extension of the file read.
//
// Args:
//
//	path string: the path of the file to read.
//	message proto.Message: the message to unmarshal the message into.
//
// Returns:
//
//	error: any error encountered.
func LoadMessage(path string, message proto.Message) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	switch filepath.Ext(path) {
	case ".json":
		return json.Unmarshal(data, message)
	case ".pb", ".protobuf", ".binaryproto":
		return proto.Unmarshal(data, message)
	case ".textproto":
		return prototext.Unmarshal(data, message)
	}
	return fmt.Errorf("Unknown message format for %s", path)
}

// Call Func for any textproto files found in {root}/{subdir}.
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

// Turn off all warning output
func DisableWarnings() {
	disableWarnings = true
}

func warnf(format string, args ...any) (n int, err error) {
	if !disableWarnings {
		return fmt.Fprintf(os.Stderr, format, args...)
	}
	return 0, nil
}

func validContainer(container string) bool {
	return containerRegexp.MatchString(container)
}

// Returns the default value for release config artifacts.
func GetDefaultOutDir() string {
	outEnv := os.Getenv("OUT_DIR")
	if outEnv == "" {
		outEnv = "out"
	}
	return filepath.Join(outEnv, "soong", "release-config")
}

// Find the top of the workspace.
//
// This mirrors the logic in build/envsetup.sh's gettop().
func GetTopDir() (topDir string, err error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return
	}
	topFile := "build/make/core/envsetup.mk"
	for topDir = workingDir; topDir != "/"; topDir = filepath.Dir(topDir) {
		if _, err = os.Stat(filepath.Join(topDir, topFile)); err == nil {
			return filepath.Rel(workingDir, topDir)
		}
	}
	return "", fmt.Errorf("Unable to locate top of workspace")
}

// Return the default list of map files to use.
func GetDefaultMapPaths(queryMaps bool) (defaultMapPaths StringList, err error) {
	var defaultLocations StringList
	workingDir, err := os.Getwd()
	if err != nil {
		return
	}
	defer func() {
		os.Chdir(workingDir)
	}()
	topDir, err := GetTopDir()
	os.Chdir(topDir)

	defaultLocations = StringList{
		"build/release/release_config_map.textproto",
		"vendor/google_shared/build/release/release_config_map.textproto",
		"vendor/google/release/release_config_map.textproto",
	}
	for _, path := range defaultLocations {
		if _, err = os.Stat(path); err == nil {
			defaultMapPaths = append(defaultMapPaths, path)
		}
	}

	var prodMaps string
	if queryMaps {
		getBuildVar := exec.Command("build/soong/soong_ui.bash", "--dumpvar-mode", "PRODUCT_RELEASE_CONFIG_MAPS")
		var stdout strings.Builder
		getBuildVar.Stdin = strings.NewReader("")
		getBuildVar.Stdout = &stdout
		getBuildVar.Stderr = os.Stderr
		err = getBuildVar.Run()
		if err != nil {
			return
		}
		prodMaps = stdout.String()
	} else {
		prodMaps = os.Getenv("PRODUCT_RELEASE_CONFIG_MAPS")
	}
	prodMaps = strings.TrimSpace(prodMaps)
	if len(prodMaps) > 0 {
		defaultMapPaths = append(defaultMapPaths, strings.Split(prodMaps, " ")...)
	}
	return
}
