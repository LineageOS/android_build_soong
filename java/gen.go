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

import (
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	pctx.SourcePathVariable("logtagsCmd", "build/make/tools/java-event-log-tags.py")
	pctx.SourcePathVariable("mergeLogtagsCmd", "build/make/tools/merge-event-log-tags.py")
	pctx.SourcePathVariable("logtagsLib", "build/make/tools/event_log_tags.py")
}

var (
	aidl = pctx.AndroidStaticRule("aidl",
		blueprint.RuleParams{
			Command:     "${config.AidlCmd} -d$depFile $aidlFlags $in $out",
			CommandDeps: []string{"${config.AidlCmd}"},
		},
		"depFile", "aidlFlags")

	logtags = pctx.AndroidStaticRule("logtags",
		blueprint.RuleParams{
			Command:     "$logtagsCmd -o $out $in",
			CommandDeps: []string{"$logtagsCmd", "$logtagsLib"},
		})

	mergeLogtags = pctx.AndroidStaticRule("mergeLogtags",
		blueprint.RuleParams{
			Command:     "$mergeLogtagsCmd -o $out $in",
			CommandDeps: []string{"$mergeLogtagsCmd", "$logtagsLib"},
		})
)

func genAidl(ctx android.ModuleContext, aidlFile android.Path, aidlFlags string, deps android.Paths) android.Path {
	javaFile := android.GenPathWithExt(ctx, "aidl", aidlFile, "java")
	depFile := javaFile.String() + ".d"

	ctx.Build(pctx, android.BuildParams{
		Rule:        aidl,
		Description: "aidl " + aidlFile.Rel(),
		Output:      javaFile,
		Input:       aidlFile,
		Implicits:   deps,
		Args: map[string]string{
			"depFile":   depFile,
			"aidlFlags": aidlFlags,
		},
	})

	return javaFile
}

func genLogtags(ctx android.ModuleContext, logtagsFile android.Path) android.Path {
	javaFile := android.GenPathWithExt(ctx, "logtags", logtagsFile, "java")

	ctx.Build(pctx, android.BuildParams{
		Rule:        logtags,
		Description: "logtags " + logtagsFile.Rel(),
		Output:      javaFile,
		Input:       logtagsFile,
	})

	return javaFile
}

func genAidlIncludeFlags(srcFiles android.Paths) string {
	var baseDirs []string
	for _, srcFile := range srcFiles {
		if srcFile.Ext() == ".aidl" {
			baseDir := strings.TrimSuffix(srcFile.String(), srcFile.Rel())
			if baseDir != "" && !android.InList(baseDir, baseDirs) {
				baseDirs = append(baseDirs, baseDir)
			}
		}
	}
	return android.JoinWithPrefix(baseDirs, " -I")
}

func (j *Module) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags javaBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))

	aidlIncludeFlags := genAidlIncludeFlags(srcFiles)

	for _, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".aidl":
			javaFile := genAidl(ctx, srcFile, flags.aidlFlags+aidlIncludeFlags, flags.aidlDeps)
			outSrcFiles = append(outSrcFiles, javaFile)
		case ".logtags":
			j.logtagsSrcs = append(j.logtagsSrcs, srcFile)
			javaFile := genLogtags(ctx, srcFile)
			outSrcFiles = append(outSrcFiles, javaFile)
		case ".proto":
			srcJarFile := genProto(ctx, srcFile, flags.proto)
			outSrcFiles = append(outSrcFiles, srcJarFile)
		default:
			outSrcFiles = append(outSrcFiles, srcFile)
		}
	}

	return outSrcFiles
}

func LogtagsSingleton() android.Singleton {
	return &logtagsSingleton{}
}

type logtagsProducer interface {
	logtags() android.Paths
}

type logtagsSingleton struct{}

func (l *logtagsSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var allLogtags android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		if logtags, ok := module.(logtagsProducer); ok {
			allLogtags = append(allLogtags, logtags.logtags()...)
		}
	})

	ctx.Build(pctx, android.BuildParams{
		Rule:        mergeLogtags,
		Description: "merge logtags",
		Output:      android.PathForIntermediates(ctx, "all-event-log-tags.txt"),
		Inputs:      allLogtags,
	})
}
