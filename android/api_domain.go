// Copyright 2022 Google Inc. All rights reserved.
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
	"github.com/google/blueprint"

	"android/soong/bazel"
)

func init() {
	RegisterApiDomainBuildComponents(InitRegistrationContext)
}

func RegisterApiDomainBuildComponents(ctx RegistrationContext) {
	ctx.RegisterModuleType("api_domain", ApiDomainFactory)
}

type ApiSurface int

// TODO(b/246656800): Reconcile with android.SdkKind
const (
	PublicApi ApiSurface = iota
	SystemApi
	VendorApi
)

func (a ApiSurface) String() string {
	switch a {
	case PublicApi:
		return "publicapi"
	case SystemApi:
		return "systemapi"
	case VendorApi:
		return "vendorapi"
	default:
		return "invalid"
	}
}

type apiDomain struct {
	ModuleBase
	BazelModuleBase

	properties apiDomainProperties
}

type apiDomainProperties struct {
	// cc library contributions (.h files/.map.txt) of this API domain
	// This dependency is a no-op in Soong, but the corresponding Bazel target in the bp2build workspace will provide a `CcApiContributionInfo` provider
	Cc_api_contributions []string
}

func ApiDomainFactory() Module {
	m := &apiDomain{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, DeviceSupported, MultilibBoth)
	return m
}

// Do not create any dependency edges in Soong for now to skip visibility checks for some systemapi libraries.
// Currently, all api_domain modules reside in build/orchestrator/apis/Android.bp
// However, cc libraries like libsigchain (com.android.art) restrict their visibility to art/*
// When the api_domain module types are collocated with their contributions, this dependency edge can be restored
func (a *apiDomain) DepsMutator(ctx BottomUpMutatorContext) {
}

// API domain does not have any builld actions yet
func (a *apiDomain) GenerateAndroidBuildActions(ctx ModuleContext) {
}

const (
	apiContributionSuffix = ".contribution"
)

// ApiContributionTargetName returns the name of the bp2build target (e.g. cc_api_contribution)  of contribution modules (e.g. ndk_library)
// A suffix is necessary to prevent a name collision with the base target in the same bp2build bazel package
func ApiContributionTargetName(moduleName string) string {
	return moduleName + apiContributionSuffix
}

// For each contributing cc_library, format the name to its corresponding contribution bazel target in the bp2build workspace
func contributionBazelAttributes(ctx TopDownMutatorContext, contributions []string) bazel.LabelListAttribute {
	addSuffix := func(ctx BazelConversionPathContext, module blueprint.Module) string {
		baseLabel := BazelModuleLabel(ctx, module)
		return ApiContributionTargetName(baseLabel)
	}
	bazelLabels := BazelLabelForModuleDepsWithFn(ctx, contributions, addSuffix)
	return bazel.MakeLabelListAttribute(bazelLabels)
}

type bazelApiDomainAttributes struct {
	Cc_api_contributions bazel.LabelListAttribute
}

var _ ApiProvider = (*apiDomain)(nil)

func (a *apiDomain) ConvertWithApiBp2build(ctx TopDownMutatorContext) {
	props := bazel.BazelTargetModuleProperties{
		Rule_class:        "api_domain",
		Bzl_load_location: "//build/bazel/rules/apis:api_domain.bzl",
	}
	attrs := &bazelApiDomainAttributes{
		Cc_api_contributions: contributionBazelAttributes(ctx, a.properties.Cc_api_contributions),
	}
	ctx.CreateBazelTargetModule(props, CommonAttributes{
		Name: ctx.ModuleName(),
	}, attrs)
}
