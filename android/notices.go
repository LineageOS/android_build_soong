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
	"strings"
)

// BuildNoticeTextOutputFromLicenseMetadata writes out a notice text file based on the module's
// generated license metadata file.
func BuildNoticeTextOutputFromLicenseMetadata(ctx ModuleContext, outputFile WritablePath) {
	depsFile := outputFile.ReplaceExtension(ctx, strings.TrimPrefix(outputFile.Ext()+".d", "."))
	rule := NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("textnotice").
		FlagWithOutput("-o ", outputFile).
		FlagWithDepFile("-d ", depsFile).
		Input(ctx.Module().base().licenseMetadataFile)
	rule.Build("text_notice", "container notice file")
}

// BuildNoticeHtmlOutputFromLicenseMetadata writes out a notice text file based on the module's
// generated license metadata file.
func BuildNoticeHtmlOutputFromLicenseMetadata(ctx ModuleContext, outputFile WritablePath) {
	depsFile := outputFile.ReplaceExtension(ctx, strings.TrimPrefix(outputFile.Ext()+".d", "."))
	rule := NewRuleBuilder(pctx, ctx)
	rule.Command().
		BuiltTool("htmlnotice").
		FlagWithOutput("-o ", outputFile).
		FlagWithDepFile("-d ", depsFile).
		Input(ctx.Module().base().licenseMetadataFile)
	rule.Build("html_notice", "container notice file")
}
