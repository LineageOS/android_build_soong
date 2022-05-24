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

package sdk

import (
	"fmt"
	"path/filepath"
	"testing"

	"android/soong/android"
	"android/soong/java"
	"github.com/google/blueprint"
)

type fakeMemberTrait struct {
	android.SdkMemberTraitBase
}

type fakeMemberType struct {
	android.SdkMemberTypeBase
}

func (t *fakeMemberType) AddDependencies(ctx android.SdkDependencyContext, dependencyTag blueprint.DependencyTag, names []string) {
	for _, name := range names {
		ctx.AddVariationDependencies(nil, dependencyTag, name)

		if ctx.RequiresTrait(name, extraTrait) {
			ctx.AddVariationDependencies(nil, dependencyTag, name+"_extra")
		}
		if ctx.RequiresTrait(name, specialTrait) {
			ctx.AddVariationDependencies(nil, dependencyTag, name+"_special")
		}
	}
}

func (t *fakeMemberType) IsInstance(module android.Module) bool {
	return true
}

func (t *fakeMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	moduleType := "java_import"
	if ctx.RequiresTrait(extraTrait) {
		moduleType = "java_test_import"
	}
	return ctx.SnapshotBuilder().AddPrebuiltModule(member, moduleType)
}

func (t *fakeMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &fakeMemberTypeProperties{}
}

type fakeMemberTypeProperties struct {
	android.SdkMemberPropertiesBase

	path android.Path
}

func (t *fakeMemberTypeProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	headerJars := variant.(java.ApexDependency).HeaderJars()
	if len(headerJars) != 1 {
		panic(fmt.Errorf("there must be only one header jar from %q", variant.Name()))
	}

	t.path = headerJars[0]
}

func (t *fakeMemberTypeProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	if t.path != nil {
		relative := filepath.Join("javalibs", t.path.Base())
		ctx.SnapshotBuilder().CopyToSnapshot(t.path, relative)
		propertySet.AddProperty("jars", []string{relative})
	}
}

var (
	extraTrait = &fakeMemberTrait{
		SdkMemberTraitBase: android.SdkMemberTraitBase{
			PropertyName: "extra",
		},
	}

	specialTrait = &fakeMemberTrait{
		SdkMemberTraitBase: android.SdkMemberTraitBase{
			PropertyName: "special",
		},
	}

	fakeType = &fakeMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "fake_members",
			SupportsSdk:  true,
			Traits: []android.SdkMemberTrait{
				extraTrait,
				specialTrait,
			},
		},
	}
)

func init() {
	android.RegisterSdkMemberTrait(extraTrait)
	android.RegisterSdkMemberTrait(specialTrait)
	android.RegisterSdkMemberType(fakeType)
}

func TestBasicTrait_WithoutTrait(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				fake_members: ["myjavalib"],
			}

			java_library {
				name: "myjavalib",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["javalibs/myjavalib.jar"],
}
`),
	)
}

func TestBasicTrait_MultipleTraits(t *testing.T) {
	result := android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				fake_members: ["myjavalib", "anotherjavalib"],
				traits: {
					extra: ["myjavalib"],
					special: ["myjavalib", "anotherjavalib"],
				},
			}

			java_library {
				name: "myjavalib",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}

			java_library {
				name: "myjavalib_extra",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}

			java_library {
				name: "myjavalib_special",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}

			java_library {
				name: "anotherjavalib",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}

			java_library {
				name: "anotherjavalib_special",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}
		`),
	).RunTest(t)

	CheckSnapshot(t, result, "mysdk", "",
		checkAndroidBpContents(`
// This is auto-generated. DO NOT EDIT.

java_test_import {
    name: "myjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["javalibs/myjavalib.jar"],
}

java_import {
    name: "myjavalib_extra",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["javalibs/myjavalib_extra.jar"],
}

java_import {
    name: "myjavalib_special",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["javalibs/myjavalib_special.jar"],
}

java_import {
    name: "anotherjavalib",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["javalibs/anotherjavalib.jar"],
}

java_import {
    name: "anotherjavalib_special",
    prefer: false,
    visibility: ["//visibility:public"],
    apex_available: ["//apex_available:platform"],
    jars: ["javalibs/anotherjavalib_special.jar"],
}
`),
	)
}

func TestTraitUnsupportedByMemberType(t *testing.T) {
	android.GroupFixturePreparers(
		prepareForSdkTestWithJava,
		android.FixtureWithRootAndroidBp(`
			sdk {
				name: "mysdk",
				java_header_libs: ["myjavalib"],
				traits: {
					extra: ["myjavalib"],
				},
			}

			java_library {
				name: "myjavalib",
				srcs: ["Test.java"],
				system_modules: "none",
				sdk_version: "none",
			}
		`),
	).ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
		`\Qsdk member "myjavalib" has traits [extra] that are unsupported by its member type "java_header_libs"\E`)).
		RunTest(t)
}
