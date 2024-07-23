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

package phony

import (
	"fmt"
	"io"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

func init() {
	registerPhonyModuleTypes(android.InitRegistrationContext)
}

func registerPhonyModuleTypes(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("phony", PhonyFactory)
	ctx.RegisterModuleType("phony_rule", PhonyRuleFactory)
	ctx.RegisterModuleType("phony_rule_defaults", PhonyRuleDefaultsFactory)
}

var PrepareForTestWithPhony = android.FixtureRegisterWithContext(registerPhonyModuleTypes)

type phony struct {
	android.ModuleBase
	requiredModuleNames       []string
	hostRequiredModuleNames   []string
	targetRequiredModuleNames []string
}

func PhonyFactory() android.Module {
	module := &phony{}

	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	return module
}

func (p *phony) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.requiredModuleNames = ctx.RequiredModuleNames(ctx)
	p.hostRequiredModuleNames = ctx.HostRequiredModuleNames()
	p.targetRequiredModuleNames = ctx.TargetRequiredModuleNames()
}

func (p *phony) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)", " # phony.phony")
			fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
			fmt.Fprintln(w, "LOCAL_MODULE :=", name)
			if p.Host() {
				fmt.Fprintln(w, "LOCAL_IS_HOST_MODULE := true")
			}
			if len(p.requiredModuleNames) > 0 {
				fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES :=",
					strings.Join(p.requiredModuleNames, " "))
			}
			if len(p.hostRequiredModuleNames) > 0 {
				fmt.Fprintln(w, "LOCAL_HOST_REQUIRED_MODULES :=",
					strings.Join(p.hostRequiredModuleNames, " "))
			}
			if len(p.targetRequiredModuleNames) > 0 {
				fmt.Fprintln(w, "LOCAL_TARGET_REQUIRED_MODULES :=",
					strings.Join(p.targetRequiredModuleNames, " "))
			}
			// AconfigUpdateAndroidMkData may have added elements to Extra.  Process them here.
			for _, extra := range data.Extra {
				extra(w, nil)
			}
			fmt.Fprintln(w, "include $(BUILD_PHONY_PACKAGE)")
		},
	}
}

type PhonyRule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	phonyDepsModuleNames []string
	properties           PhonyProperties
}

type PhonyProperties struct {
	// The Phony_deps is the set of all dependencies for this target,
	// and it can function similarly to .PHONY in a makefile.
	// Additionally, dependencies within it can even include genrule.
	Phony_deps proptools.Configurable[[]string]
}

// The phony_rule provides functionality similar to the .PHONY in a makefile.
// It can create a phony target and include relevant dependencies associated with it.
func PhonyRuleFactory() android.Module {
	module := &PhonyRule{}
	android.InitAndroidModule(module)
	module.AddProperties(&module.properties)
	android.InitDefaultableModule(module)
	return module
}

func (p *PhonyRule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.phonyDepsModuleNames = p.properties.Phony_deps.GetOrDefault(p.ConfigurableEvaluator(ctx), nil)
}

func (p *PhonyRule) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			if len(p.phonyDepsModuleNames) > 0 {
				depModulesStr := strings.Join(p.phonyDepsModuleNames, " ")
				fmt.Fprintln(w, ".PHONY:", name)
				fmt.Fprintln(w, name, ":", depModulesStr)
			}
		},
	}
}

// PhonyRuleDefaults
type PhonyRuleDefaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

// phony_rule_defaults provides a set of properties that can be inherited by other phony_rules.
//
// A module can use the properties from a phony_rule_defaults module using `defaults: ["defaults_module_name"]`.  Each
// property in the defaults module that exists in the depending module will be prepended to the depending module's
// value for that property.
//
// Example:
//
//	phony_rule_defaults {
//	    name: "add_module1_defaults",
//	    phony_deps: [
//	        "module1",
//	    ],
//	}
//
//	phony_rule {
//	    name: "example",
//	    defaults: ["add_module1_defaults"],
//	}
//
// is functionally identical to:
//
//	phony_rule {
//	    name: "example",
//	    phony_deps: [
//	        "module1",
//	    ],
//	}
func PhonyRuleDefaultsFactory() android.Module {
	module := &PhonyRuleDefaults{}
	module.AddProperties(&PhonyProperties{})
	android.InitDefaultsModule(module)

	return module
}
