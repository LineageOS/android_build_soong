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
	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	pctx.HostBinToolVariable("aidlCmd", "aidl")
	pctx.HostBinToolVariable("syspropCmd", "sysprop_java")
	pctx.SourcePathVariable("logtagsCmd", "build/tools/java-event-log-tags.py")
	pctx.SourcePathVariable("mergeLogtagsCmd", "build/tools/merge-event-log-tags.py")
}

var (
	aidl = pctx.AndroidStaticRule("aidl",
		blueprint.RuleParams{
			Command:     "$aidlCmd -d$depFile $aidlFlags $in $out",
			CommandDeps: []string{"$aidlCmd"},
		},
		"depFile", "aidlFlags")

	logtags = pctx.AndroidStaticRule("logtags",
		blueprint.RuleParams{
			Command:     "$logtagsCmd -o $out $in",
			CommandDeps: []string{"$logtagsCmd"},
		})

	mergeLogtags = pctx.AndroidStaticRule("mergeLogtags",
		blueprint.RuleParams{
			Command:     "$mergeLogtagsCmd -o $out $in",
			CommandDeps: []string{"$mergeLogtagsCmd"},
		})

	sysprop = pctx.AndroidStaticRule("sysprop",
		blueprint.RuleParams{
			Command: `rm -rf $out.tmp && mkdir -p $out.tmp && ` +
				`$syspropCmd --java-output-dir $out.tmp $in && ` +
				`${config.SoongZipCmd} -jar -o $out -C $out.tmp -D $out.tmp && rm -rf $out.tmp`,
			CommandDeps: []string{
				"$syspropCmd",
				"${config.SoongZipCmd}",
			},
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

func genSysprop(ctx android.ModuleContext, syspropFile android.Path) android.Path {
	srcJarFile := android.GenPathWithExt(ctx, "sysprop", syspropFile, "srcjar")

	ctx.Build(pctx, android.BuildParams{
		Rule:        sysprop,
		Description: "sysprop_java " + syspropFile.Rel(),
		Output:      srcJarFile,
		Input:       syspropFile,
	})

	return srcJarFile
}

func (j *Module) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags javaBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))

	for _, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".aidl":
			javaFile := genAidl(ctx, srcFile, flags.aidlFlags, flags.aidlDeps)
			outSrcFiles = append(outSrcFiles, javaFile)
		case ".logtags":
			j.logtagsSrcs = append(j.logtagsSrcs, srcFile)
			javaFile := genLogtags(ctx, srcFile)
			outSrcFiles = append(outSrcFiles, javaFile)
		case ".proto":
			srcJarFile := genProto(ctx, srcFile, flags.proto)
			outSrcFiles = append(outSrcFiles, srcJarFile)
		case ".sysprop":
			srcJarFile := genSysprop(ctx, srcFile)
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
