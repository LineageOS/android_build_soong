// Copyright 2021 The Android Open Source Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package snapshot

import (
	"encoding/json"
	"path/filepath"

	"android/soong/android"
)

// The host_snapshot module creates a snapshot of host tools to be used
// in a minimal source tree.   In order to create the host_snapshot the
// user must explicitly list the modules to be included.  The
// host-fake-snapshot, defined in this file, is a utility to help determine
// which host modules are being used in the minimal source tree.
//
// The host-fake-snapshot is designed to run in a full source tree and
// will result in a snapshot that contains an empty file for each host
// tool found in the tree.  The fake snapshot is only used to determine
// the host modules that the minimal source tree depends on, hence the
// snapshot uses an empty file for each module and saves on having to
// actually build any tool to generate the snapshot.  The fake snapshot
// is compatible with an actual host_snapshot and is installed into a
// minimal source tree via the development/vendor_snapshot/update.py
// script.
//
// After generating the fake snapshot and installing into the minimal
// source tree, the dependent modules are determined via the
// development/vendor_snapshot/update.py script (see script for more
// information).  These modules are then used to define the actual
// host_snapshot to be used.  This is a similar process to the other
// snapshots (vendor, recovery,...)
//
// Example
//
// Full source tree:
//   1/ Generate fake host snapshot
//
// Minimal source tree:
//   2/ Install the fake host snapshot
//   3/ List the host modules used from the snapshot
//   4/ Remove fake host snapshot
//
// Full source tree:
//   4/ Create host_snapshot with modules identified in step 3
//
// Minimal source tree:
//   5/ Install host snapshot
//   6/ Build
//
// The host-fake-snapshot is a singleton module, that will be built
// if HOST_FAKE_SNAPSHOT_ENABLE=true.

func init() {
	registerHostSnapshotComponents(android.InitRegistrationContext)
}

// Add prebuilt information to snapshot data
type hostSnapshotFakeJsonFlags struct {
	SnapshotJsonFlags
	Prebuilt bool `json:",omitempty"`
}

func registerHostSnapshotComponents(ctx android.RegistrationContext) {
	ctx.RegisterParallelSingletonType("host-fake-snapshot", HostToolsFakeAndroidSingleton)
}

type hostFakeSingleton struct {
	snapshotDir string
	zipFile     android.OptionalPath
}

func (c *hostFakeSingleton) init() {
	c.snapshotDir = "host-fake-snapshot"

}
func HostToolsFakeAndroidSingleton() android.Singleton {
	singleton := &hostFakeSingleton{}
	singleton.init()
	return singleton
}

func (c *hostFakeSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	if !ctx.DeviceConfig().HostFakeSnapshotEnabled() {
		return
	}
	// Find all host binary modules add 'fake' versions to snapshot
	var outputs android.Paths
	seen := make(map[string]bool)
	var jsonData []hostSnapshotFakeJsonFlags
	prebuilts := make(map[string]bool)

	ctx.VisitAllModules(func(module android.Module) {
		if module.Target().Os != ctx.Config().BuildOSTarget.Os {
			return
		}
		if module.Target().Arch.ArchType != ctx.Config().BuildOSTarget.Arch.ArchType {
			return
		}

		if android.IsModulePrebuilt(module) {
			// Add non-prebuilt module name to map of prebuilts
			prebuilts[android.RemoveOptionalPrebuiltPrefix(module.Name())] = true
			return
		}
		if !module.Enabled(ctx) || module.IsHideFromMake() {
			return
		}
		apexInfo, _ := android.SingletonModuleProvider(ctx, module, android.ApexInfoProvider)
		if !apexInfo.IsForPlatform() {
			return
		}
		path := hostToolPath(module)
		if path.Valid() && path.String() != "" {
			outFile := filepath.Join(c.snapshotDir, path.String())
			if !seen[outFile] {
				seen[outFile] = true
				outputs = append(outputs, WriteStringToFileRule(ctx, "", outFile))
				jsonData = append(jsonData, hostSnapshotFakeJsonFlags{*hostJsonDesc(ctx, module), false})
			}
		}
	})
	// Update any module prebuilt information
	for idx := range jsonData {
		if _, ok := prebuilts[jsonData[idx].ModuleName]; ok {
			// Prebuilt exists for this module
			jsonData[idx].Prebuilt = true
		}
	}
	marsh, err := json.Marshal(jsonData)
	if err != nil {
		ctx.Errorf("host fake snapshot json marshal failure: %#v", err)
		return
	}
	outputs = append(outputs, WriteStringToFileRule(ctx, string(marsh), filepath.Join(c.snapshotDir, "host_snapshot.json")))
	c.zipFile = zipSnapshot(ctx, c.snapshotDir, c.snapshotDir, outputs)

}
func (c *hostFakeSingleton) MakeVars(ctx android.MakeVarsContext) {
	if !c.zipFile.Valid() {
		return
	}
	ctx.Phony(
		"host-fake-snapshot",
		c.zipFile.Path())

	ctx.DistForGoal(
		"host-fake-snapshot",
		c.zipFile.Path())

}
