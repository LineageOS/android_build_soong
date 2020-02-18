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

// The dexpreopt package converts a global dexpreopt config and a module dexpreopt config into rules to perform
// dexpreopting.
//
// It is used in two places; in the dexpeopt_gen binary for modules defined in Make, and directly linked into Soong.
//
// For Make modules it is built into the dexpreopt_gen binary, which is executed as a Make rule using global config and
// module config specified in JSON files.  The binary writes out two shell scripts, only updating them if they have
// changed.  One script takes an APK or JAR as an input and produces a zip file containing any outputs of preopting,
// in the location they should be on the device.  The Make build rules will unzip the zip file into $(PRODUCT_OUT) when
// installing the APK, which will install the preopt outputs into $(PRODUCT_OUT)/system or $(PRODUCT_OUT)/system_other
// as necessary.  The zip file may be empty if preopting was disabled for any reason.
//
// The intermediate shell scripts allow changes to this package or to the global config to regenerate the shell scripts
// but only require re-executing preopting if the script has changed.
//
// For Soong modules this package is linked directly into Soong and run from the java package.  It generates the same
// commands as for make, using athe same global config JSON file used by make, but using a module config structure
// provided by Soong.  The generated commands are then converted into Soong rule and written directly to the ninja file,
// with no extra shell scripts involved.
package dexpreopt

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

const SystemPartition = "/system/"
const SystemOtherPartition = "/system_other/"

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var SystemServerDepTag = dependencyTag{name: "system-server-dep"}
var SystemServerForcedDepTag = dependencyTag{name: "system-server-forced-dep"}

// GenerateDexpreoptRule generates a set of commands that will preopt a module based on a GlobalConfig and a
// ModuleConfig.  The produced files and their install locations will be available through rule.Installs().
func GenerateDexpreoptRule(ctx android.PathContext, globalSoong *GlobalSoongConfig,
	global *GlobalConfig, module *ModuleConfig) (rule *android.RuleBuilder, err error) {

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			} else if e, ok := r.(error); ok {
				err = e
				rule = nil
			} else {
				panic(r)
			}
		}
	}()

	rule = android.NewRuleBuilder()

	generateProfile := module.ProfileClassListing.Valid() && !global.DisableGenerateProfile
	generateBootProfile := module.ProfileBootListing.Valid() && !global.DisableGenerateProfile

	var profile android.WritablePath
	if generateProfile {
		profile = profileCommand(ctx, globalSoong, global, module, rule)
	}
	if generateBootProfile {
		bootProfileCommand(ctx, globalSoong, global, module, rule)
	}

	if !dexpreoptDisabled(ctx, global, module) {
		// Don't preopt individual boot jars, they will be preopted together.
		if !contains(global.BootJars, module.Name) {
			appImage := (generateProfile || module.ForceCreateAppImage || global.DefaultAppImages) &&
				!module.NoCreateAppImage

			generateDM := shouldGenerateDM(module, global)

			for archIdx, _ := range module.Archs {
				dexpreoptCommand(ctx, globalSoong, global, module, rule, archIdx, profile, appImage, generateDM)
			}
		}
	}

	return rule, nil
}

func dexpreoptDisabled(ctx android.PathContext, global *GlobalConfig, module *ModuleConfig) bool {
	if contains(global.DisablePreoptModules, module.Name) {
		return true
	}

	// Don't preopt system server jars that are updatable.
	for _, p := range global.UpdatableSystemServerJars {
		if _, jar := android.SplitApexJarPair(p); jar == module.Name {
			return true
		}
	}

	// Don't preopt system server jars that are not Soong modules.
	if android.InList(module.Name, NonUpdatableSystemServerJars(ctx, global)) {
		if _, ok := ctx.(android.ModuleContext); !ok {
			return true
		}
	}

	// If OnlyPreoptBootImageAndSystemServer=true and module is not in boot class path skip
	// Also preopt system server jars since selinux prevents system server from loading anything from
	// /data. If we don't do this they will need to be extracted which is not favorable for RAM usage
	// or performance. If PreoptExtractedApk is true, we ignore the only preopt boot image options.
	if global.OnlyPreoptBootImageAndSystemServer && !contains(global.BootJars, module.Name) &&
		!contains(global.SystemServerJars, module.Name) && !module.PreoptExtractedApk {
		return true
	}

	return false
}

func profileCommand(ctx android.PathContext, globalSoong *GlobalSoongConfig, global *GlobalConfig,
	module *ModuleConfig, rule *android.RuleBuilder) android.WritablePath {

	profilePath := module.BuildPath.InSameDir(ctx, "profile.prof")
	profileInstalledPath := module.DexLocation + ".prof"

	if !module.ProfileIsTextListing {
		rule.Command().FlagWithOutput("touch ", profilePath)
	}

	cmd := rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(globalSoong.Profman)

	if module.ProfileIsTextListing {
		// The profile is a test listing of classes (used for framework jars).
		// We need to generate the actual binary profile before being able to compile.
		cmd.FlagWithInput("--create-profile-from=", module.ProfileClassListing.Path())
	} else {
		// The profile is binary profile (used for apps). Run it through profman to
		// ensure the profile keys match the apk.
		cmd.
			Flag("--copy-and-update-profile-key").
			FlagWithInput("--profile-file=", module.ProfileClassListing.Path())
	}

	cmd.
		FlagWithInput("--apk=", module.DexPath).
		Flag("--dex-location="+module.DexLocation).
		FlagWithOutput("--reference-profile-file=", profilePath)

	if !module.ProfileIsTextListing {
		cmd.Text(fmt.Sprintf(`|| echo "Profile out of date for %s"`, module.DexPath))
	}
	rule.Install(profilePath, profileInstalledPath)

	return profilePath
}

func bootProfileCommand(ctx android.PathContext, globalSoong *GlobalSoongConfig, global *GlobalConfig,
	module *ModuleConfig, rule *android.RuleBuilder) android.WritablePath {

	profilePath := module.BuildPath.InSameDir(ctx, "profile.bprof")
	profileInstalledPath := module.DexLocation + ".bprof"

	if !module.ProfileIsTextListing {
		rule.Command().FlagWithOutput("touch ", profilePath)
	}

	cmd := rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(globalSoong.Profman)

	// The profile is a test listing of methods.
	// We need to generate the actual binary profile.
	cmd.FlagWithInput("--create-profile-from=", module.ProfileBootListing.Path())

	cmd.
		Flag("--generate-boot-profile").
		FlagWithInput("--apk=", module.DexPath).
		Flag("--dex-location="+module.DexLocation).
		FlagWithOutput("--reference-profile-file=", profilePath)

	if !module.ProfileIsTextListing {
		cmd.Text(fmt.Sprintf(`|| echo "Profile out of date for %s"`, module.DexPath))
	}
	rule.Install(profilePath, profileInstalledPath)

	return profilePath
}

func dexpreoptCommand(ctx android.PathContext, globalSoong *GlobalSoongConfig, global *GlobalConfig,
	module *ModuleConfig, rule *android.RuleBuilder, archIdx int, profile android.WritablePath,
	appImage bool, generateDM bool) {

	arch := module.Archs[archIdx]

	// HACK: make soname in Soong-generated .odex files match Make.
	base := filepath.Base(module.DexLocation)
	if filepath.Ext(base) == ".jar" {
		base = "javalib.jar"
	} else if filepath.Ext(base) == ".apk" {
		base = "package.apk"
	}

	toOdexPath := func(path string) string {
		return filepath.Join(
			filepath.Dir(path),
			"oat",
			arch.String(),
			pathtools.ReplaceExtension(filepath.Base(path), "odex"))
	}

	odexPath := module.BuildPath.InSameDir(ctx, "oat", arch.String(), pathtools.ReplaceExtension(base, "odex"))
	odexInstallPath := toOdexPath(module.DexLocation)
	if odexOnSystemOther(module, global) {
		odexInstallPath = filepath.Join(SystemOtherPartition, odexInstallPath)
	}

	vdexPath := odexPath.ReplaceExtension(ctx, "vdex")
	vdexInstallPath := pathtools.ReplaceExtension(odexInstallPath, "vdex")

	invocationPath := odexPath.ReplaceExtension(ctx, "invocation")

	// The class loader context using paths in the build
	var classLoaderContextHost android.Paths

	// The class loader context using paths as they will be on the device
	var classLoaderContextTarget []string

	// Extra paths that will be appended to the class loader if the APK manifest has targetSdkVersion < 28
	var conditionalClassLoaderContextHost28 android.Paths
	var conditionalClassLoaderContextTarget28 []string

	// Extra paths that will be appended to the class loader if the APK manifest has targetSdkVersion < 29
	var conditionalClassLoaderContextHost29 android.Paths
	var conditionalClassLoaderContextTarget29 []string

	var classLoaderContextHostString, classLoaderContextDeviceString string
	var classLoaderDeps android.Paths

	if module.EnforceUsesLibraries {
		usesLibs := append(copyOf(module.UsesLibraries), module.PresentOptionalUsesLibraries...)

		// Create class loader context for dex2oat from uses libraries and filtered optional libraries
		for _, l := range usesLibs {

			classLoaderContextHost = append(classLoaderContextHost,
				pathForLibrary(module, l))
			classLoaderContextTarget = append(classLoaderContextTarget,
				filepath.Join("/system/framework", l+".jar"))
		}

		const httpLegacy = "org.apache.http.legacy"
		const httpLegacyImpl = "org.apache.http.legacy.impl"

		// org.apache.http.legacy contains classes that were in the default classpath until API 28.  If the
		// targetSdkVersion in the manifest or APK is < 28, and the module does not explicitly depend on
		// org.apache.http.legacy, then implicitly add the classes to the classpath for dexpreopt.  One the
		// device the classes will be in a file called org.apache.http.legacy.impl.jar.
		module.LibraryPaths[httpLegacyImpl] = module.LibraryPaths[httpLegacy]

		if !contains(module.UsesLibraries, httpLegacy) && !contains(module.PresentOptionalUsesLibraries, httpLegacy) {
			conditionalClassLoaderContextHost28 = append(conditionalClassLoaderContextHost28,
				pathForLibrary(module, httpLegacyImpl))
			conditionalClassLoaderContextTarget28 = append(conditionalClassLoaderContextTarget28,
				filepath.Join("/system/framework", httpLegacyImpl+".jar"))
		}

		const hidlBase = "android.hidl.base-V1.0-java"
		const hidlManager = "android.hidl.manager-V1.0-java"

		// android.hidl.base-V1.0-java and android.hidl.manager-V1.0 contain classes that were in the default
		// classpath until API 29.  If the targetSdkVersion in the manifest or APK is < 29 then implicitly add
		// the classes to the classpath for dexpreopt.
		conditionalClassLoaderContextHost29 = append(conditionalClassLoaderContextHost29,
			pathForLibrary(module, hidlManager))
		conditionalClassLoaderContextTarget29 = append(conditionalClassLoaderContextTarget29,
			filepath.Join("/system/framework", hidlManager+".jar"))
		conditionalClassLoaderContextHost29 = append(conditionalClassLoaderContextHost29,
			pathForLibrary(module, hidlBase))
		conditionalClassLoaderContextTarget29 = append(conditionalClassLoaderContextTarget29,
			filepath.Join("/system/framework", hidlBase+".jar"))

		classLoaderContextHostString = strings.Join(classLoaderContextHost.Strings(), ":")
	} else if android.InList(module.Name, NonUpdatableSystemServerJars(ctx, global)) {
		// We expect that all dexpreopted system server jars are Soong modules.
		mctx, isModule := ctx.(android.ModuleContext)
		if !isModule {
			panic("Cannot dexpreopt system server jar that is not a soong module.")
		}

		// System server jars should be dexpreopted together: class loader context of each jar
		// should include preceding jars (which can be found as dependencies of the current jar
		// with a special tag).
		var jarsOnHost android.Paths
		var jarsOnDevice []string
		mctx.VisitDirectDepsWithTag(SystemServerDepTag, func(dep android.Module) {
			depName := mctx.OtherModuleName(dep)
			if jar, ok := dep.(interface{ DexJar() android.Path }); ok {
				jarsOnHost = append(jarsOnHost, jar.DexJar())
				jarsOnDevice = append(jarsOnDevice, "/system/framework/"+depName+".jar")
			} else {
				mctx.ModuleErrorf("module \"%s\" is not a jar", depName)
			}
		})
		classLoaderContextHostString = strings.Join(jarsOnHost.Strings(), ":")
		classLoaderContextDeviceString = strings.Join(jarsOnDevice, ":")
		classLoaderDeps = jarsOnHost
	} else {
		// Pass special class loader context to skip the classpath and collision check.
		// This will get removed once LOCAL_USES_LIBRARIES is enforced.
		// Right now LOCAL_USES_LIBRARIES is opt in, for the case where it's not specified we still default
		// to the &.
		classLoaderContextHostString = `\&`
	}

	rule.Command().FlagWithArg("mkdir -p ", filepath.Dir(odexPath.String()))
	rule.Command().FlagWithOutput("rm -f ", odexPath)
	// Set values in the environment of the rule.  These may be modified by construct_context.sh.
	if classLoaderContextHostString == `\&` {
		rule.Command().Text(`class_loader_context_arg=--class-loader-context=\&`)
		rule.Command().Text(`stored_class_loader_context_arg=""`)
	} else {
		rule.Command().Text("class_loader_context_arg=--class-loader-context=PCL[" + classLoaderContextHostString + "]")
		rule.Command().Text("stored_class_loader_context_arg=--stored-class-loader-context=PCL[" + classLoaderContextDeviceString + "]")
	}

	if module.EnforceUsesLibraries {
		if module.ManifestPath != nil {
			rule.Command().Text(`target_sdk_version="$(`).
				Tool(globalSoong.ManifestCheck).
				Flag("--extract-target-sdk-version").
				Input(module.ManifestPath).
				Text(`)"`)
		} else {
			// No manifest to extract targetSdkVersion from, hope that DexJar is an APK
			rule.Command().Text(`target_sdk_version="$(`).
				Tool(globalSoong.Aapt).
				Flag("dump badging").
				Input(module.DexPath).
				Text(`| grep "targetSdkVersion" | sed -n "s/targetSdkVersion:'\(.*\)'/\1/p"`).
				Text(`)"`)
		}
		rule.Command().Textf(`dex_preopt_host_libraries="%s"`,
			strings.Join(classLoaderContextHost.Strings(), " ")).
			Implicits(classLoaderContextHost)
		rule.Command().Textf(`dex_preopt_target_libraries="%s"`,
			strings.Join(classLoaderContextTarget, " "))
		rule.Command().Textf(`conditional_host_libs_28="%s"`,
			strings.Join(conditionalClassLoaderContextHost28.Strings(), " ")).
			Implicits(conditionalClassLoaderContextHost28)
		rule.Command().Textf(`conditional_target_libs_28="%s"`,
			strings.Join(conditionalClassLoaderContextTarget28, " "))
		rule.Command().Textf(`conditional_host_libs_29="%s"`,
			strings.Join(conditionalClassLoaderContextHost29.Strings(), " ")).
			Implicits(conditionalClassLoaderContextHost29)
		rule.Command().Textf(`conditional_target_libs_29="%s"`,
			strings.Join(conditionalClassLoaderContextTarget29, " "))
		rule.Command().Text("source").Tool(globalSoong.ConstructContext).Input(module.DexPath)
	}

	// Devices that do not have a product partition use a symlink from /product to /system/product.
	// Because on-device dexopt will see dex locations starting with /product, we change the paths
	// to mimic this behavior.
	dexLocationArg := module.DexLocation
	if strings.HasPrefix(dexLocationArg, "/system/product/") {
		dexLocationArg = strings.TrimPrefix(dexLocationArg, "/system")
	}

	cmd := rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(globalSoong.Dex2oat).
		Flag("--avoid-storing-invocation").
		FlagWithOutput("--write-invocation-to=", invocationPath).ImplicitOutput(invocationPath).
		Flag("--runtime-arg").FlagWithArg("-Xms", global.Dex2oatXms).
		Flag("--runtime-arg").FlagWithArg("-Xmx", global.Dex2oatXmx).
		Flag("--runtime-arg").FlagWithInputList("-Xbootclasspath:", module.PreoptBootClassPathDexFiles, ":").
		Flag("--runtime-arg").FlagWithList("-Xbootclasspath-locations:", module.PreoptBootClassPathDexLocations, ":").
		Flag("${class_loader_context_arg}").
		Flag("${stored_class_loader_context_arg}").Implicits(classLoaderDeps).
		FlagWithArg("--boot-image=", strings.Join(module.DexPreoptImageLocations, ":")).Implicits(module.DexPreoptImagesDeps[archIdx].Paths()).
		FlagWithInput("--dex-file=", module.DexPath).
		FlagWithArg("--dex-location=", dexLocationArg).
		FlagWithOutput("--oat-file=", odexPath).ImplicitOutput(vdexPath).
		// Pass an empty directory, dex2oat shouldn't be reading arbitrary files
		FlagWithArg("--android-root=", global.EmptyDirectory).
		FlagWithArg("--instruction-set=", arch.String()).
		FlagWithArg("--instruction-set-variant=", global.CpuVariant[arch]).
		FlagWithArg("--instruction-set-features=", global.InstructionSetFeatures[arch]).
		Flag("--no-generate-debug-info").
		Flag("--generate-build-id").
		Flag("--abort-on-hard-verifier-error").
		Flag("--force-determinism").
		FlagWithArg("--no-inline-from=", "core-oj.jar")

	var preoptFlags []string
	if len(module.PreoptFlags) > 0 {
		preoptFlags = module.PreoptFlags
	} else if len(global.PreoptFlags) > 0 {
		preoptFlags = global.PreoptFlags
	}

	if len(preoptFlags) > 0 {
		cmd.Text(strings.Join(preoptFlags, " "))
	}

	if module.UncompressedDex {
		cmd.FlagWithArg("--copy-dex-files=", "false")
	}

	if !android.PrefixInList(preoptFlags, "--compiler-filter=") {
		var compilerFilter string
		if contains(global.SystemServerJars, module.Name) {
			// Jars of system server, use the product option if it is set, speed otherwise.
			if global.SystemServerCompilerFilter != "" {
				compilerFilter = global.SystemServerCompilerFilter
			} else {
				compilerFilter = "speed"
			}
		} else if contains(global.SpeedApps, module.Name) || contains(global.SystemServerApps, module.Name) {
			// Apps loaded into system server, and apps the product default to being compiled with the
			// 'speed' compiler filter.
			compilerFilter = "speed"
		} else if profile != nil {
			// For non system server jars, use speed-profile when we have a profile.
			compilerFilter = "speed-profile"
		} else if global.DefaultCompilerFilter != "" {
			compilerFilter = global.DefaultCompilerFilter
		} else {
			compilerFilter = "quicken"
		}
		cmd.FlagWithArg("--compiler-filter=", compilerFilter)
	}

	if generateDM {
		cmd.FlagWithArg("--copy-dex-files=", "false")
		dmPath := module.BuildPath.InSameDir(ctx, "generated.dm")
		dmInstalledPath := pathtools.ReplaceExtension(module.DexLocation, "dm")
		tmpPath := module.BuildPath.InSameDir(ctx, "primary.vdex")
		rule.Command().Text("cp -f").Input(vdexPath).Output(tmpPath)
		rule.Command().Tool(globalSoong.SoongZip).
			FlagWithArg("-L", "9").
			FlagWithOutput("-o", dmPath).
			Flag("-j").
			Input(tmpPath)
		rule.Install(dmPath, dmInstalledPath)
	}

	// By default, emit debug info.
	debugInfo := true
	if global.NoDebugInfo {
		// If the global setting suppresses mini-debug-info, disable it.
		debugInfo = false
	}

	// PRODUCT_SYSTEM_SERVER_DEBUG_INFO overrides WITH_DEXPREOPT_DEBUG_INFO.
	// PRODUCT_OTHER_JAVA_DEBUG_INFO overrides WITH_DEXPREOPT_DEBUG_INFO.
	if contains(global.SystemServerJars, module.Name) {
		if global.AlwaysSystemServerDebugInfo {
			debugInfo = true
		} else if global.NeverSystemServerDebugInfo {
			debugInfo = false
		}
	} else {
		if global.AlwaysOtherDebugInfo {
			debugInfo = true
		} else if global.NeverOtherDebugInfo {
			debugInfo = false
		}
	}

	// Never enable on eng.
	if global.IsEng {
		debugInfo = false
	}

	if debugInfo {
		cmd.Flag("--generate-mini-debug-info")
	} else {
		cmd.Flag("--no-generate-mini-debug-info")
	}

	// Set the compiler reason to 'prebuilt' to identify the oat files produced
	// during the build, as opposed to compiled on the device.
	cmd.FlagWithArg("--compilation-reason=", "prebuilt")

	if appImage {
		appImagePath := odexPath.ReplaceExtension(ctx, "art")
		appImageInstallPath := pathtools.ReplaceExtension(odexInstallPath, "art")
		cmd.FlagWithOutput("--app-image-file=", appImagePath).
			FlagWithArg("--image-format=", "lz4")
		if !global.DontResolveStartupStrings {
			cmd.FlagWithArg("--resolve-startup-const-strings=", "true")
		}
		rule.Install(appImagePath, appImageInstallPath)
	}

	if profile != nil {
		cmd.FlagWithInput("--profile-file=", profile)
	}

	rule.Install(odexPath, odexInstallPath)
	rule.Install(vdexPath, vdexInstallPath)
}

func shouldGenerateDM(module *ModuleConfig, global *GlobalConfig) bool {
	// Generating DM files only makes sense for verify, avoid doing for non verify compiler filter APKs.
	// No reason to use a dm file if the dex is already uncompressed.
	return global.GenerateDMFiles && !module.UncompressedDex &&
		contains(module.PreoptFlags, "--compiler-filter=verify")
}

func OdexOnSystemOtherByName(name string, dexLocation string, global *GlobalConfig) bool {
	if !global.HasSystemOther {
		return false
	}

	if global.SanitizeLite {
		return false
	}

	if contains(global.SpeedApps, name) || contains(global.SystemServerApps, name) {
		return false
	}

	for _, f := range global.PatternsOnSystemOther {
		if makefileMatch(filepath.Join(SystemPartition, f), dexLocation) {
			return true
		}
	}

	return false
}

func odexOnSystemOther(module *ModuleConfig, global *GlobalConfig) bool {
	return OdexOnSystemOtherByName(module.Name, module.DexLocation, global)
}

// PathToLocation converts .../system/framework/arm64/boot.art to .../system/framework/boot.art
func PathToLocation(path android.Path, arch android.ArchType) string {
	pathArch := filepath.Base(filepath.Dir(path.String()))
	if pathArch != arch.String() {
		panic(fmt.Errorf("last directory in %q must be %q", path, arch.String()))
	}
	return filepath.Join(filepath.Dir(filepath.Dir(path.String())), filepath.Base(path.String()))
}

func pathForLibrary(module *ModuleConfig, lib string) android.Path {
	path, ok := module.LibraryPaths[lib]
	if !ok {
		panic(fmt.Errorf("unknown library path for %q", lib))
	}
	return path
}

func makefileMatch(pattern, s string) bool {
	percent := strings.IndexByte(pattern, '%')
	switch percent {
	case -1:
		return pattern == s
	case len(pattern) - 1:
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	default:
		panic(fmt.Errorf("unsupported makefile pattern %q", pattern))
	}
}

// Expected format for apexJarValue = <apex name>:<jar name>
func GetJarLocationFromApexJarPair(apexJarValue string) string {
	apex, jar := android.SplitApexJarPair(apexJarValue)
	return filepath.Join("/apex", apex, "javalib", jar+".jar")
}

func GetJarsFromApexJarPairs(apexJarPairs []string) []string {
	modules := make([]string, len(apexJarPairs))
	for i, p := range apexJarPairs {
		_, jar := android.SplitApexJarPair(p)
		modules[i] = jar
	}
	return modules
}

var nonUpdatableSystemServerJarsKey = android.NewOnceKey("nonUpdatableSystemServerJars")

// TODO: eliminate the superficial global config parameter by moving global config definition
// from java subpackage to dexpreopt.
func NonUpdatableSystemServerJars(ctx android.PathContext, global *GlobalConfig) []string {
	return ctx.Config().Once(nonUpdatableSystemServerJarsKey, func() interface{} {
		return android.RemoveListFromList(global.SystemServerJars,
			GetJarsFromApexJarPairs(global.UpdatableSystemServerJars))
	}).([]string)
}

func contains(l []string, s string) bool {
	for _, e := range l {
		if e == s {
			return true
		}
	}
	return false
}

var copyOf = android.CopyOf
