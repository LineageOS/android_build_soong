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

package tradefed

import (
	"fmt"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

const test_xml_indent = "    "

func getTestConfigTemplate(ctx android.ModuleContext, prop *string) android.OptionalPath {
	return ctx.ExpandOptionalSource(prop, "test_config_template")
}

func getTestConfig(ctx android.ModuleContext, prop *string) android.Path {
	if p := ctx.ExpandOptionalSource(prop, "test_config"); p.Valid() {
		return p.Path()
	} else if p := android.ExistentPathForSource(ctx, ctx.ModuleDir(), "AndroidTest.xml"); p.Valid() {
		return p.Path()
	}
	return nil
}

var autogenTestConfig = pctx.StaticRule("autogenTestConfig", blueprint.RuleParams{
	Command:     "sed 's&{MODULE}&${name}&g;s&{EXTRA_CONFIGS}&'${extraConfigs}'&g;s&{EXTRA_TEST_RUNNER_CONFIGS}&'${extraTestRunnerConfigs}'&g;s&{OUTPUT_FILENAME}&'${outputFileName}'&g;s&{TEST_INSTALL_BASE}&'${testInstallBase}'&g' $template > $out",
	CommandDeps: []string{"$template"},
}, "name", "template", "extraConfigs", "outputFileName", "testInstallBase", "extraTestRunnerConfigs")

func testConfigPath(ctx android.ModuleContext, prop *string, testSuites []string, autoGenConfig *bool, testConfigTemplateProp *string) (path android.Path, autogenPath android.WritablePath) {
	p := getTestConfig(ctx, prop)
	if !Bool(autoGenConfig) && p != nil {
		return p, nil
	} else if BoolDefault(autoGenConfig, true) && (!android.InList("cts", testSuites) || testConfigTemplateProp != nil) {
		outputFile := android.PathForModuleOut(ctx, ctx.ModuleName()+".config")
		return nil, outputFile
	} else {
		// CTS modules can be used for test data, so test config files must be
		// explicitly created using AndroidTest.xml or test_config_template.
		return nil, nil
	}
}

type Config interface {
	Config() string
}

type Option struct {
	Name  string
	Key   string
	Value string
}

var _ Config = Option{}

func (o Option) Config() string {
	if o.Key != "" {
		return fmt.Sprintf(`<option name="%s" key="%s" value="%s" />`, o.Name, o.Key, o.Value)
	}
	return fmt.Sprintf(`<option name="%s" value="%s" />`, o.Name, o.Value)
}

// It can be a template of object or target_preparer.
type Object struct {
	// Set it as a target_preparer if object type == "target_preparer".
	Type    string
	Class   string
	Options []Option
}

var _ Config = Object{}

func (ob Object) Config() string {
	var optionStrings []string
	for _, option := range ob.Options {
		optionStrings = append(optionStrings, option.Config())
	}
	var options string
	if len(ob.Options) == 0 {
		options = ""
	} else {
		optionDelimiter := fmt.Sprintf("\\n%s%s", test_xml_indent, test_xml_indent)
		options = optionDelimiter + strings.Join(optionStrings, optionDelimiter)
	}
	if ob.Type == "target_preparer" {
		return fmt.Sprintf(`<target_preparer class="%s">%s\n%s</target_preparer>`, ob.Class, options, test_xml_indent)
	} else {
		return fmt.Sprintf(`<object type="%s" class="%s">%s\n%s</object>`, ob.Type, ob.Class, options, test_xml_indent)
	}

}

func autogenTemplate(ctx android.ModuleContext, name string, output android.WritablePath, template string, configs []Config, testRunnerConfigs []Option, outputFileName string, testInstallBase string) {
	if template == "" {
		ctx.ModuleErrorf("Empty template")
	}
	var configStrings []string
	for _, config := range configs {
		configStrings = append(configStrings, config.Config())
	}
	extraConfigs := strings.Join(configStrings, fmt.Sprintf("\\n%s", test_xml_indent))
	extraConfigs = proptools.NinjaAndShellEscape(extraConfigs)

	var testRunnerConfigStrings []string
	for _, config := range testRunnerConfigs {
		testRunnerConfigStrings = append(testRunnerConfigStrings, config.Config())
	}
	extraTestRunnerConfigs := strings.Join(testRunnerConfigStrings, fmt.Sprintf("\\n%s%s", test_xml_indent, test_xml_indent))
	if len(extraTestRunnerConfigs) > 0 {
		extraTestRunnerConfigs += fmt.Sprintf("\\n%s%s", test_xml_indent, test_xml_indent)
	}
	extraTestRunnerConfigs = proptools.NinjaAndShellEscape(extraTestRunnerConfigs)

	ctx.Build(pctx, android.BuildParams{
		Rule:        autogenTestConfig,
		Description: "test config",
		Output:      output,
		Args: map[string]string{
			"name":                   name,
			"template":               template,
			"extraConfigs":           extraConfigs,
			"outputFileName":         outputFileName,
			"testInstallBase":        testInstallBase,
			"extraTestRunnerConfigs": extraTestRunnerConfigs,
		},
	})
}

// AutoGenTestConfigOptions is used so that we can supply many optional
// arguments to the AutoGenTestConfig function.
type AutoGenTestConfigOptions struct {
	Name                    string
	OutputFileName          string
	TestConfigProp          *string
	TestConfigTemplateProp  *string
	TestSuites              []string
	Config                  []Config
	OptionsForAutogenerated []Option
	TestRunnerOptions       []Option
	AutoGenConfig           *bool
	UnitTest                *bool
	TestInstallBase         string
	DeviceTemplate          string
	HostTemplate            string
	HostUnitTestTemplate    string
}

func AutoGenTestConfig(ctx android.ModuleContext, options AutoGenTestConfigOptions) android.Path {
	configs := append([]Config{}, options.Config...)
	for _, c := range options.OptionsForAutogenerated {
		configs = append(configs, c)
	}
	name := options.Name
	if name == "" {
		name = ctx.ModuleName()
	}
	path, autogenPath := testConfigPath(ctx, options.TestConfigProp, options.TestSuites, options.AutoGenConfig, options.TestConfigTemplateProp)
	if autogenPath != nil {
		templatePath := getTestConfigTemplate(ctx, options.TestConfigTemplateProp)
		if templatePath.Valid() {
			autogenTemplate(ctx, name, autogenPath, templatePath.String(), configs, options.TestRunnerOptions, options.OutputFileName, options.TestInstallBase)
		} else {
			if ctx.Device() {
				autogenTemplate(ctx, name, autogenPath, options.DeviceTemplate, configs, options.TestRunnerOptions, options.OutputFileName, options.TestInstallBase)
			} else {
				if Bool(options.UnitTest) {
					autogenTemplate(ctx, name, autogenPath, options.HostUnitTestTemplate, configs, options.TestRunnerOptions, options.OutputFileName, options.TestInstallBase)
				} else {
					autogenTemplate(ctx, name, autogenPath, options.HostTemplate, configs, options.TestRunnerOptions, options.OutputFileName, options.TestInstallBase)
				}
			}
		}
		return autogenPath
	}
	if len(options.OptionsForAutogenerated) > 0 {
		ctx.ModuleErrorf("Extra tradefed configurations were provided for an autogenerated xml file, but the autogenerated xml file was not used.")
	}
	return path
}

var autogenInstrumentationTest = pctx.StaticRule("autogenInstrumentationTest", blueprint.RuleParams{
	Command: "${AutoGenTestConfigScript} $out $in ${EmptyTestConfig} $template ${extraConfigs}",
	CommandDeps: []string{
		"${AutoGenTestConfigScript}",
		"${EmptyTestConfig}",
		"$template",
	},
}, "name", "template", "extraConfigs")

func AutoGenInstrumentationTestConfig(ctx android.ModuleContext, testConfigProp *string,
	testConfigTemplateProp *string, manifest android.Path, testSuites []string, autoGenConfig *bool, configs []Config) android.Path {
	path, autogenPath := testConfigPath(ctx, testConfigProp, testSuites, autoGenConfig, testConfigTemplateProp)
	var configStrings []string
	if autogenPath != nil {
		template := "${InstrumentationTestConfigTemplate}"
		moduleTemplate := getTestConfigTemplate(ctx, testConfigTemplateProp)
		if moduleTemplate.Valid() {
			template = moduleTemplate.String()
		}
		for _, config := range configs {
			configStrings = append(configStrings, config.Config())
		}
		extraConfigs := strings.Join(configStrings, fmt.Sprintf("\\n%s", test_xml_indent))
		extraConfigs = fmt.Sprintf("--extra-configs '%s'", extraConfigs)

		ctx.Build(pctx, android.BuildParams{
			Rule:        autogenInstrumentationTest,
			Description: "test config",
			Input:       manifest,
			Output:      autogenPath,
			Args: map[string]string{
				"name":         ctx.ModuleName(),
				"template":     template,
				"extraConfigs": extraConfigs,
			},
		})
		return autogenPath
	}
	return path
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
