/*
 * Copyright (C) 2022 The Android Open Source Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package provenance

import (
	"android/soong/android"
	"github.com/google/blueprint"
)

var (
	pctx = android.NewPackageContext("android/soong/provenance")
	rule = pctx.HostBinToolVariable("gen_provenance_metadata", "gen_provenance_metadata")

	genProvenanceMetaData = pctx.AndroidStaticRule("genProvenanceMetaData",
		blueprint.RuleParams{
			Command: `rm -rf "$out" && ` +
				`${gen_provenance_metadata} --module_name=${module_name} ` +
				`--artifact_path=$in --install_path=${install_path} --metadata_path=$out`,
			CommandDeps: []string{"${gen_provenance_metadata}"},
		}, "module_name", "install_path")

	mergeProvenanceMetaData = pctx.AndroidStaticRule("mergeProvenanceMetaData",
		blueprint.RuleParams{
			Command: `rm -rf $out $out.temp && ` +
				`echo -e "# proto-file: build/soong/provenance/proto/provenance_metadata.proto\n# proto-message: ProvenanceMetaDataList" > $out && ` +
				`touch $out.temp && cat $out.temp $in | grep -v "^#.*" >> $out && rm -rf $out.temp`,
		})
)

type ProvenanceMetadata interface {
	ProvenanceMetaDataFile() android.OutputPath
}

func init() {
	RegisterProvenanceSingleton(android.InitRegistrationContext)
}

func RegisterProvenanceSingleton(ctx android.RegistrationContext) {
	ctx.RegisterSingletonType("provenance_metadata_singleton", provenanceInfoSingletonFactory)
}

var PrepareForTestWithProvenanceSingleton = android.FixtureRegisterWithContext(RegisterProvenanceSingleton)

func provenanceInfoSingletonFactory() android.Singleton {
	return &provenanceInfoSingleton{}
}

type provenanceInfoSingleton struct {
}

func (b *provenanceInfoSingleton) GenerateBuildActions(context android.SingletonContext) {
	allMetaDataFiles := make([]android.Path, 0)
	context.VisitAllModulesIf(moduleFilter, func(module android.Module) {
		if p, ok := module.(ProvenanceMetadata); ok {
			allMetaDataFiles = append(allMetaDataFiles, p.ProvenanceMetaDataFile())
		}
	})
	mergedMetaDataFile := android.PathForOutput(context, "provenance_metadata.textproto")
	context.Build(pctx, android.BuildParams{
		Rule:        mergeProvenanceMetaData,
		Description: "merge provenance metadata",
		Inputs:      allMetaDataFiles,
		Output:      mergedMetaDataFile,
	})

	context.Build(pctx, android.BuildParams{
		Rule:        blueprint.Phony,
		Description: "phony rule of merge provenance metadata",
		Inputs:      []android.Path{mergedMetaDataFile},
		Output:      android.PathForPhony(context, "provenance_metadata"),
	})

	context.Phony("droidcore", android.PathForPhony(context, "provenance_metadata"))
}

func moduleFilter(module android.Module) bool {
	if !module.Enabled() || module.IsSkipInstall() {
		return false
	}
	if p, ok := module.(ProvenanceMetadata); ok {
		return p.ProvenanceMetaDataFile().String() != ""
	}
	return false
}

func GenerateArtifactProvenanceMetaData(ctx android.ModuleContext, artifactPath android.Path, installedFile android.InstallPath) android.OutputPath {
	onDevicePathOfInstalledFile := android.InstallPathToOnDevicePath(ctx, installedFile)
	artifactMetaDataFile := android.PathForIntermediates(ctx, "provenance_metadata", ctx.ModuleDir(), ctx.ModuleName(), "provenance_metadata.textproto")
	ctx.Build(pctx, android.BuildParams{
		Rule:        genProvenanceMetaData,
		Description: "generate artifact provenance metadata",
		Inputs:      []android.Path{artifactPath},
		Output:      artifactMetaDataFile,
		Args: map[string]string{
			"module_name":  ctx.ModuleName(),
			"install_path": onDevicePathOfInstalledFile,
		}})

	return artifactMetaDataFile
}
