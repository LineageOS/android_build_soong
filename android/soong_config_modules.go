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

package android

// This file provides module types that implement wrapper module types that add conditionals on
// Soong config variables.

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/scanner"

	"github.com/google/blueprint"
	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/proptools"

	"android/soong/android/soongconfig"
)

func init() {
	RegisterModuleType("soong_config_module_type_import", soongConfigModuleTypeImportFactory)
	RegisterModuleType("soong_config_module_type", soongConfigModuleTypeFactory)
	RegisterModuleType("soong_config_string_variable", soongConfigStringVariableDummyFactory)
	RegisterModuleType("soong_config_bool_variable", soongConfigBoolVariableDummyFactory)
}

type soongConfigModuleTypeImport struct {
	ModuleBase
	properties soongConfigModuleTypeImportProperties
}

type soongConfigModuleTypeImportProperties struct {
	From         string
	Module_types []string
}

// soong_config_module_type_import imports module types with conditionals on Soong config
// variables from another Android.bp file.  The imported module type will exist for all
// modules after the import in the Android.bp file.
//
// For example, an Android.bp file could have:
//
//     soong_config_module_type_import {
//         from: "device/acme/Android.bp",
//         module_types: ["acme_cc_defaults"],
//     }
//
//     acme_cc_defaults {
//         name: "acme_defaults",
//         cflags: ["-DGENERIC"],
//         soong_config_variables: {
//             board: {
//                 soc_a: {
//                     cflags: ["-DSOC_A"],
//                 },
//                 soc_b: {
//                     cflags: ["-DSOC_B"],
//                 },
//             },
//             feature: {
//                 cflags: ["-DFEATURE"],
//             },
//             width: {
//                 cflags: ["-DWIDTH=%s"],
//             },
//         },
//     }
//
//     cc_library {
//         name: "libacme_foo",
//         defaults: ["acme_defaults"],
//         srcs: ["*.cpp"],
//     }
//
// And device/acme/Android.bp could have:
//
//     soong_config_module_type {
//         name: "acme_cc_defaults",
//         module_type: "cc_defaults",
//         config_namespace: "acme",
//         variables: ["board"],
//         bool_variables: ["feature"],
//         value_variables: ["width"],
//         properties: ["cflags", "srcs"],
//     }
//
//     soong_config_string_variable {
//         name: "board",
//         values: ["soc_a", "soc_b"],
//     }
//
// If an acme BoardConfig.mk file contained:
//
//     SOONG_CONFIG_NAMESPACES += acme
//     SOONG_CONFIG_acme += \
//         board \
//         feature \
//
//     SOONG_CONFIG_acme_board := soc_a
//     SOONG_CONFIG_acme_feature := true
//     SOONG_CONFIG_acme_width := 200
//
// Then libacme_foo would build with cflags "-DGENERIC -DSOC_A -DFEATURE -DWIDTH=200".
func soongConfigModuleTypeImportFactory() Module {
	module := &soongConfigModuleTypeImport{}

	module.AddProperties(&module.properties)
	AddLoadHook(module, func(ctx LoadHookContext) {
		importModuleTypes(ctx, module.properties.From, module.properties.Module_types...)
	})

	initAndroidModuleBase(module)
	return module
}

func (m *soongConfigModuleTypeImport) Name() string {
	// The generated name is non-deterministic, but it does not
	// matter because this module does not emit any rules.
	return soongconfig.CanonicalizeToProperty(m.properties.From) +
		"soong_config_module_type_import_" + fmt.Sprintf("%p", m)
}

func (*soongConfigModuleTypeImport) Nameless()                                 {}
func (*soongConfigModuleTypeImport) GenerateAndroidBuildActions(ModuleContext) {}

// Create dummy modules for soong_config_module_type and soong_config_*_variable

type soongConfigModuleTypeModule struct {
	ModuleBase
	properties soongconfig.ModuleTypeProperties
}

// soong_config_module_type defines module types with conditionals on Soong config
// variables.  The new module type will exist for all modules after the definition
// in an Android.bp file, and can be imported into other Android.bp files using
// soong_config_module_type_import.
//
// For example, an Android.bp file could have:
//
//     soong_config_module_type {
//         name: "acme_cc_defaults",
//         module_type: "cc_defaults",
//         config_namespace: "acme",
//         variables: ["board"],
//         bool_variables: ["feature"],
//         value_variables: ["width"],
//         properties: ["cflags", "srcs"],
//     }
//
//     soong_config_string_variable {
//         name: "board",
//         values: ["soc_a", "soc_b"],
//     }
//
//     acme_cc_defaults {
//         name: "acme_defaults",
//         cflags: ["-DGENERIC"],
//         soong_config_variables: {
//             board: {
//                 soc_a: {
//                     cflags: ["-DSOC_A"],
//                 },
//                 soc_b: {
//                     cflags: ["-DSOC_B"],
//                 },
//             },
//             feature: {
//                 cflags: ["-DFEATURE"],
//             },
//             width: {
//	               cflags: ["-DWIDTH=%s"],
//             },
//         },
//     }
//
//     cc_library {
//         name: "libacme_foo",
//         defaults: ["acme_defaults"],
//         srcs: ["*.cpp"],
//     }
//
// If an acme BoardConfig.mk file contained:
//
//     SOONG_CONFIG_NAMESPACES += acme
//     SOONG_CONFIG_acme += \
//         board \
//         feature \
//
//     SOONG_CONFIG_acme_board := soc_a
//     SOONG_CONFIG_acme_feature := true
//     SOONG_CONFIG_acme_width := 200
//
// Then libacme_foo would build with cflags "-DGENERIC -DSOC_A -DFEATURE".
func soongConfigModuleTypeFactory() Module {
	module := &soongConfigModuleTypeModule{}

	module.AddProperties(&module.properties)

	AddLoadHook(module, func(ctx LoadHookContext) {
		// A soong_config_module_type module should implicitly import itself.
		importModuleTypes(ctx, ctx.BlueprintsFile(), module.properties.Name)
	})

	initAndroidModuleBase(module)

	return module
}

func (m *soongConfigModuleTypeModule) Name() string {
	return m.properties.Name
}
func (*soongConfigModuleTypeModule) Nameless()                                     {}
func (*soongConfigModuleTypeModule) GenerateAndroidBuildActions(ctx ModuleContext) {}

type soongConfigStringVariableDummyModule struct {
	ModuleBase
	properties       soongconfig.VariableProperties
	stringProperties soongconfig.StringVariableProperties
}

type soongConfigBoolVariableDummyModule struct {
	ModuleBase
	properties soongconfig.VariableProperties
}

// soong_config_string_variable defines a variable and a set of possible string values for use
// in a soong_config_module_type definition.
func soongConfigStringVariableDummyFactory() Module {
	module := &soongConfigStringVariableDummyModule{}
	module.AddProperties(&module.properties, &module.stringProperties)
	initAndroidModuleBase(module)
	return module
}

// soong_config_string_variable defines a variable with true or false values for use
// in a soong_config_module_type definition.
func soongConfigBoolVariableDummyFactory() Module {
	module := &soongConfigBoolVariableDummyModule{}
	module.AddProperties(&module.properties)
	initAndroidModuleBase(module)
	return module
}

func (m *soongConfigStringVariableDummyModule) Name() string {
	return m.properties.Name
}
func (*soongConfigStringVariableDummyModule) Nameless()                                     {}
func (*soongConfigStringVariableDummyModule) GenerateAndroidBuildActions(ctx ModuleContext) {}

func (m *soongConfigBoolVariableDummyModule) Name() string {
	return m.properties.Name
}
func (*soongConfigBoolVariableDummyModule) Nameless()                                     {}
func (*soongConfigBoolVariableDummyModule) GenerateAndroidBuildActions(ctx ModuleContext) {}

func importModuleTypes(ctx LoadHookContext, from string, moduleTypes ...string) {
	from = filepath.Clean(from)
	if filepath.Ext(from) != ".bp" {
		ctx.PropertyErrorf("from", "%q must be a file with extension .bp", from)
		return
	}

	if strings.HasPrefix(from, "../") {
		ctx.PropertyErrorf("from", "%q must not use ../ to escape the source tree",
			from)
		return
	}

	moduleTypeDefinitions := loadSoongConfigModuleTypeDefinition(ctx, from)
	if moduleTypeDefinitions == nil {
		return
	}
	for _, moduleType := range moduleTypes {
		if factory, ok := moduleTypeDefinitions[moduleType]; ok {
			ctx.registerScopedModuleType(moduleType, factory)
		} else {
			ctx.PropertyErrorf("module_types", "module type %q not defined in %q",
				moduleType, from)
		}
	}
}

// loadSoongConfigModuleTypeDefinition loads module types from an Android.bp file.  It caches the
// result so each file is only parsed once.
func loadSoongConfigModuleTypeDefinition(ctx LoadHookContext, from string) map[string]blueprint.ModuleFactory {
	type onceKeyType string
	key := NewCustomOnceKey(onceKeyType(filepath.Clean(from)))

	reportErrors := func(ctx LoadHookContext, filename string, errs ...error) {
		for _, err := range errs {
			if parseErr, ok := err.(*parser.ParseError); ok {
				ctx.Errorf(parseErr.Pos, "%s", parseErr.Err)
			} else {
				ctx.Errorf(scanner.Position{Filename: filename}, "%s", err)
			}
		}
	}

	return ctx.Config().Once(key, func() interface{} {
		ctx.AddNinjaFileDeps(from)
		r, err := ctx.Config().fs.Open(from)
		if err != nil {
			ctx.PropertyErrorf("from", "failed to open %q: %s", from, err)
			return (map[string]blueprint.ModuleFactory)(nil)
		}

		mtDef, errs := soongconfig.Parse(r, from)

		if len(errs) > 0 {
			reportErrors(ctx, from, errs...)
			return (map[string]blueprint.ModuleFactory)(nil)
		}

		globalModuleTypes := ctx.moduleFactories()

		factories := make(map[string]blueprint.ModuleFactory)

		for name, moduleType := range mtDef.ModuleTypes {
			factory := globalModuleTypes[moduleType.BaseModuleType]
			if factory != nil {
				factories[name] = soongConfigModuleFactory(factory, moduleType)
			} else {
				reportErrors(ctx, from,
					fmt.Errorf("missing global module type factory for %q", moduleType.BaseModuleType))
			}
		}

		if ctx.Failed() {
			return (map[string]blueprint.ModuleFactory)(nil)
		}

		return factories
	}).(map[string]blueprint.ModuleFactory)
}

// soongConfigModuleFactory takes an existing soongConfigModuleFactory and a ModuleType and returns
// a new soongConfigModuleFactory that wraps the existing soongConfigModuleFactory and adds conditional on Soong config
// variables.
func soongConfigModuleFactory(factory blueprint.ModuleFactory,
	moduleType *soongconfig.ModuleType) blueprint.ModuleFactory {

	conditionalFactoryProps := soongconfig.CreateProperties(factory, moduleType)
	if conditionalFactoryProps.IsValid() {
		return func() (blueprint.Module, []interface{}) {
			module, props := factory()

			conditionalProps := proptools.CloneEmptyProperties(conditionalFactoryProps)
			props = append(props, conditionalProps.Interface())

			AddLoadHook(module, func(ctx LoadHookContext) {
				config := ctx.Config().VendorConfig(moduleType.ConfigNamespace)
				newProps, err := soongconfig.PropertiesToApply(moduleType, conditionalProps, config)
				if err != nil {
					ctx.ModuleErrorf("%s", err)
					return
				}
				for _, ps := range newProps {
					ctx.AppendProperties(ps)
				}
			})

			return module, props
		}
	} else {
		return factory
	}
}
