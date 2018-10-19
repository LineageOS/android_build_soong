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
		Command: `echo '/ 1000 1000 0644' > ${out} && ` +
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
)

var apexSuffix = ".apex"

type dependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var (
	sharedLibTag  = dependencyTag{name: "sharedLib"}
	executableTag = dependencyTag{name: "executable"}
	javaLibTag    = dependencyTag{name: "javaLib"}
	prebuiltTag   = dependencyTag{name: "prebuilt"}
	keyTag        = dependencyTag{name: "key"}
)

func init() {
	pctx.Import("android/soong/common")
	pctx.HostBinToolVariable("apexer", "apexer")
	pctx.HostBinToolVariable("aapt2", "aapt2")
	pctx.HostBinToolVariable("avbtool", "avbtool")
	pctx.HostBinToolVariable("e2fsdroid", "e2fsdroid")
	pctx.HostBinToolVariable("merge_zips", "merge_zips")
	pctx.HostBinToolVariable("mke2fs", "mke2fs")
	pctx.HostBinToolVariable("resize2fs", "resize2fs")
	pctx.HostBinToolVariable("sefcontext_compile", "sefcontext_compile")
	pctx.HostBinToolVariable("soong_zip", "soong_zip")
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
		apexBundleName := mctx.Module().Name()
		mctx.WalkDeps(func(child, parent android.Module) bool {
			if am, ok := child.(android.ApexModule); ok && am.CanHaveApexVariants() {
				moduleName := am.Name()
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
		moduleName := am.Name()
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

	// File contexts file for setting security context to each file in this APEX bundle
	// Default: "file_contexts".
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
}

type apexBundle struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties apexBundleProperties

	outputFile android.WritablePath
	installDir android.OutputPath
}

func (a *apexBundle) DepsMutator(ctx android.BottomUpMutatorContext) {
	for _, arch := range ctx.MultiTargets() {
		// Use *FarVariation* to be able to depend on modules having
		// conflicting variations with this module. This is required since
		// arch variant of an APEX bundle is 'common' but it is 'arm' or 'arm64'
		// for native shared libs.
		ctx.AddFarVariationDependencies([]blueprint.Variation{
			{Mutator: "arch", Variation: arch.String()},
			{Mutator: "image", Variation: "core"},
			{Mutator: "link", Variation: "shared"},
		}, sharedLibTag, a.properties.Native_shared_libs...)

		ctx.AddFarVariationDependencies([]blueprint.Variation{
			{Mutator: "arch", Variation: arch.String()},
			{Mutator: "image", Variation: "core"},
		}, executableTag, a.properties.Binaries...)
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
	fileToCopy = java.Srcs()[0]
	return
}

func getCopyManifestForPrebuiltEtc(prebuilt *android.PrebuiltEtc) (fileToCopy android.Path, dirInApex string) {
	dirInApex = filepath.Join("etc", prebuilt.SubDir())
	fileToCopy = prebuilt.OutputFile()
	return
}

func (a *apexBundle) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// files to copy -> dir in apex
	copyManifest := make(map[android.Path]string)

	var keyFile android.Path

	ctx.WalkDeps(func(child, parent android.Module) bool {
		if _, ok := parent.(*apexBundle); ok {
			// direct dependencies
			depTag := ctx.OtherModuleDependencyTag(child)
			depName := ctx.OtherModuleName(child)
			switch depTag {
			case sharedLibTag:
				if cc, ok := child.(*cc.Module); ok {
					fileToCopy, dirInApex := getCopyManifestForNativeLibrary(cc)
					copyManifest[fileToCopy] = dirInApex
					return true
				} else {
					ctx.PropertyErrorf("native_shared_libs", "%q is not a cc_library or cc_library_shared module", depName)
				}
			case executableTag:
				if cc, ok := child.(*cc.Module); ok {
					fileToCopy, dirInApex := getCopyManifestForExecutable(cc)
					copyManifest[fileToCopy] = dirInApex
					return true
				} else {
					ctx.PropertyErrorf("binaries", "%q is not a cc_binary module", depName)
				}
			case javaLibTag:
				if java, ok := child.(*java.Library); ok {
					fileToCopy, dirInApex := getCopyManifestForJavaLibrary(java)
					copyManifest[fileToCopy] = dirInApex
					return true
				} else {
					ctx.PropertyErrorf("java_libs", "%q is not a java_library module", depName)
				}
			case prebuiltTag:
				if prebuilt, ok := child.(*android.PrebuiltEtc); ok {
					fileToCopy, dirInApex := getCopyManifestForPrebuiltEtc(prebuilt)
					copyManifest[fileToCopy] = dirInApex
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
			}
		} else {
			// indirect dependencies
			if am, ok := child.(android.ApexModule); ok && am.CanHaveApexVariants() && am.IsInstallableToApex() {
				if cc, ok := child.(*cc.Module); ok {
					fileToCopy, dirInApex := getCopyManifestForNativeLibrary(cc)
					copyManifest[fileToCopy] = dirInApex
					return true
				}
			}
		}
		return false
	})

	// files and dirs that will be created in apex
	var readOnlyPaths []string
	var executablePaths []string // this also includes dirs
	for fileToCopy, dirInApex := range copyManifest {
		pathInApex := filepath.Join(dirInApex, fileToCopy.Base())
		if dirInApex == "bin" {
			executablePaths = append(executablePaths, pathInApex)
		} else {
			readOnlyPaths = append(readOnlyPaths, pathInApex)
		}
		if !android.InList(dirInApex, executablePaths) {
			executablePaths = append(executablePaths, dirInApex)
		}
	}
	sort.Strings(readOnlyPaths)
	sort.Strings(executablePaths)
	cannedFsConfig := android.PathForModuleOut(ctx, "canned_fs_config")
	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   generateFsConfig,
		Output: cannedFsConfig,
		Args: map[string]string{
			"ro_paths":   strings.Join(readOnlyPaths, " "),
			"exec_paths": strings.Join(executablePaths, " "),
		},
	})

	manifest := android.PathForModuleSrc(ctx, proptools.StringDefault(a.properties.Manifest, "manifest.json"))
	fileContexts := android.PathForModuleSrc(ctx, proptools.StringDefault(a.properties.File_contexts, "file_contexts"))

	a.outputFile = android.PathForModuleOut(ctx, a.ModuleBase.Name()+apexSuffix)

	filesToCopy := []android.Path{}
	for file := range copyManifest {
		filesToCopy = append(filesToCopy, file)
	}
	sort.Slice(filesToCopy, func(i, j int) bool {
		return filesToCopy[i].String() < filesToCopy[j].String()
	})

	copyCommands := []string{}
	for _, src := range filesToCopy {
		dest := filepath.Join(copyManifest[src], src.Base())
		dest_path := filepath.Join(android.PathForModuleOut(ctx, "image").String(), dest)
		copyCommands = append(copyCommands, "mkdir -p "+filepath.Dir(dest_path))
		copyCommands = append(copyCommands, "cp "+src.String()+" "+dest_path)
	}
	implicitInputs := append(android.Paths(nil), filesToCopy...)
	implicitInputs = append(implicitInputs, cannedFsConfig, manifest, fileContexts, keyFile)
	outHostBinDir := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "bin").String()
	prebuiltSdkToolsBinDir := filepath.Join("prebuilts", "sdk", "tools", runtime.GOOS, "bin")
	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:      apexRule,
		Implicits: implicitInputs,
		Output:    a.outputFile,
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

	a.installDir = android.PathForModuleInstall(ctx, "apex")
}

func (a *apexBundle) AndroidMk() android.AndroidMkData {
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
		}}
}

func apexBundleFactory() android.Module {
	module := &apexBundle{}
	module.AddProperties(&module.properties)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)
	return module
}
