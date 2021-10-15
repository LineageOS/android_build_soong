// Copyright 2019 Google Inc. All rights reserved.
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

package android

import (
	"path/filepath"

	"github.com/google/blueprint"
)

func init() {
	pctx.SourcePathVariable("merge_notices", "build/soong/scripts/mergenotice.py")
	pctx.SourcePathVariable("generate_notice", "build/soong/scripts/generate-notice-files.py")

	pctx.HostBinToolVariable("minigzip", "minigzip")
}

type NoticeOutputs struct {
	Merged       OptionalPath
	TxtOutput    OptionalPath
	HtmlOutput   OptionalPath
	HtmlGzOutput OptionalPath
}

var (
	mergeNoticesRule = pctx.AndroidStaticRule("mergeNoticesRule", blueprint.RuleParams{
		Command:     `${merge_notices} --output $out $in`,
		CommandDeps: []string{"${merge_notices}"},
		Description: "merge notice files into $out",
	})

	generateNoticeRule = pctx.AndroidStaticRule("generateNoticeRule", blueprint.RuleParams{
		Command: `rm -rf $$(dirname $txtOut) $$(dirname $htmlOut) $$(dirname $out) && ` +
			`mkdir -p $$(dirname $txtOut) $$(dirname $htmlOut)  $$(dirname $out) && ` +
			`${generate_notice} --text-output $txtOut --html-output $htmlOut -t "$title" -s $inputDir && ` +
			`${minigzip} -c $htmlOut > $out`,
		CommandDeps: []string{"${generate_notice}", "${minigzip}"},
		Description: "produce notice file $out",
	}, "txtOut", "htmlOut", "title", "inputDir")
)

func MergeNotices(ctx ModuleContext, mergedNotice WritablePath, noticePaths []Path) {
	ctx.Build(pctx, BuildParams{
		Rule:        mergeNoticesRule,
		Description: "merge notices",
		Inputs:      noticePaths,
		Output:      mergedNotice,
	})
}

func BuildNoticeOutput(ctx ModuleContext, installPath InstallPath, installFilename string,
	noticePaths []Path) NoticeOutputs {
	// Merge all NOTICE files into one.
	// TODO(jungjw): We should just produce a well-formatted NOTICE.html file in a single pass.
	//
	// generate-notice-files.py, which processes the merged NOTICE file, has somewhat strict rules
	// about input NOTICE file paths.
	// 1. Their relative paths to the src root become their NOTICE index titles. We want to use
	// on-device paths as titles, and so output the merged NOTICE file the corresponding location.
	// 2. They must end with .txt extension. Otherwise, they're ignored.
	noticeRelPath := InstallPathToOnDevicePath(ctx, installPath.Join(ctx, installFilename+".txt"))
	mergedNotice := PathForModuleOut(ctx, filepath.Join("NOTICE_FILES/src", noticeRelPath))
	MergeNotices(ctx, mergedNotice, noticePaths)

	// Transform the merged NOTICE file into a gzipped HTML file.
	txtOuptut := PathForModuleOut(ctx, "NOTICE_txt", "NOTICE.txt")
	htmlOutput := PathForModuleOut(ctx, "NOTICE_html", "NOTICE.html")
	htmlGzOutput := PathForModuleOut(ctx, "NOTICE", "NOTICE.html.gz")
	title := "Notices for " + ctx.ModuleName()
	ctx.Build(pctx, BuildParams{
		Rule:            generateNoticeRule,
		Description:     "generate notice output",
		Input:           mergedNotice,
		Output:          htmlGzOutput,
		ImplicitOutputs: WritablePaths{txtOuptut, htmlOutput},
		Args: map[string]string{
			"txtOut":   txtOuptut.String(),
			"htmlOut":  htmlOutput.String(),
			"title":    title,
			"inputDir": PathForModuleOut(ctx, "NOTICE_FILES/src").String(),
		},
	})

	return NoticeOutputs{
		Merged:       OptionalPathForPath(mergedNotice),
		TxtOutput:    OptionalPathForPath(txtOuptut),
		HtmlOutput:   OptionalPathForPath(htmlOutput),
		HtmlGzOutput: OptionalPathForPath(htmlGzOutput),
	}
}

// BuildNotices merges the supplied NOTICE files into a single file that lists notices
// for every key in noticeMap (which would normally be installed files).
func BuildNotices(ctx ModuleContext, noticeMap map[string]Paths) NoticeOutputs {
	// TODO(jungjw): We should just produce a well-formatted NOTICE.html file in a single pass.
	//
	// generate-notice-files.py, which processes the merged NOTICE file, has somewhat strict rules
	// about input NOTICE file paths.
	// 1. Their relative paths to the src root become their NOTICE index titles. We want to use
	// on-device paths as titles, and so output the merged NOTICE file the corresponding location.
	// 2. They must end with .txt extension. Otherwise, they're ignored.

	mergeTool := PathForSource(ctx, "build/soong/scripts/mergenotice.py")
	generateNoticeTool := PathForSource(ctx, "build/soong/scripts/generate-notice-files.py")

	outputDir := PathForModuleOut(ctx, "notices")
	builder := NewRuleBuilder(pctx, ctx).
		Sbox(outputDir, PathForModuleOut(ctx, "notices.sbox.textproto"))
	for _, installPath := range SortedStringKeys(noticeMap) {
		noticePath := outputDir.Join(ctx, installPath+".txt")
		// It would be nice if sbox created directories for temporaries, but until then
		// this is simple enough.
		builder.Command().
			Text("(cd").OutputDir().Text("&&").
			Text("mkdir -p").Text(filepath.Dir(installPath)).Text(")")
		builder.Temporary(noticePath)
		builder.Command().
			Tool(mergeTool).
			Flag("--output").Output(noticePath).
			Inputs(noticeMap[installPath])
	}

	// Transform the merged NOTICE file into a gzipped HTML file.
	txtOutput := outputDir.Join(ctx, "NOTICE.txt")
	htmlOutput := outputDir.Join(ctx, "NOTICE.html")
	htmlGzOutput := outputDir.Join(ctx, "NOTICE.html.gz")
	title := "\"Notices for " + ctx.ModuleName() + "\""
	builder.Command().Tool(generateNoticeTool).
		FlagWithOutput("--text-output ", txtOutput).
		FlagWithOutput("--html-output ", htmlOutput).
		FlagWithArg("-t ", title).
		Flag("-s").OutputDir()
	builder.Command().BuiltTool("minigzip").
		FlagWithInput("-c ", htmlOutput).
		FlagWithOutput("> ", htmlGzOutput)
	builder.Build("build_notices", "generate notice output")

	return NoticeOutputs{
		TxtOutput:    OptionalPathForPath(txtOutput),
		HtmlOutput:   OptionalPathForPath(htmlOutput),
		HtmlGzOutput: OptionalPathForPath(htmlGzOutput),
	}
}
