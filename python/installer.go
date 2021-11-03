// Copyright 2017 Google Inc. All rights reserved.
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

package python

import (
	"path/filepath"

	"android/soong/android"
)

// This file handles installing python executables into their final location

type installLocation int

const (
	InstallInData installLocation = iota
)

type pythonInstaller struct {
	dir      string
	dir64    string
	relative string

	path android.InstallPath

	androidMkSharedLibs []string
}

func NewPythonInstaller(dir, dir64 string) *pythonInstaller {
	return &pythonInstaller{
		dir:   dir,
		dir64: dir64,
	}
}

var _ installer = (*pythonInstaller)(nil)

func (installer *pythonInstaller) installDir(ctx android.ModuleContext) android.InstallPath {
	dir := installer.dir
	if ctx.Arch().ArchType.Multilib == "lib64" && installer.dir64 != "" {
		dir = installer.dir64
	}
	if !ctx.Host() && ctx.Config().HasMultilibConflict(ctx.Arch().ArchType) {
		dir = filepath.Join(dir, ctx.Arch().ArchType.String())
	}
	return android.PathForModuleInstall(ctx, dir, installer.relative)
}

func (installer *pythonInstaller) install(ctx android.ModuleContext, file android.Path) {
	installer.path = ctx.InstallFile(installer.installDir(ctx), file.Base(), file)
}

func (installer *pythonInstaller) setAndroidMkSharedLibs(sharedLibs []string) {
	installer.androidMkSharedLibs = sharedLibs
}
