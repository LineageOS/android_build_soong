// Copyright (C) 2019 The Android Open Source Project
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

package sdk

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/java"
)

var pctx = android.NewPackageContext("android/soong/sdk")

// generatedFile abstracts operations for writing contents into a file and emit a build rule
// for the file.
type generatedFile struct {
	path    android.OutputPath
	content strings.Builder
}

func newGeneratedFile(ctx android.ModuleContext, name string) *generatedFile {
	return &generatedFile{
		path: android.PathForModuleOut(ctx, name).OutputPath,
	}
}

func (gf *generatedFile) printfln(format string, args ...interface{}) {
	// ninja consumes newline characters in rspfile_content. Prevent it by
	// escaping the backslash in the newline character. The extra backshash
	// is removed when the rspfile is written to the actual script file
	fmt.Fprintf(&(gf.content), format+"\\n", args...)
}

func (gf *generatedFile) build(pctx android.PackageContext, ctx android.BuilderContext, implicits android.Paths) {
	rb := android.NewRuleBuilder()
	// convert \\n to \n
	rb.Command().
		Implicits(implicits).
		Text("echo").Text(proptools.ShellEscape(gf.content.String())).
		Text("| sed 's/\\\\n/\\n/g' >").Output(gf.path)
	rb.Command().
		Text("chmod a+x").Output(gf.path)
	rb.Build(pctx, ctx, gf.path.Base(), "Build "+gf.path.Base())
}

func (s *sdk) javaMemberNames(ctx android.ModuleContext) []string {
	result := []string{}
	ctx.VisitDirectDeps(func(m android.Module) {
		if _, ok := m.(*java.Library); ok {
			result = append(result, m.Name())
		}
	})
	return result
}

// buildAndroidBp creates the blueprint file that defines prebuilt modules for each of
// the SDK members, and the sdk_snapshot module for the specified version
func (s *sdk) buildAndroidBp(ctx android.ModuleContext, version string) android.OutputPath {
	bp := newGeneratedFile(ctx, "blueprint-"+version+".sh")

	makePrebuiltName := func(name string) string {
		return ctx.ModuleName() + "_" + name + string(android.SdkVersionSeparator) + version
	}

	javaLibs := s.javaMemberNames(ctx)
	for _, name := range javaLibs {
		prebuiltName := makePrebuiltName(name)
		jar := filepath.Join("java", name, "stub.jar")

		bp.printfln("java_import {")
		bp.printfln("    name: %q,", prebuiltName)
		bp.printfln("    jars: [%q],", jar)
		bp.printfln("    sdk_member_name: %q,", name)
		bp.printfln("}")
		bp.printfln("")

		// This module is for the case when the source tree for the unversioned module
		// doesn't exist (i.e. building in an unbundled tree). "prefer:" is set to false
		// so that this module does not eclipse the unversioned module if it exists.
		bp.printfln("java_import {")
		bp.printfln("    name: %q,", name)
		bp.printfln("    jars: [%q],", jar)
		bp.printfln("    prefer: false,")
		bp.printfln("}")
		bp.printfln("")

	}

	// TODO(jiyong): emit cc_prebuilt_library_shared for the native libs

	bp.printfln("sdk_snapshot {")
	bp.printfln("    name: %q,", ctx.ModuleName()+string(android.SdkVersionSeparator)+version)
	bp.printfln("    java_libs: [")
	for _, n := range javaLibs {
		bp.printfln("        %q,", makePrebuiltName(n))
	}
	bp.printfln("    ],")
	// TODO(jiyong): emit native_shared_libs
	bp.printfln("}")
	bp.printfln("")

	bp.build(pctx, ctx, nil)
	return bp.path
}

func (s *sdk) buildScript(ctx android.ModuleContext, version string) android.OutputPath {
	sh := newGeneratedFile(ctx, "update_prebuilt-"+version+".sh")

	snapshotRoot := filepath.Join(ctx.ModuleDir(), version)
	aidlIncludeDir := filepath.Join(snapshotRoot, "aidl")
	javaStubsDir := filepath.Join(snapshotRoot, "java")

	sh.printfln("#!/bin/bash")
	sh.printfln("echo Updating snapshot of %s in %s", ctx.ModuleName(), snapshotRoot)
	sh.printfln("pushd $ANDROID_BUILD_TOP > /dev/null")
	sh.printfln("rm -rf %s", snapshotRoot)
	sh.printfln("mkdir -p %s", aidlIncludeDir)
	sh.printfln("mkdir -p %s", javaStubsDir)
	// TODO(jiyong): mkdir the 'native' dir

	var implicits android.Paths
	ctx.VisitDirectDeps(func(m android.Module) {
		if javaLib, ok := m.(*java.Library); ok {
			headerJars := javaLib.HeaderJars()
			if len(headerJars) != 1 {
				panic(fmt.Errorf("there must be only one header jar from %q", m.Name()))
			}
			implicits = append(implicits, headerJars...)

			exportedAidlIncludeDirs := javaLib.AidlIncludeDirs()
			for _, dir := range exportedAidlIncludeDirs {
				// Using tar to copy with the directory structure
				// TODO(jiyong): copy parcelable declarations only
				sh.printfln("find %s -name \"*.aidl\" | tar cf - -T - | (cd %s; tar xf -)",
					dir.String(), aidlIncludeDir)
			}

			copiedHeaderJar := filepath.Join(javaStubsDir, m.Name(), "stub.jar")
			sh.printfln("mkdir -p $(dirname %s) && cp %s %s",
				copiedHeaderJar, headerJars[0].String(), copiedHeaderJar)
		}
		// TODO(jiyong): emit the commands for copying the headers and stub libraries for native libs
	})

	bp := s.buildAndroidBp(ctx, version)
	implicits = append(implicits, bp)
	sh.printfln("cp %s %s", bp.String(), filepath.Join(snapshotRoot, "Android.bp"))

	sh.printfln("popd > /dev/null")
	sh.printfln("rm -- \"$0\"") // self deleting so that stale script is not used
	sh.printfln("echo Done")

	sh.build(pctx, ctx, implicits)
	return sh.path
}

func (s *sdk) buildSnapshotGenerationScripts(ctx android.ModuleContext) {
	if s.snapshot() {
		// we don't need a script for sdk_snapshot.. as they are frozen
		return
	}

	// script to update the 'current' snapshot
	s.updateScript = s.buildScript(ctx, "current")

	versions := s.frozenVersions(ctx)
	newVersion := "1"
	if len(versions) >= 1 {
		lastVersion := versions[len(versions)-1]
		lastVersionNum, err := strconv.Atoi(lastVersion)
		if err != nil {
			panic(err)
			return
		}
		newVersion = strconv.Itoa(lastVersionNum + 1)
	}
	// script to create a new frozen version of snapshot
	s.freezeScript = s.buildScript(ctx, newVersion)
}

func (s *sdk) androidMkEntriesForScript() android.AndroidMkEntries {
	if s.snapshot() {
		// we don't need a script for sdk_snapshot.. as they are frozen
		return android.AndroidMkEntries{}
	}

	entries := android.AndroidMkEntries{
		Class: "FAKE",
		// TODO(jiyong): remove this? but androidmk.go expects OutputFile to be specified anyway
		OutputFile: android.OptionalPathForPath(s.updateScript),
		Include:    "$(BUILD_SYSTEM)/base_rules.mk",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.AddStrings("LOCAL_ADDITIONAL_DEPENDENCIES",
					s.updateScript.String(), s.freezeScript.String())
			},
		},
		ExtraFooters: []android.AndroidMkExtraFootersFunc{
			func(w io.Writer, name, prefix, moduleDir string, entries *android.AndroidMkEntries) {
				fmt.Fprintln(w, "$(LOCAL_BUILT_MODULE): $(LOCAL_ADDITIONAL_DEPENDENCIES)")
				fmt.Fprintln(w, "	touch $@")
				fmt.Fprintln(w, "	echo ##################################################")
				fmt.Fprintln(w, "	echo To update current SDK: execute", s.updateScript.String())
				fmt.Fprintln(w, "	echo To freeze current SDK: execute", s.freezeScript.String())
				fmt.Fprintln(w, "	echo ##################################################")
			},
		},
	}
	return entries
}
