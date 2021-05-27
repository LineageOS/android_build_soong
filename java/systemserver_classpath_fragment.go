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
	classpathJars := configuredJarListToClasspathJars(ctx, p.ClasspathFragmentToConfiguredJarList(ctx), p.classpathType)
	p.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, classpathJars)
}

func (p *platformSystemServerClasspathModule) ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList {
	global := dexpreopt.GetGlobalConfig(ctx)
	return global.SystemServerJars
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
	// The contents of this systemserverclasspath_fragment, could be either java_library, or java_sdk_library.
	//
	// The order of this list matters as it is the order that is used in the SYSTEMSERVERCLASSPATH.
	Contents []string
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
	if len(s.properties.Contents) == 0 {
		ctx.PropertyErrorf("contents", "empty contents are not allowed")
	}

	classpathJars := configuredJarListToClasspathJars(ctx, s.ClasspathFragmentToConfiguredJarList(ctx), s.classpathType)
	s.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, classpathJars)
}

func (s *SystemServerClasspathModule) ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList {
	global := dexpreopt.GetGlobalConfig(ctx)

	// Convert content names to their appropriate stems, in case a test library is overriding an actual boot jar
	var stems []string
	for _, name := range s.properties.Contents {
		dep := ctx.GetDirectDepWithTag(name, systemServerClasspathFragmentContentDepTag)
		if m, ok := dep.(ModuleWithStem); ok {
			stems = append(stems, m.Stem())
		} else {
			ctx.PropertyErrorf("contents", "%v is not a ModuleWithStem", name)
		}
	}

	// Only create configs for updatable boot jars. Non-updatable system server jars must be part of the
	// platform_systemserverclasspath's classpath proto config to guarantee that they come before any
	// updatable jars at runtime.
	return global.UpdatableSystemServerJars.Filter(stems)
}

type systemServerClasspathFragmentContentDependencyTag struct {
	blueprint.BaseDependencyTag
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

	for _, name := range s.properties.Contents {
		ctx.AddDependency(module, systemServerClasspathFragmentContentDepTag, name)
	}
}
