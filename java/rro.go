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

package java

// This file contains the module implementations for runtime_resource_overlay and
// override_runtime_resource_overlay.

import "android/soong/android"

func init() {
	RegisterRuntimeResourceOverlayBuildComponents(android.InitRegistrationContext)
}

func RegisterRuntimeResourceOverlayBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("runtime_resource_overlay", RuntimeResourceOverlayFactory)
	ctx.RegisterModuleType("override_runtime_resource_overlay", OverrideRuntimeResourceOverlayModuleFactory)
}

type RuntimeResourceOverlay struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.OverridableModuleBase
	aapt

	properties            RuntimeResourceOverlayProperties
	overridableProperties OverridableRuntimeResourceOverlayProperties

	certificate Certificate

	outputFile android.Path
	installDir android.InstallPath
}

type RuntimeResourceOverlayProperties struct {
	// the name of a certificate in the default certificate directory or an android_app_certificate
	// module name in the form ":module".
	Certificate *string

	// Name of the signing certificate lineage file.
	Lineage *string

	// For overriding the --rotation-min-sdk-version property of apksig
	RotationMinSdkVersion *string

	// optional theme name. If specified, the overlay package will be applied
	// only when the ro.boot.vendor.overlay.theme system property is set to the same value.
	Theme *string

	// If not blank, set to the version of the sdk to compile against. This
	// can be either an API version (e.g. "29" for API level 29 AKA Android 10)
	// or special subsets of the current platform, for example "none", "current",
	// "core", "system", "test". See build/soong/java/sdk.go for the full and
	// up-to-date list of possible values.
	// Defaults to compiling against the current platform.
	Sdk_version *string

	// if not blank, set the minimum version of the sdk that the compiled artifacts will run against.
	// Defaults to sdk_version if not set.
	Min_sdk_version *string

	// list of android_library modules whose resources are extracted and linked against statically
	Static_libs []string

	// list of android_app modules whose resources are extracted and linked against
	Resource_libs []string

	// Names of modules to be overridden. Listed modules can only be other overlays
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden overlays, but if both
	// overlays would be installed by default (in PRODUCT_PACKAGES) the other overlay will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

// RuntimeResourceOverlayModule interface is used by the apex package to gather information from
// a RuntimeResourceOverlay module.
type RuntimeResourceOverlayModule interface {
	android.Module
	OutputFile() android.Path
	Certificate() Certificate
	Theme() string
}

// RRO's partition logic is different from the partition logic of other modules defined in soong/android/paths.go
// The default partition for RRO is "/product" and not "/system"
func rroPartition(ctx android.ModuleContext) string {
	var partition string
	if ctx.DeviceSpecific() {
		partition = ctx.DeviceConfig().OdmPath()
	} else if ctx.SocSpecific() {
		partition = ctx.DeviceConfig().VendorPath()
	} else if ctx.SystemExtSpecific() {
		partition = ctx.DeviceConfig().SystemExtPath()
	} else {
		partition = ctx.DeviceConfig().ProductPath()
	}
	return partition
}

func (r *RuntimeResourceOverlay) DepsMutator(ctx android.BottomUpMutatorContext) {
	sdkDep := decodeSdkDep(ctx, android.SdkContext(r))
	if sdkDep.hasFrameworkLibs() {
		r.aapt.deps(ctx, sdkDep)
	}

	cert := android.SrcIsModule(String(r.properties.Certificate))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	ctx.AddVariationDependencies(nil, staticLibTag, r.properties.Static_libs...)
	ctx.AddVariationDependencies(nil, libTag, r.properties.Resource_libs...)
}

func (r *RuntimeResourceOverlay) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Compile and link resources
	r.aapt.hasNoCode = true
	// Do not remove resources without default values nor dedupe resource configurations with the same value
	aaptLinkFlags := []string{"--no-resource-deduping", "--no-resource-removal"}
	// Allow the override of "package name" and "overlay target package name"
	manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
	if overridden || r.overridableProperties.Package_name != nil {
		// The product override variable has a priority over the package_name property.
		if !overridden {
			manifestPackageName = *r.overridableProperties.Package_name
		}
		aaptLinkFlags = append(aaptLinkFlags, generateAaptRenamePackageFlags(manifestPackageName, false)...)
	}
	if r.overridableProperties.Target_package_name != nil {
		aaptLinkFlags = append(aaptLinkFlags,
			"--rename-overlay-target-package "+*r.overridableProperties.Target_package_name)
	}
	if r.overridableProperties.Category != nil {
		aaptLinkFlags = append(aaptLinkFlags,
			"--rename-overlay-category "+*r.overridableProperties.Category)
	}
	r.aapt.buildActions(ctx,
		aaptBuildActionOptions{
			sdkContext:                     r,
			enforceDefaultTargetSdkVersion: false,
			extraLinkFlags:                 aaptLinkFlags,
		},
	)

	// Sign the built package
	_, _, certificates := collectAppDeps(ctx, r, false, false)
	r.certificate, certificates = processMainCert(r.ModuleBase, String(r.properties.Certificate), certificates, ctx)
	signed := android.PathForModuleOut(ctx, "signed", r.Name()+".apk")
	var lineageFile android.Path
	if lineage := String(r.properties.Lineage); lineage != "" {
		lineageFile = android.PathForModuleSrc(ctx, lineage)
	}

	rotationMinSdkVersion := String(r.properties.RotationMinSdkVersion)

	SignAppPackage(ctx, signed, r.aapt.exportPackage, certificates, nil, lineageFile, rotationMinSdkVersion)

	r.outputFile = signed
	partition := rroPartition(ctx)
	r.installDir = android.PathForModuleInPartitionInstall(ctx, partition, "overlay", String(r.properties.Theme))
	ctx.InstallFile(r.installDir, r.outputFile.Base(), r.outputFile)
}

func (r *RuntimeResourceOverlay) SdkVersion(ctx android.EarlyModuleContext) android.SdkSpec {
	return android.SdkSpecFrom(ctx, String(r.properties.Sdk_version))
}

func (r *RuntimeResourceOverlay) SystemModules() string {
	return ""
}

func (r *RuntimeResourceOverlay) MinSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	if r.properties.Min_sdk_version != nil {
		return android.ApiLevelFrom(ctx, *r.properties.Min_sdk_version)
	}
	return r.SdkVersion(ctx).ApiLevel
}

func (r *RuntimeResourceOverlay) ReplaceMaxSdkVersionPlaceholder(ctx android.EarlyModuleContext) android.ApiLevel {
	return android.SdkSpecPrivate.ApiLevel
}

func (r *RuntimeResourceOverlay) TargetSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return r.SdkVersion(ctx).ApiLevel
}

func (r *RuntimeResourceOverlay) Certificate() Certificate {
	return r.certificate
}

func (r *RuntimeResourceOverlay) OutputFile() android.Path {
	return r.outputFile
}

func (r *RuntimeResourceOverlay) Theme() string {
	return String(r.properties.Theme)
}

// runtime_resource_overlay generates a resource-only apk file that can overlay application and
// system resources at run time.
func RuntimeResourceOverlayFactory() android.Module {
	module := &RuntimeResourceOverlay{}
	module.AddProperties(
		&module.properties,
		&module.aaptProperties,
		&module.overridableProperties)

	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitOverridableModule(module, &module.properties.Overrides)
	return module
}

// runtime_resource_overlay properties that can be overridden by override_runtime_resource_overlay
type OverridableRuntimeResourceOverlayProperties struct {
	// the package name of this app. The package name in the manifest file is used if one was not given.
	Package_name *string

	// the target package name of this overlay app. The target package name in the manifest file is used if one was not given.
	Target_package_name *string

	// the rro category of this overlay. The category in the manifest file is used if one was not given.
	Category *string
}

type OverrideRuntimeResourceOverlay struct {
	android.ModuleBase
	android.OverrideModuleBase
}

func (i *OverrideRuntimeResourceOverlay) GenerateAndroidBuildActions(_ android.ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_runtime_resource_overlay is used to create a module based on another
// runtime_resource_overlay module by overriding some of its properties.
func OverrideRuntimeResourceOverlayModuleFactory() android.Module {
	m := &OverrideRuntimeResourceOverlay{}
	m.AddProperties(&OverridableRuntimeResourceOverlayProperties{})

	android.InitAndroidMultiTargetsArchModule(m, android.DeviceSupported, android.MultilibCommon)
	android.InitOverrideModule(m)
	return m
}
