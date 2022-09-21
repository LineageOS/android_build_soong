// Copyright (C) 2019 The Android Open Source Project
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

package cc

import (
	"android/soong/android"
	"android/soong/bazel"
)

// TODO(b/240463568): Additional properties will be added for API validation
type bazelSyspropLibraryAttributes struct {
	Srcs bazel.LabelListAttribute
}

type bazelCcSyspropLibraryAttributes struct {
	Dep             bazel.LabelAttribute
	Min_sdk_version *string
}

type SyspropLibraryLabels struct {
	SyspropLibraryLabel string
	SharedLibraryLabel  string
	StaticLibraryLabel  string
}

func Bp2buildSysprop(ctx android.Bp2buildMutatorContext, labels SyspropLibraryLabels, srcs bazel.LabelListAttribute, minSdkVersion *string) {
	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class:        "sysprop_library",
			Bzl_load_location: "//build/bazel/rules/sysprop:sysprop_library.bzl",
		},
		android.CommonAttributes{Name: labels.SyspropLibraryLabel},
		&bazelSyspropLibraryAttributes{
			Srcs: srcs,
		})

	attrs := &bazelCcSyspropLibraryAttributes{
		Dep:             *bazel.MakeLabelAttribute(":" + labels.SyspropLibraryLabel),
		Min_sdk_version: minSdkVersion,
	}

	if labels.SharedLibraryLabel != "" {
		ctx.CreateBazelTargetModule(
			bazel.BazelTargetModuleProperties{
				Rule_class:        "cc_sysprop_library_shared",
				Bzl_load_location: "//build/bazel/rules/cc:cc_sysprop_library.bzl",
			},
			android.CommonAttributes{Name: labels.SharedLibraryLabel},
			attrs)
	}

	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class:        "cc_sysprop_library_static",
			Bzl_load_location: "//build/bazel/rules/cc:cc_sysprop_library.bzl",
		},
		android.CommonAttributes{Name: labels.StaticLibraryLabel},
		attrs)
}
