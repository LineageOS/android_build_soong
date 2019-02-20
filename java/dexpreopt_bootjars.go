// Copyright 2019 Google Inc. All rights reserved.
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

import (
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/dexpreopt"

	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
)

func init() {
	android.RegisterSingletonType("dex_bootjars", dexpreoptBootJarsFactory)
}

// The image "location" is a symbolic path that with multiarchitecture
// support doesn't really exist on the device. Typically it is
// /system/framework/boot.art and should be the same for all supported
// architectures on the device. The concrete architecture specific
// content actually ends up in a "filename" that contains an
// architecture specific directory name such as arm, arm64, mips,
// mips64, x86, x86_64.
//
// Here are some example values for an x86_64 / x86 configuration:
//
// bootImages["x86_64"] = "out/soong/generic_x86_64/dex_bootjars/system/framework/x86_64/boot.art"
// dexpreopt.PathToLocation(bootImages["x86_64"], "x86_64") = "out/soong/generic_x86_64/dex_bootjars/system/framework/boot.art"
//
// bootImages["x86"] = "out/soong/generic_x86_64/dex_bootjars/system/framework/x86/boot.art"
// dexpreopt.PathToLocation(bootImages["x86"])= "out/soong/generic_x86_64/dex_bootjars/system/framework/boot.art"
//
// The location is passed as an argument to the ART tools like dex2oat instead of the real path. The ART tools
// will then reconstruct the real path, so the rules must have a dependency on the real path.

type bootJarsInfo struct {
	dir        android.OutputPath
	symbolsDir android.OutputPath
	images     map[android.ArchType]android.OutputPath
	installs   map[android.ArchType]android.RuleBuilderInstalls

	vdexInstalls       map[android.ArchType]android.RuleBuilderInstalls
	unstrippedInstalls map[android.ArchType]android.RuleBuilderInstalls
	profileInstalls    android.RuleBuilderInstalls

	global dexpreopt.GlobalConfig

	preoptBootModules     []string
	preoptBootLocations   []string
	preoptBootDex         android.WritablePaths
	allBootModules        []string
	allBootLocations      []string
	bootclasspath         string
	systemServerClasspath string
}

var dexpreoptBootJarsInfoKey = android.NewOnceKey("dexpreoptBootJarsInfoKey")

// dexpreoptBootJarsInfo creates all the paths for singleton files the first time it is called, which may be
// from a ModuleContext that needs to reference a file that will be created by a singleton rule that hasn't
// yet been created.
func dexpreoptBootJarsInfo(ctx android.PathContext) *bootJarsInfo {
	return ctx.Config().Once(dexpreoptBootJarsInfoKey, func() interface{} {

		info := &bootJarsInfo{
			dir:        android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_bootjars"),
			symbolsDir: android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_bootjars_unstripped"),
			images:     make(map[android.ArchType]android.OutputPath),
			installs:   make(map[android.ArchType]android.RuleBuilderInstalls),

			vdexInstalls:       make(map[android.ArchType]android.RuleBuilderInstalls),
			unstrippedInstalls: make(map[android.ArchType]android.RuleBuilderInstalls),
		}

		for _, target := range ctx.Config().Targets[android.Android] {
			info.images[target.Arch.ArchType] = info.dir.Join(ctx,
				"system/framework", target.Arch.ArchType.String(), "boot.art")
		}

		info.global = dexpreoptGlobalConfig(ctx)
		computeBootClasspath(ctx, info)
		computeSystemServerClasspath(ctx, info)

		return info
	}).(*bootJarsInfo)
}

func concat(lists ...[]string) []string {
	var size int
	for _, l := range lists {
		size += len(l)
	}
	ret := make([]string, 0, size)
	for _, l := range lists {
		ret = append(ret, l...)
	}
	return ret
}

func computeBootClasspath(ctx android.PathContext, info *bootJarsInfo) {
	runtimeModules := info.global.RuntimeApexJars
	nonFrameworkModules := concat(runtimeModules, info.global.ProductUpdatableBootModules)
	frameworkModules := android.RemoveListFromList(info.global.BootJars, nonFrameworkModules)

	var nonUpdatableBootModules []string
	var nonUpdatableBootLocations []string

	for _, m := range runtimeModules {
		nonUpdatableBootModules = append(nonUpdatableBootModules, m)
		nonUpdatableBootLocations = append(nonUpdatableBootLocations,
			filepath.Join("/apex/com.android.runtime/javalib", m+".jar"))
	}

	for _, m := range frameworkModules {
		nonUpdatableBootModules = append(nonUpdatableBootModules, m)
		nonUpdatableBootLocations = append(nonUpdatableBootLocations,
			filepath.Join("/system/framework", m+".jar"))
	}

	// The path to bootclasspath dex files needs to be known at module GenerateAndroidBuildAction time, before
	// the bootclasspath modules have been compiled.  Set up known paths for them, the singleton rules will copy
	// them there.
	// TODO: use module dependencies instead
	var nonUpdatableBootDex android.WritablePaths
	for _, m := range nonUpdatableBootModules {
		nonUpdatableBootDex = append(nonUpdatableBootDex,
			android.PathForOutput(ctx, ctx.Config().DeviceName(), "dex_bootjars_input", m+".jar"))
	}

	allBootModules := concat(nonUpdatableBootModules, info.global.ProductUpdatableBootModules)
	allBootLocations := concat(nonUpdatableBootLocations, info.global.ProductUpdatableBootLocations)

	bootclasspath := strings.Join(allBootLocations, ":")

	info.preoptBootModules = nonUpdatableBootModules
	info.preoptBootLocations = nonUpdatableBootLocations
	info.preoptBootDex = nonUpdatableBootDex
	info.allBootModules = allBootModules
	info.allBootLocations = allBootLocations
	info.bootclasspath = bootclasspath
}

func computeSystemServerClasspath(ctx android.PathContext, info *bootJarsInfo) {
	var systemServerClasspathLocations []string
	for _, m := range info.global.SystemServerJars {
		systemServerClasspathLocations = append(systemServerClasspathLocations,
			filepath.Join("/system/framework", m+".jar"))
	}

	info.systemServerClasspath = strings.Join(systemServerClasspathLocations, ":")
}
func dexpreoptBootJarsFactory() android.Singleton {
	return dexpreoptBootJars{}
}

func skipDexpreoptBootJars(ctx android.PathContext) bool {
	if ctx.Config().UnbundledBuild() {
		return true
	}

	if len(ctx.Config().Targets[android.Android]) == 0 {
		// Host-only build
		return true
	}

	return false
}

type dexpreoptBootJars struct{}

// dexpreoptBoot singleton rules
func (dexpreoptBootJars) GenerateBuildActions(ctx android.SingletonContext) {
	if skipDexpreoptBootJars(ctx) {
		return
	}

	info := dexpreoptBootJarsInfo(ctx)

	// Skip recompiling the boot image for the second sanitization phase. We'll get separate paths
	// and invalidate first-stage artifacts which are crucial to SANITIZE_LITE builds.
	// Note: this is technically incorrect. Compiled code contains stack checks which may depend
	//       on ASAN settings.
	if len(ctx.Config().SanitizeDevice()) == 1 &&
		ctx.Config().SanitizeDevice()[0] == "address" &&
		info.global.SanitizeLite {
		return
	}

	bootDexJars := make(android.Paths, len(info.preoptBootModules))

	ctx.VisitAllModules(func(module android.Module) {
		// Collect dex jar paths for the modules listed above.
		if j, ok := module.(Dependency); ok {
			name := ctx.ModuleName(module)
			if i := android.IndexList(name, info.preoptBootModules); i != -1 {
				bootDexJars[i] = j.DexJar()
			}
		}
	})

	var missingDeps []string
	// Ensure all modules were converted to paths
	for i := range bootDexJars {
		if bootDexJars[i] == nil {
			if ctx.Config().AllowMissingDependencies() {
				missingDeps = append(missingDeps, info.preoptBootModules[i])
				bootDexJars[i] = android.PathForOutput(ctx, "missing")
			} else {
				ctx.Errorf("failed to find dex jar path for module %q",
					info.preoptBootModules[i])
			}
		}
	}

	// The path to bootclasspath dex files needs to be known at module GenerateAndroidBuildAction time, before
	// the bootclasspath modules have been compiled.  Copy the dex jars there so the module rules that have
	// already been set up can find them.
	for i := range bootDexJars {
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  bootDexJars[i],
			Output: info.preoptBootDex[i],
		})
	}

	profile := bootImageProfileRule(ctx, info, missingDeps)

	if !ctx.Config().DisableDexPreopt() {
		targets := ctx.Config().Targets[android.Android]
		if ctx.Config().SecondArchIsTranslated() {
			targets = targets[:1]
		}

		for _, target := range targets {
			dexPreoptBootImageRule(ctx, info, target.Arch.ArchType, profile, missingDeps)
		}
	}
}

func dexPreoptBootImageRule(ctx android.SingletonContext, info *bootJarsInfo,
	arch android.ArchType, profile android.Path, missingDeps []string) {

	symbolsDir := info.symbolsDir.Join(ctx, "system/framework", arch.String())
	symbolsFile := symbolsDir.Join(ctx, "boot.oat")
	outputDir := info.dir.Join(ctx, "system/framework", arch.String())
	outputPath := info.images[arch]
	oatLocation := pathtools.ReplaceExtension(dexpreopt.PathToLocation(outputPath.String(), arch), "oat")

	rule := android.NewRuleBuilder()
	rule.MissingDeps(missingDeps)

	rule.Command().Text("mkdir").Flag("-p").Flag(symbolsDir.String())
	rule.Command().Text("rm").Flag("-f").
		Flag(symbolsDir.Join(ctx, "*.art").String()).
		Flag(symbolsDir.Join(ctx, "*.oat").String()).
		Flag(symbolsDir.Join(ctx, "*.invocation").String())
	rule.Command().Text("rm").Flag("-f").
		Flag(outputDir.Join(ctx, "*.art").String()).
		Flag(outputDir.Join(ctx, "*.oat").String()).
		Flag(outputDir.Join(ctx, "*.invocation").String())

	cmd := rule.Command()

	extraFlags := ctx.Config().Getenv("ART_BOOT_IMAGE_EXTRA_ARGS")
	if extraFlags == "" {
		// Use ANDROID_LOG_TAGS to suppress most logging by default...
		cmd.Text(`ANDROID_LOG_TAGS="*:e"`)
	} else {
		// ...unless the boot image is generated specifically for testing, then allow all logging.
		cmd.Text(`ANDROID_LOG_TAGS="*:v"`)
	}

	invocationPath := outputPath.ReplaceExtension(ctx, "invocation")

	cmd.Tool(info.global.Tools.Dex2oat).
		Flag("--avoid-storing-invocation").
		FlagWithOutput("--write-invocation-to=", invocationPath.String()).ImplicitOutput(invocationPath.String()).
		Flag("--runtime-arg").FlagWithArg("-Xms", info.global.Dex2oatImageXms).
		Flag("--runtime-arg").FlagWithArg("-Xmx", info.global.Dex2oatImageXmx)

	if profile == nil {
		cmd.FlagWithArg("--image-classes=", info.global.PreloadedClasses)
	} else {
		cmd.FlagWithArg("--compiler-filter=", "speed-profile")
		cmd.FlagWithInput("--profile-file=", profile.String())
	}

	if info.global.DirtyImageObjects != "" {
		cmd.FlagWithArg("--dirty-image-objects=", info.global.DirtyImageObjects)
	}

	cmd.
		FlagForEachInput("--dex-file=", info.preoptBootDex.Strings()).
		FlagForEachArg("--dex-location=", info.preoptBootLocations).
		Flag("--generate-debug-info").
		Flag("--generate-build-id").
		FlagWithArg("--oat-symbols=", symbolsFile.String()).
		Flag("--strip").
		FlagWithOutput("--oat-file=", outputPath.ReplaceExtension(ctx, "oat").String()).
		FlagWithArg("--oat-location=", oatLocation).
		FlagWithOutput("--image=", outputPath.String()).
		FlagWithArg("--base=", ctx.Config().LibartImgDeviceBaseAddress()).
		FlagWithArg("--instruction-set=", arch.String()).
		FlagWithArg("--instruction-set-variant=", info.global.CpuVariant[arch]).
		FlagWithArg("--instruction-set-features=", info.global.InstructionSetFeatures[arch]).
		FlagWithArg("--android-root=", info.global.EmptyDirectory).
		FlagWithArg("--no-inline-from=", "core-oj.jar").
		Flag("--abort-on-hard-verifier-error")

	if info.global.BootFlags != "" {
		cmd.Flag(info.global.BootFlags)
	}

	if extraFlags != "" {
		cmd.Flag(extraFlags)
	}

	cmd.Textf(`|| ( echo %s ; false )`, proptools.ShellEscape([]string{failureMessage})[0])

	installDir := filepath.Join("/system/framework", arch.String())
	vdexInstallDir := filepath.Join("/system/framework")

	var extraFiles android.WritablePaths
	var vdexInstalls android.RuleBuilderInstalls
	var unstrippedInstalls android.RuleBuilderInstalls

	// dex preopt on the bootclasspath produces multiple files.  The first dex file
	// is converted into to boot.art (to match the legacy assumption that boot.art
	// exists), and the rest are converted to boot-<name>.art.
	// In addition, each .art file has an associated .oat and .vdex file, and an
	// unstripped .oat file
	for i, m := range info.preoptBootModules {
		name := "boot"
		if i != 0 {
			name += "-" + m
		}

		art := outputDir.Join(ctx, name+".art")
		oat := outputDir.Join(ctx, name+".oat")
		vdex := outputDir.Join(ctx, name+".vdex")
		unstrippedOat := symbolsDir.Join(ctx, name+".oat")

		extraFiles = append(extraFiles, art, oat, vdex, unstrippedOat)

		// Install the .oat and .art files.
		rule.Install(art.String(), filepath.Join(installDir, art.Base()))
		rule.Install(oat.String(), filepath.Join(installDir, oat.Base()))

		// The vdex files are identical between architectures, install them to a shared location.  The Make rules will
		// only use the install rules for one architecture, and will create symlinks into the architecture-specific
		// directories.
		vdexInstalls = append(vdexInstalls,
			android.RuleBuilderInstall{vdex.String(), filepath.Join(vdexInstallDir, vdex.Base())})

		// Install the unstripped oat files.  The Make rules will put these in $(TARGET_OUT_UNSTRIPPED)
		unstrippedInstalls = append(unstrippedInstalls,
			android.RuleBuilderInstall{unstrippedOat.String(), filepath.Join(installDir, unstrippedOat.Base())})
	}

	cmd.ImplicitOutputs(extraFiles.Strings())

	rule.Build(pctx, ctx, "bootJarsDexpreopt_"+arch.String(), "dexpreopt boot jars "+arch.String())

	// save output and installed files for makevars
	info.installs[arch] = rule.Installs()
	info.vdexInstalls[arch] = vdexInstalls
	info.unstrippedInstalls[arch] = unstrippedInstalls
}

const failureMessage = `ERROR: Dex2oat failed to compile a boot image.
It is likely that the boot classpath is inconsistent.
Rebuild with ART_BOOT_IMAGE_EXTRA_ARGS="--runtime-arg -verbose:verifier" to see verification errors.`

func bootImageProfileRule(ctx android.SingletonContext, info *bootJarsInfo, missingDeps []string) android.WritablePath {
	if len(info.global.BootImageProfiles) == 0 {
		return nil
	}

	tools := info.global.Tools

	rule := android.NewRuleBuilder()
	rule.MissingDeps(missingDeps)

	var bootImageProfile string
	if len(info.global.BootImageProfiles) > 1 {
		combinedBootImageProfile := info.dir.Join(ctx, "boot-image-profile.txt")
		rule.Command().Text("cat").Inputs(info.global.BootImageProfiles).Output(combinedBootImageProfile.String())
		bootImageProfile = combinedBootImageProfile.String()
	} else {
		bootImageProfile = info.global.BootImageProfiles[0]
	}

	profile := info.dir.Join(ctx, "boot.prof")

	rule.Command().
		Text(`ANDROID_LOG_TAGS="*:e"`).
		Tool(tools.Profman).
		FlagWithArg("--create-profile-from=", bootImageProfile).
		FlagForEachInput("--apk=", info.preoptBootDex.Strings()).
		FlagForEachArg("--dex-location=", info.preoptBootLocations).
		FlagWithOutput("--reference-profile-file=", profile.String())

	rule.Install(profile.String(), "/system/etc/boot-image.prof")

	rule.Build(pctx, ctx, "bootJarsProfile", "profile boot jars")

	info.profileInstalls = rule.Installs()

	return profile
}

func init() {
	android.RegisterMakeVarsProvider(pctx, bootImageMakeVars)
}

// Export paths to Make.  INTERNAL_PLATFORM_HIDDENAPI_FLAGS is used by Make rules in art/ and cts/.
// Both paths are used to call dist-for-goals.
func bootImageMakeVars(ctx android.MakeVarsContext) {
	if skipDexpreoptBootJars(ctx) {
		return
	}

	info := dexpreoptBootJarsInfo(ctx)
	for arch, _ := range info.images {
		ctx.Strict("DEXPREOPT_IMAGE_"+arch.String(), info.images[arch].String())

		var builtInstalled []string
		for _, install := range info.installs[arch] {
			builtInstalled = append(builtInstalled, install.From+":"+install.To)
		}

		var unstrippedBuiltInstalled []string
		for _, install := range info.unstrippedInstalls[arch] {
			unstrippedBuiltInstalled = append(unstrippedBuiltInstalled, install.From+":"+install.To)
		}

		ctx.Strict("DEXPREOPT_IMAGE_BUILT_INSTALLED_"+arch.String(), info.installs[arch].String())
		ctx.Strict("DEXPREOPT_IMAGE_UNSTRIPPED_BUILT_INSTALLED_"+arch.String(), info.unstrippedInstalls[arch].String())
		ctx.Strict("DEXPREOPT_IMAGE_VDEX_BUILT_INSTALLED_"+arch.String(), info.vdexInstalls[arch].String())
	}

	ctx.Strict("DEXPREOPT_IMAGE_PROFILE_BUILT_INSTALLED", info.profileInstalls.String())

	ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_FILES", strings.Join(info.preoptBootDex.Strings(), " "))
	ctx.Strict("DEXPREOPT_BOOTCLASSPATH_DEX_LOCATIONS", strings.Join(info.preoptBootLocations, " "))
	ctx.Strict("PRODUCT_BOOTCLASSPATH", info.bootclasspath)
	ctx.Strict("PRODUCT_SYSTEM_SERVER_CLASSPATH", info.systemServerClasspath)
}
