// Copyright 2022 Google Inc. All rights reserved.
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

package multitree

import (
	"android/soong/android"
	"encoding/json"
)

func init() {
	android.RegisterParallelSingletonType("update-meta", UpdateMetaSingleton)
}

func UpdateMetaSingleton() android.Singleton {
	return &updateMetaSingleton{}
}

type jsonImported struct {
	FileGroups map[string][]string `json:",omitempty"`
}

type metadataJsonFlags struct {
	Imported jsonImported        `json:",omitempty"`
	Exported map[string][]string `json:",omitempty"`
}

type updateMetaSingleton struct {
	importedModules       []string
	generatedMetadataFile android.OutputPath
}

func (s *updateMetaSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	metadata := metadataJsonFlags{
		Imported: jsonImported{
			FileGroups: make(map[string][]string),
		},
		Exported: make(map[string][]string),
	}
	ctx.VisitAllModules(func(module android.Module) {
		if ifg, ok := module.(*importedFileGroup); ok {
			metadata.Imported.FileGroups[ifg.BaseModuleName()] = ifg.properties.Imported
		}
		if e, ok := module.(ExportableModule); ok {
			if e.IsExported() && e.Exportable() {
				for tag, files := range e.TaggedOutputs() {
					// TODO(b/219846705): refactor this to a dictionary
					metadata.Exported[e.Name()+":"+tag] = append(metadata.Exported[e.Name()+":"+tag], files.Strings()...)
				}
			}
		}
	})
	jsonStr, err := json.Marshal(metadata)
	if err != nil {
		ctx.Errorf(err.Error())
	}
	s.generatedMetadataFile = android.PathForOutput(ctx, "multitree", "metadata.json")
	android.WriteFileRule(ctx, s.generatedMetadataFile, string(jsonStr))
}

func (s *updateMetaSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("MULTITREE_METADATA", s.generatedMetadataFile.String())
}
