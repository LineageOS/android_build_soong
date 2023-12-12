// Copyright 2017 Google Inc. All rights reserved.
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

package cc

import (
	"path/filepath"
	"runtime"
	"strings"

	"android/soong/android"
	"github.com/google/blueprint"
)

func init() {
	pctx.VariableFunc("rsCmd", func(ctx android.PackageVarContext) string {
		if ctx.Config().AlwaysUsePrebuiltSdks() {
			// Use RenderScript prebuilts for unbundled builds
			return filepath.Join("prebuilts/sdk/tools", runtime.GOOS, "bin/llvm-rs-cc")
		} else {
			return ctx.Config().HostToolPath(ctx, "llvm-rs-cc").String()
		}
	})
}

var rsCppCmdLine = strings.Replace(`
${rsCmd} -o ${outDir} -d ${outDir} -a ${out} -MD -reflect-c++ ${rsFlags} $in &&
echo '${out}: \' > ${out}.d &&
for f in ${depFiles}; do cat $${f} | awk 'start { sub(/( \\)?$$/, " \\"); print } /:/ { start=1 }' >> ${out}.d; done &&
touch $out
`, "\n", "", -1)

var (
	rsCpp = pctx.AndroidStaticRule("rsCpp",
		blueprint.RuleParams{
			Command:     rsCppCmdLine,
			CommandDeps: []string{"$rsCmd"},
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
		},
		"depFiles", "outDir", "rsFlags", "stampFile")
)

// Takes a path to a .rscript or .fs file, and returns a path to a generated ScriptC_*.cpp file
// This has to match the logic in llvm-rs-cc in DetermineOutputFile.
func rsGeneratedCppFile(ctx android.ModuleContext, rsFile android.Path) android.WritablePath {
	fileName := strings.TrimSuffix(rsFile.Base(), rsFile.Ext())
	return android.PathForModuleGen(ctx, "rs", "ScriptC_"+fileName+".cpp")
}

func rsGeneratedHFile(ctx android.ModuleContext, rsFile android.Path) android.WritablePath {
	fileName := strings.TrimSuffix(rsFile.Base(), rsFile.Ext())
	return android.PathForModuleGen(ctx, "rs", "ScriptC_"+fileName+".h")
}

func rsGeneratedDepFile(ctx android.ModuleContext, rsFile android.Path) android.WritablePath {
	fileName := strings.TrimSuffix(rsFile.Base(), rsFile.Ext())
	return android.PathForModuleGen(ctx, "rs", fileName+".d")
}

func rsGenerateCpp(ctx android.ModuleContext, rsFiles android.Paths, rsFlags string) android.Paths {
	stampFile := android.PathForModuleGen(ctx, "rs", "rs.stamp")
	depFiles := make(android.WritablePaths, 0, len(rsFiles))
	genFiles := make(android.WritablePaths, 0, 2*len(rsFiles))
	headers := make(android.Paths, 0, len(rsFiles))
	for _, rsFile := range rsFiles {
		depFiles = append(depFiles, rsGeneratedDepFile(ctx, rsFile))
		headerFile := rsGeneratedHFile(ctx, rsFile)
		genFiles = append(genFiles, rsGeneratedCppFile(ctx, rsFile), headerFile)
		headers = append(headers, headerFile)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            rsCpp,
		Description:     "llvm-rs-cc",
		Output:          stampFile,
		ImplicitOutputs: genFiles,
		Inputs:          rsFiles,
		Args: map[string]string{
			"rsFlags":  rsFlags,
			"outDir":   android.PathForModuleGen(ctx, "rs").String(),
			"depFiles": strings.Join(depFiles.Strings(), " "),
		},
	})

	return headers
}

func rsFlags(ctx ModuleContext, flags Flags, properties *BaseCompilerProperties) Flags {
	targetApi := String(properties.Renderscript.Target_api)
	if targetApi == "" && ctx.useSdk() {
		targetApiLevel := android.ApiLevelOrPanic(ctx, ctx.sdkVersion())
		if targetApiLevel.IsCurrent() || targetApiLevel.IsPreview() {
			// If the target level is current or preview, leave the 'target-api' unset.
			// This signals to llvm-rs-cc that the development API should be used.
		} else {
			targetApi = targetApiLevel.String()
		}
	}

	if targetApi != "" {
		flags.rsFlags = append(flags.rsFlags, "-target-api "+targetApi)
	}

	flags.rsFlags = append(flags.rsFlags, "-Wall", "-Werror")
	flags.rsFlags = append(flags.rsFlags, properties.Renderscript.Flags...)
	if ctx.Arch().ArchType.Multilib == "lib64" {
		flags.rsFlags = append(flags.rsFlags, "-m64")
	} else {
		flags.rsFlags = append(flags.rsFlags, "-m32")
	}
	flags.rsFlags = append(flags.rsFlags, "${config.RsGlobalIncludes}")

	rootRsIncludeDirs := android.PathsForSource(ctx, properties.Renderscript.Include_dirs)
	flags.rsFlags = append(flags.rsFlags, includeDirsToFlags(rootRsIncludeDirs))

	flags.Local.CommonFlags = append(flags.Local.CommonFlags,
		"-I"+android.PathForModuleGen(ctx, "rs").String(),
		"-Iframeworks/rs",
		"-Iframeworks/rs/cpp",
	)

	return flags
}
