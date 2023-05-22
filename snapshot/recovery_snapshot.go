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

import "android/soong/android"

// Interface for modules which can be captured in the recovery snapshot.
type RecoverySnapshotModuleInterface interface {
	SnapshotModuleInterfaceBase
	InRecovery() bool
	ExcludeFromRecoverySnapshot() bool
}

var recoverySnapshotSingleton = SnapshotSingleton{
	"recovery",                     // name
	"SOONG_RECOVERY_SNAPSHOT_ZIP",  // makeVar
	android.OptionalPath{},         // snapshotZipFile
	RecoverySnapshotImageSingleton, // Image
	false,                          // Fake
}

func RecoverySnapshotSingleton() android.Singleton {
	return &recoverySnapshotSingleton
}

// Determine if a dir under source tree is an SoC-owned proprietary directory based
// on recovery snapshot configuration
// Examples: device/, vendor/
func isRecoveryProprietaryPath(dir string, deviceConfig android.DeviceConfig) bool {
	return RecoverySnapshotSingleton().(*SnapshotSingleton).Image.IsProprietaryPath(dir, deviceConfig)
}

func IsRecoveryProprietaryModule(ctx android.BaseModuleContext) bool {

	// Any module in a recovery proprietary path is a recovery proprietary
	// module.
	if isRecoveryProprietaryPath(ctx.ModuleDir(), ctx.DeviceConfig()) {
		return true
	}

	// However if the module is not in a recovery proprietary path, it may
	// still be a recovery proprietary module. This happens for cc modules
	// that are excluded from the recovery snapshot, and it means that the
	// vendor has assumed control of the framework-provided module.

	if c, ok := ctx.Module().(RecoverySnapshotModuleInterface); ok {
		if c.ExcludeFromRecoverySnapshot() {
			return true
		}
	}

	return false
}

var RecoverySnapshotImageName = "recovery"

type RecoverySnapshotImage struct{}

func (RecoverySnapshotImage) Init(ctx android.RegistrationContext) {
	ctx.RegisterParallelSingletonType("recovery-snapshot", RecoverySnapshotSingleton)
}

func (RecoverySnapshotImage) RegisterAdditionalModule(ctx android.RegistrationContext, name string, factory android.ModuleFactory) {
	ctx.RegisterModuleType(name, factory)
}

func (RecoverySnapshotImage) shouldGenerateSnapshot(ctx android.SingletonContext) bool {
	// RECOVERY_SNAPSHOT_VERSION must be set to 'current' in order to generate a
	// snapshot.
	return ctx.DeviceConfig().RecoverySnapshotVersion() == "current"
}

func (RecoverySnapshotImage) InImage(m SnapshotModuleInterfaceBase) func() bool {
	r, ok := m.(RecoverySnapshotModuleInterface)

	if !ok {
		// This module does not support recovery snapshot
		return func() bool { return false }
	}
	return r.InRecovery
}

func (RecoverySnapshotImage) IsProprietaryPath(dir string, deviceConfig android.DeviceConfig) bool {
	return isDirectoryExcluded(dir, deviceConfig.RecoverySnapshotDirsExcludedMap(), deviceConfig.RecoverySnapshotDirsIncludedMap())
}

func (RecoverySnapshotImage) ExcludeFromSnapshot(m SnapshotModuleInterfaceBase) bool {
	r, ok := m.(RecoverySnapshotModuleInterface)

	if !ok {
		// This module does not support recovery snapshot
		return true
	}
	return r.ExcludeFromRecoverySnapshot()
}

func (RecoverySnapshotImage) IsUsingSnapshot(cfg android.DeviceConfig) bool {
	recoverySnapshotVersion := cfg.RecoverySnapshotVersion()
	return recoverySnapshotVersion != "current" && recoverySnapshotVersion != ""
}

func (RecoverySnapshotImage) TargetSnapshotVersion(cfg android.DeviceConfig) string {
	return cfg.RecoverySnapshotVersion()
}

func (RecoverySnapshotImage) ExcludeFromDirectedSnapshot(cfg android.DeviceConfig, name string) bool {
	// If we're using full snapshot, not directed snapshot, capture every module
	if !cfg.DirectedRecoverySnapshot() {
		return false
	}
	// Else, checks if name is in RECOVERY_SNAPSHOT_MODULES.
	return !cfg.RecoverySnapshotModules()[name]
}

func (RecoverySnapshotImage) ImageName() string {
	return RecoverySnapshotImageName
}

var RecoverySnapshotImageSingleton RecoverySnapshotImage

func init() {
	RecoverySnapshotImageSingleton.Init(android.InitRegistrationContext)
}
