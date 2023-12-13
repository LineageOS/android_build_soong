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

package config

import "strings"

var (
	metalavaFlags = []string{
		"--color",
		"--quiet",
		"--format=v2",
		"--repeat-errors-max 10",
		"--hide UnresolvedImport",

		// Force metalava to ignore classes on the classpath when an API file contains missing classes.
		// See b/285140653 for more information.
		"--api-class-resolution api",

		// Force metalava to sort overloaded methods by their order in the source code.
		// See b/285312164 for more information.
		// And add concrete overrides of abstract methods, see b/299366704 for more
		// information.
		"--format-defaults overloaded-method-order=source,add-additional-overrides=yes",
	}

	MetalavaFlags = strings.Join(metalavaFlags, " ")

	metalavaAnnotationsFlags = []string{
		"--include-annotations",
		"--exclude-annotation androidx.annotation.RequiresApi",
	}

	MetalavaAnnotationsFlags = strings.Join(metalavaAnnotationsFlags, " ")

	metalavaAnnotationsWarningsFlags = []string{
		// TODO(tnorbye): find owners to fix these warnings when annotation was enabled.
		"--hide HiddenTypedefConstant",
		"--hide SuperfluousPrefix",
	}

	MetalavaAnnotationsWarningsFlags = strings.Join(metalavaAnnotationsWarningsFlags, " ")

	metalavaHideFlaggedApis = []string{
		"--revert-annotation",
		"android.annotation.FlaggedApi",
	}

	MetalavaHideFlaggedApis = strings.Join(metalavaHideFlaggedApis, " ")
)

const (
	MetalavaAddOpens = "-J--add-opens=java.base/java.util=ALL-UNNAMED"
)

func init() {
	exportedVars.ExportStringList("MetalavaFlags", metalavaFlags)

	exportedVars.ExportString("MetalavaAddOpens", MetalavaAddOpens)

	exportedVars.ExportStringList("MetalavaHideFlaggedApis", metalavaHideFlaggedApis)

	exportedVars.ExportStringListStaticVariable("MetalavaAnnotationsFlags", metalavaAnnotationsFlags)

	exportedVars.ExportStringListStaticVariable("MetalavaAnnotationWarningsFlags", metalavaAnnotationsWarningsFlags)
}
