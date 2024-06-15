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

// sysprop package defines a module named sysprop_library that can implement sysprop as API
// See https://source.android.com/devices/architecture/sysprops-apis for details
package sysprop

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
	"android/soong/rust"
)

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

type syspropGenProperties struct {
	Srcs      []string `android:"path"`
	Scope     string
	Name      *string
	Check_api *string
}

type syspropJavaGenRule struct {
	android.ModuleBase

	properties syspropGenProperties

	genSrcjars android.Paths
}

type syspropRustGenRule struct {
	android.ModuleBase

	properties syspropGenProperties

	genSrcs android.Paths
}

var _ android.OutputFileProducer = (*syspropJavaGenRule)(nil)
var _ android.OutputFileProducer = (*syspropRustGenRule)(nil)

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
	syspropRust = pctx.AndroidStaticRule("syspropRust",
		blueprint.RuleParams{
			Command: `rm -rf $out_dir && mkdir -p $out_dir && ` +
				`$syspropRustCmd --scope $scope --rust-output-dir $out_dir $in`,
			CommandDeps: []string{
				"$syspropRustCmd",
			},
		}, "scope", "out_dir")
)

func init() {
	pctx.HostBinToolVariable("soongZipCmd", "soong_zip")
	pctx.HostBinToolVariable("syspropJavaCmd", "sysprop_java")
	pctx.HostBinToolVariable("syspropRustCmd", "sysprop_rust")
}

// syspropJavaGenRule module generates srcjar containing generated java APIs.
// It also depends on check api rule, so api check has to pass to use sysprop_library.
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

func (g *syspropJavaGenRule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add a dependency from the stubs to sysprop library so that the generator rule can depend on
	// the check API rule of the sysprop library.
	ctx.AddFarVariationDependencies(nil, nil, proptools.String(g.properties.Check_api))
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

// syspropRustGenRule module generates rust source files containing generated rust APIs.
// It also depends on check api rule, so api check has to pass to use sysprop_library.
func (g *syspropRustGenRule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	var checkApiFileTimeStamp android.WritablePath

	ctx.VisitDirectDeps(func(dep android.Module) {
		if m, ok := dep.(*syspropLibrary); ok {
			checkApiFileTimeStamp = m.checkApiFileTimeStamp
		}
	})

	for _, syspropFile := range android.PathsForModuleSrc(ctx, g.properties.Srcs) {
		syspropDir := strings.TrimSuffix(syspropFile.String(), syspropFile.Ext())
		outputDir := android.PathForModuleGen(ctx, syspropDir, "src")
		libPath := android.PathForModuleGen(ctx, syspropDir, "src", "lib.rs")
		parsersPath := android.PathForModuleGen(ctx, syspropDir, "src", "gen_parsers_and_formatters.rs")

		ctx.Build(pctx, android.BuildParams{
			Rule:        syspropRust,
			Description: "sysprop_rust " + syspropFile.Rel(),
			Outputs:     android.WritablePaths{libPath, parsersPath},
			Input:       syspropFile,
			Implicit:    checkApiFileTimeStamp,
			Args: map[string]string{
				"scope":   g.properties.Scope,
				"out_dir": outputDir.String(),
			},
		})

		g.genSrcs = append(g.genSrcs, libPath, parsersPath)
	}
}

func (g *syspropRustGenRule) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add a dependency from the stubs to sysprop library so that the generator rule can depend on
	// the check API rule of the sysprop library.
	ctx.AddFarVariationDependencies(nil, nil, proptools.String(g.properties.Check_api))
}

func (g *syspropRustGenRule) OutputFiles(_ string) (android.Paths, error) {
	return g.genSrcs, nil
}

func syspropRustGenFactory() android.Module {
	g := &syspropRustGenRule{}
	g.AddProperties(&g.properties)
	android.InitAndroidModule(g)
	return g
}

type syspropLibrary struct {
	android.ModuleBase
	android.ApexModuleBase

	properties syspropLibraryProperties

	checkApiFileTimeStamp android.WritablePath
	latestApiFile         android.OptionalPath
	currentApiFile        android.OptionalPath
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

	// Make this module available when building for ramdisk
	Ramdisk_available *bool

	// Make this module available when building for recovery
	Recovery_available *bool

	// Make this module available when building for vendor
	Vendor_available *bool

	// Make this module available when building for product
	Product_available *bool

	// list of .sysprop files which defines the properties.
	Srcs []string `android:"path"`

	// If set to true, build a variant of the module for the host.  Defaults to false.
	Host_supported *bool

	Cpp struct {
		// Minimum sdk version that the artifact should support when it runs as part of mainline modules(APEX).
		// Forwarded to cc_library.min_sdk_version
		Min_sdk_version *string

		// C compiler flags used to build library
		Cflags []string

		// Linker flags used to build binary
		Ldflags []string
	}

	Java struct {
		// Minimum sdk version that the artifact should support when it runs as part of mainline modules(APEX).
		// Forwarded to java_library.min_sdk_version
		Min_sdk_version *string
	}

	Rust struct {
		// Minimum sdk version that the artifact should support when it runs as part of mainline modules(APEX).
		// Forwarded to rust_library.min_sdk_version
		Min_sdk_version *string
	}
}

var (
	pctx         = android.NewPackageContext("android/soong/sysprop")
	syspropCcTag = dependencyTag{name: "syspropCc"}

	syspropLibrariesKey  = android.NewOnceKey("syspropLibraries")
	syspropLibrariesLock sync.Mutex
)

// List of sysprop_library used by property_contexts to perform type check.
func syspropLibraries(config android.Config) *[]string {
	return config.Once(syspropLibrariesKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func SyspropLibraries(config android.Config) []string {
	return append([]string{}, *syspropLibraries(config)...)
}

func init() {
	registerSyspropBuildComponents(android.InitRegistrationContext)
}

func registerSyspropBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("sysprop_library", syspropLibraryFactory)
}

func (m *syspropLibrary) Name() string {
	return m.BaseModuleName() + "_sysprop_library"
}

func (m *syspropLibrary) Owner() string {
	return m.properties.Property_owner
}

func (m *syspropLibrary) CcImplementationModuleName() string {
	return "lib" + m.BaseModuleName()
}

func (m *syspropLibrary) javaPublicStubName() string {
	return m.BaseModuleName() + "_public"
}

func (m *syspropLibrary) javaGenModuleName() string {
	return m.BaseModuleName() + "_java_gen"
}

func (m *syspropLibrary) javaGenPublicStubName() string {
	return m.BaseModuleName() + "_java_gen_public"
}

func (m *syspropLibrary) rustGenModuleName() string {
	return m.rustCrateName() + "_rust_gen"
}

func (m *syspropLibrary) rustGenStubName() string {
	return "lib" + m.rustCrateName() + "_rust"
}

func (m *syspropLibrary) rustCrateName() string {
	moduleName := strings.ToLower(m.BaseModuleName())
	moduleName = strings.ReplaceAll(moduleName, "-", "_")
	moduleName = strings.ReplaceAll(moduleName, ".", "_")
	return moduleName
}

func (m *syspropLibrary) BaseModuleName() string {
	return m.ModuleBase.Name()
}

func (m *syspropLibrary) CurrentSyspropApiFile() android.OptionalPath {
	return m.currentApiFile
}

// GenerateAndroidBuildActions of sysprop_library handles API dump and API check.
// generated java_library will depend on these API files.
func (m *syspropLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	baseModuleName := m.BaseModuleName()
	srcs := android.PathsForModuleSrc(ctx, m.properties.Srcs)
	for _, syspropFile := range srcs {
		if syspropFile.Ext() != ".sysprop" {
			ctx.PropertyErrorf("srcs", "srcs contains non-sysprop file %q", syspropFile.String())
		}
	}
	android.SetProvider(ctx, blueprint.SrcsFileProviderKey, blueprint.SrcsFileProviderData{SrcPaths: srcs.Strings()})

	if ctx.Failed() {
		return
	}

	apiDirectoryPath := path.Join(ctx.ModuleDir(), "api")
	currentApiFilePath := path.Join(apiDirectoryPath, baseModuleName+"-current.txt")
	latestApiFilePath := path.Join(apiDirectoryPath, baseModuleName+"-latest.txt")
	m.currentApiFile = android.ExistentPathForSource(ctx, currentApiFilePath)
	m.latestApiFile = android.ExistentPathForSource(ctx, latestApiFilePath)

	// dump API rule
	rule := android.NewRuleBuilder(pctx, ctx)
	m.dumpedApiFile = android.PathForModuleOut(ctx, "api-dump.txt")
	rule.Command().
		BuiltTool("sysprop_api_dump").
		Output(m.dumpedApiFile).
		Inputs(srcs)
	rule.Build(baseModuleName+"_api_dump", baseModuleName+" api dump")

	// check API rule
	rule = android.NewRuleBuilder(pctx, ctx)

	// We allow that the API txt files don't exist, when the sysprop_library only contains internal
	// properties. But we have to feed current api file and latest api file to the rule builder.
	// Currently we can't get android.Path representing the null device, so we add any existing API
	// txt files to implicits, and then directly feed string paths, rather than calling Input(Path)
	// method.
	var apiFileList android.Paths
	currentApiArgument := os.DevNull
	if m.currentApiFile.Valid() {
		apiFileList = append(apiFileList, m.currentApiFile.Path())
		currentApiArgument = m.currentApiFile.String()
	}

	latestApiArgument := os.DevNull
	if m.latestApiFile.Valid() {
		apiFileList = append(apiFileList, m.latestApiFile.Path())
		latestApiArgument = m.latestApiFile.String()
	}

	// 1. compares current.txt to api-dump.txt
	// current.txt should be identical to api-dump.txt.
	msg := fmt.Sprintf(`\n******************************\n`+
		`API of sysprop_library %s doesn't match with current.txt\n`+
		`Please update current.txt by:\n`+
		`m %s-dump-api && mkdir -p %q && rm -rf %q && cp -f %q %q\n`+
		`******************************\n`, baseModuleName, baseModuleName,
		apiDirectoryPath, currentApiFilePath, m.dumpedApiFile.String(), currentApiFilePath)

	rule.Command().
		Text("( cmp").Flag("-s").
		Input(m.dumpedApiFile).
		Text(currentApiArgument).
		Text("|| ( echo").Flag("-e").
		Flag(`"` + msg + `"`).
		Text("; exit 38) )")

	// 2. compares current.txt to latest.txt (frozen API)
	// current.txt should be compatible with latest.txt
	msg = fmt.Sprintf(`\n******************************\n`+
		`API of sysprop_library %s doesn't match with latest version\n`+
		`Please fix the breakage and rebuild.\n`+
		`******************************\n`, baseModuleName)

	rule.Command().
		Text("( ").
		BuiltTool("sysprop_api_checker").
		Text(latestApiArgument).
		Text(currentApiArgument).
		Text(" || ( echo").Flag("-e").
		Flag(`"` + msg + `"`).
		Text("; exit 38) )").
		Implicits(apiFileList)

	m.checkApiFileTimeStamp = android.PathForModuleOut(ctx, "check_api.timestamp")

	rule.Command().
		Text("touch").
		Output(m.checkApiFileTimeStamp)

	rule.Build(baseModuleName+"_check_api", baseModuleName+" check api")
}

func (m *syspropLibrary) AndroidMk() android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			// sysprop_library module itself is defined as a FAKE module to perform API check.
			// Actual implementation libraries are created on LoadHookMutator
			fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)", " # sysprop.syspropLibrary")
			fmt.Fprintln(w, "LOCAL_MODULE :=", m.Name())
			fmt.Fprintf(w, "LOCAL_MODULE_CLASS := FAKE\n")
			fmt.Fprintf(w, "LOCAL_MODULE_TAGS := optional\n")
			// AconfigUpdateAndroidMkData may have added elements to Extra.  Process them here.
			for _, extra := range data.Extra {
				extra(w, nil)
			}
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

var _ android.ApexModule = (*syspropLibrary)(nil)

// Implements android.ApexModule
func (m *syspropLibrary) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	return fmt.Errorf("sysprop_library is not supposed to be part of apex modules")
}

// sysprop_library creates schematized APIs from sysprop description files (.sysprop).
// Both Java and C++ modules can link against sysprop_library, and API stability check
// against latest APIs (see build/soong/scripts/freeze-sysprop-api-files.sh)
// is performed. Note that the generated C++ module has its name prefixed with
// `lib`, and it is this module that should be depended on from other C++
// modules; i.e., if the sysprop_library module is named `foo`, C++ modules
// should depend on `libfoo`.
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
	Product_available  *bool
	Ramdisk_available  *bool
	Host_supported     *bool
	Apex_available     []string
	Min_sdk_version    *string
	Cflags             []string
	Ldflags            []string
}

type javaLibraryProperties struct {
	Name              *string
	Srcs              []string
	Soc_specific      *bool
	Device_specific   *bool
	Product_specific  *bool
	Required          []string
	Sdk_version       *string
	Installable       *bool
	Libs              []string
	Stem              *string
	SyspropPublicStub string
	Apex_available    []string
	Min_sdk_version   *string
}

type rustLibraryProperties struct {
	Name              *string
	Srcs              []string
	Installable       *bool
	Crate_name        string
	Rustlibs          []string
	Vendor_available  *bool
	Product_available *bool
	Apex_available    []string
	Min_sdk_version   *string
}

func syspropLibraryHook(ctx android.LoadHookContext, m *syspropLibrary) {
	if len(m.properties.Srcs) == 0 {
		ctx.PropertyErrorf("srcs", "sysprop_library must specify srcs")
	}

	// ctx's Platform or Specific functions represent where this sysprop_library installed.
	installedInSystem := ctx.Platform() || ctx.SystemExtSpecific()
	installedInVendorOrOdm := ctx.SocSpecific() || ctx.DeviceSpecific()
	installedInProduct := ctx.ProductSpecific()
	isOwnerPlatform := false
	var javaSyspropStub string

	// javaSyspropStub contains stub libraries used by generated APIs, instead of framework stub.
	// This is to make sysprop_library link against core_current.
	if installedInVendorOrOdm {
		javaSyspropStub = "sysprop-library-stub-vendor"
	} else if installedInProduct {
		javaSyspropStub = "sysprop-library-stub-product"
	} else {
		javaSyspropStub = "sysprop-library-stub-platform"
	}

	switch m.Owner() {
	case "Platform":
		// Every partition can access platform-defined properties
		isOwnerPlatform = true
	case "Vendor":
		// System can't access vendor's properties
		if installedInSystem {
			ctx.ModuleErrorf("None of soc_specific, device_specific, product_specific is true. " +
				"System can't access sysprop_library owned by Vendor")
		}
	case "Odm":
		// Only vendor can access Odm-defined properties
		if !installedInVendorOrOdm {
			ctx.ModuleErrorf("Neither soc_speicifc nor device_specific is true. " +
				"Odm-defined properties should be accessed only in Vendor or Odm")
		}
	default:
		ctx.PropertyErrorf("property_owner",
			"Unknown value %s: must be one of Platform, Vendor or Odm", m.Owner())
	}

	// Generate a C++ implementation library.
	// cc_library can receive *.sysprop files as their srcs, generating sources itself.
	ccProps := ccLibraryProperties{}
	ccProps.Name = proptools.StringPtr(m.CcImplementationModuleName())
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
	ccProps.Product_available = m.properties.Product_available
	ccProps.Ramdisk_available = m.properties.Ramdisk_available
	ccProps.Host_supported = m.properties.Host_supported
	ccProps.Apex_available = m.ApexProperties.Apex_available
	ccProps.Min_sdk_version = m.properties.Cpp.Min_sdk_version
	ccProps.Cflags = m.properties.Cpp.Cflags
	ccProps.Ldflags = m.properties.Cpp.Ldflags
	ctx.CreateModule(cc.LibraryFactory, &ccProps)

	scope := "internal"

	// We need to only use public version, if the partition where sysprop_library will be installed
	// is different from owner.
	if ctx.ProductSpecific() {
		// Currently product partition can't own any sysprop_library. So product always uses public.
		scope = "public"
	} else if isOwnerPlatform && installedInVendorOrOdm {
		// Vendor or Odm should use public version of Platform's sysprop_library.
		scope = "public"
	}

	// Generate a Java implementation library.
	// Contrast to C++, syspropJavaGenRule module will generate srcjar and the srcjar will be fed
	// to Java implementation library.
	ctx.CreateModule(syspropJavaGenFactory, &syspropGenProperties{
		Srcs:      m.properties.Srcs,
		Scope:     scope,
		Name:      proptools.StringPtr(m.javaGenModuleName()),
		Check_api: proptools.StringPtr(ctx.ModuleName()),
	})

	// if platform sysprop_library is installed in /system or /system-ext, we regard it as an API
	// and allow any modules (even from different partition) to link against the sysprop_library.
	// To do that, we create a public stub and expose it to modules with sdk_version: system_*.
	var publicStub string
	if isOwnerPlatform && installedInSystem {
		publicStub = m.javaPublicStubName()
	}

	ctx.CreateModule(java.LibraryFactory, &javaLibraryProperties{
		Name:              proptools.StringPtr(m.BaseModuleName()),
		Srcs:              []string{":" + m.javaGenModuleName()},
		Soc_specific:      proptools.BoolPtr(ctx.SocSpecific()),
		Device_specific:   proptools.BoolPtr(ctx.DeviceSpecific()),
		Product_specific:  proptools.BoolPtr(ctx.ProductSpecific()),
		Installable:       m.properties.Installable,
		Sdk_version:       proptools.StringPtr("core_current"),
		Libs:              []string{javaSyspropStub},
		SyspropPublicStub: publicStub,
		Apex_available:    m.ApexProperties.Apex_available,
		Min_sdk_version:   m.properties.Java.Min_sdk_version,
	})

	if publicStub != "" {
		ctx.CreateModule(syspropJavaGenFactory, &syspropGenProperties{
			Srcs:      m.properties.Srcs,
			Scope:     "public",
			Name:      proptools.StringPtr(m.javaGenPublicStubName()),
			Check_api: proptools.StringPtr(ctx.ModuleName()),
		})

		ctx.CreateModule(java.LibraryFactory, &javaLibraryProperties{
			Name:        proptools.StringPtr(publicStub),
			Srcs:        []string{":" + m.javaGenPublicStubName()},
			Installable: proptools.BoolPtr(false),
			Sdk_version: proptools.StringPtr("core_current"),
			Libs:        []string{javaSyspropStub},
			Stem:        proptools.StringPtr(m.BaseModuleName()),
		})
	}

	// Generate a Rust implementation library.
	ctx.CreateModule(syspropRustGenFactory, &syspropGenProperties{
		Srcs:      m.properties.Srcs,
		Scope:     scope,
		Name:      proptools.StringPtr(m.rustGenModuleName()),
		Check_api: proptools.StringPtr(ctx.ModuleName()),
	})
	rustProps := rustLibraryProperties{
		Name:        proptools.StringPtr(m.rustGenStubName()),
		Srcs:        []string{":" + m.rustGenModuleName()},
		Installable: proptools.BoolPtr(false),
		Crate_name:  m.rustCrateName(),
		Rustlibs: []string{
			"librustutils",
		},
		Vendor_available:  m.properties.Vendor_available,
		Product_available: m.properties.Product_available,
		Apex_available:    m.ApexProperties.Apex_available,
		Min_sdk_version:   proptools.StringPtr("29"),
	}
	ctx.CreateModule(rust.RustLibraryFactory, &rustProps)

	// syspropLibraries will be used by property_contexts to check types.
	// Record absolute paths of sysprop_library to prevent soong_namespace problem.
	if m.ExportedToMake() {
		syspropLibrariesLock.Lock()
		defer syspropLibrariesLock.Unlock()

		libraries := syspropLibraries(ctx.Config())
		*libraries = append(*libraries, "//"+ctx.ModuleDir()+":"+ctx.ModuleName())
	}
}
