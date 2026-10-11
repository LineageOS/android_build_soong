// Copyright 2024 Google Inc. All rights reserved.
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

package systemfeatures

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

var (
	pctx = android.NewPackageContext("android/soong/systemfeatures")
)

func init() {
	registerSystemFeaturesComponents(android.InitRegistrationContext)
}

func registerSystemFeaturesComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_system_features_srcs", JavaSystemFeaturesSrcsFactory)
}

type javaSystemFeaturesSrcs struct {
	android.ModuleBase
	properties struct {
		// The fully qualified class name for the generated code, e.g., com.android.Foo
		Full_class_name string
		// Whether to generate only a simple metadata class with details about the full API surface.
		// This is useful for tools that rely on the mapping from feature names to their generated
		// method names, but don't want the fully generated API class (e.g., for linting).
		Metadata_only *bool
	}

	// Explicitly generated Java sources.
	outputSrcs android.Paths

	// Implicitly generated Proguard flags corresponding to any optimized read-only system features.
	outputProguardFlags android.Paths
}

var _ android.SourceFileGenerator = (*javaSystemFeaturesSrcs)(nil)
var _ android.SourceFileProducer = (*javaSystemFeaturesSrcs)(nil)

func (m *javaSystemFeaturesSrcs) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddHostToolDependencies("cp_if_changed")
}

func (m *javaSystemFeaturesSrcs) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Create a file name appropriate for the given fully qualified (w/ package) class name.
	classNameParts := strings.Split(m.properties.Full_class_name, ".")
	outputSrcFileName := classNameParts[len(classNameParts)-1] + ".java"
	outputSrcFile := android.PathForModuleGen(ctx, outputSrcFileName)
	outputProguardFlagsFile := android.PathForModuleGen(ctx, "proguard.options")
	stagingSrcFile := outputSrcFile.ReplaceExtension(ctx, "java.tmp")
	stagingProguardFlagsFile := outputProguardFlagsFile.ReplaceExtension(ctx, "options.tmp")

	// Collect all RELEASE_SYSTEM_FEATURE_$K:$V build flags into a list of "$K:$V" pairs.
	var features []string
	for k, v := range ctx.Config().ProductVariables().BuildFlags {
		if strings.HasPrefix(k, "RELEASE_SYSTEM_FEATURE_") {
			shortFeatureName := strings.TrimPrefix(k, "RELEASE_SYSTEM_FEATURE_")
			features = append(features, fmt.Sprintf("%s:%s", shortFeatureName, v))
		}
	}
	// Ensure sorted outputs for consistency of flag ordering in ninja outputs.
	sort.Strings(features)

	rule := android.NewRuleBuilder(pctx, ctx).SandboxDisabled()
	rule.Command().Text("rm -f").Text(stagingSrcFile.String())
	rule.Command().Text("rm -f").Text(stagingProguardFlagsFile.String())
	ruleCmd := rule.Command().
		BuiltTool("systemfeatures-gen-tool").
		Flag(m.properties.Full_class_name).
		FlagForEachArg("--feature=", features).
		FlagWithArg("--readonly=", fmt.Sprint(ctx.Config().ReleaseUseSystemFeatureBuildFlags())).
		FlagWithArg("--metadata-only=", fmt.Sprint(proptools.Bool(m.properties.Metadata_only)))

	if ctx.Config().ReleaseUseSystemFeatureXmlForUnavailableFeatures() {
		if featureXmlFiles := uniquePossibleFeatureXmlPaths(ctx); len(featureXmlFiles) > 0 {
			ruleCmd.FlagWithInputList("--unavailable-feature-xml-files=", featureXmlFiles, ",")
		}
	}

	ruleCmd.FlagWithOutput("--output=", stagingSrcFile)
	ruleCmd.FlagWithOutput("--output-proguard-rules=", stagingProguardFlagsFile)
	rule.Command().BuiltTool("cp_if_changed").Input(stagingSrcFile).Output(outputSrcFile)
	rule.Command().BuiltTool("cp_if_changed").Input(stagingProguardFlagsFile).Output(outputProguardFlagsFile)
	rule.Restat().Build(ctx.ModuleName(), "Generating systemfeatures srcs filegroup")

	m.outputSrcs = android.Paths{outputSrcFile}
	m.outputProguardFlags = android.Paths{outputProguardFlagsFile}

	ctx.SetOutputFiles(m.outputSrcs, "")
	ctx.SetOutputFiles(m.outputProguardFlags, ".proguardFlagsFiles")
}

func (m *javaSystemFeaturesSrcs) Srcs() android.Paths {
	return m.outputSrcs
}

func (m *javaSystemFeaturesSrcs) GeneratedSourceFiles() android.Paths {
	return m.outputSrcs
}

func (m *javaSystemFeaturesSrcs) GeneratedDeps() android.Paths {
	return m.outputSrcs
}

func (m *javaSystemFeaturesSrcs) GeneratedHeaderDirs() android.Paths {
	return nil
}

func JavaSystemFeaturesSrcsFactory() android.Module {
	module := &javaSystemFeaturesSrcs{}
	module.AddProperties(&module.properties)
	module.properties.Metadata_only = proptools.BoolPtr(false)
	android.InitAndroidModule(module)
	return module
}

// Generates a list of unique, existent src paths for potential feature XML
// files, as contained in the configured PRODUCT_COPY_FILES listing.
func uniquePossibleFeatureXmlPaths(ctx android.ModuleContext) android.Paths {
	dstPathSeen := make(map[string]bool)
	var possibleSrcPaths []android.Path
	for _, copyFilePair := range ctx.Config().ProductVariables().PartitionVarsForSoongMigrationOnlyDoNotUse.ProductCopyFiles {
		srcDstList := strings.Split(copyFilePair, ":")
		// The length may be >2 (e.g., "$src:$dst:$owner"), but we only care
		// that it has at least "$src:$dst".
		if len(srcDstList) < 2 {
			ctx.ModuleErrorf("PRODUCT_COPY_FILES must follow the format \"src:dest\", got: %s", copyFilePair)
			continue
		}
		src, dst := srcDstList[0], srcDstList[1]

		// We're only interested in `.xml` files (case-insensitive).
		if !strings.EqualFold(filepath.Ext(dst), ".xml") {
			continue
		}

		// We only care about files directly in `*/etc/permissions/` or
		// `*/etc/sysconfig` dirs, not any nested subdirs.
		normalizedDstDir := filepath.ToSlash(filepath.Dir(filepath.Clean(dst)))
		if !strings.HasSuffix(normalizedDstDir, "/etc/permissions") &&
			!strings.HasSuffix(normalizedDstDir, "/etc/sysconfig") {
			continue
		}

		// The first `dst` entry in the PRODUCT_COPY_FILES `src:dst` pairings
		// always takes precedence over latter entries.
		if _, ok := dstPathSeen[dst]; !ok {
			relSrc := android.ToRelativeSourcePath(ctx, src)
			if optionalPath := android.ExistentPathForSource(ctx, relSrc); optionalPath.Valid() {
				dstPathSeen[dst] = true
				possibleSrcPaths = append(possibleSrcPaths, optionalPath.Path())
			}
		}
	}
	// A sorted, unique list ensures stability of ninja build command outputs.
	return android.SortedUniquePaths(possibleSrcPaths)
}
