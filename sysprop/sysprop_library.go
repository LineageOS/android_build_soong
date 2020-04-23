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
	"fmt"
	"io"
	"path"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
)

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

type syspropGenProperties struct {
	Srcs  []string `android:"path"`
	Scope string
	Name  *string
}

type syspropJavaGenRule struct {
	android.ModuleBase

	properties syspropGenProperties

	genSrcjars android.Paths
}

var _ android.OutputFileProducer = (*syspropJavaGenRule)(nil)

var (
	syspropJava = pctx.AndroidStaticRule("syspropJava",
		blueprint.RuleParams{
			Command: `rm -rf $out.tmp && mkdir -p $out.tmp && ` +
				`$syspropJavaCmd --scope $scope --java-output-dir $out.tmp $in && ` +
				`$soongZipCmd -jar -o $out -C $out.tmp -D $out.tmp && rm -rf $out.tmp`,
			CommandDeps: []string{
				"$syspropJavaCmd",
				"$soongZipCmd",
			},
		}, "scope")
)

func init() {
	pctx.HostBinToolVariable("soongZipCmd", "soong_zip")
	pctx.HostBinToolVariable("syspropJavaCmd", "sysprop_java")

	android.PreArchMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("sysprop_deps", syspropDepsMutator).Parallel()
	})
}

func (g *syspropJavaGenRule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var checkApiFileTimeStamp android.WritablePath

	ctx.VisitDirectDeps(func(dep android.Module) {
		if m, ok := dep.(*syspropLibrary); ok {
			checkApiFileTimeStamp = m.checkApiFileTimeStamp
		}
	})

	for _, syspropFile := range android.PathsForModuleSrc(ctx, g.properties.Srcs) {
		srcJarFile := android.GenPathWithExt(ctx, "sysprop", syspropFile, "srcjar")

		ctx.Build(pctx, android.BuildParams{
			Rule:        syspropJava,
			Description: "sysprop_java " + syspropFile.Rel(),
			Output:      srcJarFile,
			Input:       syspropFile,
			Implicit:    checkApiFileTimeStamp,
			Args: map[string]string{
				"scope": g.properties.Scope,
			},
		})

		g.genSrcjars = append(g.genSrcjars, srcJarFile)
	}
}

func (g *syspropJavaGenRule) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return g.genSrcjars, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func syspropJavaGenFactory() android.Module {
	g := &syspropJavaGenRule{}
	g.AddProperties(&g.properties)
	android.InitAndroidModule(g)
	return g
}

type syspropLibrary struct {
	android.ModuleBase
	android.ApexModuleBase

	properties syspropLibraryProperties

	checkApiFileTimeStamp android.WritablePath
	latestApiFile         android.Path
	currentApiFile        android.Path
	dumpedApiFile         android.WritablePath
}

type syspropLibraryProperties struct {
	// Determine who owns this sysprop library. Possible values are
	// "Platform", "Vendor", or "Odm"
	Property_owner string

	// list of package names that will be documented and publicized as API
	Api_packages []string

	// If set to true, allow this module to be dexed and installed on devices.
	Installable *bool

	// Make this module available when building for recovery
	Recovery_available *bool

	// Make this module available when building for vendor
	Vendor_available *bool

	// list of .sysprop files which defines the properties.
	Srcs []string `android:"path"`

	// If set to true, build a variant of the module for the host.  Defaults to false.
	Host_supported *bool

	// Whether public stub exists or not.
	Public_stub *bool `blueprint:"mutated"`
}

var (
	pctx         = android.NewPackageContext("android/soong/sysprop")
	syspropCcTag = dependencyTag{name: "syspropCc"}
)

func init() {
	android.RegisterModuleType("sysprop_library", syspropLibraryFactory)
}

func (m *syspropLibrary) Name() string {
	return m.BaseModuleName() + "_sysprop_library"
}

func (m *syspropLibrary) Owner() string {
	return m.properties.Property_owner
}

func (m *syspropLibrary) CcModuleName() string {
	return "lib" + m.BaseModuleName()
}

func (m *syspropLibrary) JavaPublicStubName() string {
	if proptools.Bool(m.properties.Public_stub) {
		return m.BaseModuleName() + "_public"
	}
	return ""
}

func (m *syspropLibrary) javaGenModuleName() string {
	return m.BaseModuleName() + "_java_gen"
}

func (m *syspropLibrary) javaGenPublicStubName() string {
	return m.BaseModuleName() + "_java_gen_public"
}

func (m *syspropLibrary) BaseModuleName() string {
	return m.ModuleBase.Name()
}

func (m *syspropLibrary) HasPublicStub() bool {
	return proptools.Bool(m.properties.Public_stub)
}

func (m *syspropLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	baseModuleName := m.BaseModuleName()

	for _, syspropFile := range android.PathsForModuleSrc(ctx, m.properties.Srcs) {
		if syspropFile.Ext() != ".sysprop" {
			ctx.PropertyErrorf("srcs", "srcs contains non-sysprop file %q", syspropFile.String())
		}
	}

	if ctx.Failed() {
		return
	}

	m.currentApiFile = android.PathForSource(ctx, ctx.ModuleDir(), "api", baseModuleName+"-current.txt")
	m.latestApiFile = android.PathForSource(ctx, ctx.ModuleDir(), "api", baseModuleName+"-latest.txt")

	// dump API rule
	rule := android.NewRuleBuilder()
	m.dumpedApiFile = android.PathForModuleOut(ctx, "api-dump.txt")
	rule.Command().
		BuiltTool(ctx, "sysprop_api_dump").
		Output(m.dumpedApiFile).
		Inputs(android.PathsForModuleSrc(ctx, m.properties.Srcs))
	rule.Build(pctx, ctx, baseModuleName+"_api_dump", baseModuleName+" api dump")

	// check API rule
	rule = android.NewRuleBuilder()

	// 1. current.txt <-> api_dump.txt
	msg := fmt.Sprintf(`\n******************************\n`+
		`API of sysprop_library %s doesn't match with current.txt\n`+
		`Please update current.txt by:\n`+
		`m %s-dump-api && rm -rf %q && cp -f %q %q\n`+
		`******************************\n`, baseModuleName, baseModuleName,
		m.currentApiFile.String(), m.dumpedApiFile.String(), m.currentApiFile.String())

	rule.Command().
		Text("( cmp").Flag("-s").
		Input(m.dumpedApiFile).
		Input(m.currentApiFile).
		Text("|| ( echo").Flag("-e").
		Flag(`"` + msg + `"`).
		Text("; exit 38) )")

	// 2. current.txt <-> latest.txt
	msg = fmt.Sprintf(`\n******************************\n`+
		`API of sysprop_library %s doesn't match with latest version\n`+
		`Please fix the breakage and rebuild.\n`+
		`******************************\n`, baseModuleName)

	rule.Command().
		Text("( ").
		BuiltTool(ctx, "sysprop_api_checker").
		Input(m.latestApiFile).
		Input(m.currentApiFile).
		Text(" || ( echo").Flag("-e").
		Flag(`"` + msg + `"`).
		Text("; exit 38) )")

	m.checkApiFileTimeStamp = android.PathForModuleOut(ctx, "check_api.timestamp")

	rule.Command().
		Text("touch").
		Output(m.checkApiFileTimeStamp)

	rule.Build(pctx, ctx, baseModuleName+"_check_api", baseModuleName+" check api")
}

func (m *syspropLibrary) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			// sysprop_library module itself is defined as a FAKE module to perform API check.
			// Actual implementation libraries are created on LoadHookMutator
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
			fmt.Fprintf(w, "LOCAL_MODULE := %s\n", m.Name())
			fmt.Fprintf(w, "LOCAL_MODULE_CLASS := FAKE\n")
			fmt.Fprintf(w, "LOCAL_MODULE_TAGS := optional\n")
			fmt.Fprintf(w, "include $(BUILD_SYSTEM)/base_rules.mk\n\n")
			fmt.Fprintf(w, "$(LOCAL_BUILT_MODULE): %s\n", m.checkApiFileTimeStamp.String())
			fmt.Fprintf(w, "\ttouch $@\n\n")
			fmt.Fprintf(w, ".PHONY: %s-check-api %s-dump-api\n\n", name, name)

			// dump API rule
			fmt.Fprintf(w, "%s-dump-api: %s\n\n", name, m.dumpedApiFile.String())

			// check API rule
			fmt.Fprintf(w, "%s-check-api: %s\n\n", name, m.checkApiFileTimeStamp.String())
		}}
}

// sysprop_library creates schematized APIs from sysprop description files (.sysprop).
// Both Java and C++ modules can link against sysprop_library, and API stability check
// against latest APIs (see build/soong/scripts/freeze-sysprop-api-files.sh)
// is performed.
func syspropLibraryFactory() android.Module {
	m := &syspropLibrary{}

	m.AddProperties(
		&m.properties,
	)
	android.InitAndroidModule(m)
	android.InitApexModule(m)
	android.AddLoadHook(m, func(ctx android.LoadHookContext) { syspropLibraryHook(ctx, m) })
	return m
}

type ccLibraryProperties struct {
	Name             *string
	Srcs             []string
	Soc_specific     *bool
	Device_specific  *bool
	Product_specific *bool
	Sysprop          struct {
		Platform *bool
	}
	Target struct {
		Android struct {
			Header_libs []string
			Shared_libs []string
		}
		Host struct {
			Static_libs []string
		}
	}
	Required           []string
	Recovery           *bool
	Recovery_available *bool
	Vendor_available   *bool
	Host_supported     *bool
	Apex_available     []string
}

type javaLibraryProperties struct {
	Name             *string
	Srcs             []string
	Soc_specific     *bool
	Device_specific  *bool
	Product_specific *bool
	Required         []string
	Sdk_version      *string
	Installable      *bool
	Libs             []string
	Stem             *string
}

func syspropLibraryHook(ctx android.LoadHookContext, m *syspropLibrary) {
	if len(m.properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "sysprop_library must specify srcs")
	}

	missing_api := false

	for _, txt := range []string{"-current.txt", "-latest.txt"} {
		path := path.Join(ctx.ModuleDir(), "api", m.BaseModuleName()+txt)
		file := android.ExistentPathForSource(ctx, path)
		if !file.Valid() {
			ctx.ModuleErrorf("API file %#v doesn't exist", path)
			missing_api = true
		}
	}

	if missing_api {
		script := "build/soong/scripts/gen-sysprop-api-files.sh"
		p := android.ExistentPathForSource(ctx, script)

		if !p.Valid() {
			panic(fmt.Sprintf("script file %s doesn't exist", script))
		}

		ctx.ModuleErrorf("One or more api files are missing. "+
			"You can create them by:\n"+
			"%s %q %q", script, ctx.ModuleDir(), m.BaseModuleName())
		return
	}

	// ctx's Platform or Specific functions represent where this sysprop_library installed.
	installedInSystem := ctx.Platform() || ctx.SystemExtSpecific()
	installedInVendorOrOdm := ctx.SocSpecific() || ctx.DeviceSpecific()
	isOwnerPlatform := false
	stub := "sysprop-library-stub-"

	switch m.Owner() {
	case "Platform":
		// Every partition can access platform-defined properties
		stub += "platform"
		isOwnerPlatform = true
	case "Vendor":
		// System can't access vendor's properties
		if installedInSystem {
			ctx.ModuleErrorf("None of soc_specific, device_specific, product_specific is true. " +
				"System can't access sysprop_library owned by Vendor")
		}
		stub += "vendor"
	case "Odm":
		// Only vendor can access Odm-defined properties
		if !installedInVendorOrOdm {
			ctx.ModuleErrorf("Neither soc_speicifc nor device_specific is true. " +
				"Odm-defined properties should be accessed only in Vendor or Odm")
		}
		stub += "vendor"
	default:
		ctx.PropertyErrorf("property_owner",
			"Unknown value %s: must be one of Platform, Vendor or Odm", m.Owner())
	}

	ccProps := ccLibraryProperties{}
	ccProps.Name = proptools.StringPtr(m.CcModuleName())
	ccProps.Srcs = m.properties.Srcs
	ccProps.Soc_specific = proptools.BoolPtr(ctx.SocSpecific())
	ccProps.Device_specific = proptools.BoolPtr(ctx.DeviceSpecific())
	ccProps.Product_specific = proptools.BoolPtr(ctx.ProductSpecific())
	ccProps.Sysprop.Platform = proptools.BoolPtr(isOwnerPlatform)
	ccProps.Target.Android.Header_libs = []string{"libbase_headers"}
	ccProps.Target.Android.Shared_libs = []string{"liblog"}
	ccProps.Target.Host.Static_libs = []string{"libbase", "liblog"}
	ccProps.Recovery_available = m.properties.Recovery_available
	ccProps.Vendor_available = m.properties.Vendor_available
	ccProps.Host_supported = m.properties.Host_supported
	ccProps.Apex_available = m.ApexProperties.Apex_available
	ctx.CreateModule(cc.LibraryFactory, &ccProps)

	scope := "internal"

	// We need to only use public version, if the partition where sysprop_library will be installed
	// is different from owner.

	if ctx.ProductSpecific() {
		// Currently product partition can't own any sysprop_library.
		scope = "public"
	} else if isOwnerPlatform && installedInVendorOrOdm {
		// Vendor or Odm should use public version of Platform's sysprop_library.
		scope = "public"
	}

	ctx.CreateModule(syspropJavaGenFactory, &syspropGenProperties{
		Srcs:  m.properties.Srcs,
		Scope: scope,
		Name:  proptools.StringPtr(m.javaGenModuleName()),
	})

	ctx.CreateModule(java.LibraryFactory, &javaLibraryProperties{
		Name:             proptools.StringPtr(m.BaseModuleName()),
		Srcs:             []string{":" + m.javaGenModuleName()},
		Soc_specific:     proptools.BoolPtr(ctx.SocSpecific()),
		Device_specific:  proptools.BoolPtr(ctx.DeviceSpecific()),
		Product_specific: proptools.BoolPtr(ctx.ProductSpecific()),
		Installable:      m.properties.Installable,
		Sdk_version:      proptools.StringPtr("core_current"),
		Libs:             []string{stub},
	})

	// if platform sysprop_library is installed in /system or /system-ext, we regard it as an API
	// and allow any modules (even from different partition) to link against the sysprop_library.
	// To do that, we create a public stub and expose it to modules with sdk_version: system_*.
	if isOwnerPlatform && installedInSystem {
		m.properties.Public_stub = proptools.BoolPtr(true)
		ctx.CreateModule(syspropJavaGenFactory, &syspropGenProperties{
			Srcs:  m.properties.Srcs,
			Scope: "public",
			Name:  proptools.StringPtr(m.javaGenPublicStubName()),
		})

		ctx.CreateModule(java.LibraryFactory, &javaLibraryProperties{
			Name:        proptools.StringPtr(m.JavaPublicStubName()),
			Srcs:        []string{":" + m.javaGenPublicStubName()},
			Installable: proptools.BoolPtr(false),
			Sdk_version: proptools.StringPtr("core_current"),
			Libs:        []string{stub},
			Stem:        proptools.StringPtr(m.BaseModuleName()),
		})
	}
}

func syspropDepsMutator(ctx android.BottomUpMutatorContext) {
	if m, ok := ctx.Module().(*syspropLibrary); ok {
		ctx.AddReverseDependency(m, nil, m.javaGenModuleName())

		if proptools.Bool(m.properties.Public_stub) {
			ctx.AddReverseDependency(m, nil, m.javaGenPublicStubName())
		}
	}
}
