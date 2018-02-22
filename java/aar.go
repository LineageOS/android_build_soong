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

package java

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

//
// AAR (android library) prebuilts
//
func init() {
	android.RegisterModuleType("android_library_import", AARImportFactory)
}

type AARImportProperties struct {
	Aars []string

	Sdk_version *string
}

type AARImport struct {
	android.ModuleBase
	prebuilt android.Prebuilt

	properties AARImportProperties

	classpathFile android.WritablePath
	proguardFlags android.WritablePath
	exportPackage android.WritablePath
}

func (a *AARImport) Prebuilt() *android.Prebuilt {
	return &a.prebuilt
}

func (a *AARImport) Name() string {
	return a.prebuilt.Name(a.ModuleBase.Name())
}

func (a *AARImport) DepsMutator(ctx android.BottomUpMutatorContext) {
	// TODO: this should use decodeSdkDep once that knows about current
	if !ctx.Config().UnbundledBuild() {
		switch String(a.properties.Sdk_version) { // TODO: Res_sdk_version?
		case "current", "system_current", "test_current", "":
			ctx.AddDependency(ctx.Module(), frameworkResTag, "framework-res")
		}
	}
}

// Unzip an AAR into its constituent files and directories.  Any files in Outputs that don't exist in the AAR will be
// touched to create an empty file, and any directories in $expectedDirs will be created.
var unzipAAR = pctx.AndroidStaticRule("unzipAAR",
	blueprint.RuleParams{
		Command: `rm -rf $outDir && mkdir -p $outDir $expectedDirs && ` +
			`unzip -qo -d $outDir $in && touch $out`,
	},
	"expectedDirs", "outDir")

func (a *AARImport) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(a.properties.Aars) != 1 {
		ctx.PropertyErrorf("aars", "exactly one aar is required")
		return
	}

	aar := android.PathForModuleSrc(ctx, a.properties.Aars[0])

	extractedAARDir := android.PathForModuleOut(ctx, "aar")
	extractedResDir := extractedAARDir.Join(ctx, "res")
	a.classpathFile = extractedAARDir.Join(ctx, "classes.jar")
	a.proguardFlags = extractedAARDir.Join(ctx, "proguard.txt")
	manifest := extractedAARDir.Join(ctx, "AndroidManifest.xml")

	ctx.Build(pctx, android.BuildParams{
		Rule:        unzipAAR,
		Input:       aar,
		Outputs:     android.WritablePaths{a.classpathFile, a.proguardFlags, manifest},
		Description: "unzip AAR",
		Args: map[string]string{
			"expectedDirs": extractedResDir.String(),
			"outDir":       extractedAARDir.String(),
		},
	})

	compiledResDir := android.PathForModuleOut(ctx, "flat-res")
	aaptCompileDeps := android.Paths{a.classpathFile}
	aaptCompileDirs := android.Paths{extractedResDir}
	flata := compiledResDir.Join(ctx, "gen_res.flata")
	aapt2CompileDirs(ctx, flata, aaptCompileDirs, aaptCompileDeps)

	a.exportPackage = android.PathForModuleOut(ctx, "package-res.apk")
	srcJar := android.PathForModuleGen(ctx, "R.jar")
	proguardOptionsFile := android.PathForModuleGen(ctx, "proguard.options")

	var linkDeps android.Paths

	linkFlags := []string{
		"--static-lib",
		"--no-static-lib-packages",
		"--auto-add-overlay",
	}

	linkFlags = append(linkFlags, "--manifest "+manifest.String())
	linkDeps = append(linkDeps, manifest)

	// Include dirs
	ctx.VisitDirectDeps(func(module android.Module) {
		var depFiles android.Paths
		if javaDep, ok := module.(Dependency); ok {
			// TODO: shared android libraries
			if ctx.OtherModuleName(module) == "framework-res" {
				depFiles = android.Paths{javaDep.(*AndroidApp).exportPackage}
			}
		}

		for _, dep := range depFiles {
			linkFlags = append(linkFlags, "-I "+dep.String())
		}
		linkDeps = append(linkDeps, depFiles...)
	})

	sdkDep := decodeSdkDep(ctx, String(a.properties.Sdk_version))
	if sdkDep.useFiles {
		linkFlags = append(linkFlags, "-I "+sdkDep.jar.String())
		linkDeps = append(linkDeps, sdkDep.jar)
	}

	aapt2Link(ctx, a.exportPackage, srcJar, proguardOptionsFile,
		linkFlags, linkDeps, nil, android.Paths{flata})
}

var _ Dependency = (*AARImport)(nil)

func (a *AARImport) HeaderJars() android.Paths {
	return android.Paths{a.classpathFile}
}

func (a *AARImport) ImplementationJars() android.Paths {
	return android.Paths{a.classpathFile}
}

func (a *AARImport) AidlIncludeDirs() android.Paths {
	return nil
}

var _ android.PrebuiltInterface = (*Import)(nil)

func AARImportFactory() android.Module {
	module := &AARImport{}

	module.AddProperties(&module.properties)

	android.InitPrebuiltModule(module, &module.properties.Aars)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}
