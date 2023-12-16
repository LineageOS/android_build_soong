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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/android"
)

func init() {
	pctx.SourcePathVariable("logtagsCmd", "build/make/tools/java-event-log-tags.py")
	pctx.SourcePathVariable("mergeLogtagsCmd", "build/make/tools/merge-event-log-tags.py")
	pctx.SourcePathVariable("logtagsLib", "build/make/tools/event_log_tags.py")
}

var (
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

func genAidl(ctx android.ModuleContext, aidlFiles android.Paths, aidlGlobalFlags string, aidlIndividualFlags map[string]string, deps android.Paths) android.Paths {
	// Shard aidl files into groups of 50 to avoid having to recompile all of them if one changes and to avoid
	// hitting command line length limits.
	shards := android.ShardPaths(aidlFiles, 50)

	srcJarFiles := make(android.Paths, 0, len(shards))

	for i, shard := range shards {
		srcJarFile := android.PathForModuleGen(ctx, "aidl", "aidl"+strconv.Itoa(i)+".srcjar")
		srcJarFiles = append(srcJarFiles, srcJarFile)

		outDir := srcJarFile.ReplaceExtension(ctx, "tmp")

		rule := android.NewRuleBuilder(pctx, ctx)

		rule.Command().Text("rm -rf").Flag(outDir.String())
		rule.Command().Text("mkdir -p").Flag(outDir.String())
		rule.Command().Text("FLAGS=' " + aidlGlobalFlags + "'")

		for _, aidlFile := range shard {
			localFlag := aidlIndividualFlags[aidlFile.String()]
			depFile := srcJarFile.InSameDir(ctx, aidlFile.String()+".d")
			javaFile := outDir.Join(ctx, pathtools.ReplaceExtension(aidlFile.String(), "java"))
			rule.Command().
				Tool(ctx.Config().HostToolPath(ctx, "aidl")).
				FlagWithDepFile("-d", depFile).
				Flag("$FLAGS").
				Flag(localFlag).
				Input(aidlFile).
				Output(javaFile).
				Implicits(deps)
			rule.Temporary(javaFile)
		}

		rule.Command().
			Tool(ctx.Config().HostToolPath(ctx, "soong_zip")).
			Flag("-srcjar").
			Flag("-write_if_changed").
			FlagWithOutput("-o ", srcJarFile).
			FlagWithArg("-C ", outDir.String()).
			FlagWithArg("-D ", outDir.String())

		rule.Command().Text("rm -rf").Flag(outDir.String())

		rule.Restat()

		ruleName := "aidl"
		ruleDesc := "aidl"
		if len(shards) > 1 {
			ruleName += "_" + strconv.Itoa(i)
			ruleDesc += " " + strconv.Itoa(i)
		}

		rule.Build(ruleName, ruleDesc)
	}

	return srcJarFiles
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

// genAidlIncludeFlags returns additional include flags based on the relative path
// of each .aidl file passed in srcFiles. excludeDirs is a list of paths relative to
// the Android checkout root that should not be included in the returned flags.
func genAidlIncludeFlags(ctx android.PathContext, srcFiles android.Paths, excludeDirs android.Paths) string {
	var baseDirs []string
	excludeDirsStrings := excludeDirs.Strings()
	for _, srcFile := range srcFiles {
		if srcFile.Ext() == ".aidl" {
			baseDir := strings.TrimSuffix(srcFile.String(), srcFile.Rel())
			baseDir = filepath.Clean(baseDir)
			baseDirSeen := android.InList(baseDir, baseDirs) || android.InList(baseDir, excludeDirsStrings)

			if baseDir != "" && !baseDirSeen {
				baseDirs = append(baseDirs, baseDir)
			}
		}
	}
	return android.JoinWithPrefix(baseDirs, " -I")
}

func (j *Module) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags javaBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))
	var protoSrcs android.Paths
	var aidlSrcs android.Paths

	for _, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".aidl":
			aidlSrcs = append(aidlSrcs, srcFile)
		case ".logtags":
			j.logtagsSrcs = append(j.logtagsSrcs, srcFile)
			javaFile := genLogtags(ctx, srcFile)
			outSrcFiles = append(outSrcFiles, javaFile)
		case ".proto":
			protoSrcs = append(protoSrcs, srcFile)
		default:
			outSrcFiles = append(outSrcFiles, srcFile)
		}
	}

	// Process all proto files together to support sharding them into one or more rules that produce srcjars.
	if len(protoSrcs) > 0 {
		srcJarFiles := genProto(ctx, protoSrcs, flags.proto)
		outSrcFiles = append(outSrcFiles, srcJarFiles...)
	}

	// Process all aidl files together to support sharding them into one or more rules that produce srcjars.
	if len(aidlSrcs) > 0 {
		individualFlags := make(map[string]string)
		for _, aidlSrc := range aidlSrcs {
			flags := j.individualAidlFlags(ctx, aidlSrc)
			if flags != "" {
				individualFlags[aidlSrc.String()] = flags
			}
		}
		srcJarFiles := genAidl(ctx, aidlSrcs, flags.aidlFlags, individualFlags, flags.aidlDeps)
		outSrcFiles = append(outSrcFiles, srcJarFiles...)
	}

	return outSrcFiles
}

func LogtagsSingleton() android.Singleton {
	return &logtagsSingleton{}
}

type logtagsProducer interface {
	logtags() android.Paths
}

func (j *Module) logtags() android.Paths {
	return j.logtagsSrcs
}

var _ logtagsProducer = (*Module)(nil)

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
