// Copyright 2023 Google Inc. All rights reserved.
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

package etc

import (
	"android/soong/android"
	"path/filepath"
	"strings"
)

func init() {
	RegisterInstallSymlinkBuildComponents(android.InitRegistrationContext)
}

func RegisterInstallSymlinkBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("install_symlink", InstallSymlinkFactory)
}

// install_symlink can be used to install an symlink with an arbitrary target to an arbitrary path
// on the device.
func InstallSymlinkFactory() android.Module {
	module := &InstallSymlink{}
	module.AddProperties(&module.properties)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

type InstallSymlinkProperties struct {
	// Where to install this symlink, relative to the partition it's installed on.
	// Which partition it's installed on can be controlled by the vendor, system_ext, ramdisk, etc.
	// properties.
	Installed_location string
	// The target of the symlink, aka where the symlink points.
	Symlink_target string
}

type InstallSymlink struct {
	android.ModuleBase
	properties InstallSymlinkProperties

	output        android.Path
	installedPath android.InstallPath
}

func (m *InstallSymlink) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if filepath.Clean(m.properties.Symlink_target) != m.properties.Symlink_target {
		ctx.PropertyErrorf("symlink_target", "Should be a clean filepath")
		return
	}
	if filepath.Clean(m.properties.Installed_location) != m.properties.Installed_location {
		ctx.PropertyErrorf("installed_location", "Should be a clean filepath")
		return
	}
	if strings.HasPrefix(m.properties.Installed_location, "../") || strings.HasPrefix(m.properties.Installed_location, "/") {
		ctx.PropertyErrorf("installed_location", "Should not start with / or ../")
		return
	}

	out := android.PathForModuleOut(ctx, "out.txt")
	android.WriteFileRuleVerbatim(ctx, out, "")
	m.output = out

	name := filepath.Base(m.properties.Installed_location)
	installDir := android.PathForModuleInstall(ctx, filepath.Dir(m.properties.Installed_location))
	m.installedPath = ctx.InstallAbsoluteSymlink(installDir, name, m.properties.Symlink_target)
}

func (m *InstallSymlink) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{{
		Class: "FAKE",
		// Need at least one output file in order for this to take effect.
		OutputFile: android.OptionalPathForPath(m.output),
		Include:    "$(BUILD_PHONY_PACKAGE)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.AddStrings("LOCAL_SOONG_INSTALL_SYMLINKS", m.installedPath.String())
			},
		},
	}}
}
