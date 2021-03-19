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

	"github.com/google/blueprint/proptools"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("logical_partition", logicalPartitionFactory)
}

type logicalPartition struct {
	android.ModuleBase

	properties logicalPartitionProperties

	output     android.OutputPath
	installDir android.InstallPath
}

type logicalPartitionProperties struct {
	// Set the name of the output. Defaults to <module_name>.img.
	Stem *string

	// Total size of the logical partition
	Size *string

	// List of groups. A group defines a fixed sized region. It can host one or more logical
	// partitions and their total size is limited by the size of the group they are in.
	Groups []groupProperties

	// Whether the output is a sparse image or not. Default is false.
	Sparse *bool
}

type groupProperties struct {
	// Name of the partition group
	Name *string

	// Size of the partition group
	Size *string

	// List of logical partitions in this group
	Partitions []partitionProperties
}

type partitionProperties struct {
	// Name of the partition
	Name *string

	// Filesystem that is placed on the partition
	Filesystem *string `android:"path"`
}

// logical_partition is a partition image which has one or more logical partitions in it.
func logicalPartitionFactory() android.Module {
	module := &logicalPartition{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.DeviceSupported, android.MultilibFirst)
	return module
}

func (l *logicalPartition) DepsMutator(ctx android.BottomUpMutatorContext) {
	// do nothing
}

func (l *logicalPartition) installFileName() string {
	return proptools.StringDefault(l.properties.Stem, l.BaseModuleName()+".img")
}

func (l *logicalPartition) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	builder := android.NewRuleBuilder(pctx, ctx)

	// Sparse the filesystem images and calculate their sizes
	sparseImages := make(map[string]android.OutputPath)
	sparseImageSizes := make(map[string]android.OutputPath)
	for _, group := range l.properties.Groups {
		for _, part := range group.Partitions {
			sparseImg, sizeTxt := sparseFilesystem(ctx, part, builder)
			pName := proptools.String(part.Name)
			sparseImages[pName] = sparseImg
			sparseImageSizes[pName] = sizeTxt
		}
	}

	cmd := builder.Command().BuiltTool("lpmake")

	size := proptools.String(l.properties.Size)
	if size == "" {
		ctx.PropertyErrorf("size", "must be set")
	}
	if _, err := strconv.Atoi(size); err != nil {
		ctx.PropertyErrorf("size", "must be a number")
	}
	cmd.FlagWithArg("--device-size=", size)

	// TODO(jiyong): consider supporting A/B devices. Then we need to adjust num of slots.
	cmd.FlagWithArg("--metadata-slots=", "2")
	cmd.FlagWithArg("--metadata-size=", "65536")

	if proptools.Bool(l.properties.Sparse) {
		cmd.Flag("--sparse")
	}

	groupNames := make(map[string]bool)
	partitionNames := make(map[string]bool)

	for _, group := range l.properties.Groups {
		gName := proptools.String(group.Name)
		if gName == "" {
			ctx.PropertyErrorf("groups.name", "must be set")
		}
		if _, ok := groupNames[gName]; ok {
			ctx.PropertyErrorf("group.name", "already exists")
		} else {
			groupNames[gName] = true
		}
		gSize := proptools.String(group.Size)
		if gSize == "" {
			ctx.PropertyErrorf("groups.size", "must be set")
		}
		if _, err := strconv.Atoi(gSize); err != nil {
			ctx.PropertyErrorf("groups.size", "must be a number")
		}
		cmd.FlagWithArg("--group=", gName+":"+gSize)

		for _, part := range group.Partitions {
			pName := proptools.String(part.Name)
			if pName == "" {
				ctx.PropertyErrorf("groups.partitions.name", "must be set")
			}
			if _, ok := partitionNames[pName]; ok {
				ctx.PropertyErrorf("groups.partitions.name", "already exists")
			} else {
				partitionNames[pName] = true
			}
			// Get size of the partition by reading the -size.txt file
			pSize := fmt.Sprintf("$(cat %s)", sparseImageSizes[pName])
			cmd.FlagWithArg("--partition=", fmt.Sprintf("%s:readonly:%s:%s", pName, pSize, gName))
			cmd.FlagWithInput("--image="+pName+"=", sparseImages[pName])
		}
	}

	l.output = android.PathForModuleOut(ctx, l.installFileName()).OutputPath
	cmd.FlagWithOutput("--output=", l.output)

	builder.Build("build_logical_partition", fmt.Sprintf("Creating %s", l.BaseModuleName()))

	l.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(l.installDir, l.installFileName(), l.output)
}

// Add a rule that converts the filesystem for the given partition to the given rule builder. The
// path to the sparse file and the text file having the size of the partition are returned.
func sparseFilesystem(ctx android.ModuleContext, p partitionProperties, builder *android.RuleBuilder) (sparseImg android.OutputPath, sizeTxt android.OutputPath) {
	img := android.PathForModuleSrc(ctx, proptools.String(p.Filesystem))
	name := proptools.String(p.Name)
	sparseImg = android.PathForModuleOut(ctx, name+".img").OutputPath

	builder.Temporary(sparseImg)
	builder.Command().BuiltTool("img2simg").Input(img).Output(sparseImg)

	sizeTxt = android.PathForModuleOut(ctx, name+"-size.txt").OutputPath
	builder.Temporary(sizeTxt)
	builder.Command().BuiltTool("sparse_img").Flag("--get_partition_size").Input(sparseImg).
		Text("| ").Text("tr").FlagWithArg("-d ", "'\n'").
		Text("> ").Output(sizeTxt)

	return sparseImg, sizeTxt
}

var _ android.AndroidMkEntriesProvider = (*logicalPartition)(nil)

// Implements android.AndroidMkEntriesProvider
func (l *logicalPartition) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(l.output),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", l.installDir.ToMakePath().String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", l.installFileName())
			},
		},
	}}
}

var _ Filesystem = (*logicalPartition)(nil)

func (l *logicalPartition) OutputPath() android.Path {
	return l.output
}

func (l *logicalPartition) SignedOutputPath() android.Path {
	return nil // logical partition is not signed by itself
}

var _ android.OutputFileProducer = (*logicalPartition)(nil)

// Implements android.OutputFileProducer
func (l *logicalPartition) OutputFiles(tag string) (android.Paths, error) {
	if tag == "" {
		return []android.Path{l.output}, nil
	}
	return nil, fmt.Errorf("unsupported module reference tag %q", tag)
}
