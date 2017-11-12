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

package genrule

import (
	"android/soong/android"
	"io"
	"strings"
	"text/template"
)

func init() {
	android.RegisterModuleType("filegroup", FileGroupFactory)
}

type fileGroupProperties struct {
	// srcs lists files that will be included in this filegroup
	Srcs []string

	Exclude_srcs []string

	// The base path to the files.  May be used by other modules to determine which portion
	// of the path to use.  For example, when a filegroup is used as data in a cc_test rule,
	// the base path is stripped off the path and the remaining path is used as the
	// installation directory.
	Path *string

	// Create a make variable with the specified name that contains the list of files in the
	// filegroup, relative to the root of the source tree.
	Export_to_make_var *string
}

type fileGroup struct {
	android.ModuleBase
	properties fileGroupProperties
	srcs       android.Paths
}

var _ android.SourceFileProducer = (*fileGroup)(nil)

// filegroup modules contain a list of files, and can be used to export files across package
// boundaries.  filegroups (and genrules) can be referenced from srcs properties of other modules
// using the syntax ":module".
func FileGroupFactory() android.Module {
	module := &fileGroup{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (fg *fileGroup) DepsMutator(ctx android.BottomUpMutatorContext) {
	android.ExtractSourcesDeps(ctx, fg.properties.Srcs)
}

func (fg *fileGroup) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	fg.srcs = ctx.ExpandSourcesSubDir(fg.properties.Srcs, fg.properties.Exclude_srcs, String(fg.properties.Path))
}

func (fg *fileGroup) Srcs() android.Paths {
	return fg.srcs
}

var androidMkTemplate = template.Must(template.New("filegroup").Parse(`
ifdef {{.makeVar}}
  $(error variable {{.makeVar}} set by soong module is already set in make)
endif
{{.makeVar}} := {{.value}}
.KATI_READONLY := {{.makeVar}}
`))

func (fg *fileGroup) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			if makeVar := String(fg.properties.Export_to_make_var); makeVar != "" {
				androidMkTemplate.Execute(w, map[string]string{
					"makeVar": makeVar,
					"value":   strings.Join(fg.srcs.Strings(), " "),
				})
			}
		},
	}
}
