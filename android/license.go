// Copyright 2020 Google Inc. All rights reserved.
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

package android

import (
	"android/soong/bazel"
	"fmt"
	"github.com/google/blueprint"
	"os"
)

type licenseKindDependencyTag struct {
	blueprint.BaseDependencyTag
}

var (
	licenseKindTag = licenseKindDependencyTag{}
)

func init() {
	RegisterLicenseBuildComponents(InitRegistrationContext)
}

// Register the license module type.
func RegisterLicenseBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("license", LicenseFactory)
}

type licenseProperties struct {
	// Specifies the kinds of license that apply.
	License_kinds []string
	// Specifies a short copyright notice to use for the license.
	Copyright_notice *string
	// Specifies the path or label for the text of the license.
	License_text []string `android:"path"`
	// Specifies the package name to which the license applies.
	Package_name *string
	// Specifies where this license can be used
	Visibility []string
}

var _ Bazelable = &licenseModule{}

type licenseModule struct {
	ModuleBase
	DefaultableModuleBase
	SdkBase
	BazelModuleBase

	properties licenseProperties
}

type bazelLicenseAttributes struct {
	License_kinds    []string
	Copyright_notice *string
	License_text     bazel.LabelAttribute
	Package_name     *string
	Visibility       []string
}

func (m *licenseModule) ConvertWithBp2build(ctx TopDownMutatorContext) {
	attrs := &bazelLicenseAttributes{
		License_kinds:    m.properties.License_kinds,
		Copyright_notice: m.properties.Copyright_notice,
		Package_name:     m.properties.Package_name,
		Visibility:       m.properties.Visibility,
	}

	// TODO(asmundak): Soong supports multiple license texts while Bazel's license
	// rule does not. Have android_license create a genrule to concatenate multiple
	// license texts.
	if len(m.properties.License_text) > 1 && ctx.Config().IsEnvTrue("BP2BUILD_VERBOSE") {
		fmt.Fprintf(os.Stderr, "warning: using only the first license_text item from //%s:%s\n",
			ctx.ModuleDir(), m.Name())
	}
	if len(m.properties.License_text) >= 1 {
		attrs.License_text.SetValue(BazelLabelForModuleSrcSingle(ctx, m.properties.License_text[0]))
	}

	ctx.CreateBazelTargetModule(
		bazel.BazelTargetModuleProperties{
			Rule_class:        "android_license",
			Bzl_load_location: "//build/bazel/rules/license:license.bzl",
		},
		CommonAttributes{
			Name: m.Name(),
		},
		attrs)
}

func (m *licenseModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, licenseKindTag, m.properties.License_kinds...)
}

func (m *licenseModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	// license modules have no licenses, but license_kinds must refer to license_kind modules
	mergeStringProps(&m.base().commonProperties.Effective_licenses, ctx.ModuleName())
	namePathProps(&m.base().commonProperties.Effective_license_text, m.properties.Package_name, PathsForModuleSrc(ctx, m.properties.License_text)...)
	for _, module := range ctx.GetDirectDepsWithTag(licenseKindTag) {
		if lk, ok := module.(*licenseKindModule); ok {
			mergeStringProps(&m.base().commonProperties.Effective_license_conditions, lk.properties.Conditions...)
			mergeStringProps(&m.base().commonProperties.Effective_license_kinds, ctx.OtherModuleName(module))
		} else {
			ctx.ModuleErrorf("license_kinds property %q is not a license_kind module", ctx.OtherModuleName(module))
		}
	}
}

func LicenseFactory() Module {
	module := &licenseModule{}

	base := module.base()
	module.AddProperties(&base.nameProperties, &module.properties, &base.commonProperties.BazelConversionStatus)

	// The visibility property needs to be checked and parsed by the visibility module.
	setPrimaryVisibilityProperty(module, "visibility", &module.properties.Visibility)

	InitSdkAwareModule(module)
	initAndroidModuleBase(module)
	InitDefaultableModule(module)
	InitBazelModule(module)

	return module
}
