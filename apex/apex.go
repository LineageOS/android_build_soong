// Copyright (C) 2018 The Android Open Source Project
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

package apex

import (
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"
	"android/soong/python"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"
	"github.com/google/blueprint/proptools"
)

var (
	pctx = android.NewPackageContext("android/apex")

	// Create a canned fs config file where all files and directories are
	// by default set to (uid/gid/mode) = (1000/1000/0644)
	// TODO(b/113082813) make this configurable using config.fs syntax
	generateFsConfig = pctx.StaticRule("generateFsConfig", blueprint.RuleParams{
		Command: `echo '/ 1000 1000 0755' > ${out} && ` +
			`echo '/apex_manifest.json 1000 1000 0644' >> ${out} && ` +
			`echo ${ro_paths} | tr ' ' '\n' | awk '{print "/"$$1 " 1000 1000 0644"}' >> ${out} && ` +
			`echo ${exec_paths} | tr ' ' '\n' | awk '{print "/"$$1 " 0 2000 0755"}' >> ${out}`,
		Description: "fs_config ${out}",
	}, "ro_paths", "exec_paths")

	apexManifestRule = pctx.StaticRule("apexManifestRule", blueprint.RuleParams{
		Command: `rm -f $out && ${jsonmodify} $in ` +
			`-a provideNativeLibs ${provideNativeLibs} ` +
			`-a requireNativeLibs ${requireNativeLibs} ` +
			`${opt} ` +
			`-o $out`,
		CommandDeps: []string{"${jsonmodify}"},
		Description: "prepare ${out}",
	}, "provideNativeLibs", "requireNativeLibs", "opt")

	// TODO(b/113233103): make sure that file_contexts is sane, i.e., validate
	// against the binary policy using sefcontext_compiler -p <policy>.

	// TODO(b/114327326): automate the generation of file_contexts
	apexRule = pctx.StaticRule("apexRule", blueprint.RuleParams{
		Command: `rm -rf ${image_dir} && mkdir -p ${image_dir} && ` +
			`(. ${out}.copy_commands) && ` +
			`APEXER_TOOL_PATH=${tool_path} ` +
			`${apexer} --force --manifest ${manifest} ` +
			`--file_contexts ${file_contexts} ` +
			`--canned_fs_config ${canned_fs_config} ` +
			`--payload_type image ` +
			`--key ${key} ${opt_flags} ${image_dir} ${out} `,
		CommandDeps: []string{"${apexer}", "${avbtool}", "${e2fsdroid}", "${merge_zips}",
			"${mke2fs}", "${resize2fs}", "${sefcontext_compile}",
			"${soong_zip}", "${zipalign}", "${aapt2}", "prebuilts/sdk/current/public/android.jar"},
		Rspfile:        "${out}.copy_commands",
		RspfileContent: "${copy_commands}",
		Description:    "APEX ${image_dir} => ${out}",
	}, "tool_path", "image_dir", "copy_commands", "manifest", "file_contexts", "canned_fs_config", "key", "opt_flags")

	zipApexRule = pctx.StaticRule("zipApexRule", blueprint.RuleParams{
		Command: `rm -rf ${image_dir} && mkdir -p ${image_dir} && ` +
			`(. ${out}.copy_commands) && ` +
			`APEXER_TOOL_PATH=${tool_path} ` +
			`${apexer} --force --manifest ${manifest} ` +
			`--payload_type zip ` +
			`${image_dir} ${out} `,
		CommandDeps:    []string{"${apexer}", "${merge_zips}", "${soong_zip}", "${zipalign}", "${aapt2}"},
		Rspfile:        "${out}.copy_commands",
		RspfileContent: "${copy_commands}",
		Description:    "ZipAPEX ${image_dir} => ${out}",
	}, "tool_path", "image_dir", "copy_commands", "manifest")

	apexProtoConvertRule = pctx.AndroidStaticRule("apexProtoConvertRule",
		blueprint.RuleParams{
			Command:     `${aapt2} convert --output-format proto $in -o $out`,
			CommandDeps: []string{"${aapt2}"},
		})

	apexBundleRule = pctx.StaticRule("apexBundleRule", blueprint.RuleParams{
		Command: `${zip2zip} -i $in -o $out ` +
			`apex_payload.img:apex/${abi}.img ` +
			`apex_manifest.json:root/apex_manifest.json ` +
			`AndroidManifest.xml:manifest/AndroidManifest.xml ` +
			`assets/NOTICE.html.gz:assets/NOTICE.html.gz`,
		CommandDeps: []string{"${zip2zip}"},
		Description: "app bundle",
	}, "abi")

	emitApexContentRule = pctx.StaticRule("emitApexContentRule", blueprint.RuleParams{
		Command:        `rm -f ${out} && touch ${out} && (. ${out}.emit_commands)`,
		Rspfile:        "${out}.emit_commands",
		RspfileContent: "${emit_commands}",
		Description:    "Emit APEX image content",
	}, "emit_commands")

	diffApexContentRule = pctx.StaticRule("diffApexContentRule", blueprint.RuleParams{
		Command: `diff --unchanged-group-format='' \` +
			`--changed-group-format='%<' \` +
			`${image_content_file} ${whitelisted_files_file} || (` +
			`echo -e "New unexpected files were added to ${apex_module_name}." ` +
			` "To fix the build run following command:" && ` +
			`echo "system/apex/tools/update_whitelist.sh ${whitelisted_files_file} ${image_content_file}" && ` +
			`exit 1)`,
		Description: "Diff ${image_content_file} and ${whitelisted_files_file}",
	}, "image_content_file", "whitelisted_files_file", "apex_module_name")
)

var imageApexSuffix = ".apex"
var zipApexSuffix = ".zipapex"

var imageApexType = "image"
var zipApexType = "zip"

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	sharedLibTag   = dependencyTag{name: "sharedLib"}
	executableTag  = dependencyTag{name: "executable"}
	javaLibTag     = dependencyTag{name: "javaLib"}
	prebuiltTag    = dependencyTag{name: "prebuilt"}
	testTag        = dependencyTag{name: "test"}
	keyTag         = dependencyTag{name: "key"}
	certificateTag = dependencyTag{name: "certificate"}
	usesTag        = dependencyTag{name: "uses"}
	androidAppTag  = dependencyTag{name: "androidApp"}
)

func init() {
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/java")
	pctx.HostBinToolVariable("apexer", "apexer")
	// ART minimal builds (using the master-art manifest) do not have the "frameworks/base"
	// projects, and hence cannot built 'aapt2'. Use the SDK prebuilt instead.
	hostBinToolVariableWithPrebuilt := func(name, prebuiltDir, tool string) {
		pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
			if !ctx.Config().FrameworksBaseDirExists(ctx) {
				return filepath.Join(prebuiltDir, runtime.GOOS, "bin", tool)
			} else {
				return pctx.HostBinToolPath(ctx, tool).String()
			}
		})
	}
	hostBinToolVariableWithPrebuilt("aapt2", "prebuilts/sdk/tools", "aapt2")
	pctx.HostBinToolVariable("avbtool", "avbtool")
	pctx.HostBinToolVariable("e2fsdroid", "e2fsdroid")
	pctx.HostBinToolVariable("merge_zips", "merge_zips")
	pctx.HostBinToolVariable("mke2fs", "mke2fs")
	pctx.HostBinToolVariable("resize2fs", "resize2fs")
	pctx.HostBinToolVariable("sefcontext_compile", "sefcontext_compile")
	pctx.HostBinToolVariable("soong_zip", "soong_zip")
	pctx.HostBinToolVariable("zip2zip", "zip2zip")
	pctx.HostBinToolVariable("zipalign", "zipalign")
	pctx.HostBinToolVariable("jsonmodify", "jsonmodify")

	android.RegisterModuleType("apex", BundleFactory)
	android.RegisterModuleType("apex_test", testApexBundleFactory)
	android.RegisterModuleType("apex_vndk", vndkApexBundleFactory)
	android.RegisterModuleType("apex_defaults", defaultsFactory)
	android.RegisterModuleType("prebuilt_apex", PrebuiltFactory)

	android.PreDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("apex_vndk_gather", apexVndkGatherMutator).Parallel()
		ctx.BottomUp("apex_vndk_add_deps", apexVndkAddDepsMutator).Parallel()
	})
	android.PostDepsMutators(RegisterPostDepsMutators)

	android.RegisterMakeVarsProvider(pctx, func(ctx android.MakeVarsContext) {
		apexFileContextsInfos := apexFileContextsInfos(ctx.Config())
		sort.Strings(*apexFileContextsInfos)
		ctx.Strict("APEX_FILE_CONTEXTS_INFOS", strings.Join(*apexFileContextsInfos, " "))
	})
}

func RegisterPostDepsMutators(ctx android.RegisterMutatorsContext) {
	ctx.TopDown("apex_deps", apexDepsMutator)
	ctx.BottomUp("apex", apexMutator).Parallel()
	ctx.BottomUp("apex_flattened", apexFlattenedMutator).Parallel()
	ctx.BottomUp("apex_uses", apexUsesMutator).Parallel()
}

var (
	vndkApexListKey   = android.NewOnceKey("vndkApexList")
	vndkApexListMutex sync.Mutex
)

func vndkApexList(config android.Config) map[string]*apexBundle {
	return config.Once(vndkApexListKey, func() interface{} {
		return map[string]*apexBundle{}
	}).(map[string]*apexBundle)
}

// apexVndkGatherMutator gathers "apex_vndk" modules and puts them in a map with vndk_version as a key.
func apexVndkGatherMutator(mctx android.TopDownMutatorContext) {
	if ab, ok := mctx.Module().(*apexBundle); ok && ab.vndkApex {
		if ab.IsNativeBridgeSupported() {
			mctx.PropertyErrorf("native_bridge_supported", "%q doesn't support native bridge binary.", mctx.ModuleType())
		}

		vndkVersion := proptools.String(ab.vndkProperties.Vndk_version)

		vndkApexListMutex.Lock()
		defer vndkApexListMutex.Unlock()
		vndkApexList := vndkApexList(mctx.Config())
		if other, ok := vndkApexList[vndkVersion]; ok {
			mctx.PropertyErrorf("vndk_version", "%v is already defined in %q", vndkVersion, other.BaseModuleName())
		}
		vndkApexList[vndkVersion] = ab
	}
}

// apexVndkAddDepsMutator adds (reverse) dependencies from vndk libs to apex_vndk modules.
// It filters only libs with matching targets.
func apexVndkAddDepsMutator(mctx android.BottomUpMutatorContext) {
	if cc, ok := mctx.Module().(*cc.Module); ok && cc.IsVndkOnSystem() {
		vndkApexList := vndkApexList(mctx.Config())
		if ab, ok := vndkApexList[cc.VndkVersion()]; ok {
			targetArch := cc.Target().String()
			for _, target := range ab.MultiTargets() {
				if target.String() == targetArch {
					mctx.AddReverseDependency(mctx.Module(), sharedLibTag, ab.Name())
					break
				}
			}
		}
	}
}

// Mark the direct and transitive dependencies of apex bundles so that they
// can be built for the apex bundles.
func apexDepsMutator(mctx android.TopDownMutatorContext) {
	if a, ok := mctx.Module().(*apexBundle); ok {
		apexBundleName := mctx.ModuleName()
		mctx.WalkDeps(func(child, parent android.Module) bool {
			depName := mctx.OtherModuleName(child)
			// If the parent is apexBundle, this child is directly depended.
			_, directDep := parent.(*apexBundle)
			if a.installable() && !a.testApex {
				// TODO(b/123892969): Workaround for not having any way to annotate test-apexs
				// non-installable apex's cannot be installed and so should not prevent libraries from being
				// installed to the system.
				android.UpdateApexDependency(apexBundleName, depName, directDep)
			}

			if am, ok := child.(android.ApexModule); ok && am.CanHaveApexVariants() {
				am.BuildForApex(apexBundleName)
				return true
			} else {
				return false
			}
		})
	}
}

// Create apex variations if a module is included in APEX(s).
func apexMutator(mctx android.BottomUpMutatorContext) {
	if am, ok := mctx.Module().(android.ApexModule); ok && am.CanHaveApexVariants() {
		am.CreateApexVariations(mctx)
	} else if a, ok := mctx.Module().(*apexBundle); ok {
		// apex bundle itself is mutated so that it and its modules have same
		// apex variant.
		apexBundleName := mctx.ModuleName()
		mctx.CreateVariations(apexBundleName)

		// collects APEX list
		if mctx.Device() && a.installable() {
			addApexFileContextsInfos(mctx, a)
		}
	}
}

var (
	apexFileContextsInfosKey   = android.NewOnceKey("apexFileContextsInfosKey")
	apexFileContextsInfosMutex sync.Mutex
)

func apexFileContextsInfos(config android.Config) *[]string {
	return config.Once(apexFileContextsInfosKey, func() interface{} {
		return &[]string{}
	}).(*[]string)
}

func addApexFileContextsInfos(ctx android.BaseModuleContext, a *apexBundle) {
	apexName := proptools.StringDefault(a.properties.Apex_name, ctx.ModuleName())
	fileContextsName := proptools.StringDefault(a.properties.File_contexts, ctx.ModuleName())

	apexFileContextsInfosMutex.Lock()
	defer apexFileContextsInfosMutex.Unlock()
	apexFileContextsInfos := apexFileContextsInfos(ctx.Config())
	*apexFileContextsInfos = append(*apexFileContextsInfos, apexName+":"+fileContextsName)
}

func apexFlattenedMutator(mctx android.BottomUpMutatorContext) {
	if ab, ok := mctx.Module().(*apexBundle); ok {
		if !mctx.Config().FlattenApex() || mctx.Config().UnbundledBuild() {
			modules := mctx.CreateLocalVariations("", "flattened")
			modules[0].(*apexBundle).SetFlattened(false)
			modules[1].(*apexBundle).SetFlattened(true)
		} else {
			ab.SetFlattened(true)
			ab.SetFlattenedConfigValue()
		}
	}
}

func apexUsesMutator(mctx android.BottomUpMutatorContext) {
	if ab, ok := mctx.Module().(*apexBundle); ok {
		mctx.AddFarVariationDependencies(nil, usesTag, ab.properties.Uses...)
	}
}

type apexNativeDependencies struct {
	// List of native libraries
	Native_shared_libs []string

	// List of native executables
	Binaries []string

	// List of native tests
	Tests []string
}

type apexMultilibProperties struct {
	// Native dependencies whose compile_multilib is "first"
	First apexNativeDependencies

	// Native dependencies whose compile_multilib is "both"
	Both apexNativeDependencies

	// Native dependencies whose compile_multilib is "prefer32"
	Prefer32 apexNativeDependencies

	// Native dependencies whose compile_multilib is "32"
	Lib32 apexNativeDependencies

	// Native dependencies whose compile_multilib is "64"
	Lib64 apexNativeDependencies
}

type apexBundleProperties struct {
	// Json manifest file describing meta info of this APEX bundle. Default:
	// "apex_manifest.json"
	Manifest *string `android:"path"`

	// AndroidManifest.xml file used for the zip container of this APEX bundle.
	// If unspecified, a default one is automatically generated.
	AndroidManifest *string `android:"path"`

	// Canonical name of the APEX bundle. Used to determine the path to the activated APEX on
	// device (/apex/<apex_name>).
	// If unspecified, defaults to the value of name.
	Apex_name *string

	// Determines the file contexts file for setting security context to each file in this APEX bundle.
	// Specifically, when this is set to <value>, /system/sepolicy/apex/<value>_file_contexts file is
	// used.
	// Default: <name_of_this_module>
	File_contexts *string

	// List of native shared libs that are embedded inside this APEX bundle
	Native_shared_libs []string

	// List of executables that are embedded inside this APEX bundle
	Binaries []string

	// List of java libraries that are embedded inside this APEX bundle
	Java_libs []string

	// List of prebuilt files that are embedded inside this APEX bundle
	Prebuilts []string

	// List of tests that are embedded inside this APEX bundle
	Tests []string

	// Name of the apex_key module that provides the private key to sign APEX
	Key *string

	// The type of APEX to build. Controls what the APEX payload is. Either
	// 'image', 'zip' or 'both'. Default: 'image'.
	Payload_type *string

	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate *string

	// Whether this APEX is installable to one of the partitions. Default: true.
	Installable *bool

	// For native libraries and binaries, use the vendor variant instead of the core (platform) variant.
	// Default is false.
	Use_vendor *bool

	// For telling the apex to ignore special handling for system libraries such as bionic. Default is false.
	Ignore_system_library_special_case *bool

	Multilib apexMultilibProperties

	// List of sanitizer names that this APEX is enabled for
	SanitizerNames []string `blueprint:"mutated"`

	PreventInstall bool `blueprint:"mutated"`

	HideFromMake bool `blueprint:"mutated"`

	// Indicates this APEX provides C++ shared libaries to other APEXes. Default: false.
	Provide_cpp_shared_libs *bool

	// List of providing APEXes' names so that this APEX can depend on provided shared libraries.
	Uses []string

	// A txt file containing list of files that are whitelisted to be included in this APEX.
	Whitelisted_files *string

	// List of APKs to package inside APEX
	Apps []string

	// To distinguish between flattened and non-flattened apex.
	// if set true, then output files are flattened.
	Flattened bool `blueprint:"mutated"`

	// if true, it means that TARGET_FLATTEN_APEX is true and
	// TARGET_BUILD_APPS is false
	FlattenedConfigValue bool `blueprint:"mutated"`

	// List of SDKs that are used to build this APEX. A reference to an SDK should be either
	// `name#version` or `name` which is an alias for `name#current`. If left empty, `platform#current`
	// is implied. This value affects all modules included in this APEX. In other words, they are
	// also built with the SDKs specified here.
	Uses_sdks []string
}

type apexTargetBundleProperties struct {
	Target struct {
		// Multilib properties only for android.
		Android struct {
			Multilib apexMultilibProperties
		}

		// Multilib properties only for host.
		Host struct {
			Multilib apexMultilibProperties
		}

		// Multilib properties only for host linux_bionic.
		Linux_bionic struct {
			Multilib apexMultilibProperties
		}

		// Multilib properties only for host linux_glibc.
		Linux_glibc struct {
			Multilib apexMultilibProperties
		}
	}
}

type apexVndkProperties struct {
	// Indicates VNDK version of which this VNDK APEX bundles VNDK libs. Default is Platform VNDK Version.
	Vndk_version *string
}

type apexFileClass int

const (
	etc apexFileClass = iota
	nativeSharedLib
	nativeExecutable
	shBinary
	pyBinary
	goBinary
	javaSharedLib
	nativeTest
	app
)

type apexPackaging int

const (
	imageApex apexPackaging = iota
	zipApex
	both
)

func (a apexPackaging) image() bool {
	switch a {
	case imageApex, both:
		return true
	}
	return false
}

func (a apexPackaging) zip() bool {
	switch a {
	case zipApex, both:
		return true
	}
	return false
}

func (a apexPackaging) suffix() string {
	switch a {
	case imageApex:
		return imageApexSuffix
	case zipApex:
		return zipApexSuffix
	case both:
		panic(fmt.Errorf("must be either zip or image"))
	default:
		panic(fmt.Errorf("unknown APEX type %d", a))
	}
}

func (a apexPackaging) name() string {
	switch a {
	case imageApex:
		return imageApexType
	case zipApex:
		return zipApexType
	case both:
		panic(fmt.Errorf("must be either zip or image"))
	default:
		panic(fmt.Errorf("unknown APEX type %d", a))
	}
}

func (class apexFileClass) NameInMake() string {
	switch class {
	case etc:
		return "ETC"
	case nativeSharedLib:
		return "SHARED_LIBRARIES"
	case nativeExecutable, shBinary, pyBinary, goBinary:
		return "EXECUTABLES"
	case javaSharedLib:
		return "JAVA_LIBRARIES"
	case nativeTest:
		return "NATIVE_TESTS"
	case app:
		// b/142537672 Why isn't this APP? We want to have full control over
		// the paths and file names of the apk file under the flattend APEX.
		// If this is set to APP, then the paths and file names are modified
		// by the Make build system. For example, it is installed to
		// /system/apex/<apexname>/app/<Appname>/<apexname>.<Appname>/ instead of
		// /system/apex/<apexname>/app/<Appname> because the build system automatically
		// appends module name (which is <apexname>.<Appname> to the path.
		return "ETC"
	default:
		panic(fmt.Errorf("unknown class %d", class))
	}
}

type apexFile struct {
	builtFile  android.Path
	moduleName string
	installDir string
	class      apexFileClass
	module     android.Module
	symlinks   []string
}

type apexBundle struct {
	android.ModuleBase
	android.DefaultableModuleBase
	android.SdkBase

	properties       apexBundleProperties
	targetProperties apexTargetBundleProperties
	vndkProperties   apexVndkProperties

	apexTypes apexPackaging

	bundleModuleFile android.WritablePath
	outputFiles      map[apexPackaging]android.WritablePath
	flattenedOutput  android.InstallPath
	installDir       android.InstallPath

	prebuiltFileToDelete string

	public_key_file  android.Path
	private_key_file android.Path

	container_certificate_file android.Path
	container_private_key_file android.Path

	// list of files to be included in this apex
	filesInfo []apexFile

	// list of module names that this APEX is depending on
	externalDeps []string

	testApex bool
	vndkApex bool

	// intermediate path for apex_manifest.json
	manifestOut android.WritablePath
}

func addDependenciesForNativeModules(ctx android.BottomUpMutatorContext,
	native_shared_libs []string, binaries []string, tests []string,
	arch string, imageVariation string) {
	// Use *FarVariation* to be able to depend on modules having
	// conflicting variations with this module. This is required since
	// arch variant of an APEX bundle is 'common' but it is 'arm' or 'arm64'
	// for native shared libs.
	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: arch},
		{Mutator: "image", Variation: imageVariation},
		{Mutator: "link", Variation: "shared"},
		{Mutator: "version", Variation: ""}, // "" is the non-stub variant
	}, sharedLibTag, native_shared_libs...)

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: arch},
		{Mutator: "image", Variation: imageVariation},
	}, executableTag, binaries...)

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: arch},
		{Mutator: "image", Variation: imageVariation},
		{Mutator: "test_per_src", Variation: ""}, // "" is the all-tests variant
	}, testTag, tests...)
}

func (a *apexBundle) combineProperties(ctx android.BottomUpMutatorContext) {
	if ctx.Os().Class == android.Device {
		proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Android.Multilib, nil)
	} else {
		proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Host.Multilib, nil)
		if ctx.Os().Bionic() {
			proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Linux_bionic.Multilib, nil)
		} else {
			proptools.AppendProperties(&a.properties.Multilib, &a.targetProperties.Target.Linux_glibc.Multilib, nil)
		}
	}
}

func (a *apexBundle) DepsMutator(ctx android.BottomUpMutatorContext) {

	targets := ctx.MultiTargets()
	config := ctx.DeviceConfig()

	a.combineProperties(ctx)

	has32BitTarget := false
	for _, target := range targets {
		if target.Arch.ArchType.Multilib == "lib32" {
			has32BitTarget = true
		}
	}
	for i, target := range targets {
		// When multilib.* is omitted for native_shared_libs, it implies
		// multilib.both.
		ctx.AddFarVariationDependencies([]blueprint.Variation{
			{Mutator: "arch", Variation: target.String()},
			{Mutator: "image", Variation: a.getImageVariation(config)},
			{Mutator: "link", Variation: "shared"},
		}, sharedLibTag, a.properties.Native_shared_libs...)

		// When multilib.* is omitted for tests, it implies
		// multilib.both.
		ctx.AddFarVariationDependencies([]blueprint.Variation{
			{Mutator: "arch", Variation: target.String()},
			{Mutator: "image", Variation: a.getImageVariation(config)},
			{Mutator: "test_per_src", Variation: ""}, // "" is the all-tests variant
		}, testTag, a.properties.Tests...)

		// Add native modules targetting both ABIs
		addDependenciesForNativeModules(ctx,
			a.properties.Multilib.Both.Native_shared_libs,
			a.properties.Multilib.Both.Binaries,
			a.properties.Multilib.Both.Tests,
			target.String(),
			a.getImageVariation(config))

		isPrimaryAbi := i == 0
		if isPrimaryAbi {
			// When multilib.* is omitted for binaries, it implies
			// multilib.first.
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "arch", Variation: target.String()},
				{Mutator: "image", Variation: a.getImageVariation(config)},
			}, executableTag, a.properties.Binaries...)

			// Add native modules targetting the first ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.First.Native_shared_libs,
				a.properties.Multilib.First.Binaries,
				a.properties.Multilib.First.Tests,
				target.String(),
				a.getImageVariation(config))

			// When multilib.* is omitted for prebuilts, it implies multilib.first.
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "arch", Variation: target.String()},
			}, prebuiltTag, a.properties.Prebuilts...)
		}

		switch target.Arch.ArchType.Multilib {
		case "lib32":
			// Add native modules targetting 32-bit ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Lib32.Native_shared_libs,
				a.properties.Multilib.Lib32.Binaries,
				a.properties.Multilib.Lib32.Tests,
				target.String(),
				a.getImageVariation(config))

			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Prefer32.Native_shared_libs,
				a.properties.Multilib.Prefer32.Binaries,
				a.properties.Multilib.Prefer32.Tests,
				target.String(),
				a.getImageVariation(config))
		case "lib64":
			// Add native modules targetting 64-bit ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Lib64.Native_shared_libs,
				a.properties.Multilib.Lib64.Binaries,
				a.properties.Multilib.Lib64.Tests,
				target.String(),
				a.getImageVariation(config))

			if !has32BitTarget {
				addDependenciesForNativeModules(ctx,
					a.properties.Multilib.Prefer32.Native_shared_libs,
					a.properties.Multilib.Prefer32.Binaries,
					a.properties.Multilib.Prefer32.Tests,
					target.String(),
					a.getImageVariation(config))
			}

			if strings.HasPrefix(ctx.ModuleName(), "com.android.runtime") && target.Os.Class == android.Device {
				for _, sanitizer := range ctx.Config().SanitizeDevice() {
					if sanitizer == "hwaddress" {
						addDependenciesForNativeModules(ctx,
							[]string{"libclang_rt.hwasan-aarch64-android"},
							nil, nil, target.String(), a.getImageVariation(config))
						break
					}
				}
			}
		}

	}

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: "android_common"},
	}, javaLibTag, a.properties.Java_libs...)

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: "android_common"},
	}, androidAppTag, a.properties.Apps...)

	if String(a.properties.Key) == "" {
		ctx.ModuleErrorf("key is missing")
		return
	}
	ctx.AddDependency(ctx.Module(), keyTag, String(a.properties.Key))

	cert := android.SrcIsModule(a.getCertString(ctx))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}

	// TODO(jiyong): ensure that all apexes are with non-empty uses_sdks
	if len(a.properties.Uses_sdks) > 0 {
		sdkRefs := []android.SdkRef{}
		for _, str := range a.properties.Uses_sdks {
			parsed := android.ParseSdkRef(ctx, str, "uses_sdks")
			sdkRefs = append(sdkRefs, parsed)
		}
		a.BuildWithSdks(sdkRefs)
	}
}

func (a *apexBundle) getCertString(ctx android.BaseModuleContext) string {
	certificate, overridden := ctx.DeviceConfig().OverrideCertificateFor(ctx.ModuleName())
	if overridden {
		return ":" + certificate
	}
	return String(a.properties.Certificate)
}

func (a *apexBundle) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		if file, ok := a.outputFiles[imageApex]; ok {
			return android.Paths{file}, nil
		} else {
			return nil, nil
		}
	case ".flattened":
		if a.properties.Flattened {
			flattenedApexPath := a.flattenedOutput
			return android.Paths{flattenedApexPath}, nil
		} else {
			return nil, nil
		}
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (a *apexBundle) installable() bool {
	return !a.properties.PreventInstall && (a.properties.Installable == nil || proptools.Bool(a.properties.Installable))
}

func (a *apexBundle) getImageVariation(config android.DeviceConfig) string {
	if config.VndkVersion() != "" && proptools.Bool(a.properties.Use_vendor) {
		return "vendor." + config.PlatformVndkVersion()
	} else {
		return "core"
	}
}

func (a *apexBundle) EnableSanitizer(sanitizerName string) {
	if !android.InList(sanitizerName, a.properties.SanitizerNames) {
		a.properties.SanitizerNames = append(a.properties.SanitizerNames, sanitizerName)
	}
}

func (a *apexBundle) IsSanitizerEnabled(ctx android.BaseModuleContext, sanitizerName string) bool {
	if android.InList(sanitizerName, a.properties.SanitizerNames) {
		return true
	}

	// Then follow the global setting
	globalSanitizerNames := []string{}
	if a.Host() {
		globalSanitizerNames = ctx.Config().SanitizeHost()
	} else {
		arches := ctx.Config().SanitizeDeviceArch()
		if len(arches) == 0 || android.InList(a.Arch().ArchType.Name, arches) {
			globalSanitizerNames = ctx.Config().SanitizeDevice()
		}
	}
	return android.InList(sanitizerName, globalSanitizerNames)
}

func (a *apexBundle) IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool {
	return ctx.Device() && ctx.DeviceConfig().NativeCoverageEnabled()
}

func (a *apexBundle) PreventInstall() {
	a.properties.PreventInstall = true
}

func (a *apexBundle) HideFromMake() {
	a.properties.HideFromMake = true
}

func (a *apexBundle) SetFlattened(flattened bool) {
	a.properties.Flattened = flattened
}

func (a *apexBundle) SetFlattenedConfigValue() {
	a.properties.FlattenedConfigValue = true
}

// isFlattenedVariant returns true when the current module is the flattened
// variant of an apex that has both a flattened and an unflattened variant.
// It returns false when the current module is flattened but there is no
// unflattened variant, which occurs when ctx.Config().FlattenedApex() returns
// true. It can be used to avoid collisions between the install paths of the
// flattened and unflattened variants.
func (a *apexBundle) isFlattenedVariant() bool {
	return a.properties.Flattened && !a.properties.FlattenedConfigValue
}

func getCopyManifestForNativeLibrary(ccMod *cc.Module, config android.Config, handleSpecialLibs bool) (fileToCopy android.Path, dirInApex string) {
	// Decide the APEX-local directory by the multilib of the library
	// In the future, we may query this to the module.
	switch ccMod.Arch().ArchType.Multilib {
	case "lib32":
		dirInApex = "lib"
	case "lib64":
		dirInApex = "lib64"
	}
	dirInApex = filepath.Join(dirInApex, ccMod.RelativeInstallPath())
	if ccMod.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, ccMod.Target().NativeBridgeRelativePath)
	}
	if handleSpecialLibs && cc.InstallToBootstrap(ccMod.BaseModuleName(), config) {
		// Special case for Bionic libs and other libs installed with them. This is
		// to prevent those libs from being included in the search path
		// /apex/com.android.runtime/${LIB}. This exclusion is required because
		// those libs in the Runtime APEX are available via the legacy paths in
		// /system/lib/. By the init process, the libs in the APEX are bind-mounted
		// to the legacy paths and thus will be loaded into the default linker
		// namespace (aka "platform" namespace). If the libs are directly in
		// /apex/com.android.runtime/${LIB} then the same libs will be loaded again
		// into the runtime linker namespace, which will result in double loading of
		// them, which isn't supported.
		dirInApex = filepath.Join(dirInApex, "bionic")
	}

	fileToCopy = ccMod.OutputFile().Path()
	return
}

func getCopyManifestForExecutable(cc *cc.Module) (fileToCopy android.Path, dirInApex string) {
	dirInApex = filepath.Join("bin", cc.RelativeInstallPath())
	if cc.Target().NativeBridge == android.NativeBridgeEnabled {
		dirInApex = filepath.Join(dirInApex, cc.Target().NativeBridgeRelativePath)
	}
	fileToCopy = cc.OutputFile().Path()
	return
}

func getCopyManifestForPyBinary(py *python.Module) (fileToCopy android.Path, dirInApex string) {
	dirInApex = "bin"
	fileToCopy = py.HostToolPath().Path()
	return
}
func getCopyManifestForGoBinary(ctx android.ModuleContext, gb bootstrap.GoBinaryTool) (fileToCopy android.Path, dirInApex string) {
	dirInApex = "bin"
	s, err := filepath.Rel(android.PathForOutput(ctx).String(), gb.InstallPath())
	if err != nil {
		ctx.ModuleErrorf("Unable to use compiled binary at %s", gb.InstallPath())
		return
	}
	fileToCopy = android.PathForOutput(ctx, s)
	return
}

func getCopyManifestForShBinary(sh *android.ShBinary) (fileToCopy android.Path, dirInApex string) {
	dirInApex = filepath.Join("bin", sh.SubDir())
	fileToCopy = sh.OutputFile()
	return
}

func getCopyManifestForJavaLibrary(java *java.Library) (fileToCopy android.Path, dirInApex string) {
	dirInApex = "javalib"
	fileToCopy = java.DexJarFile()
	return
}

func getCopyManifestForPrebuiltJavaLibrary(java *java.Import) (fileToCopy android.Path, dirInApex string) {
	dirInApex = "javalib"
	// The output is only one, but for some reason, ImplementationJars returns Paths, not Path
	implJars := java.ImplementationJars()
	if len(implJars) != 1 {
		panic(fmt.Errorf("java.ImplementationJars() must return single Path, but got: %s",
			strings.Join(implJars.Strings(), ", ")))
	}
	fileToCopy = implJars[0]
	return
}

func getCopyManifestForPrebuiltEtc(prebuilt *android.PrebuiltEtc) (fileToCopy android.Path, dirInApex string) {
	dirInApex = filepath.Join("etc", prebuilt.SubDir())
	fileToCopy = prebuilt.OutputFile()
	return
}

func getCopyManifestForAndroidApp(app *java.AndroidApp, pkgName string) (fileToCopy android.Path, dirInApex string) {
	dirInApex = filepath.Join("app", pkgName)
	fileToCopy = app.OutputFile()
	return
}

// Context "decorator", overriding the InstallBypassMake method to always reply `true`.
type flattenedApexContext struct {
	android.ModuleContext
}

func (c *flattenedApexContext) InstallBypassMake() bool {
	return true
}

func (a *apexBundle) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	filesInfo := []apexFile{}

	if a.properties.Payload_type == nil || *a.properties.Payload_type == "image" {
		a.apexTypes = imageApex
	} else if *a.properties.Payload_type == "zip" {
		a.apexTypes = zipApex
	} else if *a.properties.Payload_type == "both" {
		a.apexTypes = both
	} else {
		ctx.PropertyErrorf("type", "%q is not one of \"image\", \"zip\", or \"both\".", *a.properties.Payload_type)
		return
	}

	if len(a.properties.Tests) > 0 && !a.testApex {
		ctx.PropertyErrorf("tests", "property not allowed in apex module type")
		return
	}

	handleSpecialLibs := !android.Bool(a.properties.Ignore_system_library_special_case)

	// native lib dependencies
	var provideNativeLibs []string
	var requireNativeLibs []string

	// Check if "uses" requirements are met with dependent apexBundles
	var providedNativeSharedLibs []string
	useVendor := proptools.Bool(a.properties.Use_vendor)
	ctx.VisitDirectDepsBlueprint(func(m blueprint.Module) {
		if ctx.OtherModuleDependencyTag(m) != usesTag {
			return
		}
		otherName := ctx.OtherModuleName(m)
		other, ok := m.(*apexBundle)
		if !ok {
			ctx.PropertyErrorf("uses", "%q is not a provider", otherName)
			return
		}
		if proptools.Bool(other.properties.Use_vendor) != useVendor {
			ctx.PropertyErrorf("use_vendor", "%q has different value of use_vendor", otherName)
			return
		}
		if !proptools.Bool(other.properties.Provide_cpp_shared_libs) {
			ctx.PropertyErrorf("uses", "%q does not provide native_shared_libs", otherName)
			return
		}
		providedNativeSharedLibs = append(providedNativeSharedLibs, other.properties.Native_shared_libs...)
	})

	ctx.WalkDepsBlueprint(func(child, parent blueprint.Module) bool {
		depTag := ctx.OtherModuleDependencyTag(child)
		depName := ctx.OtherModuleName(child)
		if _, ok := parent.(*apexBundle); ok {
			// direct dependencies
			switch depTag {
			case sharedLibTag:
				if cc, ok := child.(*cc.Module); ok {
					if cc.HasStubsVariants() {
						provideNativeLibs = append(provideNativeLibs, cc.OutputFile().Path().Base())
					}
					fileToCopy, dirInApex := getCopyManifestForNativeLibrary(cc, ctx.Config(), handleSpecialLibs)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, nativeSharedLib, cc, nil})
					return true
				} else {
					ctx.PropertyErrorf("native_shared_libs", "%q is not a cc_library or cc_library_shared module", depName)
				}
			case executableTag:
				if cc, ok := child.(*cc.Module); ok {
					fileToCopy, dirInApex := getCopyManifestForExecutable(cc)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, nativeExecutable, cc, cc.Symlinks()})
					return true
				} else if sh, ok := child.(*android.ShBinary); ok {
					fileToCopy, dirInApex := getCopyManifestForShBinary(sh)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, shBinary, sh, sh.Symlinks()})
				} else if py, ok := child.(*python.Module); ok && py.HostToolPath().Valid() {
					fileToCopy, dirInApex := getCopyManifestForPyBinary(py)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, pyBinary, py, nil})
				} else if gb, ok := child.(bootstrap.GoBinaryTool); ok && a.Host() {
					fileToCopy, dirInApex := getCopyManifestForGoBinary(ctx, gb)
					// NB: Since go binaries are static we don't need the module for anything here, which is
					// good since the go tool is a blueprint.Module not an android.Module like we would
					// normally use.
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, goBinary, nil, nil})
				} else {
					ctx.PropertyErrorf("binaries", "%q is neither cc_binary, (embedded) py_binary, (host) blueprint_go_binary, (host) bootstrap_go_binary, nor sh_binary", depName)
				}
			case javaLibTag:
				if javaLib, ok := child.(*java.Library); ok {
					fileToCopy, dirInApex := getCopyManifestForJavaLibrary(javaLib)
					if fileToCopy == nil {
						ctx.PropertyErrorf("java_libs", "%q is not configured to be compiled into dex", depName)
					} else {
						filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, javaSharedLib, javaLib, nil})
					}
					return true
				} else if javaLib, ok := child.(*java.Import); ok {
					fileToCopy, dirInApex := getCopyManifestForPrebuiltJavaLibrary(javaLib)
					if fileToCopy == nil {
						ctx.PropertyErrorf("java_libs", "%q does not have a jar output", depName)
					} else {
						filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, javaSharedLib, javaLib, nil})
					}
					return true
				} else {
					ctx.PropertyErrorf("java_libs", "%q of type %q is not supported", depName, ctx.OtherModuleType(child))
				}
			case prebuiltTag:
				if prebuilt, ok := child.(*android.PrebuiltEtc); ok {
					fileToCopy, dirInApex := getCopyManifestForPrebuiltEtc(prebuilt)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, etc, prebuilt, nil})
					return true
				} else {
					ctx.PropertyErrorf("prebuilts", "%q is not a prebuilt_etc module", depName)
				}
			case testTag:
				if ccTest, ok := child.(*cc.Module); ok {
					if ccTest.IsTestPerSrcAllTestsVariation() {
						// Multiple-output test module (where `test_per_src: true`).
						//
						// `ccTest` is the "" ("all tests") variation of a `test_per_src` module.
						// We do not add this variation to `filesInfo`, as it has no output;
						// however, we do add the other variations of this module as indirect
						// dependencies (see below).
						return true
					} else {
						// Single-output test module (where `test_per_src: false`).
						fileToCopy, dirInApex := getCopyManifestForExecutable(ccTest)
						filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, nativeTest, ccTest, nil})
					}
					return true
				} else {
					ctx.PropertyErrorf("tests", "%q is not a cc module", depName)
				}
			case keyTag:
				if key, ok := child.(*apexKey); ok {
					a.private_key_file = key.private_key_file
					a.public_key_file = key.public_key_file
					return false
				} else {
					ctx.PropertyErrorf("key", "%q is not an apex_key module", depName)
				}
			case certificateTag:
				if dep, ok := child.(*java.AndroidAppCertificate); ok {
					a.container_certificate_file = dep.Certificate.Pem
					a.container_private_key_file = dep.Certificate.Key
					return false
				} else {
					ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", depName)
				}
			case android.PrebuiltDepTag:
				// If the prebuilt is force disabled, remember to delete the prebuilt file
				// that might have been installed in the previous builds
				if prebuilt, ok := child.(*Prebuilt); ok && prebuilt.isForceDisabled() {
					a.prebuiltFileToDelete = prebuilt.InstallFilename()
				}
			case androidAppTag:
				if ap, ok := child.(*java.AndroidApp); ok {
					fileToCopy, dirInApex := getCopyManifestForAndroidApp(ap, ctx.DeviceConfig().OverridePackageNameFor(depName))
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, app, ap, nil})
					return true
				} else {
					ctx.PropertyErrorf("apps", "%q is not an android_app module", depName)
				}
			}
		} else {
			// indirect dependencies
			if am, ok := child.(android.ApexModule); ok {
				// We cannot use a switch statement on `depTag` here as the checked
				// tags used below are private (e.g. `cc.sharedDepTag`).
				if cc.IsSharedDepTag(depTag) || cc.IsRuntimeDepTag(depTag) {
					if cc, ok := child.(*cc.Module); ok {
						if android.InList(cc.Name(), providedNativeSharedLibs) {
							// If we're using a shared library which is provided from other APEX,
							// don't include it in this APEX
							return false
						}
						if !a.Host() && (cc.IsStubs() || cc.HasStubsVariants()) {
							// If the dependency is a stubs lib, don't include it in this APEX,
							// but make sure that the lib is installed on the device.
							// In case no APEX is having the lib, the lib is installed to the system
							// partition.
							//
							// Always include if we are a host-apex however since those won't have any
							// system libraries.
							if !android.DirectlyInAnyApex(ctx, cc.Name()) && !android.InList(cc.Name(), a.externalDeps) {
								a.externalDeps = append(a.externalDeps, cc.Name())
							}
							requireNativeLibs = append(requireNativeLibs, cc.OutputFile().Path().Base())
							// Don't track further
							return false
						}
						fileToCopy, dirInApex := getCopyManifestForNativeLibrary(cc, ctx.Config(), handleSpecialLibs)
						filesInfo = append(filesInfo, apexFile{fileToCopy, depName, dirInApex, nativeSharedLib, cc, nil})
						return true
					}
				} else if cc.IsTestPerSrcDepTag(depTag) {
					if cc, ok := child.(*cc.Module); ok {
						fileToCopy, dirInApex := getCopyManifestForExecutable(cc)
						// Handle modules created as `test_per_src` variations of a single test module:
						// use the name of the generated test binary (`fileToCopy`) instead of the name
						// of the original test module (`depName`, shared by all `test_per_src`
						// variations of that module).
						moduleName := filepath.Base(fileToCopy.String())
						filesInfo = append(filesInfo, apexFile{fileToCopy, moduleName, dirInApex, nativeTest, cc, nil})
						return true
					}
				} else if am.CanHaveApexVariants() && am.IsInstallableToApex() {
					ctx.ModuleErrorf("unexpected tag %q for indirect dependency %q", depTag, depName)
				}
			}
		}
		return false
	})

	if a.private_key_file == nil {
		ctx.PropertyErrorf("key", "private_key for %q could not be found", String(a.properties.Key))
		return
	}

	// remove duplicates in filesInfo
	removeDup := func(filesInfo []apexFile) []apexFile {
		encountered := make(map[string]bool)
		result := []apexFile{}
		for _, f := range filesInfo {
			dest := filepath.Join(f.installDir, f.builtFile.Base())
			if !encountered[dest] {
				encountered[dest] = true
				result = append(result, f)
			}
		}
		return result
	}
	filesInfo = removeDup(filesInfo)

	// to have consistent build rules
	sort.Slice(filesInfo, func(i, j int) bool {
		return filesInfo[i].builtFile.String() < filesInfo[j].builtFile.String()
	})

	// check apex_available requirements
	if !ctx.Host() {
		for _, fi := range filesInfo {
			if am, ok := fi.module.(android.ApexModule); ok {
				if !am.AvailableFor(ctx.ModuleName()) {
					ctx.ModuleErrorf("requires %q that is not available for the APEX", fi.module.Name())
					return
				}
			}
		}
	}

	// prepend the name of this APEX to the module names. These names will be the names of
	// modules that will be defined if the APEX is flattened.
	for i := range filesInfo {
		filesInfo[i].moduleName = ctx.ModuleName() + "." + filesInfo[i].moduleName
	}

	a.installDir = android.PathForModuleInstall(ctx, "apex")
	a.filesInfo = filesInfo

	// prepare apex_manifest.json
	a.manifestOut = android.PathForModuleOut(ctx, "apex_manifest.json")
	manifestSrc := android.PathForModuleSrc(ctx, proptools.StringDefault(a.properties.Manifest, "apex_manifest.json"))

	// put dependency({provide|require}NativeLibs) in apex_manifest.json
	provideNativeLibs = android.SortedUniqueStrings(provideNativeLibs)
	requireNativeLibs = android.SortedUniqueStrings(android.RemoveListFromList(requireNativeLibs, provideNativeLibs))

	// apex name can be overridden
	optCommands := []string{}
	if a.properties.Apex_name != nil {
		optCommands = append(optCommands, "-v name "+*a.properties.Apex_name)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:   apexManifestRule,
		Input:  manifestSrc,
		Output: a.manifestOut,
		Args: map[string]string{
			"provideNativeLibs": strings.Join(provideNativeLibs, " "),
			"requireNativeLibs": strings.Join(requireNativeLibs, " "),
			"opt":               strings.Join(optCommands, " "),
		},
	})

	// Temporarily wrap the original `ctx` into a `flattenedApexContext` to have it
	// reply true to `InstallBypassMake()` (thus making the call
	// `android.PathForModuleInstall` below use `android.pathForInstallInMakeDir`
	// instead of `android.PathForOutput`) to return the correct path to the flattened
	// APEX (as its contents is installed by Make, not Soong).
	factx := flattenedApexContext{ctx}
	apexName := proptools.StringDefault(a.properties.Apex_name, ctx.ModuleName())
	a.flattenedOutput = android.PathForModuleInstall(&factx, "apex", apexName)

	if a.apexTypes.zip() {
		a.buildUnflattenedApex(ctx, zipApex)
	}
	if a.apexTypes.image() {
		// Build rule for unflattened APEX is created even when ctx.Config().FlattenApex()
		// is true. This is to support referencing APEX via ":<module_name>" syntax
		// in other modules. It is in AndroidMk where the selection of flattened
		// or unflattened APEX is made.
		a.buildUnflattenedApex(ctx, imageApex)
		a.buildFlattenedApex(ctx)
	}
}

func (a *apexBundle) buildNoticeFile(ctx android.ModuleContext, apexFileName string) android.OptionalPath {
	noticeFiles := []android.Path{}
	for _, f := range a.filesInfo {
		if f.module != nil {
			notice := f.module.NoticeFile()
			if notice.Valid() {
				noticeFiles = append(noticeFiles, notice.Path())
			}
		}
	}
	// append the notice file specified in the apex module itself
	if a.NoticeFile().Valid() {
		noticeFiles = append(noticeFiles, a.NoticeFile().Path())
	}

	if len(noticeFiles) == 0 {
		return android.OptionalPath{}
	}

	return android.BuildNoticeOutput(ctx, a.installDir, apexFileName, android.FirstUniquePaths(noticeFiles)).HtmlGzOutput
}

func (a *apexBundle) buildUnflattenedApex(ctx android.ModuleContext, apexType apexPackaging) {
	cert := String(a.properties.Certificate)
	if cert != "" && android.SrcIsModule(cert) == "" {
		defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
		a.container_certificate_file = defaultDir.Join(ctx, cert+".x509.pem")
		a.container_private_key_file = defaultDir.Join(ctx, cert+".pk8")
	} else if cert == "" {
		pem, key := ctx.Config().DefaultAppCertificate(ctx)
		a.container_certificate_file = pem
		a.container_private_key_file = key
	}

	var abis []string
	for _, target := range ctx.MultiTargets() {
		if len(target.Arch.Abi) > 0 {
			abis = append(abis, target.Arch.Abi[0])
		}
	}

	abis = android.FirstUniqueStrings(abis)

	suffix := apexType.suffix()
	unsignedOutputFile := android.PathForModuleOut(ctx, ctx.ModuleName()+suffix+".unsigned")

	filesToCopy := []android.Path{}
	for _, f := range a.filesInfo {
		filesToCopy = append(filesToCopy, f.builtFile)
	}

	copyCommands := []string{}
	emitCommands := []string{}
	imageContentFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"-content.txt")
	emitCommands = append(emitCommands, "echo ./apex_manifest.json >> "+imageContentFile.String())
	for i, src := range filesToCopy {
		dest := filepath.Join(a.filesInfo[i].installDir, src.Base())
		emitCommands = append(emitCommands, "echo './"+dest+"' >> "+imageContentFile.String())
		dest_path := filepath.Join(android.PathForModuleOut(ctx, "image"+suffix).String(), dest)
		copyCommands = append(copyCommands, "mkdir -p "+filepath.Dir(dest_path))
		copyCommands = append(copyCommands, "cp "+src.String()+" "+dest_path)
		for _, sym := range a.filesInfo[i].symlinks {
			symlinkDest := filepath.Join(filepath.Dir(dest_path), sym)
			copyCommands = append(copyCommands, "ln -s "+filepath.Base(dest)+" "+symlinkDest)
		}
	}
	implicitInputs := append(android.Paths(nil), filesToCopy...)
	implicitInputs = append(implicitInputs, a.manifestOut)

	if a.properties.Whitelisted_files != nil {
		ctx.Build(pctx, android.BuildParams{
			Rule:        emitApexContentRule,
			Implicits:   implicitInputs,
			Output:      imageContentFile,
			Description: "emit apex image content",
			Args: map[string]string{
				"emit_commands": strings.Join(emitCommands, " && "),
			},
		})
		implicitInputs = append(implicitInputs, imageContentFile)
		whitelistedFilesFile := android.PathForModuleSrc(ctx, proptools.String(a.properties.Whitelisted_files))

		phonyOutput := android.PathForModuleOut(ctx, ctx.ModuleName()+"-diff-phony-output")
		ctx.Build(pctx, android.BuildParams{
			Rule:        diffApexContentRule,
			Implicits:   implicitInputs,
			Output:      phonyOutput,
			Description: "diff apex image content",
			Args: map[string]string{
				"whitelisted_files_file": whitelistedFilesFile.String(),
				"image_content_file":     imageContentFile.String(),
				"apex_module_name":       ctx.ModuleName(),
			},
		})

		implicitInputs = append(implicitInputs, phonyOutput)
	}

	outHostBinDir := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "bin").String()
	prebuiltSdkToolsBinDir := filepath.Join("prebuilts", "sdk", "tools", runtime.GOOS, "bin")

	if apexType.image() {
		// files and dirs that will be created in APEX
		var readOnlyPaths []string
		var executablePaths []string // this also includes dirs
		for _, f := range a.filesInfo {
			pathInApex := filepath.Join(f.installDir, f.builtFile.Base())
			if f.installDir == "bin" || strings.HasPrefix(f.installDir, "bin/") {
				executablePaths = append(executablePaths, pathInApex)
				for _, s := range f.symlinks {
					executablePaths = append(executablePaths, filepath.Join(f.installDir, s))
				}
			} else {
				readOnlyPaths = append(readOnlyPaths, pathInApex)
			}
			dir := f.installDir
			for !android.InList(dir, executablePaths) && dir != "" {
				executablePaths = append(executablePaths, dir)
				dir, _ = filepath.Split(dir) // move up to the parent
				if len(dir) > 0 {
					// remove trailing slash
					dir = dir[:len(dir)-1]
				}
			}
		}
		sort.Strings(readOnlyPaths)
		sort.Strings(executablePaths)
		cannedFsConfig := android.PathForModuleOut(ctx, "canned_fs_config")
		ctx.Build(pctx, android.BuildParams{
			Rule:        generateFsConfig,
			Output:      cannedFsConfig,
			Description: "generate fs config",
			Args: map[string]string{
				"ro_paths":   strings.Join(readOnlyPaths, " "),
				"exec_paths": strings.Join(executablePaths, " "),
			},
		})

		fcName := proptools.StringDefault(a.properties.File_contexts, ctx.ModuleName())
		fileContextsPath := "system/sepolicy/apex/" + fcName + "-file_contexts"
		fileContextsOptionalPath := android.ExistentPathForSource(ctx, fileContextsPath)
		if !fileContextsOptionalPath.Valid() {
			ctx.ModuleErrorf("Cannot find file_contexts file: %q", fileContextsPath)
			return
		}
		fileContexts := fileContextsOptionalPath.Path()

		optFlags := []string{}

		// Additional implicit inputs.
		implicitInputs = append(implicitInputs, cannedFsConfig, fileContexts, a.private_key_file, a.public_key_file)
		optFlags = append(optFlags, "--pubkey "+a.public_key_file.String())

		manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
		if overridden {
			optFlags = append(optFlags, "--override_apk_package_name "+manifestPackageName)
		}

		if a.properties.AndroidManifest != nil {
			androidManifestFile := android.PathForModuleSrc(ctx, proptools.String(a.properties.AndroidManifest))
			implicitInputs = append(implicitInputs, androidManifestFile)
			optFlags = append(optFlags, "--android_manifest "+androidManifestFile.String())
		}

		targetSdkVersion := ctx.Config().DefaultAppTargetSdk()
		if targetSdkVersion == ctx.Config().PlatformSdkCodename() &&
			ctx.Config().UnbundledBuild() &&
			!ctx.Config().UnbundledBuildUsePrebuiltSdks() &&
			ctx.Config().IsEnvTrue("UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT") {
			apiFingerprint := java.ApiFingerprintPath(ctx)
			targetSdkVersion += fmt.Sprintf(".$$(cat %s)", apiFingerprint.String())
			implicitInputs = append(implicitInputs, apiFingerprint)
		}
		optFlags = append(optFlags, "--target_sdk_version "+targetSdkVersion)

		noticeFile := a.buildNoticeFile(ctx, ctx.ModuleName()+suffix)
		if noticeFile.Valid() {
			// If there's a NOTICE file, embed it as an asset file in the APEX.
			implicitInputs = append(implicitInputs, noticeFile.Path())
			optFlags = append(optFlags, "--assets_dir "+filepath.Dir(noticeFile.String()))
		}

		if !ctx.Config().UnbundledBuild() && a.installable() {
			// Apexes which are supposed to be installed in builtin dirs(/system, etc)
			// don't need hashtree for activation. Therefore, by removing hashtree from
			// apex bundle (filesystem image in it, to be specific), we can save storage.
			optFlags = append(optFlags, "--no_hashtree")
		}

		if a.properties.Apex_name != nil {
			// If apex_name is set, apexer can skip checking if key name matches with apex name.
			// Note that apex_manifest is also mended.
			optFlags = append(optFlags, "--do_not_check_keyname")
		}

		ctx.Build(pctx, android.BuildParams{
			Rule:        apexRule,
			Implicits:   implicitInputs,
			Output:      unsignedOutputFile,
			Description: "apex (" + apexType.name() + ")",
			Args: map[string]string{
				"tool_path":        outHostBinDir + ":" + prebuiltSdkToolsBinDir,
				"image_dir":        android.PathForModuleOut(ctx, "image"+suffix).String(),
				"copy_commands":    strings.Join(copyCommands, " && "),
				"manifest":         a.manifestOut.String(),
				"file_contexts":    fileContexts.String(),
				"canned_fs_config": cannedFsConfig.String(),
				"key":              a.private_key_file.String(),
				"opt_flags":        strings.Join(optFlags, " "),
			},
		})

		apexProtoFile := android.PathForModuleOut(ctx, ctx.ModuleName()+".pb"+suffix)
		bundleModuleFile := android.PathForModuleOut(ctx, ctx.ModuleName()+suffix+"-base.zip")
		a.bundleModuleFile = bundleModuleFile

		ctx.Build(pctx, android.BuildParams{
			Rule:        apexProtoConvertRule,
			Input:       unsignedOutputFile,
			Output:      apexProtoFile,
			Description: "apex proto convert",
		})

		ctx.Build(pctx, android.BuildParams{
			Rule:        apexBundleRule,
			Input:       apexProtoFile,
			Output:      a.bundleModuleFile,
			Description: "apex bundle module",
			Args: map[string]string{
				"abi": strings.Join(abis, "."),
			},
		})
	} else {
		ctx.Build(pctx, android.BuildParams{
			Rule:        zipApexRule,
			Implicits:   implicitInputs,
			Output:      unsignedOutputFile,
			Description: "apex (" + apexType.name() + ")",
			Args: map[string]string{
				"tool_path":     outHostBinDir + ":" + prebuiltSdkToolsBinDir,
				"image_dir":     android.PathForModuleOut(ctx, "image"+suffix).String(),
				"copy_commands": strings.Join(copyCommands, " && "),
				"manifest":      a.manifestOut.String(),
			},
		})
	}

	a.outputFiles[apexType] = android.PathForModuleOut(ctx, ctx.ModuleName()+suffix)
	ctx.Build(pctx, android.BuildParams{
		Rule:        java.Signapk,
		Description: "signapk",
		Output:      a.outputFiles[apexType],
		Input:       unsignedOutputFile,
		Implicits: []android.Path{
			a.container_certificate_file,
			a.container_private_key_file,
		},
		Args: map[string]string{
			"certificates": a.container_certificate_file.String() + " " + a.container_private_key_file.String(),
			"flags":        "-a 4096", //alignment
		},
	})

	// Install to $OUT/soong/{target,host}/.../apex
	if a.installable() && (!ctx.Config().FlattenApex() || apexType.zip()) && !a.isFlattenedVariant() {
		ctx.InstallFile(a.installDir, ctx.ModuleName()+suffix, a.outputFiles[apexType])
	}
}

func (a *apexBundle) buildFlattenedApex(ctx android.ModuleContext) {
	if a.installable() {
		// For flattened APEX, do nothing but make sure that apex_manifest.json and apex_pubkey are also copied along
		// with other ordinary files.
		a.filesInfo = append(a.filesInfo, apexFile{a.manifestOut, ctx.ModuleName() + ".apex_manifest.json", ".", etc, nil, nil})

		// rename to apex_pubkey
		copiedPubkey := android.PathForModuleOut(ctx, "apex_pubkey")
		ctx.Build(pctx, android.BuildParams{
			Rule:   android.Cp,
			Input:  a.public_key_file,
			Output: copiedPubkey,
		})
		a.filesInfo = append(a.filesInfo, apexFile{copiedPubkey, ctx.ModuleName() + ".apex_pubkey", ".", etc, nil, nil})

		if ctx.Config().FlattenApex() {
			apexName := proptools.StringDefault(a.properties.Apex_name, ctx.ModuleName())
			for _, fi := range a.filesInfo {
				dir := filepath.Join("apex", apexName, fi.installDir)
				target := ctx.InstallFile(android.PathForModuleInstall(ctx, dir), fi.builtFile.Base(), fi.builtFile)
				for _, sym := range fi.symlinks {
					ctx.InstallSymlink(android.PathForModuleInstall(ctx, dir), sym, target)
				}
			}
		}
	}
}

func (a *apexBundle) AndroidMk() android.AndroidMkData {
	if a.properties.HideFromMake {
		return android.AndroidMkData{
			Disabled: true,
		}
	}
	writers := []android.AndroidMkData{}
	if a.apexTypes.image() {
		writers = append(writers, a.androidMkForType(imageApex))
	}
	if a.apexTypes.zip() {
		writers = append(writers, a.androidMkForType(zipApex))
	}
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			for _, data := range writers {
				data.Custom(w, name, prefix, moduleDir, data)
			}
		}}
}

func (a *apexBundle) androidMkForFiles(w io.Writer, apexName, moduleDir string, apexType apexPackaging) []string {
	moduleNames := []string{}

	for _, fi := range a.filesInfo {
		if cc, ok := fi.module.(*cc.Module); ok && cc.Properties.HideFromMake {
			continue
		}
		if a.properties.Flattened && !apexType.image() {
			continue
		}

		var suffix string
		if a.isFlattenedVariant() {
			suffix = ".flattened"
		}

		if !android.InList(fi.moduleName, moduleNames) {
			moduleNames = append(moduleNames, fi.moduleName+suffix)
		}

		fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
		fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
		fmt.Fprintln(w, "LOCAL_MODULE :=", fi.moduleName+suffix)
		// /apex/<apex_name>/{lib|framework|...}
		pathWhenActivated := filepath.Join("$(PRODUCT_OUT)", "apex", apexName, fi.installDir)
		if a.properties.Flattened && apexType.image() {
			// /system/apex/<name>/{lib|framework|...}
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", filepath.Join(a.installDir.ToMakePath().String(),
				apexName, fi.installDir))
			if !a.isFlattenedVariant() {
				fmt.Fprintln(w, "LOCAL_SOONG_SYMBOL_PATH :=", pathWhenActivated)
			}
			if len(fi.symlinks) > 0 {
				fmt.Fprintln(w, "LOCAL_MODULE_SYMLINKS :=", strings.Join(fi.symlinks, " "))
			}

			if fi.module != nil && fi.module.NoticeFile().Valid() {
				fmt.Fprintln(w, "LOCAL_NOTICE_FILE :=", fi.module.NoticeFile().Path().String())
			}
		} else {
			fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", pathWhenActivated)
		}
		fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", fi.builtFile.String())
		fmt.Fprintln(w, "LOCAL_MODULE_CLASS :=", fi.class.NameInMake())
		if fi.module != nil {
			archStr := fi.module.Target().Arch.ArchType.String()
			host := false
			switch fi.module.Target().Os.Class {
			case android.Host:
				if archStr != "common" {
					fmt.Fprintln(w, "LOCAL_MODULE_HOST_ARCH :=", archStr)
				}
				host = true
			case android.HostCross:
				if archStr != "common" {
					fmt.Fprintln(w, "LOCAL_MODULE_HOST_CROSS_ARCH :=", archStr)
				}
				host = true
			case android.Device:
				if archStr != "common" {
					fmt.Fprintln(w, "LOCAL_MODULE_TARGET_ARCH :=", archStr)
				}
			}
			if host {
				makeOs := fi.module.Target().Os.String()
				if fi.module.Target().Os == android.Linux || fi.module.Target().Os == android.LinuxBionic {
					makeOs = "linux"
				}
				fmt.Fprintln(w, "LOCAL_MODULE_HOST_OS :=", makeOs)
				fmt.Fprintln(w, "LOCAL_IS_HOST_MODULE := true")
			}
		}
		if fi.class == javaSharedLib {
			javaModule := fi.module.(*java.Library)
			// soong_java_prebuilt.mk sets LOCAL_MODULE_SUFFIX := .jar  Therefore
			// we need to remove the suffix from LOCAL_MODULE_STEM, otherwise
			// we will have foo.jar.jar
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", strings.TrimSuffix(fi.builtFile.Base(), ".jar"))
			fmt.Fprintln(w, "LOCAL_SOONG_CLASSES_JAR :=", javaModule.ImplementationAndResourcesJars()[0].String())
			fmt.Fprintln(w, "LOCAL_SOONG_HEADER_JAR :=", javaModule.HeaderJars()[0].String())
			fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", fi.builtFile.String())
			fmt.Fprintln(w, "LOCAL_DEX_PREOPT := false")
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_java_prebuilt.mk")
		} else if fi.class == nativeSharedLib || fi.class == nativeExecutable || fi.class == nativeTest {
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", fi.builtFile.Base())
			if cc, ok := fi.module.(*cc.Module); ok {
				if cc.UnstrippedOutputFile() != nil {
					fmt.Fprintln(w, "LOCAL_SOONG_UNSTRIPPED_BINARY :=", cc.UnstrippedOutputFile().String())
				}
				cc.AndroidMkWriteAdditionalDependenciesForSourceAbiDiff(w)
				if cc.CoverageOutputFile().Valid() {
					fmt.Fprintln(w, "LOCAL_PREBUILT_COVERAGE_ARCHIVE :=", cc.CoverageOutputFile().String())
				}
			}
			fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_cc_prebuilt.mk")
		} else {
			fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", fi.builtFile.Base())
			fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
		}
	}
	return moduleNames
}

func (a *apexBundle) androidMkForType(apexType apexPackaging) android.AndroidMkData {
	return android.AndroidMkData{
		Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
			moduleNames := []string{}
			if a.installable() {
				apexName := proptools.StringDefault(a.properties.Apex_name, name)
				moduleNames = a.androidMkForFiles(w, apexName, moduleDir, apexType)
			}

			if a.isFlattenedVariant() {
				name = name + ".flattened"
			}

			if a.properties.Flattened && apexType.image() {
				// Only image APEXes can be flattened.
				fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
				fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
				fmt.Fprintln(w, "LOCAL_MODULE :=", name)
				if len(moduleNames) > 0 {
					fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES :=", strings.Join(moduleNames, " "))
				}
				fmt.Fprintln(w, "include $(BUILD_PHONY_PACKAGE)")
				fmt.Fprintln(w, "$(LOCAL_INSTALLED_MODULE): .KATI_IMPLICIT_OUTPUTS :=", a.flattenedOutput.String())

			} else if !a.isFlattenedVariant() {
				// zip-apex is the less common type so have the name refer to the image-apex
				// only and use {name}.zip if you want the zip-apex
				if apexType == zipApex && a.apexTypes == both {
					name = name + ".zip"
				}
				fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
				fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
				fmt.Fprintln(w, "LOCAL_MODULE :=", name)
				fmt.Fprintln(w, "LOCAL_MODULE_CLASS := ETC") // do we need a new class?
				fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", a.outputFiles[apexType].String())
				fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", a.installDir.ToMakePath().String())
				fmt.Fprintln(w, "LOCAL_MODULE_STEM :=", name+apexType.suffix())
				fmt.Fprintln(w, "LOCAL_UNINSTALLABLE_MODULE :=", !a.installable())
				if len(moduleNames) > 0 {
					fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES +=", strings.Join(moduleNames, " "))
				}
				if len(a.externalDeps) > 0 {
					fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES +=", strings.Join(a.externalDeps, " "))
				}
				if a.prebuiltFileToDelete != "" {
					fmt.Fprintln(w, "LOCAL_POST_INSTALL_CMD :=", "rm -rf "+
						filepath.Join(a.installDir.ToMakePath().String(), a.prebuiltFileToDelete))
				}
				fmt.Fprintln(w, "include $(BUILD_PREBUILT)")

				if apexType == imageApex {
					fmt.Fprintln(w, "ALL_MODULES.$(LOCAL_MODULE).BUNDLE :=", a.bundleModuleFile.String())
				}
			}
		}}
}

func newApexBundle() *apexBundle {
	module := &apexBundle{
		outputFiles: map[apexPackaging]android.WritablePath{},
	}
	module.AddProperties(&module.properties)
	module.AddProperties(&module.targetProperties)
	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase, class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitExecutables()
	})
	android.InitAndroidMultiTargetsArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	android.InitSdkAwareModule(module)
	return module
}

func ApexBundleFactory(testApex bool) android.Module {
	bundle := newApexBundle()
	bundle.testApex = testApex
	return bundle
}

func testApexBundleFactory() android.Module {
	bundle := newApexBundle()
	bundle.testApex = true
	return bundle
}

func BundleFactory() android.Module {
	return newApexBundle()
}

// apex_vndk creates a special variant of apex modules which contains only VNDK libraries.
// If `vndk_version` is specified, the VNDK libraries of the specified VNDK version are gathered automatically.
// If not specified, then the "current" versions are gathered.
func vndkApexBundleFactory() android.Module {
	bundle := newApexBundle()
	bundle.vndkApex = true
	bundle.AddProperties(&bundle.vndkProperties)
	android.AddLoadHook(bundle, func(ctx android.LoadHookContext) {
		ctx.AppendProperties(&struct {
			Compile_multilib *string
		}{
			proptools.StringPtr("both"),
		})

		vndkVersion := proptools.StringDefault(bundle.vndkProperties.Vndk_version, "current")
		if vndkVersion == "current" {
			vndkVersion = ctx.DeviceConfig().PlatformVndkVersion()
			bundle.vndkProperties.Vndk_version = proptools.StringPtr(vndkVersion)
		}

		// Ensure VNDK APEX mount point is formatted as com.android.vndk.v###
		bundle.properties.Apex_name = proptools.StringPtr("com.android.vndk.v" + vndkVersion)
	})
	return bundle
}

//
// Defaults
//
type Defaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func defaultsFactory() android.Module {
	return DefaultsFactory()
}

func DefaultsFactory(props ...interface{}) android.Module {
	module := &Defaults{}

	module.AddProperties(props...)
	module.AddProperties(
		&apexBundleProperties{},
		&apexTargetBundleProperties{},
	)

	android.InitDefaultsModule(module)
	return module
}

//
// Prebuilt APEX
//
type Prebuilt struct {
	android.ModuleBase
	prebuilt android.Prebuilt

	properties PrebuiltProperties

	inputApex       android.Path
	installDir      android.InstallPath
	installFilename string
	outputApex      android.WritablePath
}

type PrebuiltProperties struct {
	// the path to the prebuilt .apex file to import.
	Source       string `blueprint:"mutated"`
	ForceDisable bool   `blueprint:"mutated"`

	Src  *string
	Arch struct {
		Arm struct {
			Src *string
		}
		Arm64 struct {
			Src *string
		}
		X86 struct {
			Src *string
		}
		X86_64 struct {
			Src *string
		}
	}

	Installable *bool
	// Optional name for the installed apex. If unspecified, name of the
	// module is used as the file name
	Filename *string

	// Names of modules to be overridden. Listed modules can only be other binaries
	// (in Make or Soong).
	// This does not completely prevent installation of the overridden binaries, but if both
	// binaries would be installed by default (in PRODUCT_PACKAGES) the other binary will be removed
	// from PRODUCT_PACKAGES.
	Overrides []string
}

func (p *Prebuilt) installable() bool {
	return p.properties.Installable == nil || proptools.Bool(p.properties.Installable)
}

func (p *Prebuilt) DepsMutator(ctx android.BottomUpMutatorContext) {
	// If the device is configured to use flattened APEX, force disable the prebuilt because
	// the prebuilt is a non-flattened one.
	forceDisable := ctx.Config().FlattenApex()

	// Force disable the prebuilts when we are doing unbundled build. We do unbundled build
	// to build the prebuilts themselves.
	forceDisable = forceDisable || ctx.Config().UnbundledBuild()

	// Force disable the prebuilts when coverage is enabled.
	forceDisable = forceDisable || ctx.DeviceConfig().NativeCoverageEnabled()
	forceDisable = forceDisable || ctx.Config().IsEnvTrue("EMMA_INSTRUMENT")

	// b/137216042 don't use prebuilts when address sanitizer is on
	forceDisable = forceDisable || android.InList("address", ctx.Config().SanitizeDevice()) ||
		android.InList("hwaddress", ctx.Config().SanitizeDevice())

	if forceDisable && p.prebuilt.SourceExists() {
		p.properties.ForceDisable = true
		return
	}

	// This is called before prebuilt_select and prebuilt_postdeps mutators
	// The mutators requires that src to be set correctly for each arch so that
	// arch variants are disabled when src is not provided for the arch.
	if len(ctx.MultiTargets()) != 1 {
		ctx.ModuleErrorf("compile_multilib shouldn't be \"both\" for prebuilt_apex")
		return
	}
	var src string
	switch ctx.MultiTargets()[0].Arch.ArchType {
	case android.Arm:
		src = String(p.properties.Arch.Arm.Src)
	case android.Arm64:
		src = String(p.properties.Arch.Arm64.Src)
	case android.X86:
		src = String(p.properties.Arch.X86.Src)
	case android.X86_64:
		src = String(p.properties.Arch.X86_64.Src)
	default:
		ctx.ModuleErrorf("prebuilt_apex does not support %q", ctx.MultiTargets()[0].Arch.String())
		return
	}
	if src == "" {
		src = String(p.properties.Src)
	}
	p.properties.Source = src
}

func (p *Prebuilt) isForceDisabled() bool {
	return p.properties.ForceDisable
}

func (p *Prebuilt) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.outputApex}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (p *Prebuilt) InstallFilename() string {
	return proptools.StringDefault(p.properties.Filename, p.BaseModuleName()+imageApexSuffix)
}

func (p *Prebuilt) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if p.properties.ForceDisable {
		return
	}

	// TODO(jungjw): Check the key validity.
	p.inputApex = p.Prebuilt().SingleSourcePath(ctx)
	p.installDir = android.PathForModuleInstall(ctx, "apex")
	p.installFilename = p.InstallFilename()
	if !strings.HasSuffix(p.installFilename, imageApexSuffix) {
		ctx.ModuleErrorf("filename should end in %s for prebuilt_apex", imageApexSuffix)
	}
	p.outputApex = android.PathForModuleOut(ctx, p.installFilename)
	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Cp,
		Input:  p.inputApex,
		Output: p.outputApex,
	})
	if p.installable() {
		ctx.InstallFile(p.installDir, p.installFilename, p.inputApex)
	}
}

func (p *Prebuilt) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *Prebuilt) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

func (p *Prebuilt) AndroidMkEntries() android.AndroidMkEntries {
	return android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(p.inputApex),
		Include:    "$(BUILD_PREBUILT)",
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", p.installDir.ToMakePath().String())
				entries.SetString("LOCAL_MODULE_STEM", p.installFilename)
				entries.SetBoolIfTrue("LOCAL_UNINSTALLABLE_MODULE", !p.installable())
				entries.AddStrings("LOCAL_OVERRIDES_PACKAGES", p.properties.Overrides...)
			},
		},
	}
}

// prebuilt_apex imports an `.apex` file into the build graph as if it was built with apex.
func PrebuiltFactory() android.Module {
	module := &Prebuilt{}
	module.AddProperties(&module.properties)
	android.InitSingleSourcePrebuiltModule(module, &module.properties, "Source")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}
