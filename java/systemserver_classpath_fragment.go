// Copyright 2021 Google Inc. All rights reserved.
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
	"android/soong/dexpreopt"

	"github.com/google/blueprint"
)

func init() {
	registerSystemserverClasspathBuildComponents(android.InitRegistrationContext)
}

func registerSystemserverClasspathBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("platform_systemserverclasspath", platformSystemServerClasspathFactory)
	ctx.RegisterModuleType("systemserverclasspath_fragment", systemServerClasspathFactory)
}

type platformSystemServerClasspathModule struct {
	android.ModuleBase

	ClasspathFragmentBase
}

func platformSystemServerClasspathFactory() android.Module {
	m := &platformSystemServerClasspathModule{}
	initClasspathFragment(m, SYSTEMSERVERCLASSPATH)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

func (p *platformSystemServerClasspathModule) AndroidMkEntries() (entries []android.AndroidMkEntries) {
	return p.classpathFragmentBase().androidMkEntries()
}

func (p *platformSystemServerClasspathModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	configuredJars := p.configuredJars(ctx)
	classpathJars := configuredJarListToClasspathJars(ctx, configuredJars, p.classpathType)
	p.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, configuredJars, classpathJars)
}

func (p *platformSystemServerClasspathModule) configuredJars(ctx android.ModuleContext) android.ConfiguredJarList {
	// TODO(satayev): include any apex jars that don't populate their classpath proto config.
	return dexpreopt.GetGlobalConfig(ctx).SystemServerJars
}

type SystemServerClasspathModule struct {
	android.ModuleBase
	android.ApexModuleBase

	ClasspathFragmentBase

	properties systemServerClasspathFragmentProperties
}

func (s *SystemServerClasspathModule) ShouldSupportSdkVersion(ctx android.BaseModuleContext, sdkVersion android.ApiLevel) error {
	return nil
}

type systemServerClasspathFragmentProperties struct {
	// List of system_server classpath jars, could be either java_library, or java_sdk_library.
	//
	// The order of this list matters as it is the order that is used in the SYSTEMSERVERCLASSPATH.
	Contents []string

	// List of jars that system_server loads dynamically using separate classloaders.
	//
	// The order does not matter.
	Standalone_contents []string
}

func systemServerClasspathFactory() android.Module {
	m := &SystemServerClasspathModule{}
	m.AddProperties(&m.properties)
	android.InitApexModule(m)
	initClasspathFragment(m, SYSTEMSERVERCLASSPATH)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

func (s *SystemServerClasspathModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(s.properties.Contents) == 0 && len(s.properties.Standalone_contents) == 0 {
		ctx.PropertyErrorf("contents", "Either contents or standalone_contents needs to be non-empty")
	}

	if s.ShouldIgnore() {
		return
	}

	configuredJars := s.configuredJars(ctx)
	classpathJars := configuredJarListToClasspathJars(ctx, configuredJars, s.classpathType)
	s.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, configuredJars, classpathJars)
}

func (s *SystemServerClasspathModule) configuredJars(ctx android.ModuleContext) android.ConfiguredJarList {
	global := dexpreopt.GetGlobalConfig(ctx)

	possibleUpdatableModules := gatherPossibleApexModuleNamesAndStems(ctx, s.properties.Contents, systemServerClasspathFragmentContentDepTag)
	return global.ApexSystemServerJars.Filter(possibleUpdatableModules)
}

type systemServerClasspathFragmentContentDependencyTag struct {
	blueprint.BaseDependencyTag
}

// The systemserverclasspath_fragment contents must never depend on prebuilts.
func (systemServerClasspathFragmentContentDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

// Contents of system server fragments in an apex are considered to be directly in the apex, as if
// they were listed in java_libs.
func (systemServerClasspathFragmentContentDependencyTag) CopyDirectlyInAnyApex() {}

var _ android.CopyDirectlyInAnyApexTag = systemServerClasspathFragmentContentDepTag

// The tag used for the dependency between the systemserverclasspath_fragment module and its contents.
var systemServerClasspathFragmentContentDepTag = systemServerClasspathFragmentContentDependencyTag{}

func IsSystemServerClasspathFragmentContentDepTag(tag blueprint.DependencyTag) bool {
	return tag == systemServerClasspathFragmentContentDepTag
}

func (s *SystemServerClasspathModule) ComponentDepsMutator(ctx android.BottomUpMutatorContext) {
	module := ctx.Module()
	var deps []string
	deps = append(deps, s.properties.Contents...)
	deps = append(deps, s.properties.Standalone_contents...)

	for _, name := range deps {
		ctx.AddDependency(module, systemServerClasspathFragmentContentDepTag, name)
	}
}

func (s *SystemServerClasspathModule) ShouldIgnore() bool {
	// Ignore this `systemserverclasspath_fragment` if it only contains `standalone_contents` because
	// it is for T and above.
	return len(s.properties.Contents) == 0
}
