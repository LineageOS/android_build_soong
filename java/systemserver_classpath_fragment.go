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
)

func init() {
	registerSystemserverClasspathBuildComponents(android.InitRegistrationContext)
}

func registerSystemserverClasspathBuildComponents(ctx android.RegistrationContext) {
	// TODO(satayev): add systemserver_classpath_fragment module
	ctx.RegisterModuleType("platform_systemserverclasspath", platformSystemServerClasspathFactory)
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

func (b *platformSystemServerClasspathModule) AndroidMkEntries() (entries []android.AndroidMkEntries) {
	return b.classpathFragmentBase().androidMkEntries()
}

func (b *platformSystemServerClasspathModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// TODO(satayev): split apex jars into separate configs.
	b.classpathFragmentBase().generateClasspathProtoBuildActions(ctx)
}
