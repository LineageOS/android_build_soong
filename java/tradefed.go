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

package java

import (
	"android/soong/android"
)

func init() {
	android.RegisterModuleType("tradefed_java_library_host", tradefedJavaLibraryFactory)
}

// tradefed_java_library_factory wraps java_library and installs an additional
// copy of the output jar to $HOST_OUT/tradefed.
func tradefedJavaLibraryFactory() android.Module {
	module := LibraryHostFactory().(*Library)
	module.InstallMixin = tradefedJavaLibraryInstall
	return module
}

func tradefedJavaLibraryInstall(ctx android.ModuleContext, path android.Path) android.InstallPaths {
	installedPath := ctx.InstallFile(android.PathForModuleInstall(ctx, "tradefed"),
		ctx.ModuleName()+".jar", path)
	return android.InstallPaths{installedPath}
}
