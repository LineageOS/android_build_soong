// Copyright (C) 2021 The Android Open Source Project
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

package filesystem

import (
	"fmt"
	"strconv"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("vbmeta", vbmetaFactory)
}

type vbmeta struct {
	android.ModuleBase

	properties vbmetaProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type vbmetaProperties struct {
	// Name of the partition stored in vbmeta desc. Defaults to the name of this module.
	Partition_name *string

	// Set the name of the output. Defaults to <module_name>.img.
	Stem *string

	// Path to the private key that avbtool will use to sign this vbmeta image.
	Private_key *string `android:"path"`

	// Algorithm that avbtool will use to sign this vbmeta image. Default is SHA256_RSA4096.
	Algorithm *string

	// File whose content will provide the rollback index. If unspecified, the rollback index
	// is from PLATFORM_SECURITY_PATCH
	Rollback_index_file *string `android:"path"`

	// Rollback index location of this vbmeta image. Must be 0, 1, 2, etc. Default is 0.
	Rollback_index_location *int64

	// List of filesystem modules that this vbmeta has descriptors for. The filesystem modules
	// have to be signed (use_avb: true).
	Partitions proptools.Configurable[[]string]

	// List of chained partitions that this vbmeta deletages the verification.
	Chained_partitions []chainedPartitionProperties

	// List of key-value pair of avb properties
	Avb_properties []avbProperty
}

type avbProperty struct {
	// Key of given avb property
	Key *string

	// Value of given avb property
	Value *string
}

type chainedPartitionProperties struct {
	// Name of the chained partition
	Name *string

	// Rollback index location of the chained partition. Must be 0, 1, 2, etc. Default is the
	// index of this partition in the list + 1.
	Rollback_index_location *int64

	// Path to the public key that the chained partition is signed with. If this is specified,
	// private_key is ignored.
	Public_key *string `android:"path"`

	// Path to the private key that the chained partition is signed with. If this is specified,
	// and public_key is not specified, a public key is extracted from this private key and
	// the extracted public key is embedded in the vbmeta image.
	Private_key *string `android:"path"`
}

// vbmeta is the partition image that has the verification information for other partitions.
func vbmetaFactory() android.Module {
	module := &vbmeta{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

type vbmetaDep struct {
	blueprint.BaseDependencyTag
	kind string
}

var vbmetaPartitionDep = vbmetaDep{kind: "partition"}

func (v *vbmeta) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), vbmetaPartitionDep, v.properties.Partitions.GetOrDefault(v.ConfigurableEvaluator(ctx), nil)...)
}

func (v *vbmeta) installFileName() string {
	return proptools.StringDefault(v.properties.Stem, v.BaseModuleName()+".img")
}

func (v *vbmeta) partitionName() string {
	return proptools.StringDefault(v.properties.Partition_name, v.BaseModuleName())
}

// See external/avb/libavb/avb_slot_verify.c#VBMETA_MAX_SIZE
const vbmetaMaxSize = 64 * 1024

func (v *vbmeta) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	extractedPublicKeys := v.extractPublicKeys(ctx)

	v.output = android.PathForModuleOut(ctx, v.installFileName()).OutputPath

	builder := android.NewRuleBuilder(pctx, ctx)
	cmd := builder.Command().BuiltTool("avbtool").Text("make_vbmeta_image")

	key := android.PathForModuleSrc(ctx, proptools.String(v.properties.Private_key))
	cmd.FlagWithInput("--key ", key)

	algorithm := proptools.StringDefault(v.properties.Algorithm, "SHA256_RSA4096")
	cmd.FlagWithArg("--algorithm ", algorithm)

	cmd.FlagWithArg("--rollback_index ", v.rollbackIndexCommand(ctx))
	ril := proptools.IntDefault(v.properties.Rollback_index_location, 0)
	if ril < 0 {
		ctx.PropertyErrorf("rollback_index_location", "must be 0, 1, 2, ...")
		return
	}
	cmd.FlagWithArg("--rollback_index_location ", strconv.Itoa(ril))

	for _, avb_prop := range v.properties.Avb_properties {
		key := proptools.String(avb_prop.Key)
		if key == "" {
			ctx.PropertyErrorf("avb_properties", "key must be specified")
			continue
		}
		value := proptools.String(avb_prop.Value)
		if value == "" {
			ctx.PropertyErrorf("avb_properties", "value must be specified")
			continue
		}
		cmd.FlagWithArg("--prop ", key+":"+value)
	}

	for _, p := range ctx.GetDirectDepsWithTag(vbmetaPartitionDep) {
		f, ok := p.(Filesystem)
		if !ok {
			ctx.PropertyErrorf("partitions", "%q(type: %s) is not supported",
				p.Name(), ctx.OtherModuleType(p))
			continue
		}
		signedImage := f.SignedOutputPath()
		if signedImage == nil {
			ctx.PropertyErrorf("partitions", "%q(type: %s) is not signed. Use `use_avb: true`",
				p.Name(), ctx.OtherModuleType(p))
			continue
		}
		cmd.FlagWithInput("--include_descriptors_from_image ", signedImage)
	}

	for i, cp := range v.properties.Chained_partitions {
		name := proptools.String(cp.Name)
		if name == "" {
			ctx.PropertyErrorf("chained_partitions", "name must be specified")
			continue
		}

		ril := proptools.IntDefault(cp.Rollback_index_location, i+1)
		if ril < 0 {
			ctx.PropertyErrorf("chained_partitions", "must be 0, 1, 2, ...")
			continue
		}

		var publicKey android.Path
		if cp.Public_key != nil {
			publicKey = android.PathForModuleSrc(ctx, proptools.String(cp.Public_key))
		} else {
			publicKey = extractedPublicKeys[name]
		}
		cmd.FlagWithArg("--chain_partition ", fmt.Sprintf("%s:%d:%s", name, ril, publicKey.String()))
		cmd.Implicit(publicKey)
	}

	cmd.FlagWithOutput("--output ", v.output)

	// libavb expects to be able to read the maximum vbmeta size, so we must provide a partition
	// which matches this or the read will fail.
	builder.Command().Text("truncate").
		FlagWithArg("-s ", strconv.Itoa(vbmetaMaxSize)).
		Output(v.output)

	builder.Build("vbmeta", fmt.Sprintf("vbmeta %s", ctx.ModuleName()))

	v.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(v.installDir, v.installFileName(), v.output)

	ctx.SetOutputFiles([]android.Path{v.output}, "")
}

// Returns the embedded shell command that prints the rollback index
func (v *vbmeta) rollbackIndexCommand(ctx android.ModuleContext) string {
	var cmd string
	if v.properties.Rollback_index_file != nil {
		f := android.PathForModuleSrc(ctx, proptools.String(v.properties.Rollback_index_file))
		cmd = "cat " + f.String()
	} else {
		cmd = "date -d 'TZ=\"GMT\" " + ctx.Config().PlatformSecurityPatch() + "' +%s"
	}
	// Take the first line and remove the newline char
	return "$(" + cmd + " | head -1 | tr -d '\n'" + ")"
}

// Extract public keys from chained_partitions.private_key. The keys are indexed with the partition
// name.
func (v *vbmeta) extractPublicKeys(ctx android.ModuleContext) map[string]android.OutputPath {
	result := make(map[string]android.OutputPath)

	builder := android.NewRuleBuilder(pctx, ctx)
	for _, cp := range v.properties.Chained_partitions {
		if cp.Private_key == nil {
			continue
		}

		name := proptools.String(cp.Name)
		if name == "" {
			ctx.PropertyErrorf("chained_partitions", "name must be specified")
			continue
		}

		if _, ok := result[name]; ok {
			ctx.PropertyErrorf("chained_partitions", "name %q is duplicated", name)
			continue
		}

		privateKeyFile := android.PathForModuleSrc(ctx, proptools.String(cp.Private_key))
		publicKeyFile := android.PathForModuleOut(ctx, name+".avbpubkey").OutputPath

		builder.Command().
			BuiltTool("avbtool").
			Text("extract_public_key").
			FlagWithInput("--key ", privateKeyFile).
			FlagWithOutput("--output ", publicKeyFile)

		result[name] = publicKeyFile
	}
	builder.Build("vbmeta_extract_public_key", fmt.Sprintf("Extract public keys for %s", ctx.ModuleName()))
	return result
}

var _ android.AndroidMkEntriesProvider = (*vbmeta)(nil)

// Implements android.AndroidMkEntriesProvider
func (v *vbmeta) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(v.output),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", v.installDir.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", v.installFileName())
			},
		},
	}}
}

var _ Filesystem = (*vbmeta)(nil)

func (v *vbmeta) OutputPath() android.Path {
	return v.output
}

func (v *vbmeta) SignedOutputPath() android.Path {
	return v.OutputPath() // vbmeta is always signed
}
