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

package java

import (
	"reflect"
	"testing"

	"android/soong/android"
)

func TestCollectJavaLibraryPropertiesAddLibsDeps(t *testing.T) {
	expected := []string{"Foo", "Bar"}
	module := LibraryFactory().(*Library)
	module.properties.Libs = append(module.properties.Libs, expected...)
	dpInfo := &android.IdeInfo{}

	module.IDEInfo(dpInfo)

	if !reflect.DeepEqual(dpInfo.Deps, expected) {
		t.Errorf("Library.IDEInfo() Deps = %v, want %v", dpInfo.Deps, expected)
	}
}

func TestCollectJavaLibraryPropertiesAddStaticLibsDeps(t *testing.T) {
	expected := []string{"Foo", "Bar"}
	module := LibraryFactory().(*Library)
	module.properties.Static_libs = append(module.properties.Static_libs, expected...)
	dpInfo := &android.IdeInfo{}

	module.IDEInfo(dpInfo)

	if !reflect.DeepEqual(dpInfo.Deps, expected) {
		t.Errorf("Library.IDEInfo() Deps = %v, want %v", dpInfo.Deps, expected)
	}
}

func TestCollectJavaLibraryPropertiesAddScrs(t *testing.T) {
	expected := []string{"Foo", "Bar"}
	module := LibraryFactory().(*Library)
	module.expandIDEInfoCompiledSrcs = append(module.expandIDEInfoCompiledSrcs, expected...)
	dpInfo := &android.IdeInfo{}

	module.IDEInfo(dpInfo)

	if !reflect.DeepEqual(dpInfo.Srcs, expected) {
		t.Errorf("Library.IDEInfo() Srcs = %v, want %v", dpInfo.Srcs, expected)
	}
}

func TestCollectJavaLibraryPropertiesAddAidlIncludeDirs(t *testing.T) {
	expected := []string{"Foo", "Bar"}
	module := LibraryFactory().(*Library)
	module.deviceProperties.Aidl.Include_dirs = append(module.deviceProperties.Aidl.Include_dirs, expected...)
	dpInfo := &android.IdeInfo{}

	module.IDEInfo(dpInfo)

	if !reflect.DeepEqual(dpInfo.Aidl_include_dirs, expected) {
		t.Errorf("Library.IDEInfo() Aidl_include_dirs = %v, want %v", dpInfo.Aidl_include_dirs, expected)
	}
}

func TestCollectJavaLibraryPropertiesAddJarjarRules(t *testing.T) {
	expected := "Jarjar_rules.txt"
	module := LibraryFactory().(*Library)
	module.expandJarjarRules = android.PathForTesting(expected)
	dpInfo := &android.IdeInfo{}

	module.IDEInfo(dpInfo)

	if dpInfo.Jarjar_rules[0] != expected {
		t.Errorf("Library.IDEInfo() Jarjar_rules = %v, want %v", dpInfo.Jarjar_rules[0], expected)
	}
}
