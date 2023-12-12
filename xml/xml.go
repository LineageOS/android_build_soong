// Copyright 2018 Google Inc. All rights reserved.
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

package xml

import (
	"android/soong/android"
	"android/soong/etc"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// prebuilt_etc_xml installs an xml file under <partition>/etc/<subdir>.
// It also optionally validates the xml file against the schema.

var (
	pctx = android.NewPackageContext("android/soong/xml")

	xmllintDtd = pctx.AndroidStaticRule("xmllint-dtd",
		blueprint.RuleParams{
			Command:     `$XmlLintCmd --dtdvalid $dtd $in > /dev/null && touch -a $out`,
			CommandDeps: []string{"$XmlLintCmd"},
			Restat:      true,
		},
		"dtd")

	xmllintXsd = pctx.AndroidStaticRule("xmllint-xsd",
		blueprint.RuleParams{
			Command:     `$XmlLintCmd --schema $xsd $in > /dev/null && touch -a $out`,
			CommandDeps: []string{"$XmlLintCmd"},
			Restat:      true,
		},
		"xsd")

	xmllintMinimal = pctx.AndroidStaticRule("xmllint-minimal",
		blueprint.RuleParams{
			Command:     `$XmlLintCmd $in > /dev/null && touch -a $out`,
			CommandDeps: []string{"$XmlLintCmd"},
			Restat:      true,
		})
)

func init() {
	registerXmlBuildComponents(android.InitRegistrationContext)
	pctx.HostBinToolVariable("XmlLintCmd", "xmllint")
}

func registerXmlBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("prebuilt_etc_xml", PrebuiltEtcXmlFactory)
}

type prebuiltEtcXmlProperties struct {
	// Optional DTD that will be used to validate the xml file.
	Schema *string `android:"path"`
}

type prebuiltEtcXml struct {
	etc.PrebuiltEtc

	properties prebuiltEtcXmlProperties
}

func (p *prebuiltEtcXml) timestampFilePath(ctx android.ModuleContext) android.WritablePath {
	return android.PathForModuleOut(ctx, p.PrebuiltEtc.SourceFilePath(ctx).Base()+"-timestamp")
}

func (p *prebuiltEtcXml) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.PrebuiltEtc.GenerateAndroidBuildActions(ctx)

	if p.properties.Schema != nil {
		schema := android.PathForModuleSrc(ctx, proptools.String(p.properties.Schema))

		switch schema.Ext() {
		case ".dtd":
			ctx.Build(pctx, android.BuildParams{
				Rule:        xmllintDtd,
				Description: "xmllint-dtd",
				Input:       p.PrebuiltEtc.SourceFilePath(ctx),
				Output:      p.timestampFilePath(ctx),
				Implicit:    schema,
				Args: map[string]string{
					"dtd": schema.String(),
				},
			})
			break
		case ".xsd":
			ctx.Build(pctx, android.BuildParams{
				Rule:        xmllintXsd,
				Description: "xmllint-xsd",
				Input:       p.PrebuiltEtc.SourceFilePath(ctx),
				Output:      p.timestampFilePath(ctx),
				Implicit:    schema,
				Args: map[string]string{
					"xsd": schema.String(),
				},
			})
			break
		default:
			ctx.PropertyErrorf("schema", "not supported extension: %q", schema.Ext())
		}
	} else {
		// when schema is not specified, just check if the xml is well-formed
		ctx.Build(pctx, android.BuildParams{
			Rule:        xmllintMinimal,
			Description: "xmllint-minimal",
			Input:       p.PrebuiltEtc.SourceFilePath(ctx),
			Output:      p.timestampFilePath(ctx),
		})
	}

	p.SetAdditionalDependencies([]android.Path{p.timestampFilePath(ctx)})
}

func PrebuiltEtcXmlFactory() android.Module {
	module := &prebuiltEtcXml{}
	module.AddProperties(&module.properties)
	etc.InitPrebuiltEtcModule(&module.PrebuiltEtc, "etc")
	// This module is device-only
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}
