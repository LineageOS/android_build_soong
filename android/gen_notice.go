// Copyright 2020 Google Inc. All rights reserved.
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

	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterGenNoticeBuildComponents(InitRegistrationContext)
}

// Register the gen_notice module type.
func RegisterGenNoticeBuildComponents(ctx RegistrationContext) {
	ctx.RegisterParallelSingletonType("gen_notice_build_rules", GenNoticeBuildRulesFactory)
	ctx.RegisterModuleType("gen_notice", GenNoticeFactory)
}

type genNoticeBuildRules struct{}

func (s *genNoticeBuildRules) GenerateBuildActions(ctx SingletonContext) {
	ctx.VisitAllModules(func(m Module) {
		gm, ok := m.(*genNoticeModule)
		if !ok {
			return
		}
		if len(gm.missing) > 0 {
			missingReferencesRule(ctx, gm)
			return
		}
		out := BuildNoticeTextOutputFromLicenseMetadata
		if proptools.Bool(gm.properties.Xml) {
			out = BuildNoticeXmlOutputFromLicenseMetadata
		} else if proptools.Bool(gm.properties.Html) {
			out = BuildNoticeHtmlOutputFromLicenseMetadata
		}
		defaultName := ""
		if len(gm.properties.For) > 0 {
			defaultName = gm.properties.For[0]
		}

		modules := make([]Module, 0)
		for _, name := range gm.properties.For {
			mods := ctx.ModuleVariantsFromName(gm, name)
			for _, mod := range mods {
				if mod == nil {
					continue
				}
				if !mod.Enabled() { // don't depend on variants without build rules
					continue
				}
				modules = append(modules, mod)
			}
		}
		if ctx.Failed() {
			return
		}
		out(ctx, gm.output, ctx.ModuleName(gm),
			proptools.StringDefault(gm.properties.ArtifactName, defaultName),
			[]string{
				filepath.Join(ctx.Config().OutDir(), "target", "product", ctx.Config().DeviceName()) + "/",
				ctx.Config().OutDir() + "/",
				ctx.Config().SoongOutDir() + "/",
			}, modules...)
	})
}

func GenNoticeBuildRulesFactory() Singleton {
	return &genNoticeBuildRules{}
}

type genNoticeProperties struct {
	// For specifies the modules for which to generate a notice file.
	For []string
	// ArtifactName specifies the internal name to use for the notice file.
	// It appears in the "used by:" list for targets whose entire name is stripped by --strip_prefix.
	ArtifactName *string
	// Stem specifies the base name of the output file.
	Stem *string `android:"arch_variant"`
	// Html indicates an html-format file is needed. The default is text. Can be Html or Xml but not both.
	Html *bool
	// Xml indicates an xml-format file is needed. The default is text. Can be Html or Xml but not both.
	Xml *bool
	// Gzipped indicates the output file must be compressed with gzip. Will append .gz to suffix if not there.
	Gzipped *bool
	// Suffix specifies the file extension to use. Defaults to .html for html, .xml for xml, or no extension for text.
	Suffix *string
	// Visibility specifies where this license can be used
	Visibility []string
}

type genNoticeModule struct {
	ModuleBase
	DefaultableModuleBase

	properties genNoticeProperties

	output  OutputPath
	missing []string
}

func (m *genNoticeModule) DepsMutator(ctx BottomUpMutatorContext) {
	if ctx.ContainsProperty("licenses") {
		ctx.PropertyErrorf("licenses", "not supported on \"gen_notice\" modules")
	}
	if proptools.Bool(m.properties.Html) && proptools.Bool(m.properties.Xml) {
		ctx.ModuleErrorf("can be html or xml but not both")
	}
	if !ctx.Config().AllowMissingDependencies() {
		var missing []string
		// Verify the modules for which to generate notices exist.
		for _, otherMod := range m.properties.For {
			if !ctx.OtherModuleExists(otherMod) {
				missing = append(missing, otherMod)
			}
		}
		if len(missing) == 1 {
			ctx.PropertyErrorf("for", "no %q module exists", missing[0])
		} else if len(missing) > 1 {
			ctx.PropertyErrorf("for", "modules \"%s\" do not exist", strings.Join(missing, "\", \""))
		}
	}
}

func (m *genNoticeModule) getStem() string {
	stem := m.base().BaseModuleName()
	if m.properties.Stem != nil {
		stem = proptools.String(m.properties.Stem)
	}
	return stem
}

func (m *genNoticeModule) getSuffix() string {
	suffix := ""
	if m.properties.Suffix == nil {
		if proptools.Bool(m.properties.Html) {
			suffix = ".html"
		} else if proptools.Bool(m.properties.Xml) {
			suffix = ".xml"
		}
	} else {
		suffix = proptools.String(m.properties.Suffix)
	}
	if proptools.Bool(m.properties.Gzipped) && !strings.HasSuffix(suffix, ".gz") {
		suffix += ".gz"
	}
	return suffix
}

func (m *genNoticeModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	if ctx.Config().AllowMissingDependencies() {
		// Verify the modules for which to generate notices exist.
		for _, otherMod := range m.properties.For {
			if !ctx.OtherModuleExists(otherMod) {
				m.missing = append(m.missing, otherMod)
			}
		}
		m.missing = append(m.missing, ctx.GetMissingDependencies()...)
		m.missing = FirstUniqueStrings(m.missing)
	}
	out := m.getStem() + m.getSuffix()
	m.output = PathForModuleOut(ctx, out).OutputPath
}

func GenNoticeFactory() Module {
	module := &genNoticeModule{}

	base := module.base()
	module.AddProperties(&base.nameProperties, &module.properties)

	// The visibility property needs to be checked and parsed by the visibility module.
	setPrimaryVisibilityProperty(module, "visibility", &module.properties.Visibility)

	InitAndroidArchModule(module, DeviceSupported, MultilibCommon)
	InitDefaultableModule(module)

	return module
}

var _ OutputFileProducer = (*genNoticeModule)(nil)

// Implements OutputFileProducer
func (m *genNoticeModule) OutputFiles(tag string) (Paths, error) {
	if tag == "" {
		return Paths{m.output}, nil
	}
	return nil, fmt.Errorf("unrecognized tag %q", tag)
}

var _ AndroidMkEntriesProvider = (*genNoticeModule)(nil)

// Implements AndroidMkEntriesProvider
func (m *genNoticeModule) AndroidMkEntries() []AndroidMkEntries {
	return []AndroidMkEntries{AndroidMkEntries{
		Class:      "ETC",
		OutputFile: OptionalPathForPath(m.output),
	}}
}

// missingReferencesRule emits an ErrorRule for missing module references.
func missingReferencesRule(ctx BuilderContext, m *genNoticeModule) {
	if len(m.missing) < 1 {
		panic(fmt.Errorf("missing references rule requested with no missing references"))
	}

	ctx.Build(pctx, BuildParams{
		Rule:        ErrorRule,
		Output:      m.output,
		Description: "notice for " + proptools.StringDefault(m.properties.ArtifactName, "container"),
		Args: map[string]string{
			"error": m.Name() + " references missing module(s): " + strings.Join(m.missing, ", "),
		},
	})
}
