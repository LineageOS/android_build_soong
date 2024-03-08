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

package cc

import (
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/snapshot"
)

// Efficiently converts a list of include directories to a single string
// of cflags with -I prepended to each directory.
func includeDirsToFlags(dirs android.Paths) string {
	return android.JoinWithPrefix(dirs.Strings(), "-I")
}

var indexList = android.IndexList[string]
var inList = android.InList[string]
var filterList = android.FilterList
var removeListFromList = android.RemoveListFromList
var removeFromList = android.RemoveFromList

func flagsToBuilderFlags(in Flags) builderFlags {
	return builderFlags{
		globalCommonFlags:     strings.Join(in.Global.CommonFlags, " "),
		globalAsFlags:         strings.Join(in.Global.AsFlags, " "),
		globalYasmFlags:       strings.Join(in.Global.YasmFlags, " "),
		globalCFlags:          strings.Join(in.Global.CFlags, " "),
		globalToolingCFlags:   strings.Join(in.Global.ToolingCFlags, " "),
		globalToolingCppFlags: strings.Join(in.Global.ToolingCppFlags, " "),
		globalConlyFlags:      strings.Join(in.Global.ConlyFlags, " "),
		globalCppFlags:        strings.Join(in.Global.CppFlags, " "),
		globalLdFlags:         strings.Join(in.Global.LdFlags, " "),

		localCommonFlags:     strings.Join(in.Local.CommonFlags, " "),
		localAsFlags:         strings.Join(in.Local.AsFlags, " "),
		localYasmFlags:       strings.Join(in.Local.YasmFlags, " "),
		localCFlags:          strings.Join(in.Local.CFlags, " "),
		localToolingCFlags:   strings.Join(in.Local.ToolingCFlags, " "),
		localToolingCppFlags: strings.Join(in.Local.ToolingCppFlags, " "),
		localConlyFlags:      strings.Join(in.Local.ConlyFlags, " "),
		localCppFlags:        strings.Join(in.Local.CppFlags, " "),
		localLdFlags:         strings.Join(in.Local.LdFlags, " "),

		aidlFlags:     strings.Join(in.aidlFlags, " "),
		rsFlags:       strings.Join(in.rsFlags, " "),
		libFlags:      strings.Join(in.libFlags, " "),
		extraLibFlags: strings.Join(in.extraLibFlags, " "),
		tidyFlags:     strings.Join(in.TidyFlags, " "),
		sAbiFlags:     strings.Join(in.SAbiFlags, " "),
		toolchain:     in.Toolchain,
		gcovCoverage:  in.GcovCoverage,
		tidy:          in.Tidy,
		needTidyFiles: in.NeedTidyFiles,
		sAbiDump:      in.SAbiDump,
		emitXrefs:     in.EmitXrefs,

		systemIncludeFlags: strings.Join(in.SystemIncludeFlags, " "),

		assemblerWithCpp: in.AssemblerWithCpp,

		proto:            in.proto,
		protoC:           in.protoC,
		protoOptionsFile: in.protoOptionsFile,

		yacc: in.Yacc,
		lex:  in.Lex,
	}
}

func flagsToStripFlags(in Flags) StripFlags {
	return StripFlags{Toolchain: in.Toolchain}
}

func addPrefix(list []string, prefix string) []string {
	for i := range list {
		list[i] = prefix + list[i]
	}
	return list
}

// linkDirOnDevice/linkName -> target
func makeSymlinkCmd(linkDirOnDevice string, linkName string, target string) string {
	dir := filepath.Join("$(PRODUCT_OUT)", linkDirOnDevice)
	return "mkdir -p " + dir + " && " +
		"ln -sf " + target + " " + filepath.Join(dir, linkName)
}

// Dump a map to a list file as:
//
// {key1} {value1}
// {key2} {value2}
// ...
func installMapListFileRule(ctx android.SingletonContext, m map[string]string, path string) android.OutputPath {
	var txtBuilder strings.Builder
	for idx, k := range android.SortedKeys(m) {
		if idx > 0 {
			txtBuilder.WriteString("\n")
		}
		txtBuilder.WriteString(k)
		txtBuilder.WriteString(" ")
		txtBuilder.WriteString(m[k])
	}
	return snapshot.WriteStringToFileRule(ctx, txtBuilder.String(), path)
}
