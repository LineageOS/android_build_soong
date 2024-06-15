// Copyright 2015 Google Inc. All rights reserved.
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
	"reflect"
	"runtime"
	"strings"

	"android/soong/bazel"

	"github.com/google/blueprint/proptools"
)

func init() {
	registerVariableBuildComponents(InitRegistrationContext)
}

func registerVariableBuildComponents(ctx RegistrationContext) {
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("variable", VariableMutator).Parallel()
	})
}

var PrepareForTestWithVariables = FixtureRegisterWithContext(registerVariableBuildComponents)

type variableProperties struct {
	Product_variables struct {
		Platform_sdk_version struct {
			Asflags []string
			Cflags  []string
			Cmd     *string
		}

		Platform_sdk_version_or_codename struct {
			Java_resource_dirs []string
		}

		Platform_sdk_extension_version struct {
			Cmd *string
		}

		Platform_version_name struct {
			Base_dir *string
		}

		// unbundled_build is a catch-all property to annotate modules that don't build in one or
		// more unbundled branches, usually due to dependencies missing from the manifest.
		Unbundled_build struct {
			Enabled *bool `android:"arch_variant"`
		} `android:"arch_variant"`

		// similar to `Unbundled_build`, but `Always_use_prebuilt_sdks` means that it uses prebuilt
		// sdk specifically.
		Always_use_prebuilt_sdks struct {
			Enabled *bool `android:"arch_variant"`
		} `android:"arch_variant"`

		Malloc_not_svelte struct {
			Cflags              []string `android:"arch_variant"`
			Shared_libs         []string `android:"arch_variant"`
			Whole_static_libs   []string `android:"arch_variant"`
			Static_libs         []string `android:"arch_variant"`
			Exclude_static_libs []string `android:"arch_variant"`
			Srcs                []string `android:"arch_variant"`
			Header_libs         []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Malloc_not_svelte_libc32 struct {
			Cflags              []string `android:"arch_variant"`
			Shared_libs         []string `android:"arch_variant"`
			Whole_static_libs   []string `android:"arch_variant"`
			Exclude_static_libs []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Malloc_zero_contents struct {
			Cflags []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Malloc_pattern_fill_contents struct {
			Cflags []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Safestack struct {
			Cflags []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Binder32bit struct {
			Cflags []string
		}

		Override_rs_driver struct {
			Cflags []string
		}

		// treble_linker_namespaces is true when the system/vendor linker namespace separation is
		// enabled.
		Treble_linker_namespaces struct {
			Cflags []string
		}
		// enforce_vintf_manifest is true when a device is required to have a vintf manifest.
		Enforce_vintf_manifest struct {
			Cflags []string
		}

		Build_from_text_stub struct {
			Static_libs         []string
			Exclude_static_libs []string
		}

		// debuggable is true for eng and userdebug builds, and can be used to turn on additional
		// debugging features that don't significantly impact runtime behavior.  userdebug builds
		// are used for dogfooding and performance testing, and should be as similar to user builds
		// as possible.
		Debuggable struct {
			Cflags          []string
			Cppflags        []string
			Init_rc         []string
			Required        []string
			Host_required   []string
			Target_required []string
			Strip           struct {
				All                          *bool
				Keep_symbols                 *bool
				Keep_symbols_and_debug_frame *bool
			}
			Static_libs       []string
			Whole_static_libs []string
			Shared_libs       []string

			Cmdline []string

			Srcs         []string
			Exclude_srcs []string
			Cmd          *string
		}

		// eng is true for -eng builds, and can be used to turn on additional heavyweight debugging
		// features.
		Eng struct {
			Cflags   []string
			Cppflags []string
			Init_rc  []string
			Lto      struct {
				Never *bool
			}
			Sanitize struct {
				Address *bool
			}
			Optimize struct {
				Enabled *bool
			}
		}

		Uml struct {
			Cppflags []string
		}

		Arc struct {
			Cflags            []string `android:"arch_variant"`
			Exclude_srcs      []string `android:"arch_variant"`
			Header_libs       []string `android:"arch_variant"`
			Include_dirs      []string `android:"arch_variant"`
			Shared_libs       []string `android:"arch_variant"`
			Static_libs       []string `android:"arch_variant"`
			Srcs              []string `android:"arch_variant"`
			Whole_static_libs []string `android:"arch_variant"`
		} `android:"arch_variant"`

		Native_coverage struct {
			Src          *string  `android:"arch_variant"`
			Srcs         []string `android:"arch_variant"`
			Exclude_srcs []string `android:"arch_variant"`
		} `android:"arch_variant"`

		// release_aidl_use_unfrozen is "true" when a device can
		// use the unfrozen versions of AIDL interfaces.
		Release_aidl_use_unfrozen struct {
			Cflags          []string
			Cmd             *string
			Vintf_fragments []string
		}
	} `android:"arch_variant"`
}

var defaultProductVariables interface{} = variableProperties{}

type ProductVariables struct {
	// Suffix to add to generated Makefiles
	Make_suffix *string `json:",omitempty"`

	BuildId         *string `json:",omitempty"`
	BuildNumberFile *string `json:",omitempty"`

	Platform_version_name                     *string  `json:",omitempty"`
	Platform_sdk_version                      *int     `json:",omitempty"`
	Platform_sdk_codename                     *string  `json:",omitempty"`
	Platform_sdk_version_or_codename          *string  `json:",omitempty"`
	Platform_sdk_final                        *bool    `json:",omitempty"`
	Platform_sdk_extension_version            *int     `json:",omitempty"`
	Platform_base_sdk_extension_version       *int     `json:",omitempty"`
	Platform_version_active_codenames         []string `json:",omitempty"`
	Platform_version_all_preview_codenames    []string `json:",omitempty"`
	Platform_vndk_version                     *string  `json:",omitempty"`
	Platform_systemsdk_versions               []string `json:",omitempty"`
	Platform_security_patch                   *string  `json:",omitempty"`
	Platform_preview_sdk_version              *string  `json:",omitempty"`
	Platform_min_supported_target_sdk_version *string  `json:",omitempty"`
	Platform_base_os                          *string  `json:",omitempty"`
	Platform_version_last_stable              *string  `json:",omitempty"`
	Platform_version_known_codenames          *string  `json:",omitempty"`

	DeviceName                            *string  `json:",omitempty"`
	DeviceProduct                         *string  `json:",omitempty"`
	DeviceArch                            *string  `json:",omitempty"`
	DeviceArchVariant                     *string  `json:",omitempty"`
	DeviceCpuVariant                      *string  `json:",omitempty"`
	DeviceAbi                             []string `json:",omitempty"`
	DeviceVndkVersion                     *string  `json:",omitempty"`
	DeviceCurrentApiLevelForVendorModules *string  `json:",omitempty"`
	DeviceSystemSdkVersions               []string `json:",omitempty"`
	DeviceMaxPageSizeSupported            *string  `json:",omitempty"`
	DeviceNoBionicPageSizeMacro           *bool    `json:",omitempty"`

	VendorApiLevel *string `json:",omitempty"`

	RecoverySnapshotVersion *string `json:",omitempty"`

	DeviceSecondaryArch        *string  `json:",omitempty"`
	DeviceSecondaryArchVariant *string  `json:",omitempty"`
	DeviceSecondaryCpuVariant  *string  `json:",omitempty"`
	DeviceSecondaryAbi         []string `json:",omitempty"`

	NativeBridgeArch         *string  `json:",omitempty"`
	NativeBridgeArchVariant  *string  `json:",omitempty"`
	NativeBridgeCpuVariant   *string  `json:",omitempty"`
	NativeBridgeAbi          []string `json:",omitempty"`
	NativeBridgeRelativePath *string  `json:",omitempty"`

	NativeBridgeSecondaryArch         *string  `json:",omitempty"`
	NativeBridgeSecondaryArchVariant  *string  `json:",omitempty"`
	NativeBridgeSecondaryCpuVariant   *string  `json:",omitempty"`
	NativeBridgeSecondaryAbi          []string `json:",omitempty"`
	NativeBridgeSecondaryRelativePath *string  `json:",omitempty"`

	HostArch          *string `json:",omitempty"`
	HostSecondaryArch *string `json:",omitempty"`
	HostMusl          *bool   `json:",omitempty"`

	CrossHost              *string `json:",omitempty"`
	CrossHostArch          *string `json:",omitempty"`
	CrossHostSecondaryArch *string `json:",omitempty"`

	DeviceResourceOverlays     []string `json:",omitempty"`
	ProductResourceOverlays    []string `json:",omitempty"`
	EnforceRROTargets          []string `json:",omitempty"`
	EnforceRROExcludedOverlays []string `json:",omitempty"`

	AAPTCharacteristics *string  `json:",omitempty"`
	AAPTConfig          []string `json:",omitempty"`
	AAPTPreferredConfig *string  `json:",omitempty"`
	AAPTPrebuiltDPI     []string `json:",omitempty"`

	DefaultAppCertificate           *string `json:",omitempty"`
	MainlineSepolicyDevCertificates *string `json:",omitempty"`

	AppsDefaultVersionName *string `json:",omitempty"`

	Allow_missing_dependencies   *bool    `json:",omitempty"`
	Unbundled_build              *bool    `json:",omitempty"`
	Unbundled_build_apps         []string `json:",omitempty"`
	Unbundled_build_image        *bool    `json:",omitempty"`
	Always_use_prebuilt_sdks     *bool    `json:",omitempty"`
	Skip_boot_jars_check         *bool    `json:",omitempty"`
	Malloc_not_svelte            *bool    `json:",omitempty"`
	Malloc_not_svelte_libc32     *bool    `json:",omitempty"`
	Malloc_zero_contents         *bool    `json:",omitempty"`
	Malloc_pattern_fill_contents *bool    `json:",omitempty"`
	Safestack                    *bool    `json:",omitempty"`
	HostStaticBinaries           *bool    `json:",omitempty"`
	Binder32bit                  *bool    `json:",omitempty"`
	UseGoma                      *bool    `json:",omitempty"`
	UseRBE                       *bool    `json:",omitempty"`
	UseRBEJAVAC                  *bool    `json:",omitempty"`
	UseRBER8                     *bool    `json:",omitempty"`
	UseRBED8                     *bool    `json:",omitempty"`
	Debuggable                   *bool    `json:",omitempty"`
	Eng                          *bool    `json:",omitempty"`
	Treble_linker_namespaces     *bool    `json:",omitempty"`
	Enforce_vintf_manifest       *bool    `json:",omitempty"`
	Uml                          *bool    `json:",omitempty"`
	Arc                          *bool    `json:",omitempty"`
	MinimizeJavaDebugInfo        *bool    `json:",omitempty"`
	Build_from_text_stub         *bool    `json:",omitempty"`

	Check_elf_files *bool `json:",omitempty"`

	UncompressPrivAppDex             *bool    `json:",omitempty"`
	ModulesLoadedByPrivilegedModules []string `json:",omitempty"`

	BootJars     ConfiguredJarList `json:",omitempty"`
	ApexBootJars ConfiguredJarList `json:",omitempty"`

	IntegerOverflowExcludePaths []string `json:",omitempty"`

	EnableCFI       *bool    `json:",omitempty"`
	CFIExcludePaths []string `json:",omitempty"`
	CFIIncludePaths []string `json:",omitempty"`

	DisableScudo *bool `json:",omitempty"`

	MemtagHeapExcludePaths      []string `json:",omitempty"`
	MemtagHeapAsyncIncludePaths []string `json:",omitempty"`
	MemtagHeapSyncIncludePaths  []string `json:",omitempty"`

	HWASanIncludePaths []string `json:",omitempty"`
	HWASanExcludePaths []string `json:",omitempty"`

	VendorPath    *string `json:",omitempty"`
	OdmPath       *string `json:",omitempty"`
	ProductPath   *string `json:",omitempty"`
	SystemExtPath *string `json:",omitempty"`

	ClangTidy  *bool   `json:",omitempty"`
	TidyChecks *string `json:",omitempty"`

	JavaCoveragePaths        []string `json:",omitempty"`
	JavaCoverageExcludePaths []string `json:",omitempty"`

	GcovCoverage                *bool    `json:",omitempty"`
	ClangCoverage               *bool    `json:",omitempty"`
	NativeCoveragePaths         []string `json:",omitempty"`
	NativeCoverageExcludePaths  []string `json:",omitempty"`
	ClangCoverageContinuousMode *bool    `json:",omitempty"`

	// Set by NewConfig
	Native_coverage *bool `json:",omitempty"`

	SanitizeHost       []string `json:",omitempty"`
	SanitizeDevice     []string `json:",omitempty"`
	SanitizeDeviceDiag []string `json:",omitempty"`
	SanitizeDeviceArch []string `json:",omitempty"`

	ArtUseReadBarrier *bool `json:",omitempty"`

	BtConfigIncludeDir *string `json:",omitempty"`

	Override_rs_driver *string `json:",omitempty"`

	DeviceKernelHeaders []string `json:",omitempty"`

	TargetSpecificHeaderPath *string `json:",omitempty"`

	ExtraVndkVersions []string `json:",omitempty"`

	NamespacesToExport []string `json:",omitempty"`

	PgoAdditionalProfileDirs []string `json:",omitempty"`

	VndkUseCoreVariant         *bool `json:",omitempty"`
	VndkSnapshotBuildArtifacts *bool `json:",omitempty"`

	DirectedVendorSnapshot bool            `json:",omitempty"`
	VendorSnapshotModules  map[string]bool `json:",omitempty"`

	DirectedRecoverySnapshot bool            `json:",omitempty"`
	RecoverySnapshotModules  map[string]bool `json:",omitempty"`

	VendorSnapshotDirsIncluded   []string `json:",omitempty"`
	VendorSnapshotDirsExcluded   []string `json:",omitempty"`
	RecoverySnapshotDirsExcluded []string `json:",omitempty"`
	RecoverySnapshotDirsIncluded []string `json:",omitempty"`
	HostFakeSnapshotEnabled      bool     `json:",omitempty"`

	MultitreeUpdateMeta bool `json:",omitempty"`

	BoardVendorSepolicyDirs      []string `json:",omitempty"`
	BoardOdmSepolicyDirs         []string `json:",omitempty"`
	SystemExtPublicSepolicyDirs  []string `json:",omitempty"`
	SystemExtPrivateSepolicyDirs []string `json:",omitempty"`
	BoardSepolicyM4Defs          []string `json:",omitempty"`

	BoardSepolicyVers       *string `json:",omitempty"`
	PlatformSepolicyVersion *string `json:",omitempty"`

	SystemExtSepolicyPrebuiltApiDir *string `json:",omitempty"`
	ProductSepolicyPrebuiltApiDir   *string `json:",omitempty"`

	PlatformSepolicyCompatVersions []string `json:",omitempty"`

	VendorVars map[string]map[string]string `json:",omitempty"`

	Ndk_abis *bool `json:",omitempty"`

	TrimmedApex                  *bool `json:",omitempty"`
	ForceApexSymlinkOptimization *bool `json:",omitempty"`
	CompressedApex               *bool `json:",omitempty"`
	Aml_abis                     *bool `json:",omitempty"`

	DexpreoptGlobalConfig *string `json:",omitempty"`

	WithDexpreopt bool `json:",omitempty"`

	ManifestPackageNameOverrides   []string `json:",omitempty"`
	CertificateOverrides           []string `json:",omitempty"`
	PackageNameOverrides           []string `json:",omitempty"`
	ConfiguredJarLocationOverrides []string `json:",omitempty"`

	ApexGlobalMinSdkVersionOverride *string `json:",omitempty"`

	EnforceSystemCertificate          *bool    `json:",omitempty"`
	EnforceSystemCertificateAllowList []string `json:",omitempty"`

	ProductHiddenAPIStubs       []string `json:",omitempty"`
	ProductHiddenAPIStubsSystem []string `json:",omitempty"`
	ProductHiddenAPIStubsTest   []string `json:",omitempty"`

	ProductPublicSepolicyDirs  []string `json:",omitempty"`
	ProductPrivateSepolicyDirs []string `json:",omitempty"`

	TargetFSConfigGen []string `json:",omitempty"`

	EnforceProductPartitionInterface *bool `json:",omitempty"`

	EnforceInterPartitionJavaSdkLibrary *bool    `json:",omitempty"`
	InterPartitionJavaLibraryAllowList  []string `json:",omitempty"`

	BoardUsesRecoveryAsBoot *bool `json:",omitempty"`

	BoardKernelBinaries                []string `json:",omitempty"`
	BoardKernelModuleInterfaceVersions []string `json:",omitempty"`

	BoardMoveRecoveryResourcesToVendorBoot *bool `json:",omitempty"`

	PrebuiltHiddenApiDir *string `json:",omitempty"`

	ShippingApiLevel *string `json:",omitempty"`

	BuildBrokenPluginValidation         []string `json:",omitempty"`
	BuildBrokenClangAsFlags             bool     `json:",omitempty"`
	BuildBrokenClangCFlags              bool     `json:",omitempty"`
	BuildBrokenClangProperty            bool     `json:",omitempty"`
	GenruleSandboxing                   *bool    `json:",omitempty"`
	BuildBrokenEnforceSyspropOwner      bool     `json:",omitempty"`
	BuildBrokenTrebleSyspropNeverallow  bool     `json:",omitempty"`
	BuildBrokenUsesSoongPython2Modules  bool     `json:",omitempty"`
	BuildBrokenVendorPropertyNamespace  bool     `json:",omitempty"`
	BuildBrokenIncorrectPartitionImages bool     `json:",omitempty"`
	BuildBrokenInputDirModules          []string `json:",omitempty"`
	BuildBrokenDontCheckSystemSdk       bool     `json:",omitempty"`

	BuildWarningBadOptionalUsesLibsAllowlist []string `json:",omitempty"`

	BuildDebugfsRestrictionsEnabled bool `json:",omitempty"`

	RequiresInsecureExecmemForSwiftshader bool `json:",omitempty"`

	SelinuxIgnoreNeverallows bool `json:",omitempty"`

	Release_aidl_use_unfrozen *bool `json:",omitempty"`

	SepolicyFreezeTestExtraDirs         []string `json:",omitempty"`
	SepolicyFreezeTestExtraPrebuiltDirs []string `json:",omitempty"`

	GenerateAidlNdkPlatformBackend bool `json:",omitempty"`

	IgnorePrefer32OnDevice bool `json:",omitempty"`

	IncludeTags    []string `json:",omitempty"`
	SourceRootDirs []string `json:",omitempty"`

	AfdoProfiles []string `json:",omitempty"`

	ProductManufacturer string   `json:",omitempty"`
	ProductBrand        string   `json:",omitempty"`
	BuildVersionTags    []string `json:",omitempty"`

	ReleaseVersion          string   `json:",omitempty"`
	ReleaseAconfigValueSets []string `json:",omitempty"`

	ReleaseAconfigFlagDefaultPermission string `json:",omitempty"`

	ReleaseDefaultModuleBuildFromSource *bool `json:",omitempty"`

	KeepVndk *bool `json:",omitempty"`

	CheckVendorSeappViolations *bool `json:",omitempty"`

	// PartitionVarsForBazelMigrationOnlyDoNotUse are extra variables that are used to define the
	// partition images. They should not be read from soong modules.
	PartitionVarsForBazelMigrationOnlyDoNotUse PartitionVariables `json:",omitempty"`

	BuildFlags map[string]string `json:",omitempty"`

	BuildFromSourceStub *bool `json:",omitempty"`

	BuildIgnoreApexContributionContents []string `json:",omitempty"`

	HiddenapiExportableStubs *bool `json:",omitempty"`

	ExportRuntimeApis *bool `json:",omitempty"`
}

type PartitionQualifiedVariablesType struct {
	BuildingImage               bool   `json:",omitempty"`
	BoardErofsCompressor        string `json:",omitempty"`
	BoardErofsCompressHints     string `json:",omitempty"`
	BoardErofsPclusterSize      string `json:",omitempty"`
	BoardExtfsInodeCount        string `json:",omitempty"`
	BoardExtfsRsvPct            string `json:",omitempty"`
	BoardF2fsSloadCompressFlags string `json:",omitempty"`
	BoardFileSystemCompress     string `json:",omitempty"`
	BoardFileSystemType         string `json:",omitempty"`
	BoardJournalSize            string `json:",omitempty"`
	BoardPartitionReservedSize  string `json:",omitempty"`
	BoardPartitionSize          string `json:",omitempty"`
	BoardSquashfsBlockSize      string `json:",omitempty"`
	BoardSquashfsCompressor     string `json:",omitempty"`
	BoardSquashfsCompressorOpt  string `json:",omitempty"`
	BoardSquashfsDisable4kAlign string `json:",omitempty"`
	ProductBaseFsPath           string `json:",omitempty"`
	ProductHeadroom             string `json:",omitempty"`
	ProductVerityPartition      string `json:",omitempty"`

	BoardAvbAddHashtreeFooterArgs string `json:",omitempty"`
	BoardAvbKeyPath               string `json:",omitempty"`
	BoardAvbAlgorithm             string `json:",omitempty"`
	BoardAvbRollbackIndex         string `json:",omitempty"`
	BoardAvbRollbackIndexLocation string `json:",omitempty"`
}

type PartitionVariables struct {
	ProductDirectory            string `json:",omitempty"`
	PartitionQualifiedVariables map[string]PartitionQualifiedVariablesType
	TargetUserimagesUseExt2     bool `json:",omitempty"`
	TargetUserimagesUseExt3     bool `json:",omitempty"`
	TargetUserimagesUseExt4     bool `json:",omitempty"`

	TargetUserimagesSparseExtDisabled      bool `json:",omitempty"`
	TargetUserimagesSparseErofsDisabled    bool `json:",omitempty"`
	TargetUserimagesSparseSquashfsDisabled bool `json:",omitempty"`
	TargetUserimagesSparseF2fsDisabled     bool `json:",omitempty"`

	BoardErofsCompressor           string `json:",omitempty"`
	BoardErofsCompressorHints      string `json:",omitempty"`
	BoardErofsPclusterSize         string `json:",omitempty"`
	BoardErofsShareDupBlocks       string `json:",omitempty"`
	BoardErofsUseLegacyCompression string `json:",omitempty"`
	BoardExt4ShareDupBlocks        string `json:",omitempty"`
	BoardFlashLogicalBlockSize     string `json:",omitempty"`
	BoardFlashEraseBlockSize       string `json:",omitempty"`
	BoardUsesRecoveryAsBoot        bool   `json:",omitempty"`
	ProductUseDynamicPartitionSize bool   `json:",omitempty"`
	CopyImagesForTargetFilesZip    bool   `json:",omitempty"`

	BoardAvbEnable bool `json:",omitempty"`

	ProductPackages []string `json:",omitempty"`
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func stringPtr(v string) *string {
	return &v
}

func (v *ProductVariables) SetDefaultConfig() {
	*v = ProductVariables{
		BuildNumberFile: stringPtr("build_number.txt"),

		Platform_version_name:                  stringPtr("S"),
		Platform_base_sdk_extension_version:    intPtr(30),
		Platform_sdk_version:                   intPtr(30),
		Platform_sdk_codename:                  stringPtr("S"),
		Platform_sdk_final:                     boolPtr(false),
		Platform_version_active_codenames:      []string{"S"},
		Platform_version_all_preview_codenames: []string{"S"},
		Platform_vndk_version:                  stringPtr("S"),

		HostArch:                    stringPtr("x86_64"),
		HostSecondaryArch:           stringPtr("x86"),
		DeviceName:                  stringPtr("generic_arm64"),
		DeviceProduct:               stringPtr("aosp_arm-eng"),
		DeviceArch:                  stringPtr("arm64"),
		DeviceArchVariant:           stringPtr("armv8-a"),
		DeviceCpuVariant:            stringPtr("generic"),
		DeviceAbi:                   []string{"arm64-v8a"},
		DeviceSecondaryArch:         stringPtr("arm"),
		DeviceSecondaryArchVariant:  stringPtr("armv8-a"),
		DeviceSecondaryCpuVariant:   stringPtr("generic"),
		DeviceSecondaryAbi:          []string{"armeabi-v7a", "armeabi"},
		DeviceMaxPageSizeSupported:  stringPtr("4096"),
		DeviceNoBionicPageSizeMacro: boolPtr(false),

		AAPTConfig:          []string{"normal", "large", "xlarge", "hdpi", "xhdpi", "xxhdpi"},
		AAPTPreferredConfig: stringPtr("xhdpi"),
		AAPTCharacteristics: stringPtr("nosdcard"),
		AAPTPrebuiltDPI:     []string{"xhdpi", "xxhdpi"},

		Malloc_not_svelte:            boolPtr(true),
		Malloc_not_svelte_libc32:     boolPtr(true),
		Malloc_zero_contents:         boolPtr(true),
		Malloc_pattern_fill_contents: boolPtr(false),
		Safestack:                    boolPtr(false),
		TrimmedApex:                  boolPtr(false),
		Build_from_text_stub:         boolPtr(false),

		BootJars:     ConfiguredJarList{apexes: []string{}, jars: []string{}},
		ApexBootJars: ConfiguredJarList{apexes: []string{}, jars: []string{}},
	}

	if runtime.GOOS == "linux" {
		v.CrossHost = stringPtr("windows")
		v.CrossHostArch = stringPtr("x86")
		v.CrossHostSecondaryArch = stringPtr("x86_64")
	}
}

func (this *ProductVariables) GetBuildFlagBool(flag string) bool {
	val, ok := this.BuildFlags[flag]
	if !ok {
		return false
	}
	return val == "true"
}

// ProductConfigContext requires the access to the Module to get product config properties.
type ProductConfigContext interface {
	Module() Module
}

// ProductConfigOrSoongConfigProperty represents either a soong config variable + its value
// or a product config variable. You can get both a ConfigurationAxis and a SelectKey from it
// for use in bazel attributes. ProductVariableProperties() will return a map from properties ->
// this interface -> property structs for use in bp2build converters
type ProductConfigOrSoongConfigProperty interface {
	// Name of the product variable or soong config variable
	Name() string
	// AlwaysEmit returns true for soong config variables but false for product variables. This
	// is intended to indicate if we need to always emit empty lists in the select statements.
	AlwaysEmit() bool
	// ConfigurationAxis returns the bazel.ConfigurationAxis that represents this variable. The
	// configuration axis will change depending on the variable and whether it's arch/os variant
	// as well.
	ConfigurationAxis() bazel.ConfigurationAxis
	// SelectKey returns a string that represents the key of a select branch, however, it is not
	// actually the real label written out to the build file.
	// this.ConfigurationAxis().SelectKey(this.SelectKey()) will give the actual label.
	SelectKey() string
}

// ProductConfigProperty represents a product config variable, and if it is arch-variant or not.
type ProductConfigProperty struct {
	// The name of the product variable, e.g. "safestack", "malloc_not_svelte",
	// "board"
	name string

	arch string
}

func (p ProductConfigProperty) Name() string {
	return p.name
}

func (p ProductConfigProperty) AlwaysEmit() bool {
	return false
}

func (p ProductConfigProperty) ConfigurationAxis() bazel.ConfigurationAxis {
	return bazel.ProductVariableConfigurationAxis(p.arch != "", p.name+"__"+p.arch)
}

func (p ProductConfigProperty) SelectKey() string {
	if p.arch == "" {
		return strings.ToLower(p.name)
	} else {
		return strings.ToLower(p.name + "-" + p.arch)
	}
}

// SoongConfigProperty represents a soong config variable, its value if it's a string variable,
// and if it's dependent on the OS or not
type SoongConfigProperty struct {
	name      string
	namespace string
	// Can be an empty string for bool/value soong config variables
	value string
	// If there is a target: field inside a soong config property struct, the os that it selects
	// on will be represented here.
	os string
}

func (p SoongConfigProperty) Name() string {
	return p.name
}

func (p SoongConfigProperty) AlwaysEmit() bool {
	return true
}

func (p SoongConfigProperty) ConfigurationAxis() bazel.ConfigurationAxis {
	return bazel.ProductVariableConfigurationAxis(false, p.namespace+"__"+p.name+"__"+p.os)
}

// SelectKey returns the literal string that represents this variable in a BUILD
// select statement.
func (p SoongConfigProperty) SelectKey() string {
	// p.value being conditions_default can happen with or without a desired os. When not using
	// an os, we want to emit literally just //conditions:default in the select statement, but
	// when using an os, we want to emit namespace__name__conditions_default__os, so that
	// the branch is only taken if the variable is not set, and we're on the desired os.
	// ConfigurationAxis#SelectKey will map the conditions_default result of this function to
	// //conditions:default.
	if p.value == bazel.ConditionsDefaultConfigKey && p.os == "" {
		return bazel.ConditionsDefaultConfigKey
	}

	parts := []string{p.namespace, p.name}
	if p.value != "" && p.value != bazel.ConditionsDefaultSelectKey {
		parts = append(parts, p.value)
	}
	if p.os != "" {
		parts = append(parts, p.os)
	}

	// e.g. acme__feature1, android__board__soc_a, my_namespace__my_variables__my_value__my_os
	return strings.ToLower(strings.Join(parts, "__"))
}

// ProductConfigProperties is a map of maps to group property values according
// their property name and the product config variable they're set under.
//
// The outer map key is the name of the property, like "cflags".
//
// The inner map key is a ProductConfigProperty, which is a struct of product
// variable name, namespace, and the "full configuration" of the product
// variable.
//
// e.g. product variable name: board, namespace: acme, full config: vendor_chip_foo
//
// The value of the map is the interface{} representing the value of the
// property, like ["-DDEFINES"] for cflags.
type ProductConfigProperties map[string]map[ProductConfigOrSoongConfigProperty]interface{}

func (p *ProductConfigProperties) AddProductConfigProperty(
	propertyName, productVariableName, arch string, propertyValue interface{}) {

	productConfigProp := ProductConfigProperty{
		name: productVariableName, // e.g. size, feature1, feature2, FEATURE3, board
		arch: arch,                // e.g. "", x86, arm64
	}

	p.AddEitherProperty(propertyName, productConfigProp, propertyValue)
}

func (p *ProductConfigProperties) AddSoongConfigProperty(
	propertyName, namespace, variableName, value, os string, propertyValue interface{}) {

	soongConfigProp := SoongConfigProperty{
		namespace: namespace,
		name:      variableName, // e.g. size, feature1, feature2, FEATURE3, board
		value:     value,
		os:        os, // e.g. android, linux_x86
	}

	p.AddEitherProperty(propertyName, soongConfigProp, propertyValue)
}

func (p *ProductConfigProperties) AddEitherProperty(
	propertyName string, key ProductConfigOrSoongConfigProperty, propertyValue interface{}) {
	if (*p)[propertyName] == nil {
		(*p)[propertyName] = make(map[ProductConfigOrSoongConfigProperty]interface{})
	}

	if existing, ok := (*p)[propertyName][key]; ok {
		switch dst := existing.(type) {
		case []string:
			src, ok := propertyValue.([]string)
			if !ok {
				panic("Conflicting types")
			}
			dst = append(dst, src...)
			(*p)[propertyName][key] = dst
		default:
			if existing != propertyValue {
				panic(fmt.Errorf("TODO: handle merging value %#v", existing))
			}
		}
	} else {
		(*p)[propertyName][key] = propertyValue
	}
}

// maybeExtractConfigVarProp attempts to read this value as a config var struct
// wrapped by interfaces and ptrs. If it's not the right type, the second return
// value is false.
func maybeExtractConfigVarProp(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() == reflect.Interface {
		// The conditions_default value can be either
		// 1) an ptr to an interface of a struct (bool config variables and product variables)
		// 2) an interface of 1) (config variables with nested structs, like string vars)
		v = v.Elem()
	}
	if v.Kind() != reflect.Ptr {
		return v, false
	}
	v = reflect.Indirect(v)
	if v.Kind() == reflect.Interface {
		// Extract the struct from the interface
		v = v.Elem()
	}

	if !v.IsValid() {
		return v, false
	}

	if v.Kind() != reflect.Struct {
		return v, false
	}
	return v, true
}

func (productConfigProperties *ProductConfigProperties) AddProductConfigProperties(variableValues reflect.Value, arch string) {
	// Example of product_variables:
	//
	// product_variables: {
	//     malloc_not_svelte: {
	//         shared_libs: ["malloc_not_svelte_shared_lib"],
	//         whole_static_libs: ["malloc_not_svelte_whole_static_lib"],
	//         exclude_static_libs: [
	//             "malloc_not_svelte_static_lib_excludes",
	//             "malloc_not_svelte_whole_static_lib_excludes",
	//         ],
	//     },
	// },

	for i := 0; i < variableValues.NumField(); i++ {
		// e.g. Platform_sdk_version, Unbundled_build, Malloc_not_svelte, etc.
		productVariableName := variableValues.Type().Field(i).Name

		variableValue := variableValues.Field(i)
		// Check if any properties were set for the module
		if variableValue.IsZero() {
			// e.g. feature1: {}, malloc_not_svelte: {}
			continue
		}

		for j := 0; j < variableValue.NumField(); j++ {
			property := variableValue.Field(j)
			// e.g. Asflags, Cflags, Enabled, etc.
			propertyName := variableValue.Type().Field(j).Name
			if property.Kind() != reflect.Interface {
				productConfigProperties.AddProductConfigProperty(propertyName, productVariableName, arch, property.Interface())
			}
		}
	}

}

func (productConfigProperties *ProductConfigProperties) AddSoongConfigProperties(namespace string, soongConfigVariablesStruct reflect.Value) error {
	//
	// Example of soong_config_variables:
	//
	// soong_config_variables: {
	//      feature1: {
	//          conditions_default: {
	//               ...
	//          },
	//          cflags: ...
	//      },
	//      feature2: {
	//          cflags: ...
	//          conditions_default: {
	//               ...
	//          },
	//      },
	//      board: {
	//         soc_a: {
	//             ...
	//         },
	//         soc_b: {
	//             ...
	//         },
	//         soc_c: {},
	//         conditions_default: {
	//              ...
	//         },
	//      },
	// }
	for i := 0; i < soongConfigVariablesStruct.NumField(); i++ {
		// e.g. feature1, feature2, board
		variableName := soongConfigVariablesStruct.Type().Field(i).Name
		variableStruct := soongConfigVariablesStruct.Field(i)
		// Check if any properties were set for the module
		if variableStruct.IsZero() {
			// e.g. feature1: {}
			continue
		}

		// Unlike product variables, config variables require a few more
		// indirections to extract the struct from the reflect.Value.
		if v, ok := maybeExtractConfigVarProp(variableStruct); ok {
			variableStruct = v
		} else if !v.IsValid() {
			// Skip invalid variables which may not used, else leads to panic
			continue
		}

		for j := 0; j < variableStruct.NumField(); j++ {
			propertyOrStruct := variableStruct.Field(j)
			// propertyOrValueName can either be:
			//  - A property, like: Asflags, Cflags, Enabled, etc.
			//  - A soong config string variable's value, like soc_a, soc_b, soc_c in the example above
			//  - "conditions_default"
			propertyOrValueName := variableStruct.Type().Field(j).Name

			// If the property wasn't set, no need to pass it along
			if propertyOrStruct.IsZero() {
				continue
			}

			if v, ok := maybeExtractConfigVarProp(propertyOrStruct); ok {
				// The field is a struct, which is used by:
				// 1) soong_config_string_variables
				//
				// soc_a: {
				//     cflags: ...,
				// }
				//
				// soc_b: {
				//     cflags: ...,
				// }
				//
				// 2) conditions_default structs for all soong config variable types.
				//
				// conditions_default: {
				//     cflags: ...,
				//     static_libs: ...
				// }
				//
				// This means that propertyOrValueName is either conditions_default, or a soong
				// config string variable's value.
				field := v
				// Iterate over fields of this struct prop.
				for k := 0; k < field.NumField(); k++ {
					// For product variables, zero values are irrelevant; however, for soong config variables,
					// empty values are relevant because there can also be a conditions default which is not
					// applied for empty variables.
					if field.Field(k).IsZero() && namespace == "" {
						continue
					}

					propertyName := field.Type().Field(k).Name
					if propertyName == "Target" {
						productConfigProperties.AddSoongConfigPropertiesFromTargetStruct(namespace, variableName, proptools.PropertyNameForField(propertyOrValueName), field.Field(k))
					} else if propertyName == "Arch" || propertyName == "Multilib" {
						return fmt.Errorf("Arch/Multilib are not currently supported in soong config variable structs")
					} else {
						productConfigProperties.AddSoongConfigProperty(propertyName, namespace, variableName, proptools.PropertyNameForField(propertyOrValueName), "", field.Field(k).Interface())
					}
				}
			} else if propertyOrStruct.Kind() != reflect.Interface {
				// If not an interface, then this is not a conditions_default or
				// a struct prop. That is, this is a bool/value config variable.
				if propertyOrValueName == "Target" {
					productConfigProperties.AddSoongConfigPropertiesFromTargetStruct(namespace, variableName, "", propertyOrStruct)
				} else if propertyOrValueName == "Arch" || propertyOrValueName == "Multilib" {
					return fmt.Errorf("Arch/Multilib are not currently supported in soong config variable structs")
				} else {
					productConfigProperties.AddSoongConfigProperty(propertyOrValueName, namespace, variableName, "", "", propertyOrStruct.Interface())
				}
			}
		}
	}
	return nil
}

func (productConfigProperties *ProductConfigProperties) AddSoongConfigPropertiesFromTargetStruct(namespace, soongConfigVariableName string, soongConfigVariableValue string, targetStruct reflect.Value) {
	// targetStruct will be a struct with fields like "android", "host", "arm", "x86",
	// "android_arm", etc. The values of each of those fields will be a regular property struct.
	for i := 0; i < targetStruct.NumField(); i++ {
		targetFieldName := targetStruct.Type().Field(i).Name
		archOrOsSpecificStruct := targetStruct.Field(i)
		for j := 0; j < archOrOsSpecificStruct.NumField(); j++ {
			property := archOrOsSpecificStruct.Field(j)
			// e.g. Asflags, Cflags, Enabled, etc.
			propertyName := archOrOsSpecificStruct.Type().Field(j).Name

			if targetFieldName == "Android" {
				productConfigProperties.AddSoongConfigProperty(propertyName, namespace, soongConfigVariableName, soongConfigVariableValue, "android", property.Interface())
			} else if targetFieldName == "Host" {
				for _, os := range osTypeList {
					if os.Class == Host {
						productConfigProperties.AddSoongConfigProperty(propertyName, namespace, soongConfigVariableName, soongConfigVariableValue, os.Name, property.Interface())
					}
				}
			} else if !archOrOsSpecificStruct.IsZero() {
				// One problem with supporting additional fields is that if multiple branches of
				// "target" overlap, we don't want them to be in the same select statement (aka
				// configuration axis). "android" and "host" are disjoint, so it's ok that we only
				// have 2 axes right now. (soongConfigVariables and soongConfigVariablesPlusOs)
				panic("TODO: support other target types in soong config variable structs: " + targetFieldName)
			}
		}
	}
}

func VariableMutator(mctx BottomUpMutatorContext) {
	var module Module
	var ok bool
	if module, ok = mctx.Module().(Module); !ok {
		return
	}

	// TODO: depend on config variable, create variants, propagate variants up tree
	a := module.base()

	if a.variableProperties == nil {
		return
	}

	variableValues := reflect.ValueOf(a.variableProperties).Elem().FieldByName("Product_variables")

	productVariables := reflect.ValueOf(mctx.Config().productVariables)

	for i := 0; i < variableValues.NumField(); i++ {
		variableValue := variableValues.Field(i)
		name := variableValues.Type().Field(i).Name
		property := "product_variables." + proptools.PropertyNameForField(name)

		// Check that the variable was set for the product
		val := productVariables.FieldByName(name)
		if !val.IsValid() || val.Kind() != reflect.Ptr || val.IsNil() {
			continue
		}

		val = val.Elem()

		// For bools, check that the value is true
		if val.Kind() == reflect.Bool && val.Bool() == false {
			continue
		}

		// Check if any properties were set for the module
		if variableValue.IsZero() {
			continue
		}
		a.setVariableProperties(mctx, property, variableValue, val.Interface())
	}
}

func (m *ModuleBase) setVariableProperties(ctx BottomUpMutatorContext,
	prefix string, productVariablePropertyValue reflect.Value, variableValue interface{}) {

	printfIntoProperties(ctx, prefix, productVariablePropertyValue, variableValue)

	err := proptools.AppendMatchingProperties(m.GetProperties(),
		productVariablePropertyValue.Addr().Interface(), nil)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

func printfIntoPropertiesError(ctx BottomUpMutatorContext, prefix string,
	productVariablePropertyValue reflect.Value, i int, err error) {

	field := productVariablePropertyValue.Type().Field(i).Name
	property := prefix + "." + proptools.PropertyNameForField(field)
	ctx.PropertyErrorf(property, "%s", err)
}

func printfIntoProperties(ctx BottomUpMutatorContext, prefix string,
	productVariablePropertyValue reflect.Value, variableValue interface{}) {

	for i := 0; i < productVariablePropertyValue.NumField(); i++ {
		propertyValue := productVariablePropertyValue.Field(i)
		kind := propertyValue.Kind()
		if kind == reflect.Ptr {
			if propertyValue.IsNil() {
				continue
			}
			propertyValue = propertyValue.Elem()
		}
		switch propertyValue.Kind() {
		case reflect.String:
			err := printfIntoProperty(propertyValue, variableValue)
			if err != nil {
				printfIntoPropertiesError(ctx, prefix, productVariablePropertyValue, i, err)
			}
		case reflect.Slice:
			for j := 0; j < propertyValue.Len(); j++ {
				err := printfIntoProperty(propertyValue.Index(j), variableValue)
				if err != nil {
					printfIntoPropertiesError(ctx, prefix, productVariablePropertyValue, i, err)
				}
			}
		case reflect.Bool:
			// Nothing
		case reflect.Struct:
			printfIntoProperties(ctx, prefix, propertyValue, variableValue)
		default:
			panic(fmt.Errorf("unsupported field kind %q", propertyValue.Kind()))
		}
	}
}

func printfIntoProperty(propertyValue reflect.Value, variableValue interface{}) error {
	s := propertyValue.String()

	count := strings.Count(s, "%")
	if count == 0 {
		return nil
	}

	if count > 1 {
		return fmt.Errorf("product variable properties only support a single '%%'")
	}

	if strings.Contains(s, "%d") {
		switch v := variableValue.(type) {
		case int:
			// Nothing
		case bool:
			if v {
				variableValue = 1
			} else {
				variableValue = 0
			}
		default:
			return fmt.Errorf("unsupported type %T for %%d", variableValue)
		}
	} else if strings.Contains(s, "%s") {
		switch variableValue.(type) {
		case string:
			// Nothing
		default:
			return fmt.Errorf("unsupported type %T for %%s", variableValue)
		}
	} else {
		return fmt.Errorf("unsupported %% in product variable property")
	}

	propertyValue.Set(reflect.ValueOf(fmt.Sprintf(s, variableValue)))

	return nil
}

var variablePropTypeMap OncePer

// sliceToTypeArray takes a slice of property structs and returns a reflection created array containing the
// reflect.Types of each property struct.  The result can be used as a key in a map.
func sliceToTypeArray(s []interface{}) interface{} {
	// Create an array using reflection whose length is the length of the input slice
	ret := reflect.New(reflect.ArrayOf(len(s), reflect.TypeOf(reflect.TypeOf(0)))).Elem()
	for i, e := range s {
		ret.Index(i).Set(reflect.ValueOf(reflect.TypeOf(e)))
	}
	return ret.Interface()
}

func initProductVariableModule(m Module) {
	base := m.base()

	// Allow tests to override the default product variables
	if base.variableProperties == nil {
		base.variableProperties = defaultProductVariables
	}
	// Filter the product variables properties to the ones that exist on this module
	base.variableProperties = createVariableProperties(m.GetProperties(), base.variableProperties)
	if base.variableProperties != nil {
		m.AddProperties(base.variableProperties)
	}
}

// createVariableProperties takes the list of property structs for a module and returns a property struct that
// contains the product variable properties that exist in the property structs, or nil if there are none.  It
// caches the result.
func createVariableProperties(moduleTypeProps []interface{}, productVariables interface{}) interface{} {
	// Convert the moduleTypeProps to an array of reflect.Types that can be used as a key in the OncePer.
	key := sliceToTypeArray(moduleTypeProps)

	// Use the variablePropTypeMap OncePer to cache the result for each set of property struct types.
	typ, _ := variablePropTypeMap.Once(NewCustomOnceKey(key), func() interface{} {
		// Compute the filtered property struct type.
		return createVariablePropertiesType(moduleTypeProps, productVariables)
	}).(reflect.Type)

	if typ == nil {
		return nil
	}

	// Create a new pointer to a filtered property struct.
	return reflect.New(typ).Interface()
}

// createVariablePropertiesType creates a new type that contains only the product variable properties that exist in
// a list of property structs.
func createVariablePropertiesType(moduleTypeProps []interface{}, productVariables interface{}) reflect.Type {
	typ, _ := proptools.FilterPropertyStruct(reflect.TypeOf(productVariables),
		func(field reflect.StructField, prefix string) (bool, reflect.StructField) {
			// Filter function, returns true if the field should be in the resulting struct
			if prefix == "" {
				// Keep the top level Product_variables field
				return true, field
			}
			_, rest := splitPrefix(prefix)
			if rest == "" {
				// Keep the 2nd level field (i.e. Product_variables.Eng)
				return true, field
			}

			// Strip off the first 2 levels of the prefix
			_, prefix = splitPrefix(rest)

			for _, p := range moduleTypeProps {
				if fieldExistsByNameRecursive(reflect.TypeOf(p).Elem(), prefix, field.Name) {
					// Keep any fields that exist in one of the property structs
					return true, field
				}
			}

			return false, field
		})
	return typ
}

func splitPrefix(prefix string) (first, rest string) {
	index := strings.IndexByte(prefix, '.')
	if index == -1 {
		return prefix, ""
	}
	return prefix[:index], prefix[index+1:]
}

func fieldExistsByNameRecursive(t reflect.Type, prefix, name string) bool {
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("fieldExistsByNameRecursive can only be called on a reflect.Struct"))
	}

	if prefix != "" {
		split := strings.SplitN(prefix, ".", 2)
		firstPrefix := split[0]
		rest := ""
		if len(split) > 1 {
			rest = split[1]
		}
		f, exists := t.FieldByName(firstPrefix)
		if !exists {
			return false
		}
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			panic(fmt.Errorf("field %q in %q is not a struct", firstPrefix, t))
		}
		return fieldExistsByNameRecursive(ft, rest, name)
	} else {
		_, exists := t.FieldByName(name)
		return exists
	}
}
