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

package filesystem

import (
	"android/soong/android"
	"android/soong/linkerconfig"
)

type systemImage struct {
	filesystem

	properties systemImageProperties
}

type systemImageProperties struct {
	// Path to the input linker config json file.
	Linker_config_src *string
}

// android_system_image is a specialization of android_filesystem for the 'system' partition.
// Currently, the only difference is the inclusion of linker.config.pb file which specifies
// the provided and the required libraries to and from APEXes.
func systemImageFactory() android.Module {
	module := &systemImage{}
	module.AddProperties(&module.properties)
	module.filesystem.buildExtraFiles = module.buildExtraFiles
	module.filesystem.filterPackagingSpec = module.filterPackagingSpec
	initFilesystemModule(&module.filesystem)
	return module
}

func (s *systemImage) buildExtraFiles(ctx android.ModuleContext, root android.OutputPath) android.OutputPaths {
	if s.filesystem.properties.Partition_type != nil {
		ctx.PropertyErrorf("partition_type", "partition_type must be unset on an android_system_image module. It is assumed to be 'system'.")
	}
	lc := s.buildLinkerConfigFile(ctx, root)
	// Add more files if needed
	return []android.OutputPath{lc}
}

func (s *systemImage) buildLinkerConfigFile(ctx android.ModuleContext, root android.OutputPath) android.OutputPath {
	input := android.PathForModuleSrc(ctx, android.String(s.properties.Linker_config_src))
	output := root.Join(ctx, "system", "etc", "linker.config.pb")

	// we need "Module"s for packaging items
	modulesInPackageByModule := make(map[android.Module]bool)
	modulesInPackageByName := make(map[string]bool)

	deps := s.gatherFilteredPackagingSpecs(ctx)
	ctx.WalkDeps(func(child, parent android.Module) bool {
		for _, ps := range child.PackagingSpecs() {
			if _, ok := deps[ps.RelPathInPackage()]; ok {
				modulesInPackageByModule[child] = true
				modulesInPackageByName[child.Name()] = true
				return true
			}
		}
		return true
	})

	provideModules := make([]android.Module, 0, len(modulesInPackageByModule))
	for mod := range modulesInPackageByModule {
		provideModules = append(provideModules, mod)
	}

	var requireModules []android.Module
	ctx.WalkDeps(func(child, parent android.Module) bool {
		_, parentInPackage := modulesInPackageByModule[parent]
		_, childInPackageName := modulesInPackageByName[child.Name()]

		// When parent is in the package, and child (or its variant) is not, this can be from an interface.
		if parentInPackage && !childInPackageName {
			requireModules = append(requireModules, child)
		}
		return true
	})

	builder := android.NewRuleBuilder(pctx, ctx)
	linkerconfig.BuildLinkerConfig(ctx, builder, input, provideModules, requireModules, output)
	builder.Build("conv_linker_config", "Generate linker config protobuf "+output.String())
	return output
}

// Filter the result of GatherPackagingSpecs to discard items targeting outside "system" partition.
// Note that "apex" module installs its contents to "apex"(fake partition) as well
// for symbol lookup by imitating "activated" paths.
func (s *systemImage) filterPackagingSpec(ps android.PackagingSpec) bool {
	return s.filesystem.filterInstallablePackagingSpec(ps) && ps.Partition() == "system"
}
