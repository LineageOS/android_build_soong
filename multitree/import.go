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
)

var (
	nameSuffix = ".imported"
)

type MultitreeImportedModuleInterface interface {
	GetMultitreeImportedModuleName() string
}

func init() {
	android.RegisterModuleType("imported_filegroup", importedFileGroupFactory)

	android.PreArchMutators(RegisterMultitreePreArchMutators)
}

type importedFileGroupProperties struct {
	// Imported modules from the other components in a multi-tree
	Imported []string
}

type importedFileGroup struct {
	android.ModuleBase

	properties importedFileGroupProperties
	srcs       android.Paths
}

func (ifg *importedFileGroup) Name() string {
	return ifg.BaseModuleName() + nameSuffix
}

func importedFileGroupFactory() android.Module {
	module := &importedFileGroup{}
	module.AddProperties(&module.properties)

	android.InitAndroidModule(module)
	return module
}

var _ MultitreeImportedModuleInterface = (*importedFileGroup)(nil)

func (ifg *importedFileGroup) GetMultitreeImportedModuleName() string {
	// The base module name of the imported filegroup is used as the imported module name
	return ifg.BaseModuleName()
}

var _ android.SourceFileProducer = (*importedFileGroup)(nil)

func (ifg *importedFileGroup) Srcs() android.Paths {
	return ifg.srcs
}

func (ifg *importedFileGroup) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// srcs from this module must not be used. Adding a dot path to avoid the empty
	// source failure. Still soong returns error when a module wants to build against
	// this source, which is intended.
	ifg.srcs = android.PathsForModuleSrc(ctx, []string{"."})
}

func RegisterMultitreePreArchMutators(ctx android.RegisterMutatorsContext) {
	ctx.BottomUp("multitree_imported_rename", MultitreeImportedRenameMutator).Parallel()
}

func MultitreeImportedRenameMutator(ctx android.BottomUpMutatorContext) {
	if m, ok := ctx.Module().(MultitreeImportedModuleInterface); ok {
		name := m.GetMultitreeImportedModuleName()
		if !ctx.OtherModuleExists(name) {
			// Provide an empty filegroup not to break the build while updating the metadata.
			// In other cases, soong will report an error to guide users to run 'm update-meta'
			// first.
			if !ctx.Config().TargetMultitreeUpdateMeta() {
				ctx.ModuleErrorf("\"%s\" filegroup must be imported.\nRun 'm update-meta' first to import the filegroup.", name)
			}
			ctx.Rename(name)
		}
	}
}
