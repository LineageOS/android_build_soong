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

// Properties that are specific to `deapexer` but which need to be provided on the `prebuilt_apex`
// module.`
type DeapexerProperties struct {
	// List of java libraries that are embedded inside this prebuilt APEX bundle and for which this
	// APEX bundle will provide dex implementation jars for use by dexpreopt and boot jars package
	// check.
	Exported_java_libs []string
}

type Deapexer struct {
	android.ModuleBase
	prebuilt android.Prebuilt

	properties         DeapexerProperties
	apexFileProperties ApexFileProperties

	inputApex android.Path
}

func privateDeapexerFactory() android.Module {
	module := &Deapexer{}
	module.AddProperties(
		&module.properties,
		&module.apexFileProperties,
	)
	android.InitSingleSourcePrebuiltModule(module, &module.apexFileProperties, "Source")
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

func (p *Deapexer) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *Deapexer) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

func (p *Deapexer) DepsMutator(ctx android.BottomUpMutatorContext) {
	if err := p.apexFileProperties.selectSource(ctx); err != nil {
		ctx.ModuleErrorf("%s", err)
		return
	}

	// Add dependencies from the java modules to which this exports files from the `.apex` file onto
	// this module so that they can access the `DeapexerInfo` object that this provides.
	for _, lib := range p.properties.Exported_java_libs {
		dep := prebuiltApexExportedModuleName(ctx, lib)
		ctx.AddReverseDependency(ctx.Module(), android.DeapexerTag, dep)
	}
}

func (p *Deapexer) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.inputApex = p.Prebuilt().SingleSourcePath(ctx)

	// Create and remember the directory into which the .apex file's contents will be unpacked.
	deapexerOutput := android.PathForModuleOut(ctx, "deapexer")

	exports := make(map[string]android.Path)

	// Create mappings from name+tag to all the required exported paths.
	for _, l := range p.properties.Exported_java_libs {
		// Populate the exports that this makes available. The path here must match the path of the
		// file in the APEX created by apexFileForJavaModule(...).
		exports[l+"{.dexjar}"] = deapexerOutput.Join(ctx, "javalib", l+".jar")
	}

	// If the prebuilt_apex exports any files then create a build rule that unpacks the apex using
	// deapexer and verifies that all the required files were created. Also, make the mapping from
	// name+tag to path available for other modules.
	if len(exports) > 0 {
		// Make the information available for other modules.
		ctx.SetProvider(android.DeapexerProvider, android.NewDeapexerInfo(exports))

		// Create a sorted list of the files that this exports.
		exportedPaths := make(android.Paths, 0, len(exports))
		for _, p := range exports {
			exportedPaths = append(exportedPaths, p)
		}
		exportedPaths = android.SortedUniquePaths(exportedPaths)

		// The apex needs to export some files so create a ninja rule to unpack the apex and check that
		// the required files are present.
		builder := android.NewRuleBuilder(pctx, ctx)
		command := builder.Command()
		command.
			Tool(android.PathForSource(ctx, "build/soong/scripts/unpack-prebuilt-apex.sh")).
			BuiltTool("deapexer").
			BuiltTool("debugfs").
			Input(p.inputApex).
			Text(deapexerOutput.String())
		for _, p := range exportedPaths {
			command.Output(p.(android.WritablePath))
		}
		builder.Build("deapexer", "deapex "+ctx.ModuleName())
	}
}
