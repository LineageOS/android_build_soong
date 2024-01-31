// Copyright (C) 2021 The Android Open Source Project
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
	"strings"

	"android/soong/android"
)

// Contains 'deapexer' a private module type used by 'prebuilt_apex' to make dex files contained
// within a .apex file referenced by `prebuilt_apex` available for use by their associated
// `java_import` modules.
//
// An 'apex' module references `java_library` modules from which .dex files are obtained that are
// stored in the resulting `.apex` file. The resulting `.apex` file is then made available as a
// prebuilt by referencing it from a `prebuilt_apex`. For each such `java_library` that is used by
// modules outside the `.apex` file a `java_import` prebuilt is made available referencing a jar
// that contains the Java classes.
//
// When building a Java module type, e.g. `java_module` or `android_app` against such prebuilts the
// `java_import` provides the classes jar  (jar containing `.class` files) against which the
// module's `.java` files are compiled. That classes jar usually contains only stub classes. The
// resulting classes jar is converted into a dex jar (jar containing `.dex` files). Then if
// necessary the dex jar is further processed by `dexpreopt` to produce an optimized form of the
// library specific to the current Android version. This process requires access to implementation
// dex jars for each `java_import`. The `java_import` will obtain the implementation dex jar from
// the `.apex` file in the associated `prebuilt_apex`.
//
// This is intentionally not registered by name as it is not intended to be used from within an
// `Android.bp` file.

// DeapexerProperties specifies the properties supported by the deapexer module.
//
// As these are never intended to be supplied in a .bp file they use a different naming convention
// to make it clear that they are different.
type DeapexerProperties struct {
	// List of common modules that may need access to files exported by this module.
	//
	// A common module in this sense is one that is not arch specific but uses a common variant for
	// all architectures, e.g. java.
	CommonModules []string

	// List of modules that use an embedded .prof to guide optimization of the equivalent dexpreopt artifact
	// This is a subset of CommonModules
	DexpreoptProfileGuidedModules []string

	// List of files exported from the .apex file by this module
	//
	// Each entry is a path from the apex root, e.g. javalib/core-libart.jar.
	ExportedFiles []string
}

type SelectedApexProperties struct {
	// The path to the apex selected for use by this module.
	//
	// Is tagged as `android:"path"` because it will usually contain a string of the form ":<module>"
	// and is tagged as "`blueprint:"mutate"` because it is only initialized in a LoadHook not an
	// Android.bp file.
	Selected_apex *string `android:"path" blueprint:"mutated"`
}

type Deapexer struct {
	android.ModuleBase

	properties             DeapexerProperties
	selectedApexProperties SelectedApexProperties

	inputApex android.Path
}

// Returns the name of the deapexer module corresponding to an APEX module with the given name.
func deapexerModuleName(apexModuleName string) string {
	return apexModuleName + ".deapexer"
}

// Returns the name of the APEX module corresponding to an deapexer module with
// the given name. This reverses deapexerModuleName.
func apexModuleName(deapexerModuleName string) string {
	return strings.TrimSuffix(deapexerModuleName, ".deapexer")
}

func privateDeapexerFactory() android.Module {
	module := &Deapexer{}
	module.AddProperties(&module.properties, &module.selectedApexProperties)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (p *Deapexer) DepsMutator(ctx android.BottomUpMutatorContext) {
	// Add dependencies from the java modules to which this exports files from the `.apex` file onto
	// this module so that they can access the `DeapexerInfo` object that this provides.
	// TODO: b/308174306 - Once all the mainline modules have been flagged, drop this dependency edge
	for _, lib := range p.properties.CommonModules {
		dep := prebuiltApexExportedModuleName(ctx, lib)
		ctx.AddReverseDependency(ctx.Module(), android.DeapexerTag, dep)
	}
}

func (p *Deapexer) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.inputApex = android.OptionalPathForModuleSrc(ctx, p.selectedApexProperties.Selected_apex).Path()

	// Create and remember the directory into which the .apex file's contents will be unpacked.
	deapexerOutput := android.PathForModuleOut(ctx, "deapexer")

	exports := make(map[string]android.WritablePath)

	// Create mappings from apex relative path to the extracted file's path.
	exportedPaths := make(android.Paths, 0, len(exports))
	for _, path := range p.properties.ExportedFiles {
		// Populate the exports that this makes available.
		extractedPath := deapexerOutput.Join(ctx, path)
		exports[path] = extractedPath
		exportedPaths = append(exportedPaths, extractedPath)
	}

	// If the prebuilt_apex exports any files then create a build rule that unpacks the apex using
	// deapexer and verifies that all the required files were created. Also, make the mapping from
	// apex relative path to extracted file path available for other modules.
	if len(exports) > 0 {
		// Make the information available for other modules.
		di := android.NewDeapexerInfo(apexModuleName(ctx.ModuleName()), exports, p.properties.CommonModules)
		di.AddDexpreoptProfileGuidedExportedModuleNames(p.properties.DexpreoptProfileGuidedModules...)
		android.SetProvider(ctx, android.DeapexerProvider, di)

		// Create a sorted list of the files that this exports.
		exportedPaths = android.SortedUniquePaths(exportedPaths)

		// The apex needs to export some files so create a ninja rule to unpack the apex and check that
		// the required files are present.
		builder := android.NewRuleBuilder(pctx, ctx)
		command := builder.Command()
		command.
			Tool(android.PathForSource(ctx, "build/soong/scripts/unpack-prebuilt-apex.sh")).
			BuiltTool("deapexer").
			BuiltTool("debugfs").
			BuiltTool("fsck.erofs").
			Input(p.inputApex).
			Text(deapexerOutput.String())
		for _, p := range exportedPaths {
			command.Output(p.(android.WritablePath))
		}
		builder.Build("deapexer", "deapex "+apexModuleName(ctx.ModuleName()))
	}
}
