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

package sysprop

import (
	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

type syspropLibrary struct {
	java.SdkLibrary

	commonProperties         commonProperties
	syspropLibraryProperties syspropLibraryProperties
}

type syspropLibraryProperties struct {
	// Determine who owns this sysprop library. Possible values are
	// "Platform", "Vendor", or "Odm"
	Property_owner string

	// list of package names that will be documented and publicized as API
	Api_packages []string
}

type commonProperties struct {
	Srcs               []string
	Recovery           *bool
	Recovery_available *bool
	Vendor_available   *bool
}

var (
	Bool         = proptools.Bool
	syspropCcTag = dependencyTag{name: "syspropCc"}
)

func init() {
	android.RegisterModuleType("sysprop_library", syspropLibraryFactory)
}

func (m *syspropLibrary) CcModuleName() string {
	return "lib" + m.Name()
}

func (m *syspropLibrary) SyspropJavaModule() *java.SdkLibrary {
	return &m.SdkLibrary
}

func syspropLibraryFactory() android.Module {
	m := &syspropLibrary{}

	m.AddProperties(
		&m.commonProperties,
		&m.syspropLibraryProperties,
	)
	m.InitSdkLibraryProperties()
	m.SetNoDist()
	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, "common")
	android.AddLoadHook(m, func(ctx android.LoadHookContext) { syspropLibraryHook(ctx, m) })

	return m
}

func syspropLibraryHook(ctx android.LoadHookContext, m *syspropLibrary) {
	if len(m.commonProperties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "sysprop_library must specify srcs")
	}

	if len(m.syspropLibraryProperties.Api_packages) == 0 {
		ctx.PropertyErrorf("api_packages", "sysprop_library must specify api_packages")
	}

	socSpecific := ctx.SocSpecific()
	deviceSpecific := ctx.DeviceSpecific()
	productSpecific := ctx.ProductSpecific()

	owner := m.syspropLibraryProperties.Property_owner

	switch owner {
	case "Platform":
		// Every partition can access platform-defined properties
		break
	case "Vendor":
		// System can't access vendor's properties
		if !socSpecific && !deviceSpecific && !productSpecific {
			ctx.ModuleErrorf("None of soc_specific, device_specific, product_specific is true. " +
				"System can't access sysprop_library owned by Vendor")
		}
	case "Odm":
		// Only vendor can access Odm-defined properties
		if !socSpecific && !deviceSpecific {
			ctx.ModuleErrorf("Neither soc_speicifc nor device_specific is true. " +
				"Odm-defined properties should be accessed only in Vendor or Odm")
		}
	default:
		ctx.PropertyErrorf("property_owner",
			"Unknown value %s: must be one of Platform, Vendor or Odm", owner)
	}

	ccProps := struct {
		Name             *string
		Soc_specific     *bool
		Device_specific  *bool
		Product_specific *bool
		Sysprop          struct {
			Platform *bool
		}
	}{}

	ccProps.Name = proptools.StringPtr(m.CcModuleName())
	ccProps.Soc_specific = proptools.BoolPtr(socSpecific)
	ccProps.Device_specific = proptools.BoolPtr(deviceSpecific)
	ccProps.Product_specific = proptools.BoolPtr(productSpecific)
	ccProps.Sysprop.Platform = proptools.BoolPtr(owner == "Platform")

	ctx.CreateModule(android.ModuleFactoryAdaptor(cc.LibraryFactory), &m.commonProperties, &ccProps)
}
