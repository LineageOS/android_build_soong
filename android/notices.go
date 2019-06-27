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
	pctx.SourcePathVariable("generate_notice", "build/make/tools/generate-notice-files.py")

	pctx.HostBinToolVariable("minigzip", "minigzip")
}

var (
	mergeNoticesRule = pctx.AndroidStaticRule("mergeNoticesRule", blueprint.RuleParams{
		Command:     `${merge_notices} --output $out $in`,
		CommandDeps: []string{"${merge_notices}"},
		Description: "merge notice files into $out",
	})

	generateNoticeRule = pctx.AndroidStaticRule("generateNoticeRule", blueprint.RuleParams{
		Command: `rm -rf $tmpDir $$(dirname $out) && ` +
			`mkdir -p $tmpDir $$(dirname $out) && ` +
			`${generate_notice} --text-output $tmpDir/NOTICE.txt --html-output $tmpDir/NOTICE.html -t "$title" -s $inputDir && ` +
			`${minigzip} -c $tmpDir/NOTICE.html > $out`,
		CommandDeps: []string{"${generate_notice}", "${minigzip}"},
		Description: "produce notice file $out",
	}, "tmpDir", "title", "inputDir")
)

func MergeNotices(ctx ModuleContext, mergedNotice WritablePath, noticePaths []Path) {
	ctx.Build(pctx, BuildParams{
		Rule:        mergeNoticesRule,
		Description: "merge notices",
		Inputs:      noticePaths,
		Output:      mergedNotice,
	})
}

func BuildNoticeOutput(ctx ModuleContext, installPath OutputPath, installFilename string,
	noticePaths []Path) ModuleOutPath {
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
	noticeOutput := PathForModuleOut(ctx, "NOTICE", "NOTICE.html.gz")
	tmpDir := PathForModuleOut(ctx, "NOTICE_tmp")
	title := "Notices for " + ctx.ModuleName()
	ctx.Build(pctx, BuildParams{
		Rule:        generateNoticeRule,
		Description: "generate notice output",
		Input:       mergedNotice,
		Output:      noticeOutput,
		Args: map[string]string{
			"tmpDir":   tmpDir.String(),
			"title":    title,
			"inputDir": PathForModuleOut(ctx, "NOTICE_FILES/src").String(),
		},
	})

	return noticeOutput
}
