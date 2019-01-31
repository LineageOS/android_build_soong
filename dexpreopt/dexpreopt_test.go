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

package dexpreopt

import (
	"reflect"
	"strings"
	"testing"
)

var testGlobalConfig = GlobalConfig{
	DefaultNoStripping:                 false,
	DisablePreoptModules:               nil,
	OnlyPreoptBootImageAndSystemServer: false,
	HasSystemOther:                     false,
	PatternsOnSystemOther:              nil,
	DisableGenerateProfile:             false,
	BootJars:                           nil,
	SystemServerJars:                   nil,
	SystemServerApps:                   nil,
	SpeedApps:                          nil,
	PreoptFlags:                        nil,
	DefaultCompilerFilter:              "",
	SystemServerCompilerFilter:         "",
	GenerateDMFiles:                    false,
	NoDebugInfo:                        false,
	AlwaysSystemServerDebugInfo:        false,
	NeverSystemServerDebugInfo:         false,
	AlwaysOtherDebugInfo:               false,
	NeverOtherDebugInfo:                false,
	MissingUsesLibraries:               nil,
	IsEng:                              false,
	SanitizeLite:                       false,
	DefaultAppImages:                   false,
	Dex2oatXmx:                         "",
	Dex2oatXms:                         "",
	EmptyDirectory:                     "",
	DefaultDexPreoptImageLocation:      nil,
	CpuVariant:                         nil,
	InstructionSetFeatures:             nil,
	Tools: Tools{
		Profman:             "profman",
		Dex2oat:             "dex2oat",
		Aapt:                "aapt",
		SoongZip:            "soong_zip",
		Zip2zip:             "zip2zip",
		VerifyUsesLibraries: "verify_uses_libraries.sh",
		ConstructContext:    "construct_context.sh",
	},
}

var testModuleConfig = ModuleConfig{
	Name:                   "",
	DexLocation:            "",
	BuildPath:              "",
	DexPath:                "",
	UseEmbeddedDex:         false,
	UncompressedDex:        false,
	HasApkLibraries:        false,
	PreoptFlags:            nil,
	ProfileClassListing:    "",
	ProfileIsTextListing:   false,
	EnforceUsesLibraries:   false,
	OptionalUsesLibraries:  nil,
	UsesLibraries:          nil,
	LibraryPaths:           nil,
	Archs:                  nil,
	DexPreoptImageLocation: "",
	PreoptExtractedApk:     false,
	NoCreateAppImage:       false,
	ForceCreateAppImage:    false,
	PresignedPrebuilt:      false,
	NoStripping:            false,
	StripInputPath:         "",
	StripOutputPath:        "",
}

func TestDexPreopt(t *testing.T) {
	global, module := testGlobalConfig, testModuleConfig

	module.Name = "test"
	module.DexLocation = "/system/app/test/test.apk"
	module.BuildPath = "out/test/test.apk"
	module.Archs = []string{"arm"}

	rule, err := GenerateDexpreoptRule(global, module)
	if err != nil {
		t.Error(err)
	}

	wantInstalls := []Install{
		{"out/test/oat/arm/package.odex", "/system/app/test/oat/arm/test.odex"},
		{"out/test/oat/arm/package.vdex", "/system/app/test/oat/arm/test.vdex"},
	}

	if !reflect.DeepEqual(rule.Installs(), wantInstalls) {
		t.Errorf("\nwant installs:\n   %v\ngot:\n   %v", wantInstalls, rule.Installs())
	}
}

func TestDexPreoptSystemOther(t *testing.T) {
	global, module := testGlobalConfig, testModuleConfig

	global.HasSystemOther = true
	global.PatternsOnSystemOther = []string{"app/%"}

	module.Name = "test"
	module.DexLocation = "/system/app/test/test.apk"
	module.BuildPath = "out/test/test.apk"
	module.Archs = []string{"arm"}

	rule, err := GenerateDexpreoptRule(global, module)
	if err != nil {
		t.Error(err)
	}

	wantInstalls := []Install{
		{"out/test/oat/arm/package.odex", "/system_other/app/test/oat/arm/test.odex"},
		{"out/test/oat/arm/package.vdex", "/system_other/app/test/oat/arm/test.vdex"},
	}

	if !reflect.DeepEqual(rule.Installs(), wantInstalls) {
		t.Errorf("\nwant installs:\n   %v\ngot:\n   %v", wantInstalls, rule.Installs())
	}
}

func TestDexPreoptProfile(t *testing.T) {
	global, module := testGlobalConfig, testModuleConfig

	module.Name = "test"
	module.DexLocation = "/system/app/test/test.apk"
	module.BuildPath = "out/test/test.apk"
	module.ProfileClassListing = "profile"
	module.Archs = []string{"arm"}

	rule, err := GenerateDexpreoptRule(global, module)
	if err != nil {
		t.Error(err)
	}

	wantInstalls := []Install{
		{"out/test/profile.prof", "/system/app/test/test.apk.prof"},
		{"out/test/oat/arm/package.art", "/system/app/test/oat/arm/test.art"},
		{"out/test/oat/arm/package.odex", "/system/app/test/oat/arm/test.odex"},
		{"out/test/oat/arm/package.vdex", "/system/app/test/oat/arm/test.vdex"},
	}

	if !reflect.DeepEqual(rule.Installs(), wantInstalls) {
		t.Errorf("\nwant installs:\n   %v\ngot:\n   %v", wantInstalls, rule.Installs())
	}
}

func TestStripDex(t *testing.T) {
	tests := []struct {
		name  string
		setup func(global *GlobalConfig, module *ModuleConfig)
		strip bool
	}{
		{
			name:  "default strip",
			setup: func(global *GlobalConfig, module *ModuleConfig) {},
			strip: true,
		},
		{
			name:  "global no stripping",
			setup: func(global *GlobalConfig, module *ModuleConfig) { global.DefaultNoStripping = true },
			strip: false,
		},
		{
			name:  "module no stripping",
			setup: func(global *GlobalConfig, module *ModuleConfig) { module.NoStripping = true },
			strip: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			global, module := testGlobalConfig, testModuleConfig

			module.Name = "test"
			module.DexLocation = "/system/app/test/test.apk"
			module.BuildPath = "out/test/test.apk"
			module.Archs = []string{"arm"}
			module.StripInputPath = "$1"
			module.StripOutputPath = "$2"

			test.setup(&global, &module)

			rule, err := GenerateStripRule(global, module)
			if err != nil {
				t.Error(err)
			}

			if test.strip {
				want := `zip2zip -i $1 -o $2 -x "classes*.dex"`
				if len(rule.Commands()) < 1 || !strings.Contains(rule.Commands()[0], want) {
					t.Errorf("\nwant commands[0] to have:\n   %v\ngot:\n   %v", want, rule.Commands()[0])
				}
			} else {
				wantCommands := []string{
					"cp -f $1 $2",
				}
				if !reflect.DeepEqual(rule.Commands(), wantCommands) {
					t.Errorf("\nwant commands:\n   %v\ngot:\n   %v", wantCommands, rule.Commands())
				}
			}
		})
	}
}
