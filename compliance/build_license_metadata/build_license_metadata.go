// Copyright 2021 Google Inc. All rights reserved.
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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"android/soong/compliance/license_metadata_proto"
	"android/soong/response"
)

func newMultiString(flags *flag.FlagSet, name, usage string) *multiString {
	var f multiString
	flags.Var(&f, name, usage)
	return &f
}

type multiString []string

func (ms *multiString) String() string     { return strings.Join(*ms, ", ") }
func (ms *multiString) Set(s string) error { *ms = append(*ms, s); return nil }

func main() {
	var expandedArgs []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "@") {
			f, err := os.Open(strings.TrimPrefix(arg, "@"))
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}

			respArgs, err := response.ReadRspFile(f)
			f.Close()
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
			expandedArgs = append(expandedArgs, respArgs...)
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	flags := flag.NewFlagSet("flags", flag.ExitOnError)

	packageName := flags.String("p", "", "license package name")
	moduleType := newMultiString(flags, "mt", "module type")
	moduleName := flags.String("mn", "", "module name")
	kinds := newMultiString(flags, "k", "license kinds")
	moduleClass := newMultiString(flags, "mc", "module class")
	conditions := newMultiString(flags, "c", "license conditions")
	notices := newMultiString(flags, "n", "license notice file")
	deps := newMultiString(flags, "d", "license metadata file dependency")
	sources := newMultiString(flags, "s", "source (input) dependency")
	built := newMultiString(flags, "t", "built targets")
	installed := newMultiString(flags, "i", "installed targets")
	roots := newMultiString(flags, "r", "root directory of project")
	installedMap := newMultiString(flags, "m", "map dependent targets to their installed names")
	isContainer := flags.Bool("is_container", false, "preserved dependent target name when given")
	outFile := flags.String("o", "", "output file")

	flags.Parse(expandedArgs)

	metadata := license_metadata_proto.LicenseMetadata{}
	metadata.PackageName = proto.String(*packageName)
	metadata.ModuleName = proto.String(*moduleName)
	metadata.ModuleTypes = *moduleType
	metadata.ModuleClasses = *moduleClass
	metadata.IsContainer = proto.Bool(*isContainer)
	metadata.Projects = findGitRoots(*roots)
	metadata.LicenseKinds = *kinds
	metadata.LicenseConditions = *conditions
	metadata.LicenseTexts = *notices
	metadata.Built = *built
	metadata.Installed = *installed
	metadata.InstallMap = convertInstalledMap(*installedMap)
	metadata.Sources = *sources
	metadata.Deps = convertDependencies(*deps)

	err := writeMetadata(*outFile, &metadata)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(2)
	}
}

func findGitRoots(dirs []string) []string {
	ret := make([]string, len(dirs))
	for i, dir := range dirs {
		ret[i] = findGitRoot(dir)
	}
	return ret
}

// findGitRoot finds the directory at or above dir that contains a ".git" directory.  This isn't
// guaranteed to exist, for example during remote execution, when sandboxed, when building from
// infrastructure that doesn't use git, or when the .git directory has been removed to save space,
// but it should be good enough for local builds.  If no .git directory is found the original value
// is returned.
func findGitRoot(dir string) string {
	orig := dir
	for dir != "" && dir != "." && dir != "/" {
		_, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil {
			// Found dir/.git, return dir.
			return dir
		} else if !os.IsNotExist(err) {
			// Error finding .git, return original input.
			return orig
		}
		dir, _ = filepath.Split(dir)
		dir = strings.TrimSuffix(dir, "/")
	}
	return orig
}

// convertInstalledMap converts a list of colon-separated from:to pairs into InstallMap proto
// messages.
func convertInstalledMap(installMaps []string) []*license_metadata_proto.InstallMap {
	var ret []*license_metadata_proto.InstallMap

	for _, installMap := range installMaps {
		components := strings.Split(installMap, ":")
		if len(components) != 2 {
			panic(fmt.Errorf("install map entry %q contains %d colons, expected 1", installMap, len(components)-1))
		}
		ret = append(ret, &license_metadata_proto.InstallMap{
			FromPath:      proto.String(components[0]),
			ContainerPath: proto.String(components[1]),
		})
	}

	return ret
}

// convertDependencies converts a colon-separated tuple of dependency:annotation:annotation...
// into AnnotatedDependency proto messages.
func convertDependencies(deps []string) []*license_metadata_proto.AnnotatedDependency {
	var ret []*license_metadata_proto.AnnotatedDependency

	for _, d := range deps {
		components := strings.Split(d, ":")
		dep := components[0]
		components = components[1:]
		ad := &license_metadata_proto.AnnotatedDependency{
			File:        proto.String(dep),
			Annotations: make([]string, 0, len(components)),
		}
		for _, ann := range components {
			if len(ann) == 0 {
				continue
			}
			ad.Annotations = append(ad.Annotations, ann)
		}
		ret = append(ret, ad)
	}

	return ret
}

func writeMetadata(file string, metadata *license_metadata_proto.LicenseMetadata) error {
	buf, err := prototext.MarshalOptions{Multiline: true}.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("error marshalling textproto: %w", err)
	}

	if file != "" {
		err = ioutil.WriteFile(file, buf, 0666)
		if err != nil {
			return fmt.Errorf("error writing textproto %q: %w", file, err)
		}
	} else {
		_, _ = os.Stdout.Write(buf)
	}

	return nil
}
