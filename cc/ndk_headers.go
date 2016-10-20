// Copyright 2016 Google Inc. All rights reserved.
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

	"github.com/google/blueprint"

	"android/soong/android"
)

// Returns the NDK base include path for use with sdk_version current. Usable with -I.
func getCurrentIncludePath(ctx android.ModuleContext) android.OutputPath {
	return getNdkSysrootBase(ctx).Join(ctx, "usr/include")
}

type headerProperies struct {
	// Base directory of the headers being installed. As an example:
	//
	// ndk_headers {
	//     name: "foo",
	//     from: "include",
	//     to: "",
	//     srcs: ["include/foo/bar/baz.h"],
	// }
	//
	// Will install $SYSROOT/usr/include/foo/bar/baz.h. If `from` were instead
	// "include/foo", it would have installed $SYSROOT/usr/include/bar/baz.h.
	From string

	// Install path within the sysroot. This is relative to usr/include.
	To string

	// List of headers to install. Glob compatible. Common case is "include/**/*.h".
	Srcs []string

	// Path to the NOTICE file associated with the headers.
	License string
}

type headerModule struct {
	android.ModuleBase

	properties headerProperies

	installPaths []string
	licensePath  android.ModuleSrcPath
}

func (m *headerModule) DepsMutator(ctx android.BottomUpMutatorContext) {
}

func (m *headerModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if m.properties.License == "" {
		ctx.PropertyErrorf("license", "field is required")
	}

	m.licensePath = android.PathForModuleSrc(ctx, m.properties.License)

	srcFiles := ctx.ExpandSources(m.properties.Srcs, nil)
	for _, header := range srcFiles {
		// Output path is the sysroot base + "usr/include" + to directory + directory component
		// of the file without the leading from directory stripped.
		//
		// Given:
		// sysroot base = "ndk/sysroot"
		// from = "include/foo"
		// to = "bar"
		// header = "include/foo/woodly/doodly.h"
		// output path = "ndk/sysroot/usr/include/bar/woodly/doodly.h"

		// full/platform/path/to/include/foo
		fullFromPath := android.PathForModuleSrc(ctx, m.properties.From)

		// full/platform/path/to/include/foo/woodly
		headerDir := filepath.Dir(header.String())

		// woodly
		strippedHeaderDir, err := filepath.Rel(fullFromPath.String(), headerDir)
		if err != nil {
			ctx.ModuleErrorf("filepath.Rel(%q, %q) failed: %s", headerDir,
				fullFromPath.String(), err)
		}

		// full/platform/path/to/sysroot/usr/include/bar/woodly
		installDir := getCurrentIncludePath(ctx).Join(ctx, m.properties.To, strippedHeaderDir)

		// full/platform/path/to/sysroot/usr/include/bar/woodly/doodly.h
		installPath := ctx.InstallFile(installDir, header)
		m.installPaths = append(m.installPaths, installPath.String())
	}

	if len(m.installPaths) == 0 {
		ctx.ModuleErrorf("srcs %q matched zero files", m.properties.Srcs)
	}
}

func ndkHeadersFactory() (blueprint.Module, []interface{}) {
	module := &headerModule{}
	// Host module rather than device module because device module install steps
	// do not get run when embedded in make. We're not any of the existing
	// module types that can be exposed via the Android.mk exporter, so just use
	// a host module.
	return android.InitAndroidArchModule(module, android.HostSupportedNoCross,
		android.MultilibFirst, &module.properties)
}
