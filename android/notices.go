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
	"fmt"
	"path/filepath"
	"strings"
)

func modulesOutputDirs(ctx BuilderContext, modules ...Module) []string {
	dirs := make([]string, 0, len(modules))
	for _, module := range modules {
		paths, err := outputFilesForModule(ctx, module, "")
		if err != nil {
			continue
		}
		for _, path := range paths {
			if path != nil {
				dirs = append(dirs, filepath.Dir(path.String()))
			}
		}
	}
	return SortedUniqueStrings(dirs)
}

func modulesLicenseMetadata(ctx BuilderContext, modules ...Module) Paths {
	result := make(Paths, 0, len(modules))
	for _, module := range modules {
		if mf := module.base().licenseMetadataFile; mf != nil {
			result = append(result, mf)
		}
	}
	return result
}

// buildNoticeOutputFromLicenseMetadata writes out a notice file.
func buildNoticeOutputFromLicenseMetadata(ctx BuilderContext, tool, ruleName string, outputFile WritablePath, libraryName, stripPrefix string, modules ...Module) {
	depsFile := outputFile.ReplaceExtension(ctx, strings.TrimPrefix(outputFile.Ext()+".d", "."))
	rule := NewRuleBuilder(pctx, ctx)
	if len(modules) == 0 {
		if mctx, ok := ctx.(ModuleContext); ok {
			modules = []Module{mctx.Module()}
		} else {
			panic(fmt.Errorf("%s %q needs a module to generate the notice for", ruleName, libraryName))
		}
	}
	if libraryName == "" {
		libraryName = modules[0].Name()
	}
	cmd := rule.Command().
		BuiltTool(tool).
		FlagWithOutput("-o ", outputFile).
		FlagWithDepFile("-d ", depsFile)
	if stripPrefix != "" {
		cmd = cmd.FlagWithArg("--strip_prefix ", stripPrefix)
	}
	outputs := modulesOutputDirs(ctx, modules...)
	if len(outputs) > 0 {
		cmd = cmd.FlagForEachArg("--strip_prefix ", outputs)
	}
	if libraryName != "" {
		cmd = cmd.FlagWithArg("--product ", libraryName)
	}
	cmd = cmd.Inputs(modulesLicenseMetadata(ctx, modules...))
	rule.Build(ruleName, "container notice file")
}

// BuildNoticeTextOutputFromLicenseMetadata writes out a notice text file based
// on the license metadata files for the input `modules` defaulting to the
// current context module if none given.
func BuildNoticeTextOutputFromLicenseMetadata(ctx BuilderContext, outputFile WritablePath, ruleName, libraryName, stripPrefix string, modules ...Module) {
	buildNoticeOutputFromLicenseMetadata(ctx, "textnotice", "text_notice_"+ruleName, outputFile, libraryName, stripPrefix, modules...)
}

// BuildNoticeHtmlOutputFromLicenseMetadata writes out a notice text file based
// on the license metadata files for the input `modules` defaulting to the
// current context module if none given.
func BuildNoticeHtmlOutputFromLicenseMetadata(ctx BuilderContext, outputFile WritablePath, ruleName, libraryName, stripPrefix string, modules ...Module) {
	buildNoticeOutputFromLicenseMetadata(ctx, "htmlnotice", "html_notice_"+ruleName, outputFile, libraryName, stripPrefix, modules...)
}

// BuildNoticeXmlOutputFromLicenseMetadata writes out a notice text file based
// on the license metadata files for the input `modules` defaulting to the
// current context module if none given.
func BuildNoticeXmlOutputFromLicenseMetadata(ctx BuilderContext, outputFile WritablePath, ruleName, libraryName, stripPrefix string, modules ...Module) {
	buildNoticeOutputFromLicenseMetadata(ctx, "xmlnotice", "xml_notice_"+ruleName, outputFile, libraryName, stripPrefix, modules...)
}
