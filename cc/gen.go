// Copyright 2015 Google Inc. All rights reserved.
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
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	pctx.SourcePathVariable("lexCmd", "prebuilts/build-tools/${config.HostPrebuiltTag}/bin/flex")
	pctx.SourcePathVariable("m4Cmd", "prebuilts/build-tools/${config.HostPrebuiltTag}/bin/m4")

	pctx.HostBinToolVariable("aidlCmd", "aidl-cpp")
	pctx.HostBinToolVariable("syspropCmd", "sysprop_cpp")
}

var (
	lex = pctx.AndroidStaticRule("lex",
		blueprint.RuleParams{
			Command:     "M4=$m4Cmd $lexCmd -o$out $in",
			CommandDeps: []string{"$lexCmd", "$m4Cmd"},
		})

	sysprop = pctx.AndroidStaticRule("sysprop",
		blueprint.RuleParams{
			Command: "$syspropCmd --header-dir=$headerOutDir --public-header-dir=$publicOutDir " +
				"--source-dir=$srcOutDir --include-name=$includeName $in",
			CommandDeps: []string{"$syspropCmd"},
		},
		"headerOutDir", "publicOutDir", "srcOutDir", "includeName")

	windmc = pctx.AndroidStaticRule("windmc",
		blueprint.RuleParams{
			Command:     "$windmcCmd -r$$(dirname $out) -h$$(dirname $out) $in",
			CommandDeps: []string{"$windmcCmd"},
		},
		"windmcCmd")
)

type YaccProperties struct {
	// list of module-specific flags that will be used for .y and .yy compiles
	Flags []string

	// whether the yacc files will produce a location.hh file
	Gen_location_hh *bool

	// whether the yacc files will product a position.hh file
	Gen_position_hh *bool
}

func genYacc(ctx android.ModuleContext, rule *android.RuleBuilder, yaccFile android.Path,
	outFile android.ModuleGenPath, props *YaccProperties) (headerFiles android.Paths) {

	outDir := android.PathForModuleGen(ctx, "yacc")
	headerFile := android.GenPathWithExt(ctx, "yacc", yaccFile, "h")
	ret := android.Paths{headerFile}

	cmd := rule.Command()

	// Fix up #line markers to not use the sbox temporary directory
	sedCmd := "sed -i.bak 's#__SBOX_OUT_DIR__#" + outDir.String() + "#'"
	rule.Command().Text(sedCmd).Input(outFile)
	rule.Command().Text(sedCmd).Input(headerFile)

	var flags []string
	if props != nil {
		flags = props.Flags

		if Bool(props.Gen_location_hh) {
			locationHeader := outFile.InSameDir(ctx, "location.hh")
			ret = append(ret, locationHeader)
			cmd.ImplicitOutput(locationHeader)
			rule.Command().Text(sedCmd).Input(locationHeader)
		}
		if Bool(props.Gen_position_hh) {
			positionHeader := outFile.InSameDir(ctx, "position.hh")
			ret = append(ret, positionHeader)
			cmd.ImplicitOutput(positionHeader)
			rule.Command().Text(sedCmd).Input(positionHeader)
		}
	}

	cmd.Text("BISON_PKGDATADIR=prebuilts/build-tools/common/bison").
		FlagWithInput("M4=", ctx.Config().PrebuiltBuildTool(ctx, "m4")).
		PrebuiltBuildTool(ctx, "bison").
		Flag("-d").
		Flags(flags).
		FlagWithOutput("--defines=", headerFile).
		Flag("-o").Output(outFile).Input(yaccFile)

	return ret
}

func genAidl(ctx android.ModuleContext, rule *android.RuleBuilder, aidlFile android.Path,
	outFile, depFile android.ModuleGenPath, aidlFlags string) android.Paths {

	aidlPackage := strings.TrimSuffix(aidlFile.Rel(), aidlFile.Base())
	baseName := strings.TrimSuffix(aidlFile.Base(), aidlFile.Ext())
	shortName := baseName
	// TODO(b/111362593): aidl_to_cpp_common.cpp uses heuristics to figure out if
	//   an interface name has a leading I. Those same heuristics have been
	//   moved here.
	if len(baseName) >= 2 && baseName[0] == 'I' &&
		strings.ToUpper(baseName)[1] == baseName[1] {
		shortName = strings.TrimPrefix(baseName, "I")
	}

	outDir := android.PathForModuleGen(ctx, "aidl")
	headerI := outDir.Join(ctx, aidlPackage, baseName+".h")
	headerBn := outDir.Join(ctx, aidlPackage, "Bn"+shortName+".h")
	headerBp := outDir.Join(ctx, aidlPackage, "Bp"+shortName+".h")

	baseDir := strings.TrimSuffix(aidlFile.String(), aidlFile.Rel())
	if baseDir != "" {
		aidlFlags += " -I" + baseDir
	}

	cmd := rule.Command()
	cmd.BuiltTool(ctx, "aidl-cpp").
		FlagWithDepFile("-d", depFile).
		Flag("--ninja").
		Flag(aidlFlags).
		Input(aidlFile).
		OutputDir().
		Output(outFile).
		ImplicitOutputs(android.WritablePaths{
			headerI,
			headerBn,
			headerBp,
		})

	return android.Paths{
		headerI,
		headerBn,
		headerBp,
	}
}

func genLex(ctx android.ModuleContext, lexFile android.Path, outFile android.ModuleGenPath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        lex,
		Description: "lex " + lexFile.Rel(),
		Output:      outFile,
		Input:       lexFile,
	})
}

func genSysprop(ctx android.ModuleContext, syspropFile android.Path) (android.Path, android.Paths) {
	headerFile := android.PathForModuleGen(ctx, "sysprop", "include", syspropFile.Rel()+".h")
	publicHeaderFile := android.PathForModuleGen(ctx, "sysprop/public", "include", syspropFile.Rel()+".h")
	cppFile := android.PathForModuleGen(ctx, "sysprop", syspropFile.Rel()+".cpp")

	headers := android.WritablePaths{headerFile, publicHeaderFile}

	ctx.Build(pctx, android.BuildParams{
		Rule:            sysprop,
		Description:     "sysprop " + syspropFile.Rel(),
		Output:          cppFile,
		ImplicitOutputs: headers,
		Input:           syspropFile,
		Args: map[string]string{
			"headerOutDir": filepath.Dir(headerFile.String()),
			"publicOutDir": filepath.Dir(publicHeaderFile.String()),
			"srcOutDir":    filepath.Dir(cppFile.String()),
			"includeName":  syspropFile.Rel() + ".h",
		},
	})

	return cppFile, headers.Paths()
}

func genWinMsg(ctx android.ModuleContext, srcFile android.Path, flags builderFlags) (android.Path, android.Path) {
	headerFile := android.GenPathWithExt(ctx, "windmc", srcFile, "h")
	rcFile := android.GenPathWithExt(ctx, "windmc", srcFile, "rc")

	windmcCmd := gccCmd(flags.toolchain, "windmc")

	ctx.Build(pctx, android.BuildParams{
		Rule:           windmc,
		Description:    "windmc " + srcFile.Rel(),
		Output:         rcFile,
		ImplicitOutput: headerFile,
		Input:          srcFile,
		Args: map[string]string{
			"windmcCmd": windmcCmd,
		},
	})

	return rcFile, headerFile
}

func genSources(ctx android.ModuleContext, srcFiles android.Paths,
	buildFlags builderFlags) (android.Paths, android.Paths) {

	var deps android.Paths
	var rsFiles android.Paths

	var aidlRule *android.RuleBuilder

	var yaccRule_ *android.RuleBuilder
	yaccRule := func() *android.RuleBuilder {
		if yaccRule_ == nil {
			yaccRule_ = android.NewRuleBuilder().Sbox(android.PathForModuleGen(ctx, "yacc"))
		}
		return yaccRule_
	}

	for i, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".y":
			cFile := android.GenPathWithExt(ctx, "yacc", srcFile, "c")
			srcFiles[i] = cFile
			deps = append(deps, genYacc(ctx, yaccRule(), srcFile, cFile, buildFlags.yacc)...)
		case ".yy":
			cppFile := android.GenPathWithExt(ctx, "yacc", srcFile, "cpp")
			srcFiles[i] = cppFile
			deps = append(deps, genYacc(ctx, yaccRule(), srcFile, cppFile, buildFlags.yacc)...)
		case ".l":
			cFile := android.GenPathWithExt(ctx, "lex", srcFile, "c")
			srcFiles[i] = cFile
			genLex(ctx, srcFile, cFile)
		case ".ll":
			cppFile := android.GenPathWithExt(ctx, "lex", srcFile, "cpp")
			srcFiles[i] = cppFile
			genLex(ctx, srcFile, cppFile)
		case ".proto":
			ccFile, headerFile := genProto(ctx, srcFile, buildFlags)
			srcFiles[i] = ccFile
			deps = append(deps, headerFile)
		case ".aidl":
			if aidlRule == nil {
				aidlRule = android.NewRuleBuilder().Sbox(android.PathForModuleGen(ctx, "aidl"))
			}
			cppFile := android.GenPathWithExt(ctx, "aidl", srcFile, "cpp")
			depFile := android.GenPathWithExt(ctx, "aidl", srcFile, "cpp.d")
			srcFiles[i] = cppFile
			deps = append(deps, genAidl(ctx, aidlRule, srcFile, cppFile, depFile, buildFlags.aidlFlags)...)
		case ".rscript", ".fs":
			cppFile := rsGeneratedCppFile(ctx, srcFile)
			rsFiles = append(rsFiles, srcFiles[i])
			srcFiles[i] = cppFile
		case ".mc":
			rcFile, headerFile := genWinMsg(ctx, srcFile, buildFlags)
			srcFiles[i] = rcFile
			deps = append(deps, headerFile)
		case ".sysprop":
			cppFile, headerFiles := genSysprop(ctx, srcFile)
			srcFiles[i] = cppFile
			deps = append(deps, headerFiles...)
		}
	}

	if aidlRule != nil {
		aidlRule.Build(pctx, ctx, "aidl", "gen aidl")
	}

	if yaccRule_ != nil {
		yaccRule_.Build(pctx, ctx, "yacc", "gen yacc")
	}

	if len(rsFiles) > 0 {
		deps = append(deps, rsGenerateCpp(ctx, rsFiles, buildFlags.rsFlags)...)
	}

	return srcFiles, deps
}
