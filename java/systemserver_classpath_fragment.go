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
	configuredJars := configuredJarListToClasspathJars(ctx, p.ClasspathFragmentToConfiguredJarList(ctx), p.classpathType)
	p.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, configuredJars)
}

var platformSystemServerClasspathKey = android.NewOnceKey("platform_systemserverclasspath")

func (p *platformSystemServerClasspathModule) ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList {
	return ctx.Config().Once(platformSystemServerClasspathKey, func() interface{} {
		global := dexpreopt.GetGlobalConfig(ctx)

		jars := global.SystemServerJars

		// TODO(satayev): split apex jars into separate configs.
		for i := 0; i < global.UpdatableSystemServerJars.Len(); i++ {
			jars = jars.Append(global.UpdatableSystemServerJars.Apex(i), global.UpdatableSystemServerJars.Jar(i))
		}
		return jars
	}).(android.ConfiguredJarList)
}

type systemServerClasspathModule struct {
	android.ModuleBase

	ClasspathFragmentBase
}

func systemServerClasspathFactory() android.Module {
	m := &systemServerClasspathModule{}
	initClasspathFragment(m, SYSTEMSERVERCLASSPATH)
	android.InitAndroidArchModule(m, android.DeviceSupported, android.MultilibCommon)
	return m
}

func (s *systemServerClasspathModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	s.classpathFragmentBase().generateClasspathProtoBuildActions(ctx, configuredJarListToClasspathJars(ctx, s.ClasspathFragmentToConfiguredJarList(ctx)))
}

func (s *systemServerClasspathModule) ClasspathFragmentToConfiguredJarList(ctx android.ModuleContext) android.ConfiguredJarList {
	// TODO(satayev): populate with actual content
	return android.EmptyConfiguredJarList()
}
