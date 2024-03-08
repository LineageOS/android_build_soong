// Copyright 2018 Google Inc. All rights reserved.
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
	"sort"
	"strings"

	"android/soong/android"
)

func init() {
	android.RegisterMakeVarsProvider(pctx, supportLibrariesMakeVarsProvider)
}

func supportLibrariesMakeVarsProvider(ctx android.MakeVarsContext) {
	var supportAars, supportJars []string

	ctx.VisitAllModules(func(module android.Module) {
		dir := ctx.ModuleDir(module)
		switch {
		case strings.HasPrefix(dir, "prebuilts/sdk/current/extras"),
			strings.HasPrefix(dir, "prebuilts/sdk/current/androidx"),
			dir == "prebuilts/sdk/current/car",
			dir == "prebuilts/sdk/current/optional",
			dir == "prebuilts/sdk/current/support":
			// Support library
		default:
			// Not a support library
			return
		}

		name := ctx.ModuleName(module)
		if strings.HasSuffix(name, "-nodeps") {
			return
		}

		switch module.(type) {
		case *AndroidLibrary, *AARImport:
			supportAars = append(supportAars, name)
		case *Library, *Import:
			supportJars = append(supportJars, name)
		}
	})

	sort.Strings(supportAars)
	sort.Strings(supportJars)

	ctx.Strict("SUPPORT_LIBRARIES_AARS", strings.Join(supportAars, " "))
	ctx.Strict("SUPPORT_LIBRARIES_JARS", strings.Join(supportJars, " "))
}
