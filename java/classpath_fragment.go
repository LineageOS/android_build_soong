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
}

// ClasspathFragmentBase is meant to be embedded in any module types that implement classpathFragment;
// such modules are expected to call initClasspathFragment().
type ClasspathFragmentBase struct {
	properties classpathFragmentProperties

	outputFilepath android.OutputPath
	installDirPath android.InstallPath
}

func (c *ClasspathFragmentBase) classpathFragmentBase() *ClasspathFragmentBase {
	return c
}

// Initializes ClasspathFragmentBase struct. Must be called by all modules that include ClasspathFragmentBase.
func initClasspathFragment(c classpathFragment) {
	base := c.classpathFragmentBase()
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

func (c *ClasspathFragmentBase) generateClasspathProtoBuildActions(ctx android.ModuleContext) {
	outputFilename := ctx.ModuleName() + ".pb"
	c.outputFilepath = android.PathForModuleOut(ctx, outputFilename).OutputPath
	c.installDirPath = android.PathForModuleInstall(ctx, "etc", "classpaths")

	var jars []classpathJar
	jars = appendClasspathJar(jars, BOOTCLASSPATH, defaultBootclasspath(ctx)...)
	jars = appendClasspathJar(jars, DEX2OATBOOTCLASSPATH, defaultBootImageConfig(ctx).getAnyAndroidVariant().dexLocationsDeps...)
	jars = appendClasspathJar(jars, SYSTEMSERVERCLASSPATH, systemServerClasspath(ctx)...)

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
}

func writeClasspathsJson(ctx android.ModuleContext, output android.WritablePath, jars []classpathJar) {
	var content strings.Builder
	fmt.Fprintf(&content, "{\n")
	fmt.Fprintf(&content, "\"jars\": [\n")
	for idx, jar := range jars {
		fmt.Fprintf(&content, "{\n")

		fmt.Fprintf(&content, "\"relativePath\": \"%s\",\n", jar.path)
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

func appendClasspathJar(slice []classpathJar, classpathType classpathType, paths ...string) (result []classpathJar) {
	result = append(result, slice...)
	for _, path := range paths {
		result = append(result, classpathJar{
			path:      path,
			classpath: classpathType,
		})
	}
	return
}

func (c *ClasspathFragmentBase) androidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
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
