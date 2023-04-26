// Copyright 2021 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package snapshot

import (
	"path/filepath"
	"sort"

	"android/soong/android"
)

// This file contains singletons to capture snapshots. This singleton will generate snapshot of each target
// image, and capturing snapshot module will be delegated to each module which implements GenerateSnapshotAction
// function and register with RegisterSnapshotAction.

var pctx = android.NewPackageContext("android/soong/snapshot")

func init() {
	pctx.Import("android/soong/android")
}

type SnapshotSingleton struct {
	// Name, e.g., "vendor", "recovery", "ramdisk".
	name string

	// Make variable that points to the snapshot file, e.g.,
	// "SOONG_RECOVERY_SNAPSHOT_ZIP".
	makeVar string

	// Path to the snapshot zip file.
	snapshotZipFile android.OptionalPath

	// Implementation of the image interface specific to the image
	// associated with this snapshot (e.g., specific to the vendor image,
	// recovery image, etc.).
	Image SnapshotImage

	// Whether this singleton is for fake snapshot or not.
	// Fake snapshot is a snapshot whose prebuilt binaries and headers are empty.
	// It is much faster to generate, and can be used to inspect dependencies.
	Fake bool
}

// The output files to be included in the snapshot.
type SnapshotPaths struct {
	// All files to be included in the snapshot
	OutputFiles android.Paths

	// Notice files of the snapshot output files
	NoticeFiles android.Paths
}

// Interface of function to capture snapshot from each module
// Returns snapshot ouputs and notice files.
type GenerateSnapshotAction func(snapshot SnapshotSingleton, ctx android.SingletonContext, snapshotArchDir string) SnapshotPaths

var snapshotActionList []GenerateSnapshotAction

// Register GenerateSnapshotAction function so it can be called while generating snapshot
func RegisterSnapshotAction(x GenerateSnapshotAction) {
	snapshotActionList = append(snapshotActionList, x)
}

func (c *SnapshotSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if !c.Image.shouldGenerateSnapshot(ctx) {
		return
	}

	var snapshotOutputs android.Paths

	// Snapshot zipped artifacts will be captured under {SNAPSHOT_ARCH} directory

	snapshotDir := c.name + "-snapshot"
	if c.Fake {
		// If this is a fake snapshot singleton, place all files under fake/ subdirectory to avoid
		// collision with real snapshot files
		snapshotDir = filepath.Join("fake", snapshotDir)
	}
	snapshotArchDir := filepath.Join(snapshotDir, ctx.DeviceConfig().DeviceArch())
	noticeDir := filepath.Join(snapshotArchDir, "NOTICE_FILES")
	installedNotices := make(map[string]bool)

	for _, f := range snapshotActionList {
		snapshotPaths := f(*c, ctx, snapshotArchDir)
		snapshotOutputs = append(snapshotOutputs, snapshotPaths.OutputFiles...)
		for _, notice := range snapshotPaths.NoticeFiles {
			if _, ok := installedNotices[notice.String()]; !ok {
				installedNotices[notice.String()] = true
				snapshotOutputs = append(snapshotOutputs, CopyFileRule(
					pctx, ctx, notice, filepath.Join(noticeDir, notice.String())))
			}
		}
	}

	// All artifacts are ready. Sort them to normalize ninja and then zip.
	sort.Slice(snapshotOutputs, func(i, j int) bool {
		return snapshotOutputs[i].String() < snapshotOutputs[j].String()
	})

	zipPath := android.PathForOutput(
		ctx,
		snapshotDir,
		c.name+"-"+ctx.Config().DeviceName()+".zip")
	zipRule := android.NewRuleBuilder(pctx, ctx)

	// filenames in rspfile from FlagWithRspFileInputList might be single-quoted. Remove it with tr
	snapshotOutputList := android.PathForOutput(
		ctx,
		snapshotDir,
		c.name+"-"+ctx.Config().DeviceName()+"_list")
	rspFile := snapshotOutputList.ReplaceExtension(ctx, "rsp")
	zipRule.Command().
		Text("tr").
		FlagWithArg("-d ", "\\'").
		FlagWithRspFileInputList("< ", rspFile, snapshotOutputs).
		FlagWithOutput("> ", snapshotOutputList)

	zipRule.Temporary(snapshotOutputList)

	zipRule.Command().
		BuiltTool("soong_zip").
		FlagWithOutput("-o ", zipPath).
		FlagWithArg("-C ", android.PathForOutput(ctx, snapshotDir).String()).
		FlagWithInput("-l ", snapshotOutputList)

	zipRule.Build(zipPath.String(), c.name+" snapshot "+zipPath.String())
	zipRule.DeleteTemporaryFiles()
	c.snapshotZipFile = android.OptionalPathForPath(zipPath)
}

func (c *SnapshotSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict(
		c.makeVar,
		c.snapshotZipFile.String())
}
