// Copyright (C) 2024 The Android Open Source Project
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

package filesystem

import (
	"path/filepath"
	"strings"

	"android/soong/android"
)

type fsverityProperties struct {
	// Patterns of files for fsverity metadata generation.  For each matched file, a .fsv_meta file
	// will be generated and included to the filesystem image.
	// etc/security/fsverity/BuildManifest.apk will also be generated which contains information
	// about generated .fsv_meta files.
	Inputs []string

	// APK libraries to link against, for etc/security/fsverity/BuildManifest.apk
	Libs []string `android:"path"`
}

func (f *filesystem) writeManifestGeneratorListFile(ctx android.ModuleContext, outputPath android.OutputPath, matchedSpecs []android.PackagingSpec, rebasedDir android.OutputPath) {
	var buf strings.Builder
	for _, spec := range matchedSpecs {
		buf.WriteString(rebasedDir.Join(ctx, spec.RelPathInPackage()).String())
		buf.WriteRune('\n')
	}
	android.WriteFileRuleVerbatim(ctx, outputPath, buf.String())
}

func (f *filesystem) buildFsverityMetadataFiles(ctx android.ModuleContext, builder *android.RuleBuilder, specs map[string]android.PackagingSpec, rootDir android.OutputPath, rebasedDir android.OutputPath) {
	match := func(path string) bool {
		for _, pattern := range f.properties.Fsverity.Inputs {
			if matched, err := filepath.Match(pattern, path); matched {
				return true
			} else if err != nil {
				ctx.PropertyErrorf("fsverity.inputs", "bad pattern %q", pattern)
				return false
			}
		}
		return false
	}

	var matchedSpecs []android.PackagingSpec
	for _, relPath := range android.SortedKeys(specs) {
		if match(relPath) {
			matchedSpecs = append(matchedSpecs, specs[relPath])
		}
	}

	if len(matchedSpecs) == 0 {
		return
	}

	fsverityBuilderPath := android.PathForModuleOut(ctx, "fsverity_builder.sh")
	metadataGeneratorPath := ctx.Config().HostToolPath(ctx, "fsverity_metadata_generator")
	fsverityPath := ctx.Config().HostToolPath(ctx, "fsverity")

	cmd := builder.Command().Tool(fsverityBuilderPath)

	// STEP 1: generate .fsv_meta
	var sb strings.Builder
	sb.WriteString("set -e\n")
	cmd.Implicit(metadataGeneratorPath).Implicit(fsverityPath)
	for _, spec := range matchedSpecs {
		// srcPath is copied by CopySpecsToDir()
		srcPath := rebasedDir.Join(ctx, spec.RelPathInPackage())
		destPath := rebasedDir.Join(ctx, spec.RelPathInPackage()+".fsv_meta")
		sb.WriteString(metadataGeneratorPath.String())
		sb.WriteString(" --fsverity-path ")
		sb.WriteString(fsverityPath.String())
		sb.WriteString(" --signature none --hash-alg sha256 --output ")
		sb.WriteString(destPath.String())
		sb.WriteRune(' ')
		sb.WriteString(srcPath.String())
		sb.WriteRune('\n')
	}

	// STEP 2: generate signed BuildManifest.apk
	// STEP 2-1: generate build_manifest.pb
	assetsPath := android.PathForModuleOut(ctx, "fsverity_manifest/assets")
	manifestPbPath := assetsPath.Join(ctx, "build_manifest.pb")
	manifestGeneratorPath := ctx.Config().HostToolPath(ctx, "fsverity_manifest_generator")
	cmd.Implicit(manifestGeneratorPath)
	sb.WriteString("rm -rf ")
	sb.WriteString(assetsPath.String())
	sb.WriteString(" && mkdir -p ")
	sb.WriteString(assetsPath.String())
	sb.WriteRune('\n')
	sb.WriteString(manifestGeneratorPath.String())
	sb.WriteString(" --fsverity-path ")
	sb.WriteString(fsverityPath.String())
	sb.WriteString(" --base-dir ")
	sb.WriteString(rootDir.String())
	sb.WriteString(" --output ")
	sb.WriteString(manifestPbPath.String())
	sb.WriteRune(' ')

	manifestGeneratorListPath := android.PathForModuleOut(ctx, "fsverity_manifest.list")
	f.writeManifestGeneratorListFile(ctx, manifestGeneratorListPath.OutputPath, matchedSpecs, rebasedDir)
	sb.WriteRune('@')
	sb.WriteString(manifestGeneratorListPath.String())
	sb.WriteRune('\n')
	cmd.Implicit(manifestGeneratorListPath)

	// STEP 2-2: generate BuildManifest.apk (unsigned)
	aapt2Path := ctx.Config().HostToolPath(ctx, "aapt2")
	apkPath := rebasedDir.Join(ctx, "etc", "security", "fsverity", "BuildManifest.apk")
	manifestTemplatePath := android.PathForSource(ctx, "system/security/fsverity/AndroidManifest.xml")
	libs := android.PathsForModuleSrc(ctx, f.properties.Fsverity.Libs)
	cmd.Implicit(aapt2Path)
	cmd.Implicit(manifestTemplatePath)
	cmd.Implicits(libs)

	sb.WriteString(aapt2Path.String())
	sb.WriteString(" link -o ")
	sb.WriteString(apkPath.String())
	sb.WriteString(" -A ")
	sb.WriteString(assetsPath.String())
	for _, lib := range libs {
		sb.WriteString(" -I ")
		sb.WriteString(lib.String())
	}
	minSdkVersion := ctx.Config().PlatformSdkCodename()
	if minSdkVersion == "REL" {
		minSdkVersion = ctx.Config().PlatformSdkVersion().String()
	}
	sb.WriteString(" --min-sdk-version ")
	sb.WriteString(minSdkVersion)
	sb.WriteString(" --version-code ")
	sb.WriteString(ctx.Config().PlatformSdkVersion().String())
	sb.WriteString(" --version-name ")
	sb.WriteString(ctx.Config().AppsDefaultVersionName())
	sb.WriteString(" --manifest ")
	sb.WriteString(manifestTemplatePath.String())
	sb.WriteString(" --rename-manifest-package com.android.security.fsverity_metadata.")
	sb.WriteString(f.partitionName())
	sb.WriteRune('\n')

	// STEP 2-3: sign BuildManifest.apk
	apksignerPath := ctx.Config().HostToolPath(ctx, "apksigner")
	pemPath, keyPath := ctx.Config().DefaultAppCertificate(ctx)
	cmd.Implicit(apksignerPath)
	cmd.Implicit(pemPath)
	cmd.Implicit(keyPath)
	sb.WriteString(apksignerPath.String())
	sb.WriteString(" sign --in ")
	sb.WriteString(apkPath.String())
	sb.WriteString(" --cert ")
	sb.WriteString(pemPath.String())
	sb.WriteString(" --key ")
	sb.WriteString(keyPath.String())
	sb.WriteRune('\n')

	android.WriteExecutableFileRuleVerbatim(ctx, fsverityBuilderPath, sb.String())
}
