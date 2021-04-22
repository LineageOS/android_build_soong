// Copyright (C) 2021 The Android Open Source Project
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

package java

import (
	"android/soong/android"
	"github.com/google/blueprint"
)

// Contains support for processing hiddenAPI in a modular fashion.

// hiddenAPIRelevantSdkKinds lists all the android.SdkKind instances that are needed by the hidden
// API processing.
var hiddenAPIRelevantSdkKinds = []android.SdkKind{
	android.SdkPublic,
	android.SdkSystem,
	android.SdkTest,
	android.SdkCorePlatform,
}

// HiddenAPIFlagFileProperties contains paths to the flag files that can be used to augment the
// information obtained from annotations within the source code in order to create the complete set
// of flags that should be applied to the dex implementation jars on the bootclasspath.
//
// Each property contains a list of paths. With the exception of the Unsupported_packages the paths
// of each property reference a plain text file that contains a java signature per line. The flags
// for each of those signatures will be updated in a property specific way.
//
// The Unsupported_packages property contains a list of paths, each of which is a plain text file
// with one Java package per line. All members of all classes within that package (but not nested
// packages) will be updated in a property specific way.
type HiddenAPIFlagFileProperties struct {
	// Marks each signature in the referenced files as being unsupported.
	Unsupported []string `android:"path"`

	// Marks each signature in the referenced files as being unsupported because it has been removed.
	// Any conflicts with other flags are ignored.
	Removed []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= R
	// and low priority.
	Max_target_r_low_priority []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= Q.
	Max_target_q []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= P.
	Max_target_p []string `android:"path"`

	// Marks each signature in the referenced files as being supported only for targetSdkVersion <= O
	// and low priority. Any conflicts with other flags are ignored.
	Max_target_o_low_priority []string `android:"path"`

	// Marks each signature in the referenced files as being blocked.
	Blocked []string `android:"path"`

	// Marks each signature in every package in the referenced files as being unsupported.
	Unsupported_packages []string `android:"path"`
}

func (p *HiddenAPIFlagFileProperties) hiddenAPIFlagFileInfo(ctx android.ModuleContext) hiddenAPIFlagFileInfo {
	info := hiddenAPIFlagFileInfo{categoryToPaths: map[*hiddenAPIFlagFileCategory]android.Paths{}}
	for _, category := range hiddenAPIFlagFileCategories {
		paths := android.PathsForModuleSrc(ctx, category.propertyValueReader(p))
		info.categoryToPaths[category] = paths
	}
	return info
}

type hiddenAPIFlagFileCategory struct {
	// propertyName is the name of the property for this category.
	propertyName string

	// propertyValueReader retrieves the value of the property for this category from the set of
	// properties.
	propertyValueReader func(properties *HiddenAPIFlagFileProperties) []string

	// commandMutator adds the appropriate command line options for this category to the supplied
	// command
	commandMutator func(command *android.RuleBuilderCommand, path android.Path)
}

var hiddenAPIFlagFileCategories = []*hiddenAPIFlagFileCategory{
	// See HiddenAPIFlagFileProperties.Unsupported
	{
		propertyName: "unsupported",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Unsupported
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--unsupported ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Removed
	{
		propertyName: "removed",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Removed
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--unsupported ", path).Flag("--ignore-conflicts ").FlagWithArg("--tag ", "removed")
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_r_low_priority
	{
		propertyName: "max_target_r_low_priority",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_r_low_priority
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-r ", path).FlagWithArg("--tag ", "lo-prio")
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_q
	{
		propertyName: "max_target_q",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_q
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-q ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_p
	{
		propertyName: "max_target_p",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_p
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-p ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Max_target_o_low_priority
	{
		propertyName: "max_target_o_low_priority",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Max_target_o_low_priority
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--max-target-o ", path).Flag("--ignore-conflicts ").FlagWithArg("--tag ", "lo-prio")
		},
	},
	// See HiddenAPIFlagFileProperties.Blocked
	{
		propertyName: "blocked",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Blocked
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--blocked ", path)
		},
	},
	// See HiddenAPIFlagFileProperties.Unsupported_packages
	{
		propertyName: "unsupported_packages",
		propertyValueReader: func(properties *HiddenAPIFlagFileProperties) []string {
			return properties.Unsupported_packages
		},
		commandMutator: func(command *android.RuleBuilderCommand, path android.Path) {
			command.FlagWithInput("--unsupported ", path).Flag("--packages ")
		},
	},
}

// hiddenAPIFlagFileInfo contains paths resolved from HiddenAPIFlagFileProperties
type hiddenAPIFlagFileInfo struct {
	// categoryToPaths maps from the flag file category to the paths containing information for that
	// category.
	categoryToPaths map[*hiddenAPIFlagFileCategory]android.Paths
}

func (i *hiddenAPIFlagFileInfo) append(other hiddenAPIFlagFileInfo) {
	for _, category := range hiddenAPIFlagFileCategories {
		i.categoryToPaths[category] = append(i.categoryToPaths[category], other.categoryToPaths[category]...)
	}
}

var hiddenAPIFlagFileInfoProvider = blueprint.NewProvider(hiddenAPIFlagFileInfo{})

// ruleToGenerateHiddenApiFlags creates a rule to create the monolithic hidden API flags from the
// flags from all the modules, the stub flags, augmented with some additional configuration files.
//
// baseFlagsPath is the path to the flags file containing all the information from the stubs plus
// an entry for every single member in the dex implementation jars of the individual modules. Every
// signature in any of the other files MUST be included in this file.
//
// moduleSpecificFlagsPaths are the paths to the flags files generated by each module using
// information from the baseFlagsPath as well as from annotations within the source.
//
// augmentationInfo is a struct containing paths to files that augment the information provided by
// the moduleSpecificFlagsPaths.
// ruleToGenerateHiddenApiFlags creates a rule to create the monolithic hidden API flags from the
// flags from all the modules, the stub flags, augmented with some additional configuration files.
//
// baseFlagsPath is the path to the flags file containing all the information from the stubs plus
// an entry for every single member in the dex implementation jars of the individual modules. Every
// signature in any of the other files MUST be included in this file.
//
// moduleSpecificFlagsPaths are the paths to the flags files generated by each module using
// information from the baseFlagsPath as well as from annotations within the source.
//
// augmentationInfo is a struct containing paths to files that augment the information provided by
// the moduleSpecificFlagsPaths.
func ruleToGenerateHiddenApiFlags(ctx android.BuilderContext, outputPath android.WritablePath, baseFlagsPath android.Path, moduleSpecificFlagsPaths android.Paths, augmentationInfo hiddenAPIFlagFileInfo) {
	tempPath := tempPathForRestat(ctx, outputPath)
	rule := android.NewRuleBuilder(pctx, ctx)
	command := rule.Command().
		BuiltTool("generate_hiddenapi_lists").
		FlagWithInput("--csv ", baseFlagsPath).
		Inputs(moduleSpecificFlagsPaths).
		FlagWithOutput("--output ", tempPath)

	// Add the options for the different categories of flag files.
	for _, category := range hiddenAPIFlagFileCategories {
		paths := augmentationInfo.categoryToPaths[category]
		for _, path := range paths {
			category.commandMutator(command, path)
		}
	}

	commitChangeForRestat(rule, tempPath, outputPath)

	rule.Build("hiddenAPIFlagsFile", "hiddenapi flags")
}
