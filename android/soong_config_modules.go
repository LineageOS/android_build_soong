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
	"reflect"
	"strings"
	"sync"
	"text/scanner"

	"github.com/google/blueprint"
	"github.com/google/blueprint/parser"
	"github.com/google/blueprint/proptools"

	"android/soong/android/soongconfig"
)

func init() {
	RegisterSoongConfigModuleBuildComponents(InitRegistrationContext)
}

func RegisterSoongConfigModuleBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("soong_config_module_type_import", SoongConfigModuleTypeImportFactory)
	ctx.RegisterModuleType("soong_config_module_type", SoongConfigModuleTypeFactory)
	ctx.RegisterModuleType("soong_config_string_variable", SoongConfigStringVariableDummyFactory)
	ctx.RegisterModuleType("soong_config_bool_variable", SoongConfigBoolVariableDummyFactory)
}

var PrepareForTestWithSoongConfigModuleBuildComponents = FixtureRegisterWithContext(RegisterSoongConfigModuleBuildComponents)

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
// Each soong_config_variable supports an additional value `conditions_default`. The properties
// specified in `conditions_default` will only be used under the following conditions:
//   bool variable: the variable is unspecified or not set to a true value
//   value variable: the variable is unspecified
//   string variable: the variable is unspecified or the variable is set to a string unused in the
//                    given module. For example, string variable `test` takes values: "a" and "b",
//                    if the module contains a property `a` and `conditions_default`, when test=b,
//                    the properties under `conditions_default` will be used. To specify that no
//                    properties should be amended for `b`, you can set `b: {},`.
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
//                 conditions_default: {
//                     cflags: ["-DSOC_DEFAULT"],
//                 },
//             },
//             feature: {
//                 cflags: ["-DFEATURE"],
//                 conditions_default: {
//                     cflags: ["-DFEATURE_DEFAULT"],
//                 },
//             },
//             width: {
//                 cflags: ["-DWIDTH=%s"],
//                 conditions_default: {
//                     cflags: ["-DWIDTH=DEFAULT"],
//                 },
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
//         values: ["soc_a", "soc_b", "soc_c"],
//     }
//
// If an acme BoardConfig.mk file contained:
//     $(call add_sonng_config_namespace, acme)
//     $(call add_soong_config_var_value, acme, board, soc_a)
//     $(call add_soong_config_var_value, acme, feature, true)
//     $(call add_soong_config_var_value, acme, width, 200)
//
// Then libacme_foo would build with cflags "-DGENERIC -DSOC_A -DFEATURE -DWIDTH=200".
//
// Alternatively, if acme BoardConfig.mk file contained:
//
//     SOONG_CONFIG_NAMESPACES += acme
//     SOONG_CONFIG_acme += \
//         board \
//         feature \
//
//     SOONG_CONFIG_acme_feature := false
//
// Then libacme_foo would build with cflags:
//   "-DGENERIC -DSOC_DEFAULT -DFEATURE_DEFAULT -DSIZE=DEFAULT".
//
// Similarly, if acme BoardConfig.mk file contained:
//
//     SOONG_CONFIG_NAMESPACES += acme
//     SOONG_CONFIG_acme += \
//         board \
//         feature \
//
//     SOONG_CONFIG_acme_board := soc_c
//
// Then libacme_foo would build with cflags:
//   "-DGENERIC -DSOC_DEFAULT -DFEATURE_DEFAULT -DSIZE=DEFAULT".

func SoongConfigModuleTypeImportFactory() Module {
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

func (*soongConfigModuleTypeImport) Namespaceless()                            {}
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
// Each soong_config_variable supports an additional value `conditions_default`. The properties
// specified in `conditions_default` will only be used under the following conditions:
//
//	bool variable: the variable is unspecified or not set to a true value
//	value variable: the variable is unspecified
//	string variable: the variable is unspecified or the variable is set to a string unused in the
//	                 given module. For example, string variable `test` takes values: "a" and "b",
//	                 if the module contains a property `a` and `conditions_default`, when test=b,
//	                 the properties under `conditions_default` will be used. To specify that no
//	                 properties should be amended for `b`, you can set `b: {},`.
//
// For example, an Android.bp file could have:
//
//	    soong_config_module_type {
//	        name: "acme_cc_defaults",
//	        module_type: "cc_defaults",
//	        config_namespace: "acme",
//	        variables: ["board"],
//	        bool_variables: ["feature"],
//	        value_variables: ["width"],
//	        properties: ["cflags", "srcs"],
//	    }
//
//	    soong_config_string_variable {
//	        name: "board",
//	        values: ["soc_a", "soc_b"],
//	    }
//
//	    acme_cc_defaults {
//	        name: "acme_defaults",
//	        cflags: ["-DGENERIC"],
//	        soong_config_variables: {
//	            board: {
//	                soc_a: {
//	                    cflags: ["-DSOC_A"],
//	                },
//	                soc_b: {
//	                    cflags: ["-DSOC_B"],
//	                },
//	                conditions_default: {
//	                    cflags: ["-DSOC_DEFAULT"],
//	                },
//	            },
//	            feature: {
//	                cflags: ["-DFEATURE"],
//	                conditions_default: {
//	                    cflags: ["-DFEATURE_DEFAULT"],
//	                },
//	            },
//	            width: {
//		               cflags: ["-DWIDTH=%s"],
//	                conditions_default: {
//	                    cflags: ["-DWIDTH=DEFAULT"],
//	                },
//	            },
//	        },
//	    }
//
//	    cc_library {
//	        name: "libacme_foo",
//	        defaults: ["acme_defaults"],
//	        srcs: ["*.cpp"],
//	    }
//
// If an acme BoardConfig.mk file contained:
//
//	SOONG_CONFIG_NAMESPACES += acme
//	SOONG_CONFIG_acme += \
//	    board \
//	    feature \
//
//	SOONG_CONFIG_acme_board := soc_a
//	SOONG_CONFIG_acme_feature := true
//	SOONG_CONFIG_acme_width := 200
//
// Then libacme_foo would build with cflags "-DGENERIC -DSOC_A -DFEATURE".
func SoongConfigModuleTypeFactory() Module {
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
	return m.properties.Name + fmt.Sprintf("%p", m)
}
func (*soongConfigModuleTypeModule) Namespaceless()                                {}
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
func SoongConfigStringVariableDummyFactory() Module {
	module := &soongConfigStringVariableDummyModule{}
	module.AddProperties(&module.properties, &module.stringProperties)
	initAndroidModuleBase(module)
	return module
}

// soong_config_string_variable defines a variable with true or false values for use
// in a soong_config_module_type definition.
func SoongConfigBoolVariableDummyFactory() Module {
	module := &soongConfigBoolVariableDummyModule{}
	module.AddProperties(&module.properties)
	initAndroidModuleBase(module)
	return module
}

func (m *soongConfigStringVariableDummyModule) Name() string {
	return m.properties.Name + fmt.Sprintf("%p", m)
}
func (*soongConfigStringVariableDummyModule) Namespaceless()                                {}
func (*soongConfigStringVariableDummyModule) GenerateAndroidBuildActions(ctx ModuleContext) {}

func (m *soongConfigBoolVariableDummyModule) Name() string {
	return m.properties.Name + fmt.Sprintf("%p", m)
}
func (*soongConfigBoolVariableDummyModule) Namespaceless()                                {}
func (*soongConfigBoolVariableDummyModule) GenerateAndroidBuildActions(ctx ModuleContext) {}

// importModuleTypes registers the module factories for a list of module types defined
// in an Android.bp file. These module factories are scoped for the current Android.bp
// file only.
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
		defer r.Close()

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
				factories[name] = configModuleFactory(factory, moduleType)
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

// tracingConfig is a wrapper to soongconfig.SoongConfig which records all accesses to SoongConfig.
type tracingConfig struct {
	config    soongconfig.SoongConfig
	boolSet   map[string]bool
	stringSet map[string]string
	isSetSet  map[string]bool
}

func (c *tracingConfig) Bool(name string) bool {
	c.boolSet[name] = c.config.Bool(name)
	return c.boolSet[name]
}

func (c *tracingConfig) String(name string) string {
	c.stringSet[name] = c.config.String(name)
	return c.stringSet[name]
}

func (c *tracingConfig) IsSet(name string) bool {
	c.isSetSet[name] = c.config.IsSet(name)
	return c.isSetSet[name]
}

func (c *tracingConfig) getTrace() soongConfigTrace {
	ret := soongConfigTrace{}

	for k, v := range c.boolSet {
		ret.Bools = append(ret.Bools, fmt.Sprintf("%q:%t", k, v))
	}
	for k, v := range c.stringSet {
		ret.Strings = append(ret.Strings, fmt.Sprintf("%q:%q", k, v))
	}
	for k, v := range c.isSetSet {
		ret.IsSets = append(ret.IsSets, fmt.Sprintf("%q:%t", k, v))
	}

	return ret
}

func newTracingConfig(config soongconfig.SoongConfig) *tracingConfig {
	c := tracingConfig{
		config:    config,
		boolSet:   make(map[string]bool),
		stringSet: make(map[string]string),
		isSetSet:  make(map[string]bool),
	}
	return &c
}

var _ soongconfig.SoongConfig = (*tracingConfig)(nil)

// configModuleFactory takes an existing soongConfigModuleFactory and a
// ModuleType to create a new ModuleFactory that uses a custom loadhook.
func configModuleFactory(factory blueprint.ModuleFactory, moduleType *soongconfig.ModuleType) blueprint.ModuleFactory {
	// Defer creation of conditional properties struct until the first call from the factory
	// method. That avoids having to make a special call to the factory to create the properties
	// structs from which the conditional properties struct is created. This is needed in order to
	// allow singleton modules to be customized by soong_config_module_type as the
	// SingletonModuleFactoryAdaptor factory registers a load hook for the singleton module
	// everytime that it is called. Calling the factory twice causes a build failure as the load
	// hook is called twice, the first time it updates the singleton module to indicate that it has
	// been registered as a module, and the second time it fails because it thinks it has been
	// registered again and a singleton module can only be registered once.
	//
	// This is an issue for singleton modules because:
	// * Load hooks are registered on the module object and are only called when the module object
	//   is created by Blueprint while processing the Android.bp file.
	// * The module factory for a singleton module returns the same module object each time it is
	//   called, and registers its load hook on that same module object.
	// * When the module factory is called by Blueprint it then calls all the load hooks that have
	//   been registered for every call to that module factory.
	//
	// It is not an issue for normal modules because they return a new module object each time the
	// factory is called and so any load hooks registered on module objects which are discarded will
	// not be run.
	once := &sync.Once{}
	conditionalFactoryProps := reflect.Value{}
	getConditionalFactoryProps := func(props []interface{}) reflect.Value {
		once.Do(func() {
			conditionalFactoryProps = soongconfig.CreateProperties(props, moduleType)
		})
		return conditionalFactoryProps
	}

	return func() (blueprint.Module, []interface{}) {
		module, props := factory()
		conditionalFactoryProps := getConditionalFactoryProps(props)
		if !conditionalFactoryProps.IsValid() {
			return module, props
		}

		conditionalProps := proptools.CloneEmptyProperties(conditionalFactoryProps)
		props = append(props, conditionalProps.Interface())

		// Regular Soong operation wraps the existing module factory with a
		// conditional on Soong config variables by reading the product
		// config variables from Make.
		AddLoadHook(module, func(ctx LoadHookContext) {
			tracingConfig := newTracingConfig(ctx.Config().VendorConfig(moduleType.ConfigNamespace))
			newProps, err := soongconfig.PropertiesToApply(moduleType, conditionalProps, tracingConfig)
			if err != nil {
				ctx.ModuleErrorf("%s", err)
				return
			}
			for _, ps := range newProps {
				ctx.AppendProperties(ps)
			}

			module.(Module).base().commonProperties.SoongConfigTrace = tracingConfig.getTrace()
		})
		return module, props
	}
}
