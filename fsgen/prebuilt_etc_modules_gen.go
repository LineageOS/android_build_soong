// Copyright (C) 2024 The Android Open Source Project
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

package fsgen

import (
	"android/soong/android"
	"android/soong/etc"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"
)

type srcBaseFileInstallBaseFileTuple struct {
	srcBaseFile     string
	installBaseFile string
}

// prebuilt src files grouped by the install partitions.
// Each groups are a mapping of the relative install path to the name of the files
type prebuiltSrcGroupByInstallPartition struct {
	system     map[string][]srcBaseFileInstallBaseFileTuple
	system_ext map[string][]srcBaseFileInstallBaseFileTuple
	product    map[string][]srcBaseFileInstallBaseFileTuple
	vendor     map[string][]srcBaseFileInstallBaseFileTuple
	recovery   map[string][]srcBaseFileInstallBaseFileTuple
}

func newPrebuiltSrcGroupByInstallPartition() *prebuiltSrcGroupByInstallPartition {
	return &prebuiltSrcGroupByInstallPartition{
		system:     map[string][]srcBaseFileInstallBaseFileTuple{},
		system_ext: map[string][]srcBaseFileInstallBaseFileTuple{},
		product:    map[string][]srcBaseFileInstallBaseFileTuple{},
		vendor:     map[string][]srcBaseFileInstallBaseFileTuple{},
		recovery:   map[string][]srcBaseFileInstallBaseFileTuple{},
	}
}

func isSubdirectory(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

func appendIfCorrectInstallPartition(partitionToInstallPathList []partitionToInstallPath, destPath, srcPath string, srcGroup *prebuiltSrcGroupByInstallPartition) {
	for _, part := range partitionToInstallPathList {
		partition := part.name
		installPath := part.installPath

		if isSubdirectory(installPath, destPath) {
			relativeInstallPath, _ := filepath.Rel(installPath, destPath)
			relativeInstallDir := filepath.Dir(relativeInstallPath)
			var srcMap map[string][]srcBaseFileInstallBaseFileTuple
			switch partition {
			case "system":
				srcMap = srcGroup.system
			case "system_ext":
				srcMap = srcGroup.system_ext
			case "product":
				srcMap = srcGroup.product
			case "vendor":
				srcMap = srcGroup.vendor
			case "recovery":
				srcMap = srcGroup.recovery
			}
			if srcMap != nil {
				srcMap[relativeInstallDir] = append(srcMap[relativeInstallDir], srcBaseFileInstallBaseFileTuple{
					srcBaseFile:     filepath.Base(srcPath),
					installBaseFile: filepath.Base(destPath),
				})
			}
			return
		}
	}
}

// Create a map of source files to the list of destination files from PRODUCT_COPY_FILES entries.
// Note that the value of the map is a list of string, given that a single source file can be
// copied to multiple files.
// This function also checks the existence of the source files, and validates that there is no
// multiple source files copying to the same dest file.
func uniqueExistingProductCopyFileMap(ctx android.LoadHookContext) map[string][]string {
	seen := make(map[string]bool)
	filtered := make(map[string][]string)

	for _, copyFilePair := range ctx.Config().ProductVariables().PartitionVarsForSoongMigrationOnlyDoNotUse.ProductCopyFiles {
		srcDestList := strings.Split(copyFilePair, ":")
		if len(srcDestList) < 2 {
			ctx.ModuleErrorf("PRODUCT_COPY_FILES must follow the format \"src:dest\", got: %s", copyFilePair)
		}
		src, dest := srcDestList[0], srcDestList[1]

		// Some downstream branches use absolute path as entries in PRODUCT_COPY_FILES.
		// Convert them to relative path from top and check if they do not escape the tree root.
		relSrc := android.ToRelativeSourcePath(ctx, src)

		if _, ok := seen[dest]; !ok {
			if optionalPath := android.ExistentPathForSource(ctx, relSrc); optionalPath.Valid() {
				seen[dest] = true
				filtered[relSrc] = append(filtered[relSrc], dest)
			}
		}
	}

	return filtered
}

type partitionToInstallPath struct {
	name        string
	installPath string
}

func processProductCopyFiles(ctx android.LoadHookContext) map[string]*prebuiltSrcGroupByInstallPartition {
	// Filter out duplicate dest entries and non existing src entries
	productCopyFileMap := uniqueExistingProductCopyFileMap(ctx)

	// System is intentionally added at the last to consider the scenarios where
	// non-system partitions are installed as part of the system partition
	partitionToInstallPathList := []partitionToInstallPath{
		{name: "recovery", installPath: "recovery/root"},
		{name: "vendor", installPath: ctx.DeviceConfig().VendorPath()},
		{name: "product", installPath: ctx.DeviceConfig().ProductPath()},
		{name: "system_ext", installPath: ctx.DeviceConfig().SystemExtPath()},
		{name: "system", installPath: "system"},
	}

	groupedSources := map[string]*prebuiltSrcGroupByInstallPartition{}
	for _, src := range android.SortedKeys(productCopyFileMap) {
		destFiles := productCopyFileMap[src]
		srcFileDir := filepath.Dir(src)
		if _, ok := groupedSources[srcFileDir]; !ok {
			groupedSources[srcFileDir] = newPrebuiltSrcGroupByInstallPartition()
		}
		for _, dest := range destFiles {
			appendIfCorrectInstallPartition(partitionToInstallPathList, dest, filepath.Base(src), groupedSources[srcFileDir])
		}
	}

	return groupedSources
}

type prebuiltModuleProperties struct {
	Name *string

	Soc_specific        *bool
	Product_specific    *bool
	System_ext_specific *bool
	Recovery            *bool
	Ramdisk             *bool

	Srcs []string
	Dsts []string

	No_full_install *bool

	NamespaceExportedToMake bool

	Visibility []string
}

// Split relative_install_path to a separate struct, because it is not supported for every
// modules listed in [etcInstallPathToFactoryMap]
type prebuiltSubdirProperties struct {
	// If the base file name of the src and dst all match, dsts property does not need to be
	// set, and only relative_install_path can be set.
	Relative_install_path *string
}

// Split install_in_root to a separate struct as it is part of rootProperties instead of
// properties
type prebuiltInstallInRootProperties struct {
	Install_in_root *bool
}

var (
	etcInstallPathToFactoryList = map[string]android.ModuleFactory{
		"":                    etc.PrebuiltRootFactory,
		"avb":                 etc.PrebuiltAvbFactory,
		"bin":                 etc.PrebuiltBinaryFactory,
		"bt_firmware":         etc.PrebuiltBtFirmwareFactory,
		"cacerts":             etc.PrebuiltEtcCaCertsFactory,
		"dsp":                 etc.PrebuiltDSPFactory,
		"etc":                 etc.PrebuiltEtcFactory,
		"etc/dsp":             etc.PrebuiltDSPFactory,
		"etc/firmware":        etc.PrebuiltFirmwareFactory,
		"firmware":            etc.PrebuiltFirmwareFactory,
		"first_stage_ramdisk": etc.PrebuiltFirstStageRamdiskFactory,
		"fonts":               etc.PrebuiltFontFactory,
		"framework":           etc.PrebuiltFrameworkFactory,
		"lib":                 etc.PrebuiltRenderScriptBitcodeFactory,
		"lib64":               etc.PrebuiltRenderScriptBitcodeFactory,
		"lib/rfsa":            etc.PrebuiltRFSAFactory,
		"media":               etc.PrebuiltMediaFactory,
		"odm":                 etc.PrebuiltOdmFactory,
		"optee":               etc.PrebuiltOpteeFactory,
		"overlay":             etc.PrebuiltOverlayFactory,
		"priv-app":            etc.PrebuiltPrivAppFactory,
		"radio":               etc.PrebuiltRadioFactory,
		"sbin":                etc.PrebuiltSbinFactory,
		"system":              etc.PrebuiltSystemFactory,
		"res":                 etc.PrebuiltResFactory,
		"rfs":                 etc.PrebuiltRfsFactory,
		"tts":                 etc.PrebuiltVoicepackFactory,
		"tvconfig":            etc.PrebuiltTvConfigFactory,
		"tvservice":           etc.PrebuiltTvServiceFactory,
		"usr/share":           etc.PrebuiltUserShareFactory,
		"usr/hyphen-data":     etc.PrebuiltUserHyphenDataFactory,
		"usr/keylayout":       etc.PrebuiltUserKeyLayoutFactory,
		"usr/keychars":        etc.PrebuiltUserKeyCharsFactory,
		"usr/srec":            etc.PrebuiltUserSrecFactory,
		"usr/idc":             etc.PrebuiltUserIdcFactory,
		"vendor":              etc.PrebuiltVendorFactory,
		"vendor_dlkm":         etc.PrebuiltVendorDlkmFactory,
		"wallpaper":           etc.PrebuiltWallpaperFactory,
		"wlc_upt":             etc.PrebuiltWlcUptFactory,
	}
)

func generatedPrebuiltEtcModuleName(partition, srcDir, destDir string, count int) string {
	// generated module name follows the pattern:
	// <install partition>-<src file path>-<relative install path from partition root>-<number>
	// Note that all path separators are replaced with "_" in the name
	moduleName := partition
	if !android.InList(srcDir, []string{"", "."}) {
		moduleName += fmt.Sprintf("-%s", strings.ReplaceAll(srcDir, string(filepath.Separator), "_"))
	}
	if !android.InList(destDir, []string{"", "."}) {
		moduleName += fmt.Sprintf("-%s", strings.ReplaceAll(destDir, string(filepath.Separator), "_"))
	}
	moduleName += fmt.Sprintf("-%d", count)

	return moduleName
}

func groupDestFilesBySrc(destFiles []srcBaseFileInstallBaseFileTuple) (ret map[string][]srcBaseFileInstallBaseFileTuple, maxLen int) {
	ret = map[string][]srcBaseFileInstallBaseFileTuple{}
	maxLen = 0
	for _, tuple := range destFiles {
		if _, ok := ret[tuple.srcBaseFile]; !ok {
			ret[tuple.srcBaseFile] = []srcBaseFileInstallBaseFileTuple{}
		}
		ret[tuple.srcBaseFile] = append(ret[tuple.srcBaseFile], tuple)
		maxLen = max(maxLen, len(ret[tuple.srcBaseFile]))
	}
	return ret, maxLen
}

func prebuiltEtcModuleProps(ctx android.LoadHookContext, moduleName, partition, destDir string) prebuiltModuleProperties {
	moduleProps := prebuiltModuleProperties{}
	moduleProps.Name = proptools.StringPtr(moduleName)

	// Set partition specific properties
	switch partition {
	case "system_ext":
		moduleProps.System_ext_specific = proptools.BoolPtr(true)
	case "product":
		moduleProps.Product_specific = proptools.BoolPtr(true)
	case "vendor":
		moduleProps.Soc_specific = proptools.BoolPtr(true)
	case "recovery":
		// To match the logic in modulePartition() in android/paths.go
		if ctx.DeviceConfig().BoardUsesRecoveryAsBoot() && strings.HasPrefix(destDir, "first_stage_ramdisk") {
			moduleProps.Ramdisk = proptools.BoolPtr(true)
		} else {
			moduleProps.Recovery = proptools.BoolPtr(true)
		}
	}

	moduleProps.No_full_install = proptools.BoolPtr(true)
	moduleProps.NamespaceExportedToMake = true
	moduleProps.Visibility = []string{"//visibility:public"}

	return moduleProps
}

func createPrebuiltEtcModulesInDirectory(ctx android.LoadHookContext, partition, srcDir, destDir string, destFiles []srcBaseFileInstallBaseFileTuple) (moduleNames []string) {
	groupedDestFiles, maxLen := groupDestFilesBySrc(destFiles)

	// Find out the most appropriate module type to generate
	var etcInstallPathKey string
	for _, etcInstallPath := range android.SortedKeys(etcInstallPathToFactoryList) {
		// Do not break when found but iterate until the end to find a module with more
		// specific install path
		if strings.HasPrefix(destDir, etcInstallPath) {
			etcInstallPathKey = etcInstallPath
		}
	}
	relDestDirFromInstallDirBase, _ := filepath.Rel(etcInstallPathKey, destDir)

	for fileIndex := range maxLen {
		srcTuple := []srcBaseFileInstallBaseFileTuple{}
		for _, srcFile := range android.SortedKeys(groupedDestFiles) {
			groupedDestFile := groupedDestFiles[srcFile]
			if len(groupedDestFile) > fileIndex {
				srcTuple = append(srcTuple, groupedDestFile[fileIndex])
			}
		}

		moduleName := generatedPrebuiltEtcModuleName(partition, srcDir, destDir, fileIndex)
		moduleProps := prebuiltEtcModuleProps(ctx, moduleName, partition, destDir)
		modulePropsPtr := &moduleProps
		propsList := []interface{}{modulePropsPtr}

		allCopyFileNamesUnchanged := true
		var srcBaseFiles, installBaseFiles []string
		for _, tuple := range srcTuple {
			if tuple.srcBaseFile != tuple.installBaseFile {
				allCopyFileNamesUnchanged = false
			}
			srcBaseFiles = append(srcBaseFiles, tuple.srcBaseFile)
			installBaseFiles = append(installBaseFiles, tuple.installBaseFile)
		}

		// Recovery partition-installed modules are installed to `recovery/root/system` by
		// default (See modulePartition() in android/paths.go). If the destination file
		// directory is not `recovery/root/system/...`, it should set install_in_root to true
		// to prevent being installed in `recovery/root/system`.
		if partition == "recovery" && !strings.HasPrefix(destDir, "system") {
			propsList = append(propsList, &prebuiltInstallInRootProperties{
				Install_in_root: proptools.BoolPtr(true),
			})
		}

		// Set appropriate srcs, dsts, and releative_install_path based on
		// the source and install file names
		if allCopyFileNamesUnchanged {
			modulePropsPtr.Srcs = srcBaseFiles

			// Specify relative_install_path if it is not installed in the root directory of the
			// partition
			if !android.InList(relDestDirFromInstallDirBase, []string{"", "."}) {
				propsList = append(propsList, &prebuiltSubdirProperties{
					Relative_install_path: proptools.StringPtr(relDestDirFromInstallDirBase),
				})
			}
		} else {
			modulePropsPtr.Srcs = srcBaseFiles
			dsts := []string{}
			for _, installBaseFile := range installBaseFiles {
				dsts = append(dsts, filepath.Join(relDestDirFromInstallDirBase, installBaseFile))
			}
			modulePropsPtr.Dsts = dsts
		}

		ctx.CreateModuleInDirectory(etcInstallPathToFactoryList[etcInstallPathKey], srcDir, propsList...)
		moduleNames = append(moduleNames, moduleName)
	}

	return moduleNames
}

func createPrebuiltEtcModulesForPartition(ctx android.LoadHookContext, partition, srcDir string, destDirFilesMap map[string][]srcBaseFileInstallBaseFileTuple) (ret []string) {
	for _, destDir := range android.SortedKeys(destDirFilesMap) {
		ret = append(ret, createPrebuiltEtcModulesInDirectory(ctx, partition, srcDir, destDir, destDirFilesMap[destDir])...)
	}
	return ret
}

// Creates prebuilt_* modules based on the install paths and returns the list of generated
// module names
func createPrebuiltEtcModules(ctx android.LoadHookContext) (ret []string) {
	groupedSources := processProductCopyFiles(ctx)
	for _, srcDir := range android.SortedKeys(groupedSources) {
		groupedSource := groupedSources[srcDir]
		ret = append(ret, createPrebuiltEtcModulesForPartition(ctx, "system", srcDir, groupedSource.system)...)
		ret = append(ret, createPrebuiltEtcModulesForPartition(ctx, "system_ext", srcDir, groupedSource.system_ext)...)
		ret = append(ret, createPrebuiltEtcModulesForPartition(ctx, "product", srcDir, groupedSource.product)...)
		ret = append(ret, createPrebuiltEtcModulesForPartition(ctx, "vendor", srcDir, groupedSource.vendor)...)
		ret = append(ret, createPrebuiltEtcModulesForPartition(ctx, "recovery", srcDir, groupedSource.recovery)...)
	}

	return ret
}

func createAvbpubkeyModule(ctx android.LoadHookContext) bool {
	avbKeyPath := ctx.Config().ProductVariables().PartitionVarsForSoongMigrationOnlyDoNotUse.BoardAvbKeyPath
	if avbKeyPath == "" {
		return false
	}
	ctx.CreateModuleInDirectory(
		etc.AvbpubkeyModuleFactory,
		".",
		&struct {
			Name             *string
			Product_specific *bool
			Private_key      *string
			No_full_install  *bool
			Visibility       []string
		}{
			Name:             proptools.StringPtr("system_other_avbpubkey"),
			Product_specific: proptools.BoolPtr(true),
			Private_key:      proptools.StringPtr(avbKeyPath),
			No_full_install:  proptools.BoolPtr(true),
			Visibility:       []string{"//visibility:public"},
		},
	)
	return true
}
