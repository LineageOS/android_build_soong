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

// This file generates the final rules for compiling all C/C++.  All properties related to
// compiling should have been translated into builderFlags or another argument to the Transform*
// functions.

import (
	"path/filepath"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/common"
)

func init() {
	pctx.StaticVariable("lexCmd", "${SrcDir}/prebuilts/misc/${HostPrebuiltTag}/flex/flex-2.5.39")
	pctx.StaticVariable("yaccCmd", "${SrcDir}/prebuilts/misc/${HostPrebuiltTag}/bison/bison")
	pctx.StaticVariable("yaccDataDir", "${SrcDir}/external/bison/data")
}

var (
	yacc = pctx.StaticRule("yacc",
		blueprint.RuleParams{
			Command: "BISON_PKGDATADIR=$yaccDataDir $yaccCmd -d $yaccFlags -o $cppFile $in && " +
				"cp -f $hppFile $hFile",
			CommandDeps: []string{"$yaccCmd"},
			Description: "yacc $out",
		},
		"yaccFlags", "cppFile", "hppFile", "hFile")

	lex = pctx.StaticRule("lex",
		blueprint.RuleParams{
			Command:     "$lexCmd -o$out $in",
			CommandDeps: []string{"$lexCmd"},
			Description: "lex $out",
		})
)

func genYacc(ctx common.AndroidModuleContext, yaccFile, yaccFlags string) (cppFile, headerFile string) {
	cppFile = common.SrcDirRelPath(ctx, yaccFile)
	cppFile = filepath.Join(common.ModuleGenDir(ctx), cppFile)
	cppFile = pathtools.ReplaceExtension(cppFile, "cpp")
	hppFile := pathtools.ReplaceExtension(cppFile, "hpp")
	headerFile = pathtools.ReplaceExtension(cppFile, "h")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    yacc,
		Outputs: []string{cppFile, headerFile},
		Inputs:  []string{yaccFile},
		Args: map[string]string{
			"yaccFlags": yaccFlags,
			"cppFile":   cppFile,
			"hppFile":   hppFile,
			"hFile":     headerFile,
		},
	})

	return cppFile, headerFile
}

func genLex(ctx common.AndroidModuleContext, lexFile string) (cppFile string) {
	cppFile = common.SrcDirRelPath(ctx, lexFile)
	cppFile = filepath.Join(common.ModuleGenDir(ctx), cppFile)
	cppFile = pathtools.ReplaceExtension(cppFile, "cpp")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    lex,
		Outputs: []string{cppFile},
		Inputs:  []string{lexFile},
	})

	return cppFile
}

func genSources(ctx common.AndroidModuleContext, srcFiles []string,
	buildFlags builderFlags) ([]string, []string) {

	var deps []string

	for i, srcFile := range srcFiles {
		switch filepath.Ext(srcFile) {
		case ".y", ".yy":
			cppFile, headerFile := genYacc(ctx, srcFile, buildFlags.yaccFlags)
			srcFiles[i] = cppFile
			deps = append(deps, headerFile)
		case ".l", ".ll":
			cppFile := genLex(ctx, srcFile)
			srcFiles[i] = cppFile
		}
	}

	return srcFiles, deps
}
