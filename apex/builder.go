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

package apex

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"android/soong/android"
	"android/soong/java"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

var (
	pctx = android.NewPackageContext("android/apex")
)

func init() {
	pctx.Import("android/soong/android")
	pctx.Import("android/soong/cc/config")
	pctx.Import("android/soong/java")
	pctx.HostBinToolVariable("apexer", "apexer")
	// ART minimal builds (using the master-art manifest) do not have the "frameworks/base"
	// projects, and hence cannot build 'aapt2'. Use the SDK prebuilt instead.
	hostBinToolVariableWithPrebuilt := func(name, prebuiltDir, tool string) {
		pctx.VariableFunc(name, func(ctx android.PackageVarContext) string {
			if !ctx.Config().FrameworksBaseDirExists(ctx) {
				return filepath.Join(prebuiltDir, runtime.GOOS, "bin", tool)
			} else {
				return ctx.Config().HostToolPath(ctx, tool).String()
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
	pctx.HostBinToolVariable("conv_apex_manifest", "conv_apex_manifest")
	pctx.HostBinToolVariable("extract_apks", "extract_apks")
	pctx.HostBinToolVariable("make_f2fs", "make_f2fs")
	pctx.HostBinToolVariable("sload_f2fs", "sload_f2fs")
	pctx.HostBinToolVariable("apex_compression_tool", "apex_compression_tool")
	pctx.SourcePathVariable("genNdkUsedbyApexPath", "build/soong/scripts/gen_ndk_usedby_apex.sh")
}

var (
	// Create a canned fs config file where all files and directories are
	// by default set to (uid/gid/mode) = (1000/1000/0644)
	// TODO(b/113082813) make this configurable using config.fs syntax
	generateFsConfig = pctx.StaticRule("generateFsConfig", blueprint.RuleParams{
		Command: `( echo '/ 1000 1000 0755' ` +
			`&& for i in ${ro_paths}; do echo "/$$i 1000 1000 0644"; done ` +
			`&& for i in  ${exec_paths}; do echo "/$$i 0 2000 0755"; done ` +
			`&& ( tr ' ' '\n' <${out}.apklist | for i in ${apk_paths}; do read apk; echo "/$$i 0 2000 0755"; zipinfo -1 $$apk | sed "s:\(.*\):/$$i/\1 1000 1000 0644:"; done ) ) > ${out}`,
		Description:    "fs_config ${out}",
		Rspfile:        "$out.apklist",
		RspfileContent: "$in",
	}, "ro_paths", "exec_paths", "apk_paths")

	apexManifestRule = pctx.StaticRule("apexManifestRule", blueprint.RuleParams{
		Command: `rm -f $out && ${jsonmodify} $in ` +
			`-a provideNativeLibs ${provideNativeLibs} ` +
			`-a requireNativeLibs ${requireNativeLibs} ` +
			`${opt} ` +
			`-o $out`,
		CommandDeps: []string{"${jsonmodify}"},
		Description: "prepare ${out}",
	}, "provideNativeLibs", "requireNativeLibs", "opt")

	stripApexManifestRule = pctx.StaticRule("stripApexManifestRule", blueprint.RuleParams{
		Command:     `rm -f $out && ${conv_apex_manifest} strip $in -o $out`,
		CommandDeps: []string{"${conv_apex_manifest}"},
		Description: "strip ${in}=>${out}",
	})

	pbApexManifestRule = pctx.StaticRule("pbApexManifestRule", blueprint.RuleParams{
		Command:     `rm -f $out && ${conv_apex_manifest} proto $in -o $out`,
		CommandDeps: []string{"${conv_apex_manifest}"},
		Description: "convert ${in}=>${out}",
	})

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
			`--include_build_info ` +
			`--payload_type image ` +
			`--key ${key} ${opt_flags} ${image_dir} ${out} `,
		CommandDeps: []string{"${apexer}", "${avbtool}", "${e2fsdroid}", "${merge_zips}",
			"${mke2fs}", "${resize2fs}", "${sefcontext_compile}", "${make_f2fs}", "${sload_f2fs}",
			"${soong_zip}", "${zipalign}", "${aapt2}", "prebuilts/sdk/current/public/android.jar"},
		Rspfile:        "${out}.copy_commands",
		RspfileContent: "${copy_commands}",
		Description:    "APEX ${image_dir} => ${out}",
	}, "tool_path", "image_dir", "copy_commands", "file_contexts", "canned_fs_config", "key", "opt_flags", "manifest", "payload_fs_type")

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
		Command: `${zip2zip} -i $in -o $out.base ` +
			`apex_payload.img:apex/${abi}.img ` +
			`apex_build_info.pb:apex/${abi}.build_info.pb ` +
			`apex_manifest.json:root/apex_manifest.json ` +
			`apex_manifest.pb:root/apex_manifest.pb ` +
			`AndroidManifest.xml:manifest/AndroidManifest.xml ` +
			`assets/NOTICE.html.gz:assets/NOTICE.html.gz &&` +
			`${soong_zip} -o $out.config -C $$(dirname ${config}) -f ${config} && ` +
			`${merge_zips} $out $out.base $out.config`,
		CommandDeps: []string{"${zip2zip}", "${soong_zip}", "${merge_zips}"},
		Description: "app bundle",
	}, "abi", "config")

	emitApexContentRule = pctx.StaticRule("emitApexContentRule", blueprint.RuleParams{
		Command:        `rm -f ${out} && touch ${out} && (. ${out}.emit_commands)`,
		Rspfile:        "${out}.emit_commands",
		RspfileContent: "${emit_commands}",
		Description:    "Emit APEX image content",
	}, "emit_commands")

	diffApexContentRule = pctx.StaticRule("diffApexContentRule", blueprint.RuleParams{
		Command: `diff --unchanged-group-format='' \` +
			`--changed-group-format='%<' \` +
			`${image_content_file} ${allowed_files_file} || (` +
			`echo -e "New unexpected files were added to ${apex_module_name}." ` +
			` "To fix the build run following command:" && ` +
			`echo "system/apex/tools/update_allowed_list.sh ${allowed_files_file} ${image_content_file}" && ` +
			`exit 1); touch ${out}`,
		Description: "Diff ${image_content_file} and ${allowed_files_file}",
	}, "image_content_file", "allowed_files_file", "apex_module_name")

	generateAPIsUsedbyApexRule = pctx.StaticRule("generateAPIsUsedbyApexRule", blueprint.RuleParams{
		Command:     "$genNdkUsedbyApexPath ${image_dir} ${readelf} ${out}",
		CommandDeps: []string{"${genNdkUsedbyApexPath}"},
		Description: "Generate symbol list used by Apex",
	}, "image_dir", "readelf")

	// Don't add more rules here. Consider using android.NewRuleBuilder instead.
)

// buildManifest creates buile rules to modify the input apex_manifest.json to add information
// gathered by the build system such as provided/required native libraries. Two output files having
// different formats are generated. a.manifestJsonOut is JSON format for Q devices, and
// a.manifest.PbOut is protobuf format for R+ devices.
// TODO(jiyong): make this to return paths instead of directly storing the paths to apexBundle
func (a *apexBundle) buildManifest(ctx android.ModuleContext, provideNativeLibs, requireNativeLibs []string) {
	src := android.PathForModuleSrc(ctx, proptools.StringDefault(a.properties.Manifest, "apex_manifest.json"))

	// Put dependency({provide|require}NativeLibs) in apex_manifest.json
	provideNativeLibs = android.SortedUniqueStrings(provideNativeLibs)
	requireNativeLibs = android.SortedUniqueStrings(android.RemoveListFromList(requireNativeLibs, provideNativeLibs))

	// APEX name can be overridden
	optCommands := []string{}
	if a.properties.Apex_name != nil {
		optCommands = append(optCommands, "-v name "+*a.properties.Apex_name)
	}

	// Collect jniLibs. Notice that a.filesInfo is already sorted
	var jniLibs []string
	for _, fi := range a.filesInfo {
		if fi.isJniLib && !android.InList(fi.stem(), jniLibs) {
			jniLibs = append(jniLibs, fi.stem())
		}
	}
	if len(jniLibs) > 0 {
		optCommands = append(optCommands, "-a jniLibs "+strings.Join(jniLibs, " "))
	}

	manifestJsonFullOut := android.PathForModuleOut(ctx, "apex_manifest_full.json")
	ctx.Build(pctx, android.BuildParams{
		Rule:   apexManifestRule,
		Input:  src,
		Output: manifestJsonFullOut,
		Args: map[string]string{
			"provideNativeLibs": strings.Join(provideNativeLibs, " "),
			"requireNativeLibs": strings.Join(requireNativeLibs, " "),
			"opt":               strings.Join(optCommands, " "),
		},
	})

	// b/143654022 Q apexd can't understand newly added keys in apex_manifest.json prepare
	// stripped-down version so that APEX modules built from R+ can be installed to Q
	minSdkVersion := a.minSdkVersion(ctx)
	if minSdkVersion.EqualTo(android.SdkVersion_Android10) {
		a.manifestJsonOut = android.PathForModuleOut(ctx, "apex_manifest.json")
		ctx.Build(pctx, android.BuildParams{
			Rule:   stripApexManifestRule,
			Input:  manifestJsonFullOut,
			Output: a.manifestJsonOut,
		})
	}

	// From R+, protobuf binary format (.pb) is the standard format for apex_manifest
	a.manifestPbOut = android.PathForModuleOut(ctx, "apex_manifest.pb")
	ctx.Build(pctx, android.BuildParams{
		Rule:   pbApexManifestRule,
		Input:  manifestJsonFullOut,
		Output: a.manifestPbOut,
	})
}

// buildFileContexts create build rules to append an entry for apex_manifest.pb to the file_contexts
// file for this APEX which is either from /systme/sepolicy/apex/<apexname>-file_contexts or from
// the file_contexts property of this APEX. This is to make sure that the manifest file is correctly
// labeled as system_file.
func (a *apexBundle) buildFileContexts(ctx android.ModuleContext) android.OutputPath {
	var fileContexts android.Path
	if a.properties.File_contexts == nil {
		fileContexts = android.PathForSource(ctx, "system/sepolicy/apex", ctx.ModuleName()+"-file_contexts")
	} else {
		fileContexts = android.PathForModuleSrc(ctx, *a.properties.File_contexts)
	}
	if a.Platform() {
		if matched, err := path.Match("system/sepolicy/**/*", fileContexts.String()); err != nil || !matched {
			ctx.PropertyErrorf("file_contexts", "should be under system/sepolicy, but %q", fileContexts)
		}
	}
	if !android.ExistentPathForSource(ctx, fileContexts.String()).Valid() {
		ctx.PropertyErrorf("file_contexts", "cannot find file_contexts file: %q", fileContexts.String())
	}

	output := android.PathForModuleOut(ctx, "file_contexts")
	rule := android.NewRuleBuilder(pctx, ctx)

	switch a.properties.ApexType {
	case imageApex:
		// remove old file
		rule.Command().Text("rm").FlagWithOutput("-f ", output)
		// copy file_contexts
		rule.Command().Text("cat").Input(fileContexts).Text(">>").Output(output)
		// new line
		rule.Command().Text("echo").Text(">>").Output(output)
		// force-label /apex_manifest.pb and / as system_file so that apexd can read them
		rule.Command().Text("echo").Flag("/apex_manifest\\\\.pb u:object_r:system_file:s0").Text(">>").Output(output)
		rule.Command().Text("echo").Flag("/ u:object_r:system_file:s0").Text(">>").Output(output)
	case flattenedApex:
		// For flattened apexes, install path should be prepended.
		// File_contexts file should be emiited to make via LOCAL_FILE_CONTEXTS
		// so that it can be merged into file_contexts.bin
		apexPath := android.InstallPathToOnDevicePath(ctx, a.installDir.Join(ctx, a.Name()))
		apexPath = strings.ReplaceAll(apexPath, ".", `\\.`)
		// remove old file
		rule.Command().Text("rm").FlagWithOutput("-f ", output)
		// copy file_contexts
		rule.Command().Text("awk").Text(`'/object_r/{printf("` + apexPath + `%s\n", $0)}'`).Input(fileContexts).Text(">").Output(output)
		// new line
		rule.Command().Text("echo").Text(">>").Output(output)
		// force-label /apex_manifest.pb and / as system_file so that apexd can read them
		rule.Command().Text("echo").Flag(apexPath + `/apex_manifest\\.pb u:object_r:system_file:s0`).Text(">>").Output(output)
		rule.Command().Text("echo").Flag(apexPath + "/ u:object_r:system_file:s0").Text(">>").Output(output)
	default:
		panic(fmt.Errorf("unsupported type %v", a.properties.ApexType))
	}

	rule.Build("file_contexts."+a.Name(), "Generate file_contexts")
	return output.OutputPath
}

// buildNoticeFiles creates a buile rule for aggregating notice files from the modules that
// contributes to this APEX. The notice files are merged into a big notice file.
func (a *apexBundle) buildNoticeFiles(ctx android.ModuleContext, apexFileName string) android.NoticeOutputs {
	var noticeFiles android.Paths

	a.WalkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		if externalDep {
			// As soon as the dependency graph crosses the APEX boundary, don't go further.
			return false
		}
		noticeFiles = append(noticeFiles, to.NoticeFiles()...)
		return true
	})

	// TODO(jiyong): why do we need this? WalkPayloadDeps should have already covered this.
	for _, fi := range a.filesInfo {
		noticeFiles = append(noticeFiles, fi.noticeFiles...)
	}

	if len(noticeFiles) == 0 {
		return android.NoticeOutputs{}
	}

	return android.BuildNoticeOutput(ctx, a.installDir, apexFileName, android.SortedUniquePaths(noticeFiles))
}

// buildInstalledFilesFile creates a build rule for the installed-files.txt file where the list of
// files included in this APEX is shown. The text file is dist'ed so that people can see what's
// included in the APEX without actually downloading and extracting it.
func (a *apexBundle) buildInstalledFilesFile(ctx android.ModuleContext, builtApex android.Path, imageDir android.Path) android.OutputPath {
	output := android.PathForModuleOut(ctx, "installed-files.txt")
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Command().
		Implicit(builtApex).
		Text("(cd " + imageDir.String() + " ; ").
		Text("find . \\( -type f -o -type l \\) -printf \"%s %p\\n\") ").
		Text(" | sort -nr > ").
		Output(output)
	rule.Build("installed-files."+a.Name(), "Installed files")
	return output.OutputPath
}

// buildBundleConfig creates a build rule for the bundle config file that will control the bundle
// creation process.
func (a *apexBundle) buildBundleConfig(ctx android.ModuleContext) android.OutputPath {
	output := android.PathForModuleOut(ctx, "bundle_config.json")

	type ApkConfig struct {
		Package_name string `json:"package_name"`
		Apk_path     string `json:"path"`
	}
	config := struct {
		Compression struct {
			Uncompressed_glob []string `json:"uncompressed_glob"`
		} `json:"compression"`
		Apex_config struct {
			Apex_embedded_apk_config []ApkConfig `json:"apex_embedded_apk_config,omitempty"`
		} `json:"apex_config,omitempty"`
	}{}

	config.Compression.Uncompressed_glob = []string{
		"apex_payload.img",
		"apex_manifest.*",
	}

	// Collect the manifest names and paths of android apps if their manifest names are
	// overridden.
	for _, fi := range a.filesInfo {
		if fi.class != app && fi.class != appSet {
			continue
		}
		packageName := fi.overriddenPackageName
		if packageName != "" {
			config.Apex_config.Apex_embedded_apk_config = append(
				config.Apex_config.Apex_embedded_apk_config,
				ApkConfig{
					Package_name: packageName,
					Apk_path:     fi.path(),
				})
		}
	}

	j, err := json.Marshal(config)
	if err != nil {
		panic(fmt.Errorf("error while marshalling to %q: %#v", output, err))
	}

	android.WriteFileRule(ctx, output, string(j))

	return output.OutputPath
}

// buildUnflattendApex creates build rules to build an APEX using apexer.
func (a *apexBundle) buildUnflattenedApex(ctx android.ModuleContext) {
	apexType := a.properties.ApexType
	suffix := apexType.suffix()

	////////////////////////////////////////////////////////////////////////////////////////////
	// Step 1: copy built files to appropriate directories under the image directory

	imageDir := android.PathForModuleOut(ctx, "image"+suffix)

	// TODO(jiyong): use the RuleBuilder
	var copyCommands []string
	var implicitInputs []android.Path
	for _, fi := range a.filesInfo {
		destPath := imageDir.Join(ctx, fi.path()).String()

		// Prepare the destination path
		destPathDir := filepath.Dir(destPath)
		if fi.class == appSet {
			copyCommands = append(copyCommands, "rm -rf "+destPathDir)
		}
		copyCommands = append(copyCommands, "mkdir -p "+destPathDir)

		// Copy the built file to the directory. But if the symlink optimization is turned
		// on, place a symlink to the corresponding file in /system partition instead.
		if a.linkToSystemLib && fi.transitiveDep && fi.availableToPlatform() {
			// TODO(jiyong): pathOnDevice should come from fi.module, not being calculated here
			pathOnDevice := filepath.Join("/system", fi.path())
			copyCommands = append(copyCommands, "ln -sfn "+pathOnDevice+" "+destPath)
		} else {
			if fi.class == appSet {
				copyCommands = append(copyCommands,
					fmt.Sprintf("unzip -qDD -d %s %s", destPathDir, fi.builtFile.String()))
			} else {
				copyCommands = append(copyCommands, "cp -f "+fi.builtFile.String()+" "+destPath)
			}
			implicitInputs = append(implicitInputs, fi.builtFile)
		}

		// Create additional symlinks pointing the file inside the APEX (if any). Note that
		// this is independent from the symlink optimization.
		for _, symlinkPath := range fi.symlinkPaths() {
			symlinkDest := imageDir.Join(ctx, symlinkPath).String()
			copyCommands = append(copyCommands, "ln -sfn "+filepath.Base(destPath)+" "+symlinkDest)
		}

		// Copy the test files (if any)
		for _, d := range fi.dataPaths {
			// TODO(eakammer): This is now the third repetition of ~this logic for test paths, refactoring should be possible
			relPath := d.SrcPath.Rel()
			dataPath := d.SrcPath.String()
			if !strings.HasSuffix(dataPath, relPath) {
				panic(fmt.Errorf("path %q does not end with %q", dataPath, relPath))
			}

			dataDest := imageDir.Join(ctx, fi.apexRelativePath(relPath), d.RelativeInstallPath).String()

			copyCommands = append(copyCommands, "cp -f "+d.SrcPath.String()+" "+dataDest)
			implicitInputs = append(implicitInputs, d.SrcPath)
		}
	}
	implicitInputs = append(implicitInputs, a.manifestPbOut)

	////////////////////////////////////////////////////////////////////////////////////////////
	// Step 1.a: Write the list of files in this APEX to a txt file and compare it against
	// the allowed list given via the allowed_files property. Build fails when the two lists
	// differ.
	//
	// TODO(jiyong): consider removing this. Nobody other than com.android.apex.cts.shim.* seems
	// to be using this at this moment. Furthermore, this looks very similar to what
	// buildInstalledFilesFile does. At least, move this to somewhere else so that this doesn't
	// hurt readability.
	// TODO(jiyong): use RuleBuilder
	if a.overridableProperties.Allowed_files != nil {
		// Build content.txt
		var emitCommands []string
		imageContentFile := android.PathForModuleOut(ctx, "content.txt")
		emitCommands = append(emitCommands, "echo ./apex_manifest.pb >> "+imageContentFile.String())
		minSdkVersion := a.minSdkVersion(ctx)
		if minSdkVersion.EqualTo(android.SdkVersion_Android10) {
			emitCommands = append(emitCommands, "echo ./apex_manifest.json >> "+imageContentFile.String())
		}
		for _, fi := range a.filesInfo {
			emitCommands = append(emitCommands, "echo './"+fi.path()+"' >> "+imageContentFile.String())
		}
		emitCommands = append(emitCommands, "sort -o "+imageContentFile.String()+" "+imageContentFile.String())
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

		// Compare content.txt against allowed_files.
		allowedFilesFile := android.PathForModuleSrc(ctx, proptools.String(a.overridableProperties.Allowed_files))
		phonyOutput := android.PathForModuleOut(ctx, a.Name()+"-diff-phony-output")
		ctx.Build(pctx, android.BuildParams{
			Rule:        diffApexContentRule,
			Implicits:   implicitInputs,
			Output:      phonyOutput,
			Description: "diff apex image content",
			Args: map[string]string{
				"allowed_files_file": allowedFilesFile.String(),
				"image_content_file": imageContentFile.String(),
				"apex_module_name":   a.Name(),
			},
		})
		implicitInputs = append(implicitInputs, phonyOutput)
	}

	unsignedOutputFile := android.PathForModuleOut(ctx, a.Name()+suffix+".unsigned")
	outHostBinDir := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "bin").String()
	prebuiltSdkToolsBinDir := filepath.Join("prebuilts", "sdk", "tools", runtime.GOOS, "bin")

	if apexType == imageApex {
		////////////////////////////////////////////////////////////////////////////////////
		// Step 2: create canned_fs_config which encodes filemode,uid,gid of each files
		// in this APEX. The file will be used by apexer in later steps.
		// TODO(jiyong): make this as a function
		// TODO(jiyong): use the RuleBuilder
		var readOnlyPaths = []string{"apex_manifest.json", "apex_manifest.pb"}
		var executablePaths []string // this also includes dirs
		var extractedAppSetPaths android.Paths
		var extractedAppSetDirs []string
		for _, f := range a.filesInfo {
			pathInApex := f.path()
			if f.installDir == "bin" || strings.HasPrefix(f.installDir, "bin/") {
				executablePaths = append(executablePaths, pathInApex)
				for _, d := range f.dataPaths {
					readOnlyPaths = append(readOnlyPaths, filepath.Join(f.installDir, d.RelativeInstallPath, d.SrcPath.Rel()))
				}
				for _, s := range f.symlinks {
					executablePaths = append(executablePaths, filepath.Join(f.installDir, s))
				}
			} else if f.class == appSet {
				extractedAppSetPaths = append(extractedAppSetPaths, f.builtFile)
				extractedAppSetDirs = append(extractedAppSetDirs, f.installDir)
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
			Inputs:      extractedAppSetPaths,
			Args: map[string]string{
				"ro_paths":   strings.Join(readOnlyPaths, " "),
				"exec_paths": strings.Join(executablePaths, " "),
				"apk_paths":  strings.Join(extractedAppSetDirs, " "),
			},
		})
		implicitInputs = append(implicitInputs, cannedFsConfig)

		////////////////////////////////////////////////////////////////////////////////////
		// Step 3: Prepare option flags for apexer and invoke it to create an unsigned APEX.
		// TODO(jiyong): use the RuleBuilder
		optFlags := []string{}

		fileContexts := a.buildFileContexts(ctx)
		implicitInputs = append(implicitInputs, fileContexts)

		implicitInputs = append(implicitInputs, a.privateKeyFile, a.publicKeyFile)
		optFlags = append(optFlags, "--pubkey "+a.publicKeyFile.String())

		manifestPackageName := a.getOverrideManifestPackageName(ctx)
		if manifestPackageName != "" {
			optFlags = append(optFlags, "--override_apk_package_name "+manifestPackageName)
		}

		if a.properties.AndroidManifest != nil {
			androidManifestFile := android.PathForModuleSrc(ctx, proptools.String(a.properties.AndroidManifest))
			implicitInputs = append(implicitInputs, androidManifestFile)
			optFlags = append(optFlags, "--android_manifest "+androidManifestFile.String())
		}

		// Determine target/min sdk version from the context
		// TODO(jiyong): make this as a function
		moduleMinSdkVersion := a.minSdkVersion(ctx)
		minSdkVersion := moduleMinSdkVersion.String()

		// bundletool doesn't understand what "current" is. We need to transform it to
		// codename
		if moduleMinSdkVersion.IsCurrent() {
			minSdkVersion = ctx.Config().DefaultAppTargetSdk(ctx).String()
		}
		// apex module doesn't have a concept of target_sdk_version, hence for the time
		// being targetSdkVersion == default targetSdkVersion of the branch.
		targetSdkVersion := strconv.Itoa(ctx.Config().DefaultAppTargetSdk(ctx).FinalOrFutureInt())

		if java.UseApiFingerprint(ctx) {
			targetSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", java.ApiFingerprintPath(ctx).String())
			implicitInputs = append(implicitInputs, java.ApiFingerprintPath(ctx))
		}
		if java.UseApiFingerprint(ctx) {
			minSdkVersion = ctx.Config().PlatformSdkCodename() + fmt.Sprintf(".$$(cat %s)", java.ApiFingerprintPath(ctx).String())
			implicitInputs = append(implicitInputs, java.ApiFingerprintPath(ctx))
		}
		optFlags = append(optFlags, "--target_sdk_version "+targetSdkVersion)
		optFlags = append(optFlags, "--min_sdk_version "+minSdkVersion)

		if a.overridableProperties.Logging_parent != "" {
			optFlags = append(optFlags, "--logging_parent ", a.overridableProperties.Logging_parent)
		}

		a.mergedNotices = a.buildNoticeFiles(ctx, a.Name()+suffix)
		if a.mergedNotices.HtmlGzOutput.Valid() {
			// If there's a NOTICE file, embed it as an asset file in the APEX.
			implicitInputs = append(implicitInputs, a.mergedNotices.HtmlGzOutput.Path())
			optFlags = append(optFlags, "--assets_dir "+filepath.Dir(a.mergedNotices.HtmlGzOutput.String()))
		}

		if ctx.ModuleDir() != "system/apex/apexd/apexd_testdata" && ctx.ModuleDir() != "system/apex/shim/build" && a.testOnlyShouldSkipHashtreeGeneration() {
			ctx.PropertyErrorf("test_only_no_hashtree", "not available")
			return
		}
		if moduleMinSdkVersion.GreaterThan(android.SdkVersion_Android10) || a.testOnlyShouldSkipHashtreeGeneration() {
			// Apexes which are supposed to be installed in builtin dirs(/system, etc)
			// don't need hashtree for activation. Therefore, by removing hashtree from
			// apex bundle (filesystem image in it, to be specific), we can save storage.
			optFlags = append(optFlags, "--no_hashtree")
		}

		if a.testOnlyShouldSkipPayloadSign() {
			optFlags = append(optFlags, "--unsigned_payload")
		}

		if a.properties.Apex_name != nil {
			// If apex_name is set, apexer can skip checking if key name matches with
			// apex name.  Note that apex_manifest is also mended.
			optFlags = append(optFlags, "--do_not_check_keyname")
		}

		if moduleMinSdkVersion == android.SdkVersion_Android10 {
			implicitInputs = append(implicitInputs, a.manifestJsonOut)
			optFlags = append(optFlags, "--manifest_json "+a.manifestJsonOut.String())
		}

		optFlags = append(optFlags, "--payload_fs_type "+a.payloadFsType.string())

		ctx.Build(pctx, android.BuildParams{
			Rule:        apexRule,
			Implicits:   implicitInputs,
			Output:      unsignedOutputFile,
			Description: "apex (" + apexType.name() + ")",
			Args: map[string]string{
				"tool_path":        outHostBinDir + ":" + prebuiltSdkToolsBinDir,
				"image_dir":        imageDir.String(),
				"copy_commands":    strings.Join(copyCommands, " && "),
				"manifest":         a.manifestPbOut.String(),
				"file_contexts":    fileContexts.String(),
				"canned_fs_config": cannedFsConfig.String(),
				"key":              a.privateKeyFile.String(),
				"opt_flags":        strings.Join(optFlags, " "),
			},
		})

		// TODO(jiyong): make the two rules below as separate functions
		apexProtoFile := android.PathForModuleOut(ctx, a.Name()+".pb"+suffix)
		bundleModuleFile := android.PathForModuleOut(ctx, a.Name()+suffix+"-base.zip")
		a.bundleModuleFile = bundleModuleFile

		ctx.Build(pctx, android.BuildParams{
			Rule:        apexProtoConvertRule,
			Input:       unsignedOutputFile,
			Output:      apexProtoFile,
			Description: "apex proto convert",
		})

		implicitInputs = append(implicitInputs, unsignedOutputFile)

		// Run coverage analysis
		apisUsedbyOutputFile := android.PathForModuleOut(ctx, a.Name()+".txt")
		ctx.Build(pctx, android.BuildParams{
			Rule:        generateAPIsUsedbyApexRule,
			Implicits:   implicitInputs,
			Description: "coverage",
			Output:      apisUsedbyOutputFile,
			Args: map[string]string{
				"image_dir": imageDir.String(),
				"readelf":   "${config.ClangBin}/llvm-readelf",
			},
		})
		a.coverageOutputPath = apisUsedbyOutputFile

		bundleConfig := a.buildBundleConfig(ctx)

		var abis []string
		for _, target := range ctx.MultiTargets() {
			if len(target.Arch.Abi) > 0 {
				abis = append(abis, target.Arch.Abi[0])
			}
		}

		abis = android.FirstUniqueStrings(abis)

		ctx.Build(pctx, android.BuildParams{
			Rule:        apexBundleRule,
			Input:       apexProtoFile,
			Implicit:    bundleConfig,
			Output:      a.bundleModuleFile,
			Description: "apex bundle module",
			Args: map[string]string{
				"abi":    strings.Join(abis, "."),
				"config": bundleConfig.String(),
			},
		})
	} else { // zipApex
		ctx.Build(pctx, android.BuildParams{
			Rule:        zipApexRule,
			Implicits:   implicitInputs,
			Output:      unsignedOutputFile,
			Description: "apex (" + apexType.name() + ")",
			Args: map[string]string{
				"tool_path":     outHostBinDir + ":" + prebuiltSdkToolsBinDir,
				"image_dir":     imageDir.String(),
				"copy_commands": strings.Join(copyCommands, " && "),
				"manifest":      a.manifestPbOut.String(),
			},
		})
	}

	////////////////////////////////////////////////////////////////////////////////////
	// Step 4: Sign the APEX using signapk
	signedOutputFile := android.PathForModuleOut(ctx, a.Name()+suffix)

	pem, key := a.getCertificateAndPrivateKey(ctx)
	rule := java.Signapk
	args := map[string]string{
		"certificates": pem.String() + " " + key.String(),
		"flags":        "-a 4096", //alignment
	}
	implicits := android.Paths{pem, key}
	if ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_SIGNAPK") {
		rule = java.SignapkRE
		args["implicits"] = strings.Join(implicits.Strings(), ",")
		args["outCommaList"] = signedOutputFile.String()
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
		Description: "signapk",
		Output:      signedOutputFile,
		Input:       unsignedOutputFile,
		Implicits:   implicits,
		Args:        args,
	})
	a.outputFile = signedOutputFile

	// Process APEX compression if enabled
	compressionEnabled := ctx.Config().CompressedApex() && proptools.BoolDefault(a.properties.Compressible, true)
	if compressionEnabled && apexType == imageApex {
		a.isCompressed = true
		unsignedCompressedOutputFile := android.PathForModuleOut(ctx, a.Name()+".capex.unsigned")

		compressRule := android.NewRuleBuilder(pctx, ctx)
		compressRule.Command().
			Text("rm").
			FlagWithOutput("-f ", unsignedCompressedOutputFile)
		compressRule.Command().
			BuiltTool("apex_compression_tool").
			Flag("compress").
			FlagWithArg("--apex_compression_tool ", outHostBinDir+":"+prebuiltSdkToolsBinDir).
			FlagWithInput("--input ", signedOutputFile).
			FlagWithOutput("--output ", unsignedCompressedOutputFile)
		compressRule.Build("compressRule", "Generate unsigned compressed APEX file")

		signedCompressedOutputFile := android.PathForModuleOut(ctx, a.Name()+".capex")
		ctx.Build(pctx, android.BuildParams{
			Rule:        rule,
			Description: "sign compressedApex",
			Output:      signedCompressedOutputFile,
			Input:       unsignedCompressedOutputFile,
			Implicits:   implicits,
			Args:        args,
		})
		a.outputFile = signedCompressedOutputFile
	}

	// Install to $OUT/soong/{target,host}/.../apex
	if a.installable() {
		ctx.InstallFile(a.installDir, a.Name()+suffix, a.outputFile)
	}

	// installed-files.txt is dist'ed
	a.installedFilesFile = a.buildInstalledFilesFile(ctx, a.outputFile, imageDir)
}

// Context "decorator", overriding the InstallBypassMake method to always reply `true`.
type flattenedApexContext struct {
	android.ModuleContext
}

func (c *flattenedApexContext) InstallBypassMake() bool {
	return true
}

// buildFlattenedApex creates rules for a flattened APEX. Flattened APEX actually doesn't have a
// single output file. It is a phony target for all the files under /system/apex/<name> directory.
// This function creates the installation rules for the files.
func (a *apexBundle) buildFlattenedApex(ctx android.ModuleContext) {
	bundleName := a.Name()
	if a.installable() {
		for _, fi := range a.filesInfo {
			dir := filepath.Join("apex", bundleName, fi.installDir)
			target := ctx.InstallFile(android.PathForModuleInstall(ctx, dir), fi.stem(), fi.builtFile)
			for _, sym := range fi.symlinks {
				ctx.InstallSymlink(android.PathForModuleInstall(ctx, dir), sym, target)
			}
		}
	}

	a.fileContexts = a.buildFileContexts(ctx)

	// Temporarily wrap the original `ctx` into a `flattenedApexContext` to have it reply true
	// to `InstallBypassMake()` (thus making the call `android.PathForModuleInstall` below use
	// `android.pathForInstallInMakeDir` instead of `android.PathForOutput`) to return the
	// correct path to the flattened APEX (as its contents is installed by Make, not Soong).
	// TODO(jiyong): Why do we need to set outputFile for flattened APEX? We don't seem to use
	// it and it actually points to a path that can never be built. Remove this.
	factx := flattenedApexContext{ctx}
	a.outputFile = android.PathForModuleInstall(&factx, "apex", bundleName)
}

// getCertificateAndPrivateKey retrieves the cert and the private key that will be used to sign
// the zip container of this APEX. See the description of the 'certificate' property for how
// the cert and the private key are found.
func (a *apexBundle) getCertificateAndPrivateKey(ctx android.PathContext) (pem, key android.Path) {
	if a.containerCertificateFile != nil {
		return a.containerCertificateFile, a.containerPrivateKeyFile
	}

	cert := String(a.properties.Certificate)
	if cert == "" {
		return ctx.Config().DefaultAppCertificate(ctx)
	}

	defaultDir := ctx.Config().DefaultAppCertificateDir(ctx)
	pem = defaultDir.Join(ctx, cert+".x509.pem")
	key = defaultDir.Join(ctx, cert+".pk8")
	return pem, key
}

func (a *apexBundle) getOverrideManifestPackageName(ctx android.ModuleContext) string {
	// For VNDK APEXes, check "com.android.vndk" in PRODUCT_MANIFEST_PACKAGE_NAME_OVERRIDES
	// to see if it should be overridden because their <apex name> is dynamically generated
	// according to its VNDK version.
	if a.vndkApex {
		overrideName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(vndkApexName)
		if overridden {
			return strings.Replace(*a.properties.Apex_name, vndkApexName, overrideName, 1)
		}
		return ""
	}
	if a.overridableProperties.Package_name != "" {
		return a.overridableProperties.Package_name
	}
	manifestPackageName, overridden := ctx.DeviceConfig().OverrideManifestPackageNameFor(ctx.ModuleName())
	if overridden {
		return manifestPackageName
	}
	return ""
}

func (a *apexBundle) buildApexDependencyInfo(ctx android.ModuleContext) {
	if !a.primaryApexType {
		return
	}

	if a.properties.IsCoverageVariant {
		// Otherwise, we will have duplicated rules for coverage and
		// non-coverage variants of the same APEX
		return
	}

	if ctx.Host() {
		// No need to generate dependency info for host variant
		return
	}

	depInfos := android.DepNameToDepInfoMap{}
	a.WalkPayloadDeps(ctx, func(ctx android.ModuleContext, from blueprint.Module, to android.ApexModule, externalDep bool) bool {
		if from.Name() == to.Name() {
			// This can happen for cc.reuseObjTag. We are not interested in tracking this.
			// As soon as the dependency graph crosses the APEX boundary, don't go further.
			return !externalDep
		}

		depTag := ctx.OtherModuleDependencyTag(to)
		if skipDepCheck, ok := depTag.(android.SkipApexAllowedDependenciesCheck); ok && skipDepCheck.SkipApexAllowedDependenciesCheck() {
			// Check to see if dependency been marked to skip the dependency check
			return !externalDep
		}

		if info, exists := depInfos[to.Name()]; exists {
			if !android.InList(from.Name(), info.From) {
				info.From = append(info.From, from.Name())
			}
			info.IsExternal = info.IsExternal && externalDep
			depInfos[to.Name()] = info
		} else {
			toMinSdkVersion := "(no version)"
			if m, ok := to.(interface{ MinSdkVersion() string }); ok {
				if v := m.MinSdkVersion(); v != "" {
					toMinSdkVersion = v
				}
			}

			depInfos[to.Name()] = android.ApexModuleDepInfo{
				To:            to.Name(),
				From:          []string{from.Name()},
				IsExternal:    externalDep,
				MinSdkVersion: toMinSdkVersion,
			}
		}

		// As soon as the dependency graph crosses the APEX boundary, don't go further.
		return !externalDep
	})

	a.ApexBundleDepsInfo.BuildDepsInfoLists(ctx, proptools.String(a.properties.Min_sdk_version), depInfos)

	ctx.Build(pctx, android.BuildParams{
		Rule:   android.Phony,
		Output: android.PathForPhony(ctx, a.Name()+"-deps-info"),
		Inputs: []android.Path{
			a.ApexBundleDepsInfo.FullListPath(),
			a.ApexBundleDepsInfo.FlatListPath(),
		},
	})
}

func (a *apexBundle) buildLintReports(ctx android.ModuleContext) {
	depSetsBuilder := java.NewLintDepSetBuilder()
	for _, fi := range a.filesInfo {
		depSetsBuilder.Transitive(fi.lintDepSets)
	}

	a.lintReports = java.BuildModuleLintReportZips(ctx, depSetsBuilder.Build())
}
