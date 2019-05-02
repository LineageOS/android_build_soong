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
// dexpreopting and to strip the dex files from the APK or JAR.
//
// It is used in two places; in the dexpeopt_gen binary for modules defined in Make, and directly linked into Soong.
//
// For Make modules it is built into the dexpreopt_gen binary, which is executed as a Make rule using global config and
// module config specified in JSON files.  The binary writes out two shell scripts, only updating them if they have
// changed.  One script takes an APK or JAR as an input and produces a zip file containing any outputs of preopting,
// in the location they should be on the device.  The Make build rules will unzip the zip file into $(PRODUCT_OUT) when
// installing the APK, which will install the preopt outputs into $(PRODUCT_OUT)/system or $(PRODUCT_OUT)/system_other
// as necessary.  The zip file may be empty if preopting was disabled for any reason.  The second script takes an APK or
// JAR as an input and strips the dex files in it as necessary.
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

	"github.com/google/blueprint/pathtools"
)

const SystemPartition = "/system/"
const SystemOtherPartition = "/system_other/"

// GenerateStripRule generates a set of commands that will take an APK or JAR as an input and strip the dex files if
// they are no longer necessary after preopting.
func GenerateStripRule(global GlobalConfig, module ModuleConfig) (rule *android.RuleBuilder, err error) {
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

	tools := global.Tools

	rule = android.NewRuleBuilder()

	strip := shouldStripDex(module, global)

	if strip {
		if global.NeverAllowStripping {
			panic(fmt.Errorf("Stripping requested on %q, though the product does not allow it", module.DexLocation))
		}
		// Only strips if the dex files are not already uncompressed
		rule.Command().
			Textf(`if (zipinfo %s '*.dex' 2>/dev/null | grep -v ' stor ' >/dev/null) ; then`, module.StripInputPath).
			Tool(tools.Zip2zip).FlagWithInput("-i ", module.StripInputPath).FlagWithOutput("-o ", module.StripOutputPath).
			FlagWithArg("-x ", `"classes*.dex"`).
			Textf(`; else cp -f %s %s; fi`, module.StripInputPath, module.StripOutputPath)
	} else {
		rule.Command().Text("cp -f").Input(module.StripInputPath).Output(module.StripOutputPath)
	}

	return rule, nil
}

// GenerateDexpreoptRule generates a set of commands that will preopt a module based on a GlobalConfig and a
// ModuleConfig.  The produced files and their install locations will be available through rule.Installs().
func GenerateDexpreoptRule(ctx android.PathContext,
	global GlobalConfig, module ModuleConfig) (rule *android.RuleBuilder, err error) {

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

	var profile android.WritablePath
	if generateProfile {
		profile = profileCommand(ctx, global, module, rule)
	}

	if !dexpreoptDisabled(global, module) {
		// Don't preopt individual boot jars, they will be preopted together.
		// This check is outside dexpreoptDisabled because they still need to be stripped.
		if !contains(global.BootJars, module.Name) {
			appImage := (generateProfile || module.ForceCreateAppImage || global.DefaultAppImages) &&
				!module.NoCreateAppImage

			generateDM := shouldGenerateDM(module, global)

			for i, arch := range module.Archs {
				image := module.DexPreoptImages[i]
				dexpreoptCommand(ctx, global, module, rule, arch, profile, image, appImage, generateDM)
			}
		}
	}

	return rule, nil
}

func dexpreoptDisabled(global GlobalConfig, module ModuleConfig) bool {
	if contains(global.DisablePreoptModules, module.Name) {
		return true
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

func profileCommand(ctx android.PathContext, global GlobalConfig, module ModuleConfig,
	rule *android.RuleBuilder) android.WritablePath {

	profilePath := module.BuildPath.InSameDir(ctx, "profile.prof")
	profileInstalledPath := module.DexLocation + ".prof"

	if !module.ProfileIsTextListing {
		rule.Command().FlagWithOutput("touch ", profilePath)
	}

	cmd := rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(global.Tools.Profman)

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

func dexpreoptCommand(ctx android.PathContext, global GlobalConfig, module ModuleConfig, rule *android.RuleBuilder,
	arch android.ArchType, profile, bootImage android.Path, appImage, generateDM bool) {

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
		odexInstallPath = strings.Replace(odexInstallPath, SystemPartition, SystemOtherPartition, 1)
	}

	vdexPath := odexPath.ReplaceExtension(ctx, "vdex")
	vdexInstallPath := pathtools.ReplaceExtension(odexInstallPath, "vdex")

	invocationPath := odexPath.ReplaceExtension(ctx, "invocation")

	// bootImage is .../dex_bootjars/system/framework/arm64/boot.art, but dex2oat wants
	// .../dex_bootjars/system/framework/boot.art on the command line
	var bootImageLocation string
	if bootImage != nil {
		bootImageLocation = PathToLocation(bootImage, arch)
	}

	// Lists of used and optional libraries from the build config to be verified against the manifest in the APK
	var verifyUsesLibs []string
	var verifyOptionalUsesLibs []string

	// Lists of used and optional libraries from the build config, with optional libraries that are known to not
	// be present in the current product removed.
	var filteredUsesLibs []string
	var filteredOptionalUsesLibs []string

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

	var classLoaderContextHostString string

	if module.EnforceUsesLibraries {
		verifyUsesLibs = copyOf(module.UsesLibraries)
		verifyOptionalUsesLibs = copyOf(module.OptionalUsesLibraries)

		filteredOptionalUsesLibs = filterOut(global.MissingUsesLibraries, module.OptionalUsesLibraries)
		filteredUsesLibs = append(copyOf(module.UsesLibraries), filteredOptionalUsesLibs...)

		// Create class loader context for dex2oat from uses libraries and filtered optional libraries
		for _, l := range filteredUsesLibs {

			classLoaderContextHost = append(classLoaderContextHost,
				pathForLibrary(module, l))
			classLoaderContextTarget = append(classLoaderContextTarget,
				filepath.Join("/system/framework", l+".jar"))
		}

		const httpLegacy = "org.apache.http.legacy"
		const httpLegacyImpl = "org.apache.http.legacy.impl"

		// Fix up org.apache.http.legacy.impl since it should be org.apache.http.legacy in the manifest.
		replace(verifyUsesLibs, httpLegacyImpl, httpLegacy)
		replace(verifyOptionalUsesLibs, httpLegacyImpl, httpLegacy)

		if !contains(verifyUsesLibs, httpLegacy) && !contains(verifyOptionalUsesLibs, httpLegacy) {
			conditionalClassLoaderContextHost28 = append(conditionalClassLoaderContextHost28,
				pathForLibrary(module, httpLegacyImpl))
			conditionalClassLoaderContextTarget28 = append(conditionalClassLoaderContextTarget28,
				filepath.Join("/system/framework", httpLegacyImpl+".jar"))
		}

		const hidlBase = "android.hidl.base-V1.0-java"
		const hidlManager = "android.hidl.manager-V1.0-java"

		conditionalClassLoaderContextHost29 = append(conditionalClassLoaderContextHost29,
			pathForLibrary(module, hidlManager))
		conditionalClassLoaderContextTarget29 = append(conditionalClassLoaderContextTarget29,
			filepath.Join("/system/framework", hidlManager+".jar"))
		conditionalClassLoaderContextHost29 = append(conditionalClassLoaderContextHost29,
			pathForLibrary(module, hidlBase))
		conditionalClassLoaderContextTarget29 = append(conditionalClassLoaderContextTarget29,
			filepath.Join("/system/framework", hidlBase+".jar"))

		classLoaderContextHostString = strings.Join(classLoaderContextHost.Strings(), ":")
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
	rule.Command().FlagWithArg("class_loader_context_arg=--class-loader-context=", classLoaderContextHostString)
	rule.Command().Text(`stored_class_loader_context_arg=""`)

	if module.EnforceUsesLibraries {
		rule.Command().Textf(`uses_library_names="%s"`, strings.Join(verifyUsesLibs, " "))
		rule.Command().Textf(`optional_uses_library_names="%s"`, strings.Join(verifyOptionalUsesLibs, " "))
		rule.Command().Textf(`aapt_binary="%s"`, global.Tools.Aapt)
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
		rule.Command().Text("source").Tool(global.Tools.VerifyUsesLibraries).Input(module.DexPath)
		rule.Command().Text("source").Tool(global.Tools.ConstructContext)
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
		Tool(global.Tools.Dex2oat).
		Flag("--avoid-storing-invocation").
		FlagWithOutput("--write-invocation-to=", invocationPath).ImplicitOutput(invocationPath).
		Flag("--runtime-arg").FlagWithArg("-Xms", global.Dex2oatXms).
		Flag("--runtime-arg").FlagWithArg("-Xmx", global.Dex2oatXmx).
		Flag("--runtime-arg").FlagWithInputList("-Xbootclasspath:", module.PreoptBootClassPathDexFiles, ":").
		Flag("--runtime-arg").FlagWithList("-Xbootclasspath-locations:", module.PreoptBootClassPathDexLocations, ":").
		Flag("${class_loader_context_arg}").
		Flag("${stored_class_loader_context_arg}").
		FlagWithArg("--boot-image=", bootImageLocation).Implicit(bootImage).
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

	if !anyHavePrefix(preoptFlags, "--compiler-filter=") {
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
		rule.Command().Tool(global.Tools.SoongZip).
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

// Return if the dex file in the APK should be stripped.  If an APK is found to contain uncompressed dex files at
// dex2oat time it will not be stripped even if strip=true.
func shouldStripDex(module ModuleConfig, global GlobalConfig) bool {
	strip := !global.DefaultNoStripping

	if dexpreoptDisabled(global, module) {
		strip = false
	}

	if module.NoStripping {
		strip = false
	}

	// Don't strip modules that are not on the system partition in case the oat/vdex version in system ROM
	// doesn't match the one in other partitions. It needs to be able to fall back to the APK for that case.
	if !strings.HasPrefix(module.DexLocation, SystemPartition) {
		strip = false
	}

	// system_other isn't there for an OTA, so don't strip if module is on system, and odex is on system_other.
	if odexOnSystemOther(module, global) {
		strip = false
	}

	if module.HasApkLibraries {
		strip = false
	}

	// Don't strip with dex files we explicitly uncompress (dexopt will not store the dex code).
	if module.UncompressedDex {
		strip = false
	}

	if shouldGenerateDM(module, global) {
		strip = false
	}

	if module.PresignedPrebuilt {
		// Only strip out files if we can re-sign the package.
		strip = false
	}

	return strip
}

func shouldGenerateDM(module ModuleConfig, global GlobalConfig) bool {
	// Generating DM files only makes sense for verify, avoid doing for non verify compiler filter APKs.
	// No reason to use a dm file if the dex is already uncompressed.
	return global.GenerateDMFiles && !module.UncompressedDex &&
		contains(module.PreoptFlags, "--compiler-filter=verify")
}

func OdexOnSystemOtherByName(name string, dexLocation string, global GlobalConfig) bool {
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

func odexOnSystemOther(module ModuleConfig, global GlobalConfig) bool {
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

func pathForLibrary(module ModuleConfig, lib string) android.Path {
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

func contains(l []string, s string) bool {
	for _, e := range l {
		if e == s {
			return true
		}
	}
	return false
}

// remove all elements in a from b, returning a new slice
func filterOut(a []string, b []string) []string {
	var ret []string
	for _, x := range b {
		if !contains(a, x) {
			ret = append(ret, x)
		}
	}
	return ret
}

func replace(l []string, from, to string) {
	for i := range l {
		if l[i] == from {
			l[i] = to
		}
	}
}

var copyOf = android.CopyOf

func anyHavePrefix(l []string, prefix string) bool {
	for _, x := range l {
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}
