// Copyright 2023 Google Inc. All rights reserved.
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

package build_flags

import (
	"fmt"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
)

type BuildFlagDeclarationsProviderData struct {
	IntermediateCacheOutputPath android.WritablePath
	IntermediateDumpOutputPath  android.WritablePath
}

var BuildFlagDeclarationsProviderKey = blueprint.NewProvider[BuildFlagDeclarationsProviderData]()

type DeclarationsModule struct {
	android.ModuleBase
	android.DefaultableModuleBase

	// Properties for "aconfig_declarations"
	properties struct {
		// aconfig files, relative to this Android.bp file
		Srcs []string `android:"path"`
	}

	intermediatePath android.WritablePath
}

func DeclarationsFactory() android.Module {
	module := &DeclarationsModule{}

	android.InitAndroidModule(module)
	android.InitDefaultableModule(module)
	module.AddProperties(&module.properties)

	return module
}

func (module *DeclarationsModule) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		// The default output of this module is the intermediates format, which is
		// not installable and in a private format that no other rules can handle
		// correctly.
		return []android.Path{module.intermediatePath}, nil
	default:
		return nil, fmt.Errorf("unsupported build_flags_declarations module reference tag %q", tag)
	}
}

func joinAndPrefix(prefix string, values []string) string {
	var sb strings.Builder
	for _, v := range values {
		sb.WriteString(prefix)
		sb.WriteString(v)
	}
	return sb.String()
}

func optionalVariable(prefix string, value string) string {
	var sb strings.Builder
	if value != "" {
		sb.WriteString(prefix)
		sb.WriteString(value)
	}
	return sb.String()
}

func (module *DeclarationsModule) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// Intermediate format
	declarationFiles := android.PathsForModuleSrc(ctx, module.properties.Srcs)
	intermediateCacheFilePath := android.PathForModuleOut(ctx, "build_flag_intermediate.pb")
	inputFiles := make([]android.Path, len(declarationFiles))
	copy(inputFiles, declarationFiles)

	// TODO(lamont): generate the rc_proto.FlagArtifacts message for the sources.
	args := map[string]string{
		"release_version": ctx.Config().ReleaseVersion(),
		"declarations":    android.JoinPathsWithPrefix(declarationFiles, "--decl "),
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        buildFlagRule,
		Output:      intermediateCacheFilePath,
		Inputs:      inputFiles,
		Description: "build_flag_declarations",
		Args:        args,
	})

	intermediateDumpFilePath := android.PathForModuleOut(ctx, "build_flag_intermediate.textproto")
	ctx.Build(pctx, android.BuildParams{
		Rule:        buildFlagTextRule,
		Output:      intermediateDumpFilePath,
		Input:       intermediateCacheFilePath,
		Description: "build_flag_declarations_text",
	})

	android.SetProvider(ctx, BuildFlagDeclarationsProviderKey, BuildFlagDeclarationsProviderData{
		IntermediateCacheOutputPath: intermediateCacheFilePath,
		IntermediateDumpOutputPath:  intermediateDumpFilePath,
	})
}
