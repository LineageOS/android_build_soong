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

	"android/soong/android"
	"android/soong/cc"
	"android/soong/java"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	pctx = android.NewPackageContext("android/apex")

	// Create a canned fs config file where all files and directories are
	// by default set to (uid/gid/mode) = (1000/1000/0644)
	// TODO(b/113082813) make this configurable using config.fs syntax
	generateFsConfig = pctx.StaticRule("generateFsConfig", blueprint.RuleParams{
		Command: `echo '/ 1000 1000 0755' > ${out} && ` +
			`echo '/manifest.json 1000 1000 0644' >> ${out} && ` +
			`echo ${ro_paths} | tr ' ' '\n' | awk '{print "/"$$1 " 1000 1000 0644"}' >> ${out} && ` +
			`echo ${exec_paths} | tr ' ' '\n' | awk '{print "/"$$1 " 1000 1000 0755"}' >> ${out}`,
		Description: "fs_config ${out}",
	}, "ro_paths", "exec_paths")

	// TODO(b/113233103): make sure that file_contexts is sane, i.e., validate
	// against the binary policy using sefcontext_compiler -p <policy>.

	// TODO(b/114327326): automate the generation of file_contexts
	apexRule = pctx.StaticRule("apexRule", blueprint.RuleParams{
		Command: `rm -rf ${image_dir} && mkdir -p ${image_dir} && ` +
			`(${copy_commands}) && ` +
			`APEXER_TOOL_PATH=${tool_path} ` +
			`${apexer} --verbose --force --manifest ${manifest} ` +
			`--file_contexts ${file_contexts} ` +
			`--canned_fs_config ${canned_fs_config} ` +
			`--key ${key} ${image_dir} ${out} `,
		CommandDeps: []string{"${apexer}", "${avbtool}", "${e2fsdroid}", "${merge_zips}",
			"${mke2fs}", "${resize2fs}", "${sefcontext_compile}",
			"${soong_zip}", "${zipalign}", "${aapt2}"},
		Description: "APEX ${image_dir} => ${out}",
	}, "tool_path", "image_dir", "copy_commands", "manifest", "file_contexts", "canned_fs_config", "key")

	apexProtoConvertRule = pctx.AndroidStaticRule("apexProtoConvertRule",
		blueprint.RuleParams{
			Command:     `${aapt2} convert --output-format proto $in -o $out`,
			CommandDeps: []string{"${aapt2}"},
		})

	apexBundleRule = pctx.StaticRule("apexBundleRule", blueprint.RuleParams{
		Command:     `${zip2zip} -i $in -o $out image.img:apex/${abi}.img manifest.json:root/manifest.json AndroidManifest.xml:manifest/AndroidManifest.xml`,
		CommandDeps: []string{"${zip2zip}"},
		Description: "app bundle",
	}, "abi")
)

var apexSuffix = ".apex"

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	sharedLibTag   = dependencyTag{name: "sharedLib"}
	executableTag  = dependencyTag{name: "executable"}
	javaLibTag     = dependencyTag{name: "javaLib"}
	prebuiltTag    = dependencyTag{name: "prebuilt"}
	keyTag         = dependencyTag{name: "key"}
	certificateTag = dependencyTag{name: "certificate"}
)

func init() {
	pctx.Import("android/soong/common")
	pctx.Import("android/soong/java")
	pctx.HostBinToolVariable("apexer", "apexer")
	// ART minimal builds (using the master-art manifest) do not have the "frameworks/base"
	// projects, and hence cannot built 'aapt2'. Use the SDK prebuilt instead.
	hostBinToolVariableWithPrebuilt := func(name, prebuiltDir, tool string) {
		pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
			if !android.ExistentPathForSource(ctx, "frameworks/base").Valid() {
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

	android.RegisterModuleType("apex", apexBundleFactory)

	android.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.TopDown("apex_deps", apexDepsMutator)
		ctx.BottomUp("apex", apexMutator)
	})
}

// maps a module name to set of apex bundle names that the module should be built for
func apexBundleNamesFor(config android.Config) map[string]map[string]bool {
	return config.Once("apexBundleNames", func() interface{} {
		return make(map[string]map[string]bool)
	}).(map[string]map[string]bool)
}

// Mark the direct and transitive dependencies of apex bundles so that they
// can be built for the apex bundles.
func apexDepsMutator(mctx android.TopDownMutatorContext) {
	if _, ok := mctx.Module().(*apexBundle); ok {
		apexBundleName := mctx.ModuleName()
		mctx.WalkDeps(func(child, parent android.Module) bool {
			if am, ok := child.(android.ApexModule); ok && am.CanHaveApexVariants() {
				moduleName := mctx.OtherModuleName(am) + "-" + am.Target().String()
				bundleNames, ok := apexBundleNamesFor(mctx.Config())[moduleName]
				if !ok {
					bundleNames = make(map[string]bool)
					apexBundleNamesFor(mctx.Config())[moduleName] = bundleNames
				}
				bundleNames[apexBundleName] = true
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
		moduleName := mctx.ModuleName() + "-" + am.Target().String()
		if bundleNames, ok := apexBundleNamesFor(mctx.Config())[moduleName]; ok {
			variations := []string{"platform"}
			for bn := range bundleNames {
				variations = append(variations, bn)
			}
			modules := mctx.CreateVariations(variations...)
			for i, m := range modules {
				if i == 0 {
					continue // platform
				}
				m.(android.ApexModule).BuildForApex(variations[i])
			}
		}
	} else if _, ok := mctx.Module().(*apexBundle); ok {
		// apex bundle itself is mutated so that it and its modules have same
		// apex variant.
		apexBundleName := mctx.ModuleName()
		mctx.CreateVariations(apexBundleName)
	}
}

type apexBundleProperties struct {
	// Json manifest file describing meta info of this APEX bundle. Default:
	// "manifest.json"
	Manifest *string

	// Determines the file contexts file for setting security context to each file in this APEX bundle.
	// Specifically, when this is set to <value>, /system/sepolicy/apex/<value>_file_contexts file is
	// used.
	// Default: <name_of_this_module>
	File_contexts *string

	// List of native shared libs that are embedded inside this APEX bundle
	Native_shared_libs []string

	// List of native executables that are embedded inside this APEX bundle
	Binaries []string

	// List of java libraries that are embedded inside this APEX bundle
	Java_libs []string

	// List of prebuilt files that are embedded inside this APEX bundle
	Prebuilts []string

	// Name of the apex_key module that provides the private key to sign APEX
	Key *string

	// The name of a certificate in the default certificate directory, blank to use the default product certificate,
	// or an android_app_certificate module name in the form ":module".
	Certificate *string

	Multilib struct {
		First struct {
			// List of native libraries whose compile_multilib is "first"
			Native_shared_libs []string
			// List of native executables whose compile_multilib is "first"
			Binaries []string
		}
		Both struct {
			// List of native libraries whose compile_multilib is "both"
			Native_shared_libs []string
			// List of native executables whose compile_multilib is "both"
			Binaries []string
		}
		Prefer32 struct {
			// List of native libraries whose compile_multilib is "prefer32"
			Native_shared_libs []string
			// List of native executables whose compile_multilib is "prefer32"
			Binaries []string
		}
		Lib32 struct {
			// List of native libraries whose compile_multilib is "32"
			Native_shared_libs []string
			// List of native executables whose compile_multilib is "32"
			Binaries []string
		}
		Lib64 struct {
			// List of native libraries whose compile_multilib is "64"
			Native_shared_libs []string
			// List of native executables whose compile_multilib is "64"
			Binaries []string
		}
	}
}

type apexFileClass int

const (
	etc apexFileClass = iota
	nativeSharedLib
	nativeExecutable
	javaSharedLib
)

func (class apexFileClass) NameInMake() string {
	switch class {
	case etc:
		return "ETC"
	case nativeSharedLib:
		return "SHARED_LIBRARIES"
	case nativeExecutable:
		return "EXECUTABLES"
	case javaSharedLib:
		return "JAVA_LIBRARIES"
	default:
		panic(fmt.Errorf("unkonwn class %d", class))
	}
}

type apexFile struct {
	builtFile  android.Path
	moduleName string
	archType   android.ArchType
	installDir string
	class      apexFileClass
}

type apexBundle struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties apexBundleProperties

	bundleModuleFile android.WritablePath
	outputFile       android.WritablePath
	installDir       android.OutputPath

	// list of files to be included in this apex
	filesInfo []apexFile

	flattened bool
}

func addDependenciesForNativeModules(ctx android.BottomUpMutatorContext,
	native_shared_libs []string, binaries []string, arch string) {
	// Use *FarVariation* to be able to depend on modules having
	// conflicting variations with this module. This is required since
	// arch variant of an APEX bundle is 'common' but it is 'arm' or 'arm64'
	// for native shared libs.
	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: arch},
		{Mutator: "image", Variation: "core"},
		{Mutator: "link", Variation: "shared"},
	}, sharedLibTag, native_shared_libs...)

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: arch},
		{Mutator: "image", Variation: "core"},
	}, executableTag, binaries...)
}

func (a *apexBundle) DepsMutator(ctx android.BottomUpMutatorContext) {
	targets := ctx.MultiTargets()
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
			{Mutator: "image", Variation: "core"},
			{Mutator: "link", Variation: "shared"},
		}, sharedLibTag, a.properties.Native_shared_libs...)

		// Add native modules targetting both ABIs
		addDependenciesForNativeModules(ctx,
			a.properties.Multilib.Both.Native_shared_libs,
			a.properties.Multilib.Both.Binaries, target.String())

		if i == 0 {
			// When multilib.* is omitted for binaries, it implies
			// multilib.first.
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{Mutator: "arch", Variation: target.String()},
				{Mutator: "image", Variation: "core"},
			}, executableTag, a.properties.Binaries...)

			// Add native modules targetting the first ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.First.Native_shared_libs,
				a.properties.Multilib.First.Binaries, target.String())
		}

		switch target.Arch.ArchType.Multilib {
		case "lib32":
			// Add native modules targetting 32-bit ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Lib32.Native_shared_libs,
				a.properties.Multilib.Lib32.Binaries, target.String())

			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Prefer32.Native_shared_libs,
				a.properties.Multilib.Prefer32.Binaries, target.String())
		case "lib64":
			// Add native modules targetting 64-bit ABI
			addDependenciesForNativeModules(ctx,
				a.properties.Multilib.Lib64.Native_shared_libs,
				a.properties.Multilib.Lib64.Binaries, target.String())

			if !has32BitTarget {
				addDependenciesForNativeModules(ctx,
					a.properties.Multilib.Prefer32.Native_shared_libs,
					a.properties.Multilib.Prefer32.Binaries, target.String())
			}
		}

	}

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: "android_common"},
	}, javaLibTag, a.properties.Java_libs...)

	ctx.AddFarVariationDependencies([]blueprint.Variation{
		{Mutator: "arch", Variation: "android_common"},
	}, prebuiltTag, a.properties.Prebuilts...)

	if String(a.properties.Key) == "" {
		ctx.ModuleErrorf("key is missing")
		return
	}
	ctx.AddDependency(ctx.Module(), keyTag, String(a.properties.Key))

	cert := android.SrcIsModule(String(a.properties.Certificate))
	if cert != "" {
		ctx.AddDependency(ctx.Module(), certificateTag, cert)
	}
}

func getCopyManifestForNativeLibrary(cc *cc.Module) (fileToCopy android.Path, dirInApex string) {
	// Decide the APEX-local directory by the multilib of the library
	// In the future, we may query this to the module.
	switch cc.Arch().ArchType.Multilib {
	case "lib32":
		dirInApex = "lib"
	case "lib64":
		dirInApex = "lib64"
	}
	if !cc.Arch().Native {
		dirInApex = filepath.Join(dirInApex, cc.Arch().ArchType.String())
	}

	fileToCopy = cc.OutputFile().Path()
	return
}

func getCopyManifestForExecutable(cc *cc.Module) (fileToCopy android.Path, dirInApex string) {
	dirInApex = "bin"
	fileToCopy = cc.OutputFile().Path()
	return
}

func getCopyManifestForJavaLibrary(java *java.Library) (fileToCopy android.Path, dirInApex string) {
	dirInApex = "javalib"
	fileToCopy = java.DexJarFile()
	return
}

func getCopyManifestForPrebuiltEtc(prebuilt *android.PrebuiltEtc) (fileToCopy android.Path, dirInApex string) {
	dirInApex = filepath.Join("etc", prebuilt.SubDir())
	fileToCopy = prebuilt.OutputFile()
	return
}

func (a *apexBundle) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	filesInfo := []apexFile{}

	var keyFile android.Path
	var certificate java.Certificate

	ctx.WalkDeps(func(child, parent android.Module) bool {
		if _, ok := parent.(*apexBundle); ok {
			// direct dependencies
			depTag := ctx.OtherModuleDependencyTag(child)
			depName := ctx.OtherModuleName(child)
			switch depTag {
			case sharedLibTag:
				if cc, ok := child.(*cc.Module); ok {
					fileToCopy, dirInApex := getCopyManifestForNativeLibrary(cc)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, cc.Arch().ArchType, dirInApex, nativeSharedLib})
					return true
				} else {
					ctx.PropertyErrorf("native_shared_libs", "%q is not a cc_library or cc_library_shared module", depName)
				}
			case executableTag:
				if cc, ok := child.(*cc.Module); ok {
					fileToCopy, dirInApex := getCopyManifestForExecutable(cc)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, cc.Arch().ArchType, dirInApex, nativeExecutable})
					return true
				} else {
					ctx.PropertyErrorf("binaries", "%q is not a cc_binary module", depName)
				}
			case javaLibTag:
				if java, ok := child.(*java.Library); ok {
					fileToCopy, dirInApex := getCopyManifestForJavaLibrary(java)
					if fileToCopy == nil {
						ctx.PropertyErrorf("java_libs", "%q is not configured to be compiled into dex", depName)
					} else {
						filesInfo = append(filesInfo, apexFile{fileToCopy, depName, java.Arch().ArchType, dirInApex, javaSharedLib})
					}
					return true
				} else {
					ctx.PropertyErrorf("java_libs", "%q is not a java_library module", depName)
				}
			case prebuiltTag:
				if prebuilt, ok := child.(*android.PrebuiltEtc); ok {
					fileToCopy, dirInApex := getCopyManifestForPrebuiltEtc(prebuilt)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, prebuilt.Arch().ArchType, dirInApex, etc})
					return true
				} else {
					ctx.PropertyErrorf("prebuilts", "%q is not a prebuilt_etc module", depName)
				}
			case keyTag:
				if key, ok := child.(*apexKey); ok {
					keyFile = key.private_key_file
					return false
				} else {
					ctx.PropertyErrorf("key", "%q is not an apex_key module", depName)
				}
			case certificateTag:
				if dep, ok := child.(*java.AndroidAppCertificate); ok {
					certificate = dep.Certificate
					return false
				} else {
					ctx.ModuleErrorf("certificate dependency %q must be an android_app_certificate module", depName)
				}
			}
		} else {
			// indirect dependencies
			if am, ok := child.(android.ApexModule); ok && am.CanHaveApexVariants() && am.IsInstallableToApex() {
				if cc, ok := child.(*cc.Module); ok {
					depName := ctx.OtherModuleName(child)
					fileToCopy, dirInApex := getCopyManifestForNativeLibrary(cc)
					filesInfo = append(filesInfo, apexFile{fileToCopy, depName, cc.Arch().ArchType, dirInApex, nativeSharedLib})
					return true
				}
			}
		}
		return false
	})

	if keyFile == nil {
		ctx.PropertyErrorf("key", "private_key for %q could not be found", String(a.properties.Key))
		return
	}

	// remove duplicates in filesInfo
	removeDup := func(filesInfo []apexFile) []apexFile {
		encountered := make(map[android.Path]bool)
		result := []apexFile{}
		for _, f := range filesInfo {
			if !encountered[f.builtFile] {
				encountered[f.builtFile] = true
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

	// prepend the name of this APEX to the module names. These names will be the names of
	// modules that will be defined if the APEX is flattened.
	for i := range filesInfo {
		filesInfo[i].moduleName = ctx.ModuleName() + "." + filesInfo[i].moduleName
	}

	a.flattened = ctx.Config().FlattenApex() && !ctx.Config().UnbundledBuild()
	a.installDir = android.PathForModuleInstall(ctx, "apex")
	a.filesInfo = filesInfo
	if ctx.Config().FlattenApex() {
		a.buildFlattenedApex(ctx)
	} else {
		a.buildUnflattenedApex(ctx, keyFile, certificate)
	}
}

func (a *apexBundle) buildUnflattenedApex(ctx android.ModuleContext, keyFile android.Path, certificate java.Certificate) {
	cert := String(a.properties.Certificate)
	if cert != "" && android.SrcIsModule(cert) == "" {
		defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
		certificate = java.Certificate{
			defaultDir.Join(ctx, cert+".x509.pem"),
			defaultDir.Join(ctx, cert+".pk8"),
		}
	} else if cert == "" {
		pem, key := ctx.Config().DefaultAppCertificate(ctx)
		certificate = java.Certificate{pem, key}
	}

	// files and dirs that will be created in apex
	var readOnlyPaths []string
	var executablePaths []string // this also includes dirs
	for _, f := range a.filesInfo {
		pathInApex := filepath.Join(f.installDir, f.builtFile.Base())
		if f.installDir == "bin" {
			executablePaths = append(executablePaths, pathInApex)
		} else {
			readOnlyPaths = append(readOnlyPaths, pathInApex)
		}
		if !android.InList(f.installDir, executablePaths) {
			executablePaths = append(executablePaths, f.installDir)
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

	manifest := android.PathForModuleSrc(ctx, proptools.StringDefault(a.properties.Manifest, "manifest.json"))

	fcName := proptools.StringDefault(a.properties.File_contexts, ctx.ModuleName())
	fileContextsPath := "system/sepolicy/apex/" + fcName + "-file_contexts"
	fileContextsOptionalPath := android.ExistentPathForSource(ctx, fileContextsPath)
	if !fileContextsOptionalPath.Valid() {
		ctx.ModuleErrorf("Cannot find file_contexts file: %q", fileContextsPath)
		return
	}
	fileContexts := fileContextsOptionalPath.Path()

	unsignedOutputFile := android.PathForModuleOut(ctx, ctx.ModuleName()+apexSuffix+".unsigned")

	filesToCopy := []android.Path{}
	for _, f := range a.filesInfo {
		filesToCopy = append(filesToCopy, f.builtFile)
	}

	copyCommands := []string{}
	for i, src := range filesToCopy {
		dest := filepath.Join(a.filesInfo[i].installDir, src.Base())
		dest_path := filepath.Join(android.PathForModuleOut(ctx, "image").String(), dest)
		copyCommands = append(copyCommands, "mkdir -p "+filepath.Dir(dest_path))
		copyCommands = append(copyCommands, "cp "+src.String()+" "+dest_path)
	}
	implicitInputs := append(android.Paths(nil), filesToCopy...)
	implicitInputs = append(implicitInputs, cannedFsConfig, manifest, fileContexts, keyFile)
	outHostBinDir := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "bin").String()
	prebuiltSdkToolsBinDir := filepath.Join("prebuilts", "sdk", "tools", runtime.GOOS, "bin")
	ctx.Build(pctx, android.BuildParams{
		Rule:        apexRule,
		Implicits:   implicitInputs,
		Output:      unsignedOutputFile,
		Description: "apex",
		Args: map[string]string{
			"tool_path":        outHostBinDir + ":" + prebuiltSdkToolsBinDir,
			"image_dir":        android.PathForModuleOut(ctx, "image").String(),
			"copy_commands":    strings.Join(copyCommands, " && "),
			"manifest":         manifest.String(),
			"file_contexts":    fileContexts.String(),
			"canned_fs_config": cannedFsConfig.String(),
			"key":              keyFile.String(),
		},
	})

	var abis []string
	for _, target := range ctx.MultiTargets() {
		abis = append(abis, target.Arch.Abi[0])
	}
	abis = android.FirstUniqueStrings(abis)

	apexProtoFile := android.PathForModuleOut(ctx, ctx.ModuleName()+".pb"+apexSuffix)
	bundleModuleFile := android.PathForModuleOut(ctx, ctx.ModuleName()+"-base.zip")
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
		Output:      bundleModuleFile,
		Description: "apex bundle module",
		Args: map[string]string{
			"abi": strings.Join(abis, "."),
		},
	})

	a.outputFile = android.PathForModuleOut(ctx, ctx.ModuleName()+apexSuffix)
	ctx.Build(pctx, android.BuildParams{
		Rule:        java.Signapk,
		Description: "signapk",
		Output:      a.outputFile,
		Input:       unsignedOutputFile,
		Args: map[string]string{
			"certificates": strings.Join([]string{certificate.Pem.String(), certificate.Key.String()}, " "),
		},
	})
}

func (a *apexBundle) buildFlattenedApex(ctx android.ModuleContext) {
	// For flattened APEX, do nothing but make sure that manifest.json file is also copied along
	// with other ordinary files.
	manifest := android.PathForModuleSrc(ctx, proptools.StringDefault(a.properties.Manifest, "manifest.json"))
	a.filesInfo = append(a.filesInfo, apexFile{manifest, ctx.ModuleName() + ".manifest.json", android.Common, ".", etc})

	for _, fi := range a.filesInfo {
		dir := filepath.Join("apex", ctx.ModuleName(), fi.installDir)
		ctx.InstallFile(android.PathForModuleInstall(ctx, dir), fi.builtFile.Base(), fi.builtFile)
	}
}

func (a *apexBundle) AndroidMk() android.AndroidMkData {
	if a.flattened {
		return android.AndroidMkData{
			Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
				moduleNames := []string{}
				for _, fi := range a.filesInfo {
					if !android.InList(fi.moduleName, moduleNames) {
						moduleNames = append(moduleNames, fi.moduleName)
					}
				}
				fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
				fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
				fmt.Fprintln(w, "LOCAL_MODULE :=", name)
				fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES :=", strings.Join(moduleNames, " "))
				fmt.Fprintln(w, "include $(BUILD_PHONY_PACKAGE)")

				for _, fi := range a.filesInfo {
					fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
					fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
					fmt.Fprintln(w, "LOCAL_MODULE :=", fi.moduleName)
					fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", filepath.Join("$(OUT_DIR)", a.installDir.RelPathString(), name, fi.installDir))
					fmt.Fprintln(w, "LOCAL_INSTALLED_MODULE_STEM :=", fi.builtFile.Base())
					fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", fi.builtFile.String())
					fmt.Fprintln(w, "LOCAL_MODULE_CLASS :=", fi.class.NameInMake())
					archStr := fi.archType.String()
					if archStr != "common" {
						fmt.Fprintln(w, "LOCAL_MODULE_TARGET_ARCH :=", archStr)
					}
					if fi.class == javaSharedLib {
						fmt.Fprintln(w, "LOCAL_SOONG_DEX_JAR :=", fi.builtFile.String())
						fmt.Fprintln(w, "LOCAL_DEX_PREOPT := false")
						fmt.Fprintln(w, "include $(BUILD_SYSTEM)/soong_java_prebuilt.mk")
					} else {
						fmt.Fprintln(w, "include $(BUILD_PREBUILT)")
					}
				}
			}}
	} else {
		return android.AndroidMkData{
			Custom: func(w io.Writer, name, prefix, moduleDir string, data android.AndroidMkData) {
				fmt.Fprintln(w, "\ninclude $(CLEAR_VARS)")
				fmt.Fprintln(w, "LOCAL_PATH :=", moduleDir)
				fmt.Fprintln(w, "LOCAL_MODULE :=", name)
				fmt.Fprintln(w, "LOCAL_MODULE_CLASS := ETC") // do we need a new class?
				fmt.Fprintln(w, "LOCAL_PREBUILT_MODULE_FILE :=", a.outputFile.String())
				fmt.Fprintln(w, "LOCAL_MODULE_PATH :=", filepath.Join("$(OUT_DIR)", a.installDir.RelPathString()))
				fmt.Fprintln(w, "LOCAL_INSTALLED_MODULE_STEM :=", name+apexSuffix)
				fmt.Fprintln(w, "LOCAL_REQUIRED_MODULES :=", String(a.properties.Key))
				fmt.Fprintln(w, "include $(BUILD_PREBUILT)")

				fmt.Fprintln(w, "ALL_MODULES.$(LOCAL_MODULE).BUNDLE :=", a.bundleModuleFile.String())
			}}
	}
}

func apexBundleFactory() android.Module {
	module := &apexBundle{}
	module.AddProperties(&module.properties)
	module.Prefer32(func(ctx android.BaseModuleContext, base *android.ModuleBase,
		class android.OsClass) bool {
		return class == android.Device && ctx.Config().DevicePrefer32BitExecutables()
	})
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}
