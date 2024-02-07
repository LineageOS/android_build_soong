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
	"github.com/google/blueprint/proptools"
	"strings"

	"android/soong/android"
)

// Build rules and utilities to generate individual packages/modules/common/proto/classpaths.proto
// config files based on build configuration to embed into /system and /apex on a device.
//
// See `derive_classpath` service that reads the configs at runtime and defines *CLASSPATH variables
// on the device.

type classpathType int

const (
	// Matches definition in packages/modules/common/proto/classpaths.proto
	BOOTCLASSPATH classpathType = iota
	DEX2OATBOOTCLASSPATH
	SYSTEMSERVERCLASSPATH
	STANDALONE_SYSTEMSERVER_JARS
)

func (c classpathType) String() string {
	return [...]string{"BOOTCLASSPATH", "DEX2OATBOOTCLASSPATH", "SYSTEMSERVERCLASSPATH", "STANDALONE_SYSTEMSERVER_JARS"}[c]
}

type classpathFragmentProperties struct {
	// Whether to generated classpaths.proto config instance for the fragment. If the config is not
	// generated, then relevant boot jars are added to platform classpath, i.e. platform_bootclasspath
	// or platform_systemserverclasspath. This is useful for non-updatable APEX boot jars, to keep
	// them as part of dexopt on device. Defaults to true.
	Generate_classpaths_proto *bool
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
	path          string
	classpath     classpathType
	minSdkVersion string
	maxSdkVersion string
}

// gatherPossibleApexModuleNamesAndStems returns a set of module and stem names from the
// supplied contents that may be in the apex boot jars.
//
// The module names are included because sometimes the stem is set to just change the name of
// the installed file and it expects the configuration to still use the actual module name.
//
// The stem names are included because sometimes the stem is set to change the effective name of the
// module that is used in the configuration as well,e .g. when a test library is overriding an
// actual boot jar
func gatherPossibleApexModuleNamesAndStems(ctx android.ModuleContext, contents []string, tag blueprint.DependencyTag) []string {
	set := map[string]struct{}{}
	for _, name := range contents {
		dep, _ := ctx.GetDirectDepWithTag(name, tag).(android.Module)
		set[ModuleStemForDeapexing(dep)] = struct{}{}
		if m, ok := dep.(ModuleWithStem); ok {
			set[m.Stem()] = struct{}{}
		} else {
			ctx.PropertyErrorf("contents", "%v is not a ModuleWithStem", name)
		}
	}
	return android.SortedKeys(set)
}

// Converts android.ConfiguredJarList into a list of classpathJars for each given classpathType.
func configuredJarListToClasspathJars(ctx android.ModuleContext, configuredJars android.ConfiguredJarList, classpaths ...classpathType) []classpathJar {
	paths := configuredJars.DevicePaths(ctx.Config(), android.Android)
	jars := make([]classpathJar, 0, len(paths)*len(classpaths))
	for i := 0; i < len(paths); i++ {
		for _, classpathType := range classpaths {
			jar := classpathJar{
				classpath: classpathType,
				path:      paths[i],
			}
			ctx.VisitDirectDepsIf(func(m android.Module) bool {
				return m.Name() == configuredJars.Jar(i)
			}, func(m android.Module) {
				if s, ok := m.(*SdkLibrary); ok {
					// TODO(208456999): instead of mapping "current" to latest, min_sdk_version should never be set to "current"
					if s.minSdkVersion.Specified() {
						if s.minSdkVersion.IsCurrent() {
							jar.minSdkVersion = ctx.Config().DefaultAppTargetSdk(ctx).String()
						} else {
							jar.minSdkVersion = s.minSdkVersion.String()
						}
					}
					if s.maxSdkVersion.Specified() {
						if s.maxSdkVersion.IsCurrent() {
							jar.maxSdkVersion = ctx.Config().DefaultAppTargetSdk(ctx).String()
						} else {
							jar.maxSdkVersion = s.maxSdkVersion.String()
						}
					}
				}
			})
			jars = append(jars, jar)
		}
	}
	return jars
}

func (c *ClasspathFragmentBase) generateClasspathProtoBuildActions(ctx android.ModuleContext, configuredJars android.ConfiguredJarList, jars []classpathJar) {
	generateProto := proptools.BoolDefault(c.properties.Generate_classpaths_proto, true)
	if generateProto {
		outputFilename := strings.ToLower(c.classpathType.String()) + ".pb"
		c.outputFilepath = android.PathForModuleOut(ctx, outputFilename).OutputPath
		c.installDirPath = android.PathForModuleInstall(ctx, "etc", "classpaths")

		generatedTextproto := android.PathForModuleOut(ctx, outputFilename+".textproto")
		writeClasspathsTextproto(ctx, generatedTextproto, jars)

		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			BuiltTool("conv_classpaths_proto").
			Flag("encode").
			Flag("--format=textproto").
			FlagWithInput("--input=", generatedTextproto).
			FlagWithOutput("--output=", c.outputFilepath)

		rule.Build("classpath_fragment", "Compiling "+c.outputFilepath.String())
	}

	classpathProtoInfo := ClasspathFragmentProtoContentInfo{
		ClasspathFragmentProtoGenerated:  generateProto,
		ClasspathFragmentProtoContents:   configuredJars,
		ClasspathFragmentProtoInstallDir: c.installDirPath,
		ClasspathFragmentProtoOutput:     c.outputFilepath,
	}
	android.SetProvider(ctx, ClasspathFragmentProtoContentInfoProvider, classpathProtoInfo)
}

func writeClasspathsTextproto(ctx android.ModuleContext, output android.WritablePath, jars []classpathJar) {
	var content strings.Builder

	for _, jar := range jars {
		fmt.Fprintf(&content, "jars {\n")
		fmt.Fprintf(&content, "path: \"%s\"\n", jar.path)
		fmt.Fprintf(&content, "classpath: %s\n", jar.classpath)
		fmt.Fprintf(&content, "min_sdk_version: \"%s\"\n", jar.minSdkVersion)
		fmt.Fprintf(&content, "max_sdk_version: \"%s\"\n", jar.maxSdkVersion)
		fmt.Fprintf(&content, "}\n")
	}

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
				entries.SetString("LOCAL_MODULE_PATH", c.installDirPath.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", c.outputFilepath.Base())
			},
		},
	}}
}

var ClasspathFragmentProtoContentInfoProvider = blueprint.NewProvider[ClasspathFragmentProtoContentInfo]()

type ClasspathFragmentProtoContentInfo struct {
	// Whether the classpaths.proto config is generated for the fragment.
	ClasspathFragmentProtoGenerated bool

	// ClasspathFragmentProtoContents contains a list of jars that are part of this classpath fragment.
	ClasspathFragmentProtoContents android.ConfiguredJarList

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
