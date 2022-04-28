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

	"github.com/google/blueprint/proptools"
)

type moduleExportProperty struct {
	// True if the module is exported to the other components in a multi-tree.
	// Any components in the multi-tree can import this module to use.
	Export *bool
}

type ExportableModuleBase struct {
	properties moduleExportProperty
}

type Exportable interface {
	// Properties for the exporable module.
	exportableModuleProps() *moduleExportProperty

	// Check if this module can be exported.
	// If this returns false, the module will not be exported regardless of the 'export' value.
	Exportable() bool

	// Returns 'true' if this module has 'export: true'
	// This module will not be exported if it returns 'false' to 'Exportable()' interface even if
	// it has 'export: true'.
	IsExported() bool

	// Map from tags to outputs.
	// Each module can tag their outputs for convenience.
	TaggedOutputs() map[string]android.Paths
}

type ExportableModule interface {
	android.Module
	android.OutputFileProducer
	Exportable
}

func InitExportableModule(module ExportableModule) {
	module.AddProperties(module.exportableModuleProps())
}

func (m *ExportableModuleBase) exportableModuleProps() *moduleExportProperty {
	return &m.properties
}

func (m *ExportableModuleBase) IsExported() bool {
	return proptools.Bool(m.properties.Export)
}
