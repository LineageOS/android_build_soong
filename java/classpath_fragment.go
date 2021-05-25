/*
 * Copyright (C) 2021 The Android Open Source Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package java

import (
	"fmt"
	"github.com/google/blueprint"
	"strings"

	"android/soong/android"
)

// Build rules and utilities to generate individual packages/modules/SdkExtensions/proto/classpaths.proto
// config files based on build configuration to embed into /system and /apex on a device.
//
// See `derive_classpath` service that reads the configs at runtime and defines *CLASSPATH variables
// on the device.

type classpathType int

const (
	// Matches definition in packages/modules/SdkExtensions/proto/classpaths.proto
	BOOTCLASSPATH classpathType = iota
	DEX2OATBOOTCLASSPATH
	SYSTEMSERVERCLASSPATH
)

func (c classpathType) String() string {
	return [...]string{"BOOTCLASSPATH", "DEX2OATBOOTCLASSPATH", "SYSTEMSERVERCLASSPATH"}[c]
}

type classpathFragmentProperties struct {
}

// classpathFragment interface is implemented by a module that contributes jars to a *CLASSPATH
// variables at runtime.
type classpathFragment interface {
	android.Module

	classpathFragmentBase() *ClasspathFragmentBase

	// ClasspathFragmentToConfiguredJarList returns android.ConfiguredJarList representation of all
	// the jars in this classpath fragment.
	ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList
}

// ClasspathFragmentBase is meant to be embedded in any module types that implement classpathFragment;
// such modules are expected to call initClasspathFragment().
type ClasspathFragmentBase struct {
	properties classpathFragmentProperties

	classpathType classpathType

	outputFilepath android.OutputPath
	installDirPath android.InstallPath
}

func (c *ClasspathFragmentBase) classpathFragmentBase() *ClasspathFragmentBase {
	return c
}

// Initializes ClasspathFragmentBase struct. Must be called by all modules that include ClasspathFragmentBase.
func initClasspathFragment(c classpathFragment, classpathType classpathType) {
	base := c.classpathFragmentBase()
	base.classpathType = classpathType
	c.AddProperties(&base.properties)
}

// Matches definition of Jar in packages/modules/SdkExtensions/proto/classpaths.proto
type classpathJar struct {
	path      string
	classpath classpathType
	// TODO(satayev): propagate min/max sdk versions for the jars
	minSdkVersion int32
	maxSdkVersion int32
}

// Converts android.ConfiguredJarList into a list of classpathJars for each given classpathType.
func configuredJarListToClasspathJars(ctx android.ModuleContext, configuredJars android.ConfiguredJarList, classpaths ...classpathType) []classpathJar {
	paths := configuredJars.DevicePaths(ctx.Config(), android.Android)
	jars := make([]classpathJar, 0, len(paths)*len(classpaths))
	for i := 0; i < len(paths); i++ {
		for _, classpathType := range classpaths {
			jars = append(jars, classpathJar{
				classpath: classpathType,
				path:      paths[i],
			})
		}
	}
	return jars
}

func (c *ClasspathFragmentBase) generateClasspathProtoBuildActions(ctx android.ModuleContext, jars []classpathJar) {
	outputFilename := strings.ToLower(c.classpathType.String()) + ".pb"
	c.outputFilepath = android.PathForModuleOut(ctx, outputFilename).OutputPath
	c.installDirPath = android.PathForModuleInstall(ctx, "etc", "classpaths")

	generatedJson := android.PathForModuleOut(ctx, outputFilename+".json")
	writeClasspathsJson(ctx, generatedJson, jars)

	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("conv_classpaths_proto").
		Flag("encode").
		Flag("--format=json").
		FlagWithInput("--input=", generatedJson).
		FlagWithOutput("--output=", c.outputFilepath)

	rule.Build("classpath_fragment", "Compiling "+c.outputFilepath.String())

	classpathProtoInfo := ClasspathFragmentProtoContentInfo{
		ClasspathFragmentProtoInstallDir: c.installDirPath,
		ClasspathFragmentProtoOutput:     c.outputFilepath,
	}
	ctx.SetProvider(ClasspathFragmentProtoContentInfoProvider, classpathProtoInfo)
}

func writeClasspathsJson(ctx android.ModuleContext, output android.WritablePath, jars []classpathJar) {
	var content strings.Builder
	fmt.Fprintf(&content, "{\n")
	fmt.Fprintf(&content, "\"jars\": [\n")
	for idx, jar := range jars {
		fmt.Fprintf(&content, "{\n")

		fmt.Fprintf(&content, "\"path\": \"%s\",\n", jar.path)
		fmt.Fprintf(&content, "\"classpath\": \"%s\"\n", jar.classpath)

		if idx < len(jars)-1 {
			fmt.Fprintf(&content, "},\n")
		} else {
			fmt.Fprintf(&content, "}\n")
		}
	}
	fmt.Fprintf(&content, "]\n")
	fmt.Fprintf(&content, "}\n")
	android.WriteFileRule(ctx, output, content.String())
}

// Returns AndroidMkEntries objects to install generated classpath.proto.
// Do not use this to install into APEXes as the injection of the generated files happen separately for APEXes.
func (c *ClasspathFragmentBase) androidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(c.outputFilepath),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", c.installDirPath.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", c.outputFilepath.Base())
			},
		},
	}}
}

var ClasspathFragmentProtoContentInfoProvider = blueprint.NewProvider(ClasspathFragmentProtoContentInfo{})

type ClasspathFragmentProtoContentInfo struct {
	// ClasspathFragmentProtoOutput is an output path for the generated classpaths.proto config of this module.
	//
	// The file should be copied to a relevant place on device, see ClasspathFragmentProtoInstallDir
	// for more details.
	ClasspathFragmentProtoOutput android.OutputPath

	// ClasspathFragmentProtoInstallDir contains information about on device location for the generated classpaths.proto file.
	//
	// The path encodes expected sub-location within partitions, i.e. etc/classpaths/<proto-file>,
	// for ClasspathFragmentProtoOutput. To get sub-location, instead of the full output / make path
	// use android.InstallPath#Rel().
	//
	// This is only relevant for APEX modules as they perform their own installation; while regular
	// system files are installed via ClasspathFragmentBase#androidMkEntries().
	ClasspathFragmentProtoInstallDir android.InstallPath
}
