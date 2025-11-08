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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("vbmeta", VbmetaFactory)
	pctx.HostBinToolVariable("avbtool", "avbtool")
}

var (
	extractPublicKeyRule = pctx.AndroidStaticRule("avb_extract_public_key",
		blueprint.RuleParams{
			Command: `${avbtool} extract_public_key --key $in --output $out`,
			CommandDeps: []string{
				"${avbtool}",
			},
		})
)

type vbmeta struct {
	android.ModuleBase

	properties VbmetaProperties

	output     android.Path
	installDir android.InstallPath
}

type VbmetaProperties struct {
	// Name of the partition stored in vbmeta desc. Defaults to the name of this module.
	Partition_name *string

	// Type of the `android_filesystem` for which the vbmeta.img is created.
	// Examples are system, vendor, product.
	Filesystem_partition_type *string

	// Set the name of the output. Defaults to <module_name>.img.
	Stem *string

	// Path to the private key that avbtool will use to sign this vbmeta image.
	Private_key *string `android:"path"`

	// Algorithm that avbtool will use to sign this vbmeta image. Default is SHA256_RSA4096.
	Algorithm *string

	// The rollback index. If unspecified, the rollback index is from PLATFORM_SECURITY_PATCH
	Rollback_index *int64

	// Rollback index location of this vbmeta image. Must be 0, 1, 2, etc. Default is 0.
	Rollback_index_location *int64

	// List of filesystem modules that this vbmeta has descriptors for. The filesystem modules
	// have to be signed (use_avb: true).
	Partitions proptools.Configurable[[]string]

	// Metadata about the chained partitions that this vbmeta delegates the verification.
	// This is an alternative to chained_partitions, using chained_partitions instead is simpler
	// in most cases. However, this property allows building this vbmeta partition without
	// its chained partitions existing in this build.
	Chained_partition_metadata []ChainedPartitionProperties

	// List of chained partitions that this vbmeta delegates the verification. They are the
	// names of other vbmeta modules.
	Chained_partitions []string

	// List of key-value pair of avb properties
	Avb_properties []avbProperty
}

type avbProperty struct {
	// Key of given avb property
	Key *string

	// Value of given avb property
	Value *string
}

type ChainedPartitionProperties struct {
	// Name of the chained partition
	Name *string

	// Rollback index location of the chained partition. Must be 1, 2, 3, etc. Default is the
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

type vbmetaPartitionInfo struct {
	// Name of the partition
	Name string

	// Partition type of the correspdonding android_filesystem.
	FilesystemPartitionType string

	// Rollback index location, non-negative int
	RollbackIndexLocation int

	// The path to the public key of the private key used to sign this partition. Derived from
	// the private key.
	PublicKey android.Path

	// The output of the vbmeta module
	Output android.Path

	// Information about the vbmeta partition that will be added to misc_info.txt
	// created by android_device
	PropFileForMiscInfo android.Path
}

var vbmetaPartitionProvider = blueprint.NewProvider[vbmetaPartitionInfo]()

// vbmeta is the partition image that has the verification information for other partitions.
func VbmetaFactory() android.Module {
	module := &vbmeta{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibCommon)
	return module
}

type vbmetaDep struct {
	blueprint.BaseDependencyTag
}

type chainedPartitionDep struct {
	blueprint.BaseDependencyTag
}

var vbmetaPartitionDep = vbmetaDep{}
var vbmetaChainedPartitionDep = chainedPartitionDep{}

func (v *vbmeta) DepsMutator(ctx android.BottomUpMutatorContext) {
	ctx.AddVariationDependencies(ctx.Config().AndroidFirstDeviceTarget.Variations(), vbmetaPartitionDep, v.properties.Partitions.GetOrDefault(ctx, nil)...)
	ctx.AddVariationDependencies(ctx.Config().AndroidFirstDeviceTarget.Variations(), vbmetaChainedPartitionDep, v.properties.Chained_partitions...)
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
	builder := android.NewRuleBuilder(pctx, ctx)
	cmd := builder.Command().BuiltTool("avbtool").Text("make_vbmeta_image")

	key := android.PathForModuleSrc(ctx, proptools.String(v.properties.Private_key))
	cmd.FlagWithInput("--key ", key)

	algorithm := proptools.StringDefault(v.properties.Algorithm, "SHA256_RSA4096")
	cmd.FlagWithArg("--algorithm ", algorithm)

	cmd.FlagWithArg("--padding_size ", "4096")

	cmd.FlagWithArg("--rollback_index ", v.rollbackIndexCommand(ctx))
	ril := proptools.IntDefault(v.properties.Rollback_index_location, 0)
	if ril < 0 {
		ctx.PropertyErrorf("rollback_index_location", "must be 0, 1, 2, ...")
		return
	}

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

	seenRils := make(map[int]bool)
	for _, cp := range ctx.GetDirectDepsWithTag(vbmetaChainedPartitionDep) {
		info, ok := android.OtherModuleProvider(ctx, cp, vbmetaPartitionProvider)
		if !ok {
			ctx.PropertyErrorf("chained_partitions", "Expected all modules in chained_partitions to provide vbmetaPartitionProvider, but %s did not", cp.Name())
			continue
		}
		if info.Name == "" {
			ctx.PropertyErrorf("chained_partitions", "name must be specified")
			continue
		}

		ril := info.RollbackIndexLocation
		if ril < 1 {
			ctx.PropertyErrorf("chained_partitions", "rollback index location must be 1, 2, 3, ...")
			continue
		} else if seenRils[ril] {
			ctx.PropertyErrorf("chained_partitions", "Multiple chained partitions with the same rollback index location %d", ril)
			continue
		}
		seenRils[ril] = true

		publicKey := info.PublicKey
		cmd.FlagWithArg("--chain_partition ", fmt.Sprintf("%s:%d:%s", info.Name, ril, publicKey.String()))
		cmd.Implicit(publicKey)
	}
	for _, cpm := range v.properties.Chained_partition_metadata {
		name := proptools.String(cpm.Name)
		if name == "" {
			ctx.PropertyErrorf("chained_partition_metadata", "name must be specified")
			continue
		}

		ril := proptools.IntDefault(cpm.Rollback_index_location, 0)
		if ril < 1 {
			ctx.PropertyErrorf("chained_partition_metadata", "rollback index location must be 1, 2, 3, ...")
			continue
		} else if seenRils[ril] {
			ctx.PropertyErrorf("chained_partition_metadata", "Multiple chained partitions with the same rollback index location %d", ril)
			continue
		}
		seenRils[ril] = true

		var publicKey android.Path
		if cpm.Public_key != nil {
			publicKey = android.PathForModuleSrc(ctx, *cpm.Public_key)
		} else if cpm.Private_key != nil {
			privateKey := android.PathForModuleSrc(ctx, *cpm.Private_key)
			extractedPublicKey := android.PathForModuleOut(ctx, "chained_metadata", name+".avbpubkey")
			ctx.Build(pctx, android.BuildParams{
				Rule:   extractPublicKeyRule,
				Input:  privateKey,
				Output: extractedPublicKey,
			})
			publicKey = extractedPublicKey
		} else {
			ctx.PropertyErrorf("public_key", "Either public_key or private_key must be specified")
			continue
		}

		cmd.FlagWithArg("--chain_partition ", fmt.Sprintf("%s:%d:%s", name, ril, publicKey.String()))
		cmd.Implicit(publicKey)
	}

	output := android.PathForModuleOut(ctx, v.installFileName())
	cmd.FlagWithOutput("--output ", output)

	// libavb expects to be able to read the maximum vbmeta size, so we must provide a partition
	// which matches this or the read will fail.
	builder.Command().Text("truncate").
		FlagWithArg("-s ", strconv.Itoa(vbmetaMaxSize)).
		Output(output)

	builder.Build("vbmeta", fmt.Sprintf("vbmeta %s", ctx.ModuleName()))

	v.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(v.installDir, v.installFileName(), output)

	extractedPublicKey := android.PathForModuleOut(ctx, v.partitionName()+".avbpubkey")
	ctx.Build(pctx, android.BuildParams{
		Rule:   extractPublicKeyRule,
		Input:  key,
		Output: extractedPublicKey,
	})

	android.SetProvider(ctx, vbmetaPartitionProvider, vbmetaPartitionInfo{
		Name:                    v.partitionName(),
		FilesystemPartitionType: proptools.String(v.properties.Filesystem_partition_type),
		RollbackIndexLocation:   ril,
		PublicKey:               extractedPublicKey,
		Output:                  output,
		PropFileForMiscInfo:     v.buildPropFileForMiscInfo(ctx),
	})

	ctx.SetOutputFiles([]android.Path{output}, "")
	v.output = output

	setCommonFilesystemInfo(ctx, v)
}

func (v *vbmeta) buildPropFileForMiscInfo(ctx android.ModuleContext) android.Path {
	var lines []string
	addStr := func(name string, value string) {
		lines = append(lines, fmt.Sprintf("%s=%s", name, value))
	}

	addStr(fmt.Sprintf("avb_%s_algorithm", v.partitionName()), proptools.StringDefault(v.properties.Algorithm, "SHA256_RSA4096"))
	if v.properties.Private_key != nil {
		addStr(fmt.Sprintf("avb_%s_key_path", v.partitionName()), android.PathForModuleSrc(ctx, proptools.String(v.properties.Private_key)).String())
	}
	if v.properties.Rollback_index_location != nil {
		addStr(fmt.Sprintf("avb_%s_rollback_index_location", v.partitionName()), strconv.FormatInt(*v.properties.Rollback_index_location, 10))
	}

	var partitionDepNames []string
	ctx.VisitDirectDepsProxyWithTag(vbmetaPartitionDep, func(child android.ModuleProxy) {
		if info, ok := android.OtherModuleProvider(ctx, child, vbmetaPartitionProvider); ok {
			partitionDepNames = append(partitionDepNames, info.Name)
		} else {
			ctx.ModuleErrorf("vbmeta dep %s does not set vbmetaPartitionProvider\n", child)
		}
	})
	if v.partitionName() != "vbmeta" { // skip for vbmeta to match Make's misc_info.txt
		addStr(fmt.Sprintf("avb_%s", v.partitionName()), strings.Join(android.SortedUniqueStrings(partitionDepNames), " "))
	}

	addStr(fmt.Sprintf("avb_%s_args", v.partitionName()), fmt.Sprintf("--padding_size 4096 --rollback_index %s", v.rollbackIndexString(ctx)))

	sort.Strings(lines)

	propFile := android.PathForModuleOut(ctx, "prop_file_for_misc_info")
	android.WriteFileRule(ctx, propFile, strings.Join(lines, "\n"))
	return propFile
}

// Returns the embedded shell command that prints the rollback index
func (v *vbmeta) rollbackIndexCommand(ctx android.ModuleContext) string {
	if v.properties.Rollback_index != nil {
		return fmt.Sprintf("%d", *v.properties.Rollback_index)
	} else {
		// Take the first line and remove the newline char
		return "$(date -u -d  " + ctx.Config().PlatformSecurityPatch() + " +%s | head -1 | tr -d '\n'" + ")"
	}
}

// Similar to rollbackIndexCommand, but guarantees that the rollback index is
// always computed during Soong analysis, even if v.properties.Rollback_index is nil
func (v *vbmeta) rollbackIndexString(ctx android.ModuleContext) string {
	if v.properties.Rollback_index != nil {
		return fmt.Sprintf("%d", *v.properties.Rollback_index)
	} else {
		t, _ := time.Parse(time.DateOnly, ctx.Config().PlatformSecurityPatch())
		return fmt.Sprintf("%d", t.Unix())
	}
}

var _ android.AndroidMkProviderInfoProducer = (*vbmeta)(nil)

func (v *vbmeta) PrepareAndroidMKProviderInfo(config android.Config) *android.AndroidMkProviderInfo {
	providerData := android.AndroidMkProviderInfo{
		PrimaryInfo: android.AndroidMkInfo{
			Class:      "ETC",
			OutputFile: android.OptionalPathForPath(v.output),
			EntryMap:   make(map[string][]string),
		},
	}
	providerData.PrimaryInfo.SetString("LOCAL_MODULE_PATH", v.installDir.String())
	providerData.PrimaryInfo.SetString("LOCAL_INSTALLED_MODULE_STEM", v.installFileName())
	return &providerData
}

var _ Filesystem = (*vbmeta)(nil)

func (v *vbmeta) OutputPath() android.Path {
	return v.output
}

func (v *vbmeta) SignedOutputPath() android.Path {
	return v.OutputPath() // vbmeta is always signed
}
