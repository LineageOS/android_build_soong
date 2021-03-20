// Copyright 2015 Google Inc. All rights reserved.
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

// This file contains the module types for compiling Java for Android, and converts the properties
// into the flags and filenames necessary to pass to the Module.  The final creation of the rules
// is handled in builder.go

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/dexpreopt"
	"android/soong/java/config"
	"android/soong/tradefed"
)

func init() {
	RegisterJavaBuildComponents(android.InitRegistrationContext)

	RegisterJavaSdkMemberTypes()
}

func RegisterJavaBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_defaults", DefaultsFactory)

	ctx.RegisterModuleType("java_library", LibraryFactory)
	ctx.RegisterModuleType("java_library_static", LibraryStaticFactory)
	ctx.RegisterModuleType("java_library_host", LibraryHostFactory)
	ctx.RegisterModuleType("java_binary", BinaryFactory)
	ctx.RegisterModuleType("java_binary_host", BinaryHostFactory)
	ctx.RegisterModuleType("java_test", TestFactory)
	ctx.RegisterModuleType("java_test_helper_library", TestHelperLibraryFactory)
	ctx.RegisterModuleType("java_test_host", TestHostFactory)
	ctx.RegisterModuleType("java_test_import", JavaTestImportFactory)
	ctx.RegisterModuleType("java_import", ImportFactory)
	ctx.RegisterModuleType("java_import_host", ImportFactoryHost)
	ctx.RegisterModuleType("java_device_for_host", DeviceForHostFactory)
	ctx.RegisterModuleType("java_host_for_device", HostForDeviceFactory)
	ctx.RegisterModuleType("dex_import", DexImportFactory)

	ctx.FinalDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.BottomUp("dexpreopt_tool_deps", dexpreoptToolDepsMutator).Parallel()
	})

	ctx.RegisterSingletonType("logtags", LogtagsSingleton)
	ctx.RegisterSingletonType("kythe_java_extract", kytheExtractJavaFactory)
}

func RegisterJavaSdkMemberTypes() {
	// Register sdk member types.
	android.RegisterSdkMemberType(javaHeaderLibsSdkMemberType)

	// Export implementation classes jar as part of the sdk.
	exportImplementationClassesJar := func(_ android.SdkMemberContext, j *Library) android.Path {
		implementationJars := j.ImplementationAndResourcesJars()
		if len(implementationJars) != 1 {
			panic(fmt.Errorf("there must be only one implementation jar from %q", j.Name()))
		}
		return implementationJars[0]
	}

	// Register java implementation libraries for use only in module_exports (not sdk).
	android.RegisterSdkMemberType(&librarySdkMemberType{
		android.SdkMemberTypeBase{
			PropertyName: "java_libs",
		},
		exportImplementationClassesJar,
		sdkSnapshotFilePathForJar,
		copyEverythingToSnapshot,
	})

	// Register java boot libraries for use in sdk.
	//
	// The build has some implicit dependencies (via the boot jars configuration) on a number of
	// modules, e.g. core-oj, apache-xml, that are part of the java boot class path and which are
	// provided by mainline modules (e.g. art, conscrypt, runtime-i18n) but which are not otherwise
	// used outside those mainline modules.
	//
	// As they are not needed outside the mainline modules adding them to the sdk/module-exports as
	// either java_libs, or java_header_libs would end up exporting more information than was strictly
	// necessary. The java_boot_libs property to allow those modules to be exported as part of the
	// sdk/module_exports without exposing any unnecessary information.
	android.RegisterSdkMemberType(&librarySdkMemberType{
		android.SdkMemberTypeBase{
			PropertyName: "java_boot_libs",
			SupportsSdk:  true,
		},
		// Temporarily export implementation classes jar for java_boot_libs as it is required for the
		// hiddenapi processing.
		// TODO(b/179354495): Revert once hiddenapi processing has been modularized.
		exportImplementationClassesJar,
		sdkSnapshotFilePathForJar,
		onlyCopyJarToSnapshot,
	})

	// Register java test libraries for use only in module_exports (not sdk).
	android.RegisterSdkMemberType(&testSdkMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "java_tests",
		},
	})

}

// JavaInfo contains information about a java module for use by modules that depend on it.
type JavaInfo struct {
	// HeaderJars is a list of jars that can be passed as the javac classpath in order to link
	// against this module.  If empty, ImplementationJars should be used instead.
	HeaderJars android.Paths

	// ImplementationAndResourceJars is a list of jars that contain the implementations of classes
	// in the module as well as any resources included in the module.
	ImplementationAndResourcesJars android.Paths

	// ImplementationJars is a list of jars that contain the implementations of classes in the
	//module.
	ImplementationJars android.Paths

	// ResourceJars is a list of jars that contain the resources included in the module.
	ResourceJars android.Paths

	// AidlIncludeDirs is a list of directories that should be passed to the aidl tool when
	// depending on this module.
	AidlIncludeDirs android.Paths

	// SrcJarArgs is a list of arguments to pass to soong_zip to package the sources of this
	// module.
	SrcJarArgs []string

	// SrcJarDeps is a list of paths to depend on when packaging the sources of this module.
	SrcJarDeps android.Paths

	// ExportedPlugins is a list of paths that should be used as annotation processors for any
	// module that depends on this module.
	ExportedPlugins android.Paths

	// ExportedPluginClasses is a list of classes that should be run as annotation processors for
	// any module that depends on this module.
	ExportedPluginClasses []string

	// ExportedPluginDisableTurbine is true if this module's annotation processors generate APIs,
	// requiring disbling turbine for any modules that depend on it.
	ExportedPluginDisableTurbine bool

	// JacocoReportClassesFile is the path to a jar containing uninstrumented classes that will be
	// instrumented by jacoco.
	JacocoReportClassesFile android.Path
}

var JavaInfoProvider = blueprint.NewProvider(JavaInfo{})

// SyspropPublicStubInfo contains info about the sysprop public stub library that corresponds to
// the sysprop implementation library.
type SyspropPublicStubInfo struct {
	// JavaInfo is the JavaInfoProvider of the sysprop public stub library that corresponds to
	// the sysprop implementation library.
	JavaInfo JavaInfo
}

var SyspropPublicStubInfoProvider = blueprint.NewProvider(SyspropPublicStubInfo{})

// Methods that need to be implemented for a module that is added to apex java_libs property.
type ApexDependency interface {
	HeaderJars() android.Paths
	ImplementationAndResourcesJars() android.Paths
}

// Provides build path and install path to DEX jars.
type UsesLibraryDependency interface {
	DexJarBuildPath() android.Path
	DexJarInstallPath() android.Path
	ClassLoaderContexts() dexpreopt.ClassLoaderContextMap
}

// TODO(jungjw): Move this to kythe.go once it's created.
type xref interface {
	XrefJavaFiles() android.Paths
}

func (j *Module) XrefJavaFiles() android.Paths {
	return j.kytheFiles
}

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

// installDependencyTag is a dependency tag that is annotated to cause the installed files of the
// dependency to be installed when the parent module is installed.
type installDependencyTag struct {
	blueprint.BaseDependencyTag
	android.InstallAlwaysNeededDependencyTag
	name string
}

type usesLibraryDependencyTag struct {
	dependencyTag
	sdkVersion int // SDK version in which the library appared as a standalone library.
}

func makeUsesLibraryDependencyTag(sdkVersion int) usesLibraryDependencyTag {
	return usesLibraryDependencyTag{
		dependencyTag: dependencyTag{name: fmt.Sprintf("uses-library-%d", sdkVersion)},
		sdkVersion:    sdkVersion,
	}
}

func IsJniDepTag(depTag blueprint.DependencyTag) bool {
	return depTag == jniLibTag
}

var (
	dataNativeBinsTag       = dependencyTag{name: "dataNativeBins"}
	staticLibTag            = dependencyTag{name: "staticlib"}
	libTag                  = dependencyTag{name: "javalib"}
	java9LibTag             = dependencyTag{name: "java9lib"}
	pluginTag               = dependencyTag{name: "plugin"}
	errorpronePluginTag     = dependencyTag{name: "errorprone-plugin"}
	exportedPluginTag       = dependencyTag{name: "exported-plugin"}
	bootClasspathTag        = dependencyTag{name: "bootclasspath"}
	systemModulesTag        = dependencyTag{name: "system modules"}
	frameworkResTag         = dependencyTag{name: "framework-res"}
	kotlinStdlibTag         = dependencyTag{name: "kotlin-stdlib"}
	kotlinAnnotationsTag    = dependencyTag{name: "kotlin-annotations"}
	proguardRaiseTag        = dependencyTag{name: "proguard-raise"}
	certificateTag          = dependencyTag{name: "certificate"}
	instrumentationForTag   = dependencyTag{name: "instrumentation_for"}
	extraLintCheckTag       = dependencyTag{name: "extra-lint-check"}
	jniLibTag               = dependencyTag{name: "jnilib"}
	syspropPublicStubDepTag = dependencyTag{name: "sysprop public stub"}
	jniInstallTag           = installDependencyTag{name: "jni install"}
	binaryInstallTag        = installDependencyTag{name: "binary install"}
	usesLibTag              = makeUsesLibraryDependencyTag(dexpreopt.AnySdkVersion)
	usesLibCompat28Tag      = makeUsesLibraryDependencyTag(28)
	usesLibCompat29Tag      = makeUsesLibraryDependencyTag(29)
	usesLibCompat30Tag      = makeUsesLibraryDependencyTag(30)
)

func IsLibDepTag(depTag blueprint.DependencyTag) bool {
	return depTag == libTag
}

func IsStaticLibDepTag(depTag blueprint.DependencyTag) bool {
	return depTag == staticLibTag
}

type sdkDep struct {
	useModule, useFiles, invalidVersion bool

	// The modules that will be added to the bootclasspath when targeting 1.8 or lower
	bootclasspath []string

	// The default system modules to use. Will be an empty string if no system
	// modules are to be used.
	systemModules string

	// The modules that will be added to the classpath regardless of the Java language level targeted
	classpath []string

	// The modules that will be added ot the classpath when targeting 1.9 or higher
	// (normally these will be on the bootclasspath when targeting 1.8 or lower)
	java9Classpath []string

	frameworkResModule string

	jars android.Paths
	aidl android.OptionalPath

	noStandardLibs, noFrameworksLibs bool
}

func (s sdkDep) hasStandardLibs() bool {
	return !s.noStandardLibs
}

func (s sdkDep) hasFrameworkLibs() bool {
	return !s.noStandardLibs && !s.noFrameworksLibs
}

type jniLib struct {
	name           string
	path           android.Path
	target         android.Target
	coverageFile   android.OptionalPath
	unstrippedFile android.Path
}

func sdkDeps(ctx android.BottomUpMutatorContext, sdkContext sdkContext, d dexer) {
	sdkDep := decodeSdkDep(ctx, sdkContext)
	if sdkDep.useModule {
		ctx.AddVariationDependencies(nil, bootClasspathTag, sdkDep.bootclasspath...)
		ctx.AddVariationDependencies(nil, java9LibTag, sdkDep.java9Classpath...)
		ctx.AddVariationDependencies(nil, libTag, sdkDep.classpath...)
		if d.effectiveOptimizeEnabled() && sdkDep.hasStandardLibs() {
			ctx.AddVariationDependencies(nil, proguardRaiseTag, config.LegacyCorePlatformBootclasspathLibraries...)
		}
		if d.effectiveOptimizeEnabled() && sdkDep.hasFrameworkLibs() {
			ctx.AddVariationDependencies(nil, proguardRaiseTag, config.FrameworkLibraries...)
		}
	}
	if sdkDep.systemModules != "" {
		ctx.AddVariationDependencies(nil, systemModulesTag, sdkDep.systemModules)
	}
}

type deps struct {
	classpath               classpath
	java9Classpath          classpath
	bootClasspath           classpath
	processorPath           classpath
	errorProneProcessorPath classpath
	processorClasses        []string
	staticJars              android.Paths
	staticHeaderJars        android.Paths
	staticResourceJars      android.Paths
	aidlIncludeDirs         android.Paths
	srcs                    android.Paths
	srcJars                 android.Paths
	systemModules           *systemModules
	aidlPreprocess          android.OptionalPath
	kotlinStdlib            android.Paths
	kotlinAnnotations       android.Paths

	disableTurbine bool
}

func checkProducesJars(ctx android.ModuleContext, dep android.SourceFileProducer) {
	for _, f := range dep.Srcs() {
		if f.Ext() != ".jar" {
			ctx.ModuleErrorf("genrule %q must generate files ending with .jar to be used as a libs or static_libs dependency",
				ctx.OtherModuleName(dep.(blueprint.Module)))
		}
	}
}

func getJavaVersion(ctx android.ModuleContext, javaVersion string, sdkContext sdkContext) javaVersion {
	if javaVersion != "" {
		return normalizeJavaVersion(ctx, javaVersion)
	} else if ctx.Device() {
		return sdkContext.sdkVersion().defaultJavaLanguageVersion(ctx)
	} else {
		return JAVA_VERSION_9
	}
}

type javaVersion int

const (
	JAVA_VERSION_UNSUPPORTED = 0
	JAVA_VERSION_6           = 6
	JAVA_VERSION_7           = 7
	JAVA_VERSION_8           = 8
	JAVA_VERSION_9           = 9
)

func (v javaVersion) String() string {
	switch v {
	case JAVA_VERSION_6:
		return "1.6"
	case JAVA_VERSION_7:
		return "1.7"
	case JAVA_VERSION_8:
		return "1.8"
	case JAVA_VERSION_9:
		return "1.9"
	default:
		return "unsupported"
	}
}

// Returns true if javac targeting this version uses system modules instead of a bootclasspath.
func (v javaVersion) usesJavaModules() bool {
	return v >= 9
}

func normalizeJavaVersion(ctx android.BaseModuleContext, javaVersion string) javaVersion {
	switch javaVersion {
	case "1.6", "6":
		return JAVA_VERSION_6
	case "1.7", "7":
		return JAVA_VERSION_7
	case "1.8", "8":
		return JAVA_VERSION_8
	case "1.9", "9":
		return JAVA_VERSION_9
	case "10", "11":
		ctx.PropertyErrorf("java_version", "Java language levels above 9 are not supported")
		return JAVA_VERSION_UNSUPPORTED
	default:
		ctx.PropertyErrorf("java_version", "Unrecognized Java language level")
		return JAVA_VERSION_UNSUPPORTED
	}
}

//
// Java libraries (.jar file)
//

type Library struct {
	Module

	InstallMixin func(ctx android.ModuleContext, installPath android.Path) (extraInstallDeps android.Paths)
}

var _ android.ApexModule = (*Library)(nil)

// Provides access to the list of permitted packages from updatable boot jars.
type PermittedPackagesForUpdatableBootJars interface {
	PermittedPackagesForUpdatableBootJars() []string
}

var _ PermittedPackagesForUpdatableBootJars = (*Library)(nil)

func (j *Library) PermittedPackagesForUpdatableBootJars() []string {
	return j.properties.Permitted_packages
}

func shouldUncompressDex(ctx android.ModuleContext, dexpreopter *dexpreopter) bool {
	// Store uncompressed (and aligned) any dex files from jars in APEXes.
	if apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo); !apexInfo.IsForPlatform() {
		return true
	}

	// Store uncompressed (and do not strip) dex files from boot class path jars.
	if inList(ctx.ModuleName(), ctx.Config().BootJars()) {
		return true
	}

	// Store uncompressed dex files that are preopted on /system.
	if !dexpreopter.dexpreoptDisabled(ctx) && (ctx.Host() || !odexOnSystemOther(ctx, dexpreopter.installPath)) {
		return true
	}
	if ctx.Config().UncompressPrivAppDex() &&
		inList(ctx.ModuleName(), ctx.Config().ModulesLoadedByPrivilegedModules()) {
		return true
	}

	return false
}

func (j *Library) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Initialize the hiddenapi structure. Pass in the configuration name rather than the module name
	// so the hidden api will encode the <x>.impl java_ library created by java_sdk_library just as it
	// would the <x> library if <x> was configured as a boot jar.
	j.initHiddenAPI(ctx, j.ConfigurationName())

	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if !apexInfo.IsForPlatform() {
		j.hideApexVariantFromMake = true
	}

	j.checkSdkVersions(ctx)
	j.dexpreopter.installPath = android.PathForModuleInstall(ctx, "framework", j.Stem()+".jar")
	j.dexpreopter.isSDKLibrary = j.deviceProperties.IsSDKLibrary
	if j.dexProperties.Uncompress_dex == nil {
		// If the value was not force-set by the user, use reasonable default based on the module.
		j.dexProperties.Uncompress_dex = proptools.BoolPtr(shouldUncompressDex(ctx, &j.dexpreopter))
	}
	j.dexpreopter.uncompressedDex = *j.dexProperties.Uncompress_dex
	j.classLoaderContexts = make(dexpreopt.ClassLoaderContextMap)
	j.compile(ctx, nil)

	// Collect the module directory for IDE info in java/jdeps.go.
	j.modulePaths = append(j.modulePaths, ctx.ModuleDir())

	exclusivelyForApex := !apexInfo.IsForPlatform()
	if (Bool(j.properties.Installable) || ctx.Host()) && !exclusivelyForApex {
		var extraInstallDeps android.Paths
		if j.InstallMixin != nil {
			extraInstallDeps = j.InstallMixin(ctx, j.outputFile)
		}
		j.installFile = ctx.InstallFile(android.PathForModuleInstall(ctx, "framework"),
			j.Stem()+".jar", j.outputFile, extraInstallDeps...)
	}
}

func (j *Library) DepsMutator(ctx android.BottomUpMutatorContext) {
	j.deps(ctx)
}

const (
	aidlIncludeDir   = "aidl"
	javaDir          = "java"
	jarFileSuffix    = ".jar"
	testConfigSuffix = "-AndroidTest.xml"
)

// path to the jar file of a java library. Relative to <sdk_root>/<api_dir>
func sdkSnapshotFilePathForJar(osPrefix, name string) string {
	return sdkSnapshotFilePathForMember(osPrefix, name, jarFileSuffix)
}

func sdkSnapshotFilePathForMember(osPrefix, name string, suffix string) string {
	return filepath.Join(javaDir, osPrefix, name+suffix)
}

type librarySdkMemberType struct {
	android.SdkMemberTypeBase

	// Function to retrieve the appropriate output jar (implementation or header) from
	// the library.
	jarToExportGetter func(ctx android.SdkMemberContext, j *Library) android.Path

	// Function to compute the snapshot relative path to which the named library's
	// jar should be copied.
	snapshotPathGetter func(osPrefix, name string) string

	// True if only the jar should be copied to the snapshot, false if the jar plus any additional
	// files like aidl files should also be copied.
	onlyCopyJarToSnapshot bool
}

const (
	onlyCopyJarToSnapshot    = true
	copyEverythingToSnapshot = false
)

func (mt *librarySdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	mctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (mt *librarySdkMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*Library)
	return ok
}

func (mt *librarySdkMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	return ctx.SnapshotBuilder().AddPrebuiltModule(member, "java_import")
}

func (mt *librarySdkMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &librarySdkMemberProperties{}
}

type librarySdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	JarToExport     android.Path `android:"arch_variant"`
	AidlIncludeDirs android.Paths
}

func (p *librarySdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	j := variant.(*Library)

	p.JarToExport = ctx.MemberType().(*librarySdkMemberType).jarToExportGetter(ctx, j)

	p.AidlIncludeDirs = j.AidlIncludeDirs()
}

func (p *librarySdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	builder := ctx.SnapshotBuilder()

	memberType := ctx.MemberType().(*librarySdkMemberType)

	exportedJar := p.JarToExport
	if exportedJar != nil {
		// Delegate the creation of the snapshot relative path to the member type.
		snapshotRelativeJavaLibPath := memberType.snapshotPathGetter(p.OsPrefix(), ctx.Name())

		// Copy the exported jar to the snapshot.
		builder.CopyToSnapshot(exportedJar, snapshotRelativeJavaLibPath)

		propertySet.AddProperty("jars", []string{snapshotRelativeJavaLibPath})
	}

	// Do not copy anything else to the snapshot.
	if memberType.onlyCopyJarToSnapshot {
		return
	}

	aidlIncludeDirs := p.AidlIncludeDirs
	if len(aidlIncludeDirs) != 0 {
		sdkModuleContext := ctx.SdkModuleContext()
		for _, dir := range aidlIncludeDirs {
			// TODO(jiyong): copy parcelable declarations only
			aidlFiles, _ := sdkModuleContext.GlobWithDeps(dir.String()+"/**/*.aidl", nil)
			for _, file := range aidlFiles {
				builder.CopyToSnapshot(android.PathForSource(sdkModuleContext, file), filepath.Join(aidlIncludeDir, file))
			}
		}

		// TODO(b/151933053) - add aidl include dirs property
	}
}

var javaHeaderLibsSdkMemberType android.SdkMemberType = &librarySdkMemberType{
	android.SdkMemberTypeBase{
		PropertyName: "java_header_libs",
		SupportsSdk:  true,
	},
	func(_ android.SdkMemberContext, j *Library) android.Path {
		headerJars := j.HeaderJars()
		if len(headerJars) != 1 {
			panic(fmt.Errorf("there must be only one header jar from %q", j.Name()))
		}

		return headerJars[0]
	},
	sdkSnapshotFilePathForJar,
	copyEverythingToSnapshot,
}

// java_library builds and links sources into a `.jar` file for the device, and possibly for the host as well.
//
// By default, a java_library has a single variant that produces a `.jar` file containing `.class` files that were
// compiled against the device bootclasspath.  This jar is not suitable for installing on a device, but can be used
// as a `static_libs` dependency of another module.
//
// Specifying `installable: true` will product a `.jar` file containing `classes.dex` files, suitable for installing on
// a device.
//
// Specifying `host_supported: true` will produce two variants, one compiled against the device bootclasspath and one
// compiled against the host bootclasspath.
func LibraryFactory() android.Module {
	module := &Library{}

	module.addHostAndDeviceProperties()

	module.initModuleAndImport(&module.ModuleBase)

	android.InitApexModule(module)
	android.InitSdkAwareModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)
	return module
}

// java_library_static is an obsolete alias for java_library.
func LibraryStaticFactory() android.Module {
	return LibraryFactory()
}

// java_library_host builds and links sources into a `.jar` file for the host.
//
// A java_library_host has a single variant that produces a `.jar` file containing `.class` files that were
// compiled against the host bootclasspath.
func LibraryHostFactory() android.Module {
	module := &Library{}

	module.addHostProperties()

	module.Module.properties.Installable = proptools.BoolPtr(true)

	android.InitApexModule(module)
	InitJavaModule(module, android.HostSupported)
	return module
}

//
// Java Tests
//

// Test option struct.
type TestOptions struct {
	// a list of extra test configuration files that should be installed with the module.
	Extra_test_configs []string `android:"path,arch_variant"`

	// If the test is a hostside(no device required) unittest that shall be run during presubmit check.
	Unit_test *bool
}

type testProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`

	// the name of the test configuration template (for example "AndroidTestTemplate.xml") that
	// should be installed with the module.
	Test_config_template *string `android:"path,arch_variant"`

	// list of files or filegroup modules that provide data that should be installed alongside
	// the test
	Data []string `android:"path"`

	// Flag to indicate whether or not to create test config automatically. If AndroidTest.xml
	// doesn't exist next to the Android.bp, this attribute doesn't need to be set to true
	// explicitly.
	Auto_gen_config *bool

	// Add parameterized mainline modules to auto generated test config. The options will be
	// handled by TradeFed to do downloading and installing the specified modules on the device.
	Test_mainline_modules []string

	// Test options.
	Test_options TestOptions
}

type hostTestProperties struct {
	// list of native binary modules that should be installed alongside the test
	Data_native_bins []string `android:"arch_variant"`
}

type testHelperLibraryProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`
}

type prebuiltTestProperties struct {
	// list of compatibility suites (for example "cts", "vts") that the module should be
	// installed into.
	Test_suites []string `android:"arch_variant"`

	// the name of the test configuration (for example "AndroidTest.xml") that should be
	// installed with the module.
	Test_config *string `android:"path,arch_variant"`
}

type Test struct {
	Library

	testProperties testProperties

	testConfig       android.Path
	extraTestConfigs android.Paths
	data             android.Paths
}

type TestHost struct {
	Test

	testHostProperties hostTestProperties
}

type TestHelperLibrary struct {
	Library

	testHelperLibraryProperties testHelperLibraryProperties
}

type JavaTestImport struct {
	Import

	prebuiltTestProperties prebuiltTestProperties

	testConfig android.Path
	dexJarFile android.Path
}

func (j *TestHost) DepsMutator(ctx android.BottomUpMutatorContext) {
	if len(j.testHostProperties.Data_native_bins) > 0 {
		for _, target := range ctx.MultiTargets() {
			ctx.AddVariationDependencies(target.Variations(), dataNativeBinsTag, j.testHostProperties.Data_native_bins...)
		}
	}

	j.deps(ctx)
}

func (j *TestHost) AddExtraResource(p android.Path) {
	j.extraResources = append(j.extraResources, p)
}

func (j *Test) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if j.testProperties.Test_options.Unit_test == nil && ctx.Host() {
		// TODO(b/): Clean temporary heuristic to avoid unexpected onboarding.
		defaultUnitTest := !inList("tradefed", j.properties.Static_libs) && !inList("tradefed", j.properties.Libs) && !inList("cts", j.testProperties.Test_suites)
		j.testProperties.Test_options.Unit_test = proptools.BoolPtr(defaultUnitTest)
	}
	j.testConfig = tradefed.AutoGenJavaTestConfig(ctx, j.testProperties.Test_config, j.testProperties.Test_config_template,
		j.testProperties.Test_suites, j.testProperties.Auto_gen_config, j.testProperties.Test_options.Unit_test)

	j.data = android.PathsForModuleSrc(ctx, j.testProperties.Data)

	j.extraTestConfigs = android.PathsForModuleSrc(ctx, j.testProperties.Test_options.Extra_test_configs)

	ctx.VisitDirectDepsWithTag(dataNativeBinsTag, func(dep android.Module) {
		j.data = append(j.data, android.OutputFileForModule(ctx, dep, ""))
	})

	j.Library.GenerateAndroidBuildActions(ctx)
}

func (j *TestHelperLibrary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	j.Library.GenerateAndroidBuildActions(ctx)
}

func (j *JavaTestImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	j.testConfig = tradefed.AutoGenJavaTestConfig(ctx, j.prebuiltTestProperties.Test_config, nil,
		j.prebuiltTestProperties.Test_suites, nil, nil)

	j.Import.GenerateAndroidBuildActions(ctx)
}

type testSdkMemberType struct {
	android.SdkMemberTypeBase
}

func (mt *testSdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	mctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (mt *testSdkMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*Test)
	return ok
}

func (mt *testSdkMemberType) AddPrebuiltModule(ctx android.SdkMemberContext, member android.SdkMember) android.BpModule {
	return ctx.SnapshotBuilder().AddPrebuiltModule(member, "java_test_import")
}

func (mt *testSdkMemberType) CreateVariantPropertiesStruct() android.SdkMemberProperties {
	return &testSdkMemberProperties{}
}

type testSdkMemberProperties struct {
	android.SdkMemberPropertiesBase

	JarToExport android.Path
	TestConfig  android.Path
}

func (p *testSdkMemberProperties) PopulateFromVariant(ctx android.SdkMemberContext, variant android.Module) {
	test := variant.(*Test)

	implementationJars := test.ImplementationJars()
	if len(implementationJars) != 1 {
		panic(fmt.Errorf("there must be only one implementation jar from %q", test.Name()))
	}

	p.JarToExport = implementationJars[0]
	p.TestConfig = test.testConfig
}

func (p *testSdkMemberProperties) AddToPropertySet(ctx android.SdkMemberContext, propertySet android.BpPropertySet) {
	builder := ctx.SnapshotBuilder()

	exportedJar := p.JarToExport
	if exportedJar != nil {
		snapshotRelativeJavaLibPath := sdkSnapshotFilePathForJar(p.OsPrefix(), ctx.Name())
		builder.CopyToSnapshot(exportedJar, snapshotRelativeJavaLibPath)

		propertySet.AddProperty("jars", []string{snapshotRelativeJavaLibPath})
	}

	testConfig := p.TestConfig
	if testConfig != nil {
		snapshotRelativeTestConfigPath := sdkSnapshotFilePathForMember(p.OsPrefix(), ctx.Name(), testConfigSuffix)
		builder.CopyToSnapshot(testConfig, snapshotRelativeTestConfigPath)
		propertySet.AddProperty("test_config", snapshotRelativeTestConfigPath)
	}
}

// java_test builds a and links sources into a `.jar` file for the device, and possibly for the host as well, and
// creates an `AndroidTest.xml` file to allow running the test with `atest` or a `TEST_MAPPING` file.
//
// By default, a java_test has a single variant that produces a `.jar` file containing `classes.dex` files that were
// compiled against the device bootclasspath.
//
// Specifying `host_supported: true` will produce two variants, one compiled against the device bootclasspath and one
// compiled against the host bootclasspath.
func TestFactory() android.Module {
	module := &Test{}

	module.addHostAndDeviceProperties()
	module.AddProperties(&module.testProperties)

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.Module.dexpreopter.isTest = true
	module.Module.linter.test = true

	InitJavaModule(module, android.HostAndDeviceSupported)
	return module
}

// java_test_helper_library creates a java library and makes sure that it is added to the appropriate test suite.
func TestHelperLibraryFactory() android.Module {
	module := &TestHelperLibrary{}

	module.addHostAndDeviceProperties()
	module.AddProperties(&module.testHelperLibraryProperties)

	module.Module.properties.Installable = proptools.BoolPtr(true)
	module.Module.dexpreopter.isTest = true
	module.Module.linter.test = true

	InitJavaModule(module, android.HostAndDeviceSupported)
	return module
}

// java_test_import imports one or more `.jar` files into the build graph as if they were built by a java_test module
// and makes sure that it is added to the appropriate test suite.
//
// By default, a java_test_import has a single variant that expects a `.jar` file containing `.class` files that were
// compiled against an Android classpath.
//
// Specifying `host_supported: true` will produce two variants, one for use as a dependency of device modules and one
// for host modules.
func JavaTestImportFactory() android.Module {
	module := &JavaTestImport{}

	module.AddProperties(
		&module.Import.properties,
		&module.prebuiltTestProperties)

	module.Import.properties.Installable = proptools.BoolPtr(true)

	android.InitPrebuiltModule(module, &module.properties.Jars)
	android.InitApexModule(module)
	android.InitSdkAwareModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)
	return module
}

// java_test_host builds a and links sources into a `.jar` file for the host, and creates an `AndroidTest.xml` file to
// allow running the test with `atest` or a `TEST_MAPPING` file.
//
// A java_test_host has a single variant that produces a `.jar` file containing `.class` files that were
// compiled against the host bootclasspath.
func TestHostFactory() android.Module {
	module := &TestHost{}

	module.addHostProperties()
	module.AddProperties(&module.testProperties)
	module.AddProperties(&module.testHostProperties)

	InitTestHost(
		module,
		proptools.BoolPtr(true),
		nil,
		nil)

	InitJavaModuleMultiTargets(module, android.HostSupported)

	return module
}

func InitTestHost(th *TestHost, installable *bool, testSuites []string, autoGenConfig *bool) {
	th.properties.Installable = installable
	th.testProperties.Auto_gen_config = autoGenConfig
	th.testProperties.Test_suites = testSuites
}

//
// Java Binaries (.jar file plus wrapper script)
//

type binaryProperties struct {
	// installable script to execute the resulting jar
	Wrapper *string `android:"path"`

	// Name of the class containing main to be inserted into the manifest as Main-Class.
	Main_class *string

	// Names of modules containing JNI libraries that should be installed alongside the host
	// variant of the binary.
	Jni_libs []string
}

type Binary struct {
	Library

	binaryProperties binaryProperties

	isWrapperVariant bool

	wrapperFile android.Path
	binaryFile  android.InstallPath
}

func (j *Binary) HostToolPath() android.OptionalPath {
	return android.OptionalPathForPath(j.binaryFile)
}

func (j *Binary) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if ctx.Arch().ArchType == android.Common {
		// Compile the jar
		if j.binaryProperties.Main_class != nil {
			if j.properties.Manifest != nil {
				ctx.PropertyErrorf("main_class", "main_class cannot be used when manifest is set")
			}
			manifestFile := android.PathForModuleOut(ctx, "manifest.txt")
			GenerateMainClassManifest(ctx, manifestFile, String(j.binaryProperties.Main_class))
			j.overrideManifest = android.OptionalPathForPath(manifestFile)
		}

		j.Library.GenerateAndroidBuildActions(ctx)
	} else {
		// Handle the binary wrapper
		j.isWrapperVariant = true

		if j.binaryProperties.Wrapper != nil {
			j.wrapperFile = android.PathForModuleSrc(ctx, *j.binaryProperties.Wrapper)
		} else {
			j.wrapperFile = android.PathForSource(ctx, "build/soong/scripts/jar-wrapper.sh")
		}

		// The host installation rules make the installed wrapper depend on all the dependencies
		// of the wrapper variant, which will include the common variant's jar file and any JNI
		// libraries.  This is verified by TestBinary.
		j.binaryFile = ctx.InstallExecutable(android.PathForModuleInstall(ctx, "bin"),
			ctx.ModuleName(), j.wrapperFile)
	}
}

func (j *Binary) DepsMutator(ctx android.BottomUpMutatorContext) {
	if ctx.Arch().ArchType == android.Common || ctx.BazelConversionMode() {
		j.deps(ctx)
	}
	if ctx.Arch().ArchType != android.Common || ctx.BazelConversionMode() {
		// These dependencies ensure the host installation rules will install the jar file and
		// the jni libraries when the wrapper is installed.
		ctx.AddVariationDependencies(nil, jniInstallTag, j.binaryProperties.Jni_libs...)
		ctx.AddVariationDependencies(
			[]blueprint.Variation{{Mutator: "arch", Variation: android.CommonArch.String()}},
			binaryInstallTag, ctx.ModuleName())
	}
}

// java_binary builds a `.jar` file and a shell script that executes it for the device, and possibly for the host
// as well.
//
// By default, a java_binary has a single variant that produces a `.jar` file containing `classes.dex` files that were
// compiled against the device bootclasspath.
//
// Specifying `host_supported: true` will produce two variants, one compiled against the device bootclasspath and one
// compiled against the host bootclasspath.
func BinaryFactory() android.Module {
	module := &Binary{}

	module.addHostAndDeviceProperties()
	module.AddProperties(&module.binaryProperties)

	module.Module.properties.Installable = proptools.BoolPtr(true)

	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommonFirst)
	android.InitDefaultableModule(module)
	return module
}

// java_binary_host builds a `.jar` file and a shell script that executes it for the host.
//
// A java_binary_host has a single variant that produces a `.jar` file containing `.class` files that were
// compiled against the host bootclasspath.
func BinaryHostFactory() android.Module {
	module := &Binary{}

	module.addHostProperties()
	module.AddProperties(&module.binaryProperties)

	module.Module.properties.Installable = proptools.BoolPtr(true)

	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommonFirst)
	android.InitDefaultableModule(module)
	return module
}

//
// Java prebuilts
//

type ImportProperties struct {
	Jars []string `android:"path,arch_variant"`

	Sdk_version *string

	Installable *bool

	// List of shared java libs that this module has dependencies to
	Libs []string

	// List of files to remove from the jar file(s)
	Exclude_files []string

	// List of directories to remove from the jar file(s)
	Exclude_dirs []string

	// if set to true, run Jetifier against .jar file. Defaults to false.
	Jetifier *bool

	// set the name of the output
	Stem *string

	Aidl struct {
		// directories that should be added as include directories for any aidl sources of modules
		// that depend on this module, as well as to aidl for this module.
		Export_include_dirs []string
	}
}

type Import struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	prebuilt android.Prebuilt
	android.SdkBase

	// Functionality common to Module and Import.
	embeddableInModuleAndImport

	hiddenAPI
	dexer
	dexpreopter

	properties ImportProperties

	// output file containing classes.dex and resources
	dexJarFile android.Path

	combinedClasspathFile android.Path
	classLoaderContexts   dexpreopt.ClassLoaderContextMap
	exportAidlIncludeDirs android.Paths

	hideApexVariantFromMake bool
}

func (j *Import) sdkVersion() sdkSpec {
	return sdkSpecFrom(String(j.properties.Sdk_version))
}

func (j *Import) makeSdkVersion() string {
	return j.sdkVersion().raw
}

func (j *Import) systemModules() string {
	return "none"
}

func (j *Import) minSdkVersion() sdkSpec {
	return j.sdkVersion()
}

func (j *Import) targetSdkVersion() sdkSpec {
	return j.sdkVersion()
}

func (j *Import) MinSdkVersion() string {
	return j.minSdkVersion().version.String()
}

func (j *Import) Prebuilt() *android.Prebuilt {
	return &j.prebuilt
}

func (j *Import) PrebuiltSrcs() []string {
	return j.properties.Jars
}

func (j *Import) Name() string {
	return j.prebuilt.Name(j.ModuleBase.Name())
}

func (j *Import) Stem() string {
	return proptools.StringDefault(j.properties.Stem, j.ModuleBase.Name())
}

func (a *Import) JacocoReportClassesFile() android.Path {
	return nil
}

func (j *Import) LintDepSets() LintDepSets {
	return LintDepSets{}
}

func (j *Import) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, libTag, j.properties.Libs...)

	if ctx.Device() && Bool(j.dexProperties.Compile_dex) {
		sdkDeps(ctx, sdkContext(j), j.dexer)
	}
}

func (j *Import) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Initialize the hiddenapi structure.
	j.initHiddenAPI(ctx, j.BaseModuleName())

	if !ctx.Provider(android.ApexInfoProvider).(android.ApexInfo).IsForPlatform() {
		j.hideApexVariantFromMake = true
	}

	jars := android.PathsForModuleSrc(ctx, j.properties.Jars)

	jarName := j.Stem() + ".jar"
	outputFile := android.PathForModuleOut(ctx, "combined", jarName)
	TransformJarsToJar(ctx, outputFile, "for prebuilts", jars, android.OptionalPath{},
		false, j.properties.Exclude_files, j.properties.Exclude_dirs)
	if Bool(j.properties.Jetifier) {
		inputFile := outputFile
		outputFile = android.PathForModuleOut(ctx, "jetifier", jarName)
		TransformJetifier(ctx, outputFile, inputFile)
	}
	j.combinedClasspathFile = outputFile
	j.classLoaderContexts = make(dexpreopt.ClassLoaderContextMap)

	var flags javaBuilderFlags
	var deapexerModule android.Module

	ctx.VisitDirectDeps(func(module android.Module) {
		tag := ctx.OtherModuleDependencyTag(module)

		if ctx.OtherModuleHasProvider(module, JavaInfoProvider) {
			dep := ctx.OtherModuleProvider(module, JavaInfoProvider).(JavaInfo)
			switch tag {
			case libTag, staticLibTag:
				flags.classpath = append(flags.classpath, dep.HeaderJars...)
			case bootClasspathTag:
				flags.bootClasspath = append(flags.bootClasspath, dep.HeaderJars...)
			}
		} else if dep, ok := module.(SdkLibraryDependency); ok {
			switch tag {
			case libTag:
				flags.classpath = append(flags.classpath, dep.SdkHeaderJars(ctx, j.sdkVersion())...)
			}
		}

		addCLCFromDep(ctx, module, j.classLoaderContexts)

		// Save away the `deapexer` module on which this depends, if any.
		if tag == android.DeapexerTag {
			deapexerModule = module
		}
	})

	if Bool(j.properties.Installable) {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "framework"),
			jarName, outputFile)
	}

	j.exportAidlIncludeDirs = android.PathsForModuleSrc(ctx, j.properties.Aidl.Export_include_dirs)

	if ctx.Device() {
		// If this is a variant created for a prebuilt_apex then use the dex implementation jar
		// obtained from the associated deapexer module.
		ai := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
		if ai.ForPrebuiltApex {
			if deapexerModule == nil {
				// This should never happen as a variant for a prebuilt_apex is only created if the
				// deapxer module has been configured to export the dex implementation jar for this module.
				ctx.ModuleErrorf("internal error: module %q does not depend on a `deapexer` module for prebuilt_apex %q",
					j.Name(), ai.ApexVariationName)
			}

			// Get the path of the dex implementation jar from the `deapexer` module.
			di := ctx.OtherModuleProvider(deapexerModule, android.DeapexerProvider).(android.DeapexerInfo)
			if dexOutputPath := di.PrebuiltExportPath(j.BaseModuleName(), ".dexjar"); dexOutputPath != nil {
				j.dexJarFile = dexOutputPath
				j.hiddenAPIExtractInformation(ctx, dexOutputPath, outputFile)
			} else {
				// This should never happen as a variant for a prebuilt_apex is only created if the
				// prebuilt_apex has been configured to export the java library dex file.
				ctx.ModuleErrorf("internal error: no dex implementation jar available from prebuilt_apex %q", deapexerModule.Name())
			}
		} else if Bool(j.dexProperties.Compile_dex) {
			sdkDep := decodeSdkDep(ctx, sdkContext(j))
			if sdkDep.invalidVersion {
				ctx.AddMissingDependencies(sdkDep.bootclasspath)
				ctx.AddMissingDependencies(sdkDep.java9Classpath)
			} else if sdkDep.useFiles {
				// sdkDep.jar is actually equivalent to turbine header.jar.
				flags.classpath = append(flags.classpath, sdkDep.jars...)
			}

			// Dex compilation

			j.dexpreopter.installPath = android.PathForModuleInstall(ctx, "framework", jarName)
			if j.dexProperties.Uncompress_dex == nil {
				// If the value was not force-set by the user, use reasonable default based on the module.
				j.dexProperties.Uncompress_dex = proptools.BoolPtr(shouldUncompressDex(ctx, &j.dexpreopter))
			}
			j.dexpreopter.uncompressedDex = *j.dexProperties.Uncompress_dex

			var dexOutputFile android.OutputPath
			dexOutputFile = j.dexer.compileDex(ctx, flags, j.minSdkVersion(), outputFile, jarName)
			if ctx.Failed() {
				return
			}

			// Hidden API CSV generation and dex encoding
			dexOutputFile = j.hiddenAPIExtractAndEncode(ctx, dexOutputFile, outputFile,
				proptools.Bool(j.dexProperties.Uncompress_dex))

			j.dexJarFile = dexOutputFile
		}
	}

	ctx.SetProvider(JavaInfoProvider, JavaInfo{
		HeaderJars:                     android.PathsIfNonNil(j.combinedClasspathFile),
		ImplementationAndResourcesJars: android.PathsIfNonNil(j.combinedClasspathFile),
		ImplementationJars:             android.PathsIfNonNil(j.combinedClasspathFile),
		AidlIncludeDirs:                j.exportAidlIncludeDirs,
	})
}

func (j *Import) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "", ".jar":
		return android.Paths{j.combinedClasspathFile}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

var _ android.OutputFileProducer = (*Import)(nil)

func (j *Import) HeaderJars() android.Paths {
	if j.combinedClasspathFile == nil {
		return nil
	}
	return android.Paths{j.combinedClasspathFile}
}

func (j *Import) ImplementationAndResourcesJars() android.Paths {
	if j.combinedClasspathFile == nil {
		return nil
	}
	return android.Paths{j.combinedClasspathFile}
}

func (j *Import) DexJarBuildPath() android.Path {
	return j.dexJarFile
}

func (j *Import) DexJarInstallPath() android.Path {
	return nil
}

func (j *Import) ClassLoaderContexts() dexpreopt.ClassLoaderContextMap {
	return j.classLoaderContexts
}

var _ android.ApexModule = (*Import)(nil)

// Implements android.ApexModule
func (j *Import) DepIsInSameApex(ctx android.BaseModuleContext, dep android.Module) bool {
	return j.depIsInSameApex(ctx, dep)
}

// Implements android.ApexModule
func (j *Import) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	// Do not check for prebuilts against the min_sdk_version of enclosing APEX
	return nil
}

// Add compile time check for interface implementation
var _ android.IDEInfo = (*Import)(nil)
var _ android.IDECustomizedModuleName = (*Import)(nil)

// Collect information for opening IDE project files in java/jdeps.go.
const (
	removedPrefix = "prebuilt_"
)

func (j *Import) IDEInfo(dpInfo *android.IdeInfo) {
	dpInfo.Jars = append(dpInfo.Jars, j.PrebuiltSrcs()...)
}

func (j *Import) IDECustomizedModuleName() string {
	// TODO(b/113562217): Extract the base module name from the Import name, often the Import name
	// has a prefix "prebuilt_". Remove the prefix explicitly if needed until we find a better
	// solution to get the Import name.
	name := j.Name()
	if strings.HasPrefix(name, removedPrefix) {
		name = strings.TrimPrefix(name, removedPrefix)
	}
	return name
}

var _ android.PrebuiltInterface = (*Import)(nil)

func (j *Import) IsInstallable() bool {
	return Bool(j.properties.Installable)
}

var _ dexpreopterInterface = (*Import)(nil)

// java_import imports one or more `.jar` files into the build graph as if they were built by a java_library module.
//
// By default, a java_import has a single variant that expects a `.jar` file containing `.class` files that were
// compiled against an Android classpath.
//
// Specifying `host_supported: true` will produce two variants, one for use as a dependency of device modules and one
// for host modules.
func ImportFactory() android.Module {
	module := &Import{}

	module.AddProperties(
		&module.properties,
		&module.dexer.dexProperties,
	)

	module.initModuleAndImport(&module.ModuleBase)

	module.dexProperties.Optimize.EnabledByDefault = false

	android.InitPrebuiltModule(module, &module.properties.Jars)
	android.InitApexModule(module)
	android.InitSdkAwareModule(module)
	InitJavaModule(module, android.HostAndDeviceSupported)
	return module
}

// java_import imports one or more `.jar` files into the build graph as if they were built by a java_library_host
// module.
//
// A java_import_host has a single variant that expects a `.jar` file containing `.class` files that were
// compiled against a host bootclasspath.
func ImportFactoryHost() android.Module {
	module := &Import{}

	module.AddProperties(&module.properties)

	android.InitPrebuiltModule(module, &module.properties.Jars)
	android.InitApexModule(module)
	InitJavaModule(module, android.HostSupported)
	return module
}

// dex_import module

type DexImportProperties struct {
	Jars []string `android:"path"`

	// set the name of the output
	Stem *string
}

type DexImport struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.ApexModuleBase
	prebuilt android.Prebuilt

	properties DexImportProperties

	dexJarFile android.Path

	dexpreopter

	hideApexVariantFromMake bool
}

func (j *DexImport) Prebuilt() *android.Prebuilt {
	return &j.prebuilt
}

func (j *DexImport) PrebuiltSrcs() []string {
	return j.properties.Jars
}

func (j *DexImport) Name() string {
	return j.prebuilt.Name(j.ModuleBase.Name())
}

func (j *DexImport) Stem() string {
	return proptools.StringDefault(j.properties.Stem, j.ModuleBase.Name())
}

func (a *DexImport) JacocoReportClassesFile() android.Path {
	return nil
}

func (a *DexImport) LintDepSets() LintDepSets {
	return LintDepSets{}
}

func (j *DexImport) IsInstallable() bool {
	return true
}

func (j *DexImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(j.properties.Jars) != 1 {
		ctx.PropertyErrorf("jars", "exactly one jar must be provided")
	}

	apexInfo := ctx.Provider(android.ApexInfoProvider).(android.ApexInfo)
	if !apexInfo.IsForPlatform() {
		j.hideApexVariantFromMake = true
	}

	j.dexpreopter.installPath = android.PathForModuleInstall(ctx, "framework", j.Stem()+".jar")
	j.dexpreopter.uncompressedDex = shouldUncompressDex(ctx, &j.dexpreopter)

	inputJar := ctx.ExpandSource(j.properties.Jars[0], "jars")
	dexOutputFile := android.PathForModuleOut(ctx, ctx.ModuleName()+".jar")

	if j.dexpreopter.uncompressedDex {
		rule := android.NewRuleBuilder(pctx, ctx)

		temporary := android.PathForModuleOut(ctx, ctx.ModuleName()+".jar.unaligned")
		rule.Temporary(temporary)

		// use zip2zip to uncompress classes*.dex files
		rule.Command().
			BuiltTool("zip2zip").
			FlagWithInput("-i ", inputJar).
			FlagWithOutput("-o ", temporary).
			FlagWithArg("-0 ", "'classes*.dex'")

		// use zipalign to align uncompressed classes*.dex files
		rule.Command().
			BuiltTool("zipalign").
			Flag("-f").
			Text("4").
			Input(temporary).
			Output(dexOutputFile)

		rule.DeleteTemporaryFiles()

		rule.Build("uncompress_dex", "uncompress dex")
	} else {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  inputJar,
			Output: dexOutputFile,
		})
	}

	j.dexJarFile = dexOutputFile

	j.dexpreopt(ctx, dexOutputFile)

	if apexInfo.IsForPlatform() {
		ctx.InstallFile(android.PathForModuleInstall(ctx, "framework"),
			j.Stem()+".jar", dexOutputFile)
	}
}

func (j *DexImport) DexJarBuildPath() android.Path {
	return j.dexJarFile
}

var _ android.ApexModule = (*DexImport)(nil)

// Implements android.ApexModule
func (j *DexImport) ShouldSupportSdkVersion(ctx android.BaseModuleContext,
	sdkVersion android.ApiLevel) error {
	// we don't check prebuilt modules for sdk_version
	return nil
}

// dex_import imports a `.jar` file containing classes.dex files.
//
// A dex_import module cannot be used as a dependency of a java_* or android_* module, it can only be installed
// to the device.
func DexImportFactory() android.Module {
	module := &DexImport{}

	module.AddProperties(&module.properties)

	android.InitPrebuiltModule(module, &module.properties.Jars)
	android.InitApexModule(module)
	InitJavaModule(module, android.DeviceSupported)
	return module
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
	android.ApexModuleBase
}

// java_defaults provides a set of properties that can be inherited by other java or android modules.
//
// A module can use the properties from a java_defaults module using `defaults: ["defaults_module_name"]`.  Each
// property in the defaults module that exists in the depending module will be prepended to the depending module's
// value for that property.
//
// Example:
//
//     java_defaults {
//         name: "example_defaults",
//         srcs: ["common/**/*.java"],
//         javacflags: ["-Xlint:all"],
//         aaptflags: ["--auto-add-overlay"],
//     }
//
//     java_library {
//         name: "example",
//         defaults: ["example_defaults"],
//         srcs: ["example/**/*.java"],
//     }
//
// is functionally identical to:
//
//     java_library {
//         name: "example",
//         srcs: [
//             "common/**/*.java",
//             "example/**/*.java",
//         ],
//         javacflags: ["-Xlint:all"],
//     }
func DefaultsFactory() android.Module {
	module := &Defaults{}

	module.AddProperties(
		&CommonProperties{},
		&DeviceProperties{},
		&DexProperties{},
		&DexpreoptProperties{},
		&android.ProtoProperties{},
		&aaptProperties{},
		&androidLibraryProperties{},
		&appProperties{},
		&appTestProperties{},
		&overridableAppProperties{},
		&testProperties{},
		&ImportProperties{},
		&AARImportProperties{},
		&sdkLibraryProperties{},
		&commonToSdkLibraryAndImportProperties{},
		&DexImportProperties{},
		&android.ApexProperties{},
		&RuntimeResourceOverlayProperties{},
		&LintProperties{},
		&appTestHelperAppProperties{},
	)

	android.InitDefaultsModule(module)
	return module
}

func kytheExtractJavaFactory() android.Singleton {
	return &kytheExtractJavaSingleton{}
}

type kytheExtractJavaSingleton struct {
}

func (ks *kytheExtractJavaSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var xrefTargets android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		if javaModule, ok := module.(xref); ok {
			xrefTargets = append(xrefTargets, javaModule.XrefJavaFiles()...)
		}
	})
	// TODO(asmundak): perhaps emit a rule to output a warning if there were no xrefTargets
	if len(xrefTargets) > 0 {
		ctx.Phony("xref_java", xrefTargets...)
	}
}

var Bool = proptools.Bool
var BoolDefault = proptools.BoolDefault
var String = proptools.String
var inList = android.InList

// Add class loader context (CLC) of a given dependency to the current CLC.
func addCLCFromDep(ctx android.ModuleContext, depModule android.Module,
	clcMap dexpreopt.ClassLoaderContextMap) {

	dep, ok := depModule.(UsesLibraryDependency)
	if !ok {
		return
	}

	// Find out if the dependency is either an SDK library or an ordinary library that is disguised
	// as an SDK library by the means of `provides_uses_lib` property. If yes, the library is itself
	// a <uses-library> and should be added as a node in the CLC tree, and its CLC should be added
	// as subtree of that node. Otherwise the library is not a <uses_library> and should not be
	// added to CLC, but the transitive <uses-library> dependencies from its CLC should be added to
	// the current CLC.
	var implicitSdkLib *string
	comp, isComp := depModule.(SdkLibraryComponentDependency)
	if isComp {
		implicitSdkLib = comp.OptionalImplicitSdkLibrary()
		// OptionalImplicitSdkLibrary() may be nil so need to fall through to ProvidesUsesLib().
	}
	if implicitSdkLib == nil {
		if ulib, ok := depModule.(ProvidesUsesLib); ok {
			implicitSdkLib = ulib.ProvidesUsesLib()
		}
	}

	depTag := ctx.OtherModuleDependencyTag(depModule)
	if depTag == libTag || depTag == usesLibTag {
		// Ok, propagate <uses-library> through non-static library dependencies.
	} else if depTag == staticLibTag {
		// Propagate <uses-library> through static library dependencies, unless it is a component
		// library (such as stubs). Component libraries have a dependency on their SDK library,
		// which should not be pulled just because of a static component library.
		if implicitSdkLib != nil {
			return
		}
	} else {
		// Don't propagate <uses-library> for other dependency tags.
		return
	}

	if implicitSdkLib != nil {
		clcMap.AddContext(ctx, dexpreopt.AnySdkVersion, *implicitSdkLib,
			dep.DexJarBuildPath(), dep.DexJarInstallPath(), dep.ClassLoaderContexts())
	} else {
		depName := ctx.OtherModuleName(depModule)
		clcMap.AddContextMap(dep.ClassLoaderContexts(), depName)
	}
}
