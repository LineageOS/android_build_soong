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

package java

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
	pctx.VariableFunc("aidlCmd", func(c interface{}) (string, error) {
		return c.(common.Config).HostBinTool("aidl")
	})
	pctx.StaticVariable("logtagsCmd", "${srcDir}/build/tools/java-event-log-tags.py")
	pctx.StaticVariable("mergeLogtagsCmd", "${srcDir}/build/tools/merge-event-log-tags.py")
	pctx.VariableConfigMethod("srcDir", common.Config.SrcDir)

	pctx.VariableFunc("allLogtagsFile", func(c interface{}) (string, error) {
		return filepath.Join(c.(common.Config).IntermediatesDir(), "all-event-log-tags.txt"), nil
	})
}

var (
	aidl = pctx.StaticRule("aidl",
		blueprint.RuleParams{
			Command:     "$aidlCmd -d$depFile $aidlFlags $in $out",
			Description: "aidl $out",
		},
		"depFile", "aidlFlags")

	logtags = pctx.StaticRule("logtags",
		blueprint.RuleParams{
			Command:     "$logtagsCmd -o $out $in $allLogtagsFile",
			Description: "logtags $out",
		})

	mergeLogtags = pctx.StaticRule("mergeLogtags",
		blueprint.RuleParams{
			Command:     "$mergeLogtagsCmd -o $out $in",
			Description: "merge logtags $out",
		})
)

func genAidl(ctx common.AndroidModuleContext, aidlFile, aidlFlags string) string {
	javaFile := common.SrcDirRelPath(ctx, aidlFile)
	javaFile = filepath.Join(common.ModuleGenDir(ctx), javaFile)
	javaFile = pathtools.ReplaceExtension(javaFile, "java")
	depFile := javaFile + ".d"

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      aidl,
		Outputs:   []string{javaFile},
		Inputs:    []string{aidlFile},
		Implicits: []string{"$aidlCmd"},
		Args: map[string]string{
			"depFile":   depFile,
			"aidlFlags": aidlFlags,
		},
	})

	return javaFile
}

func genLogtags(ctx common.AndroidModuleContext, logtagsFile string) string {
	javaFile := common.SrcDirRelPath(ctx, logtagsFile)
	javaFile = filepath.Join(common.ModuleGenDir(ctx), javaFile)
	javaFile = pathtools.ReplaceExtension(javaFile, "java")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      logtags,
		Outputs:   []string{javaFile},
		Inputs:    []string{logtagsFile},
		Implicits: []string{"$logtagsCmd"},
	})

	return javaFile
}

func (j *javaBase) genSources(ctx common.AndroidModuleContext, srcFiles []string,
	flags javaBuilderFlags) []string {

	for i, srcFile := range srcFiles {
		switch filepath.Ext(srcFile) {
		case ".aidl":
			javaFile := genAidl(ctx, srcFile, flags.aidlFlags)
			srcFiles[i] = javaFile
		case ".logtags":
			j.logtagsSrcs = append(j.logtagsSrcs, srcFile)
			javaFile := genLogtags(ctx, srcFile)
			srcFiles[i] = javaFile
		}
	}

	return srcFiles
}

func LogtagsSingleton() blueprint.Singleton {
	return &logtagsSingleton{}
}

type logtagsProducer interface {
	logtags() []string
}

type logtagsSingleton struct{}

func (l *logtagsSingleton) GenerateBuildActions(ctx blueprint.SingletonContext) {
	var allLogtags []string
	ctx.VisitAllModules(func(module blueprint.Module) {
		if logtags, ok := module.(logtagsProducer); ok {
			allLogtags = append(allLogtags, logtags.logtags()...)
		}
	})

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    mergeLogtags,
		Outputs: []string{"$allLogtagsFile"},
		Inputs:  allLogtags,
	})
}
