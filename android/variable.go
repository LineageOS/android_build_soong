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

	"android/soong/android/soongconfig"
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

		Pdk struct {
			Enabled *bool `android:"arch_variant"`
		} `android:"arch_variant"`

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

		Flatten_apex struct {
			Enabled *bool
		}

		Native_coverage struct {
			Src          *string  `android:"arch_variant"`
			Srcs         []string `android:"arch_variant"`
			Exclude_srcs []string `android:"arch_variant"`
		} `android:"arch_variant"`
	} `android:"arch_variant"`
}

var defaultProductVariables interface{} = variableProperties{}

type productVariables struct {
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

	BoardVendorSepolicyDirs           []string `json:",omitempty"`
	BoardOdmSepolicyDirs              []string `json:",omitempty"`
	BoardReqdMaskPolicy               []string `json:",omitempty"`
	BoardPlatVendorPolicy             []string `json:",omitempty"`
	BoardSystemExtPublicPrebuiltDirs  []string `json:",omitempty"`
	BoardSystemExtPrivatePrebuiltDirs []string `json:",omitempty"`
	BoardProductPublicPrebuiltDirs    []string `json:",omitempty"`
	BoardProductPrivatePrebuiltDirs   []string `json:",omitempty"`
	SystemExtPublicSepolicyDirs       []string `json:",omitempty"`
	SystemExtPrivateSepolicyDirs      []string `json:",omitempty"`
	BoardSepolicyM4Defs               []string `json:",omitempty"`

	BoardSepolicyVers       *string `json:",omitempty"`
	PlatformSepolicyVersion *string `json:",omitempty"`
	TotSepolicyVersion      *string `json:",omitempty"`

	SystemExtSepolicyPrebuiltApiDir *string `json:",omitempty"`
	ProductSepolicyPrebuiltApiDir   *string `json:",omitempty"`

	PlatformSepolicyCompatVersions []string `json:",omitempty"`

	VendorVars map[string]map[string]string `json:",omitempty"`

	Ndk_abis *bool `json:",omitempty"`

	TrimmedApex                  *bool `json:",omitempty"`
	Flatten_apex                 *bool `json:",omitempty"`
	ForceApexSymlinkOptimization *bool `json:",omitempty"`
	CompressedApex               *bool `json:",omitempty"`
	Aml_abis                     *bool `json:",omitempty"`

	DexpreoptGlobalConfig *string `json:",omitempty"`

	WithDexpreopt bool `json:",omitempty"`

	ManifestPackageNameOverrides []string `json:",omitempty"`
	CertificateOverrides         []string `json:",omitempty"`
	PackageNameOverrides         []string `json:",omitempty"`

	ApexGlobalMinSdkVersionOverride *string `json:",omitempty"`

	EnforceSystemCertificate          *bool    `json:",omitempty"`
	EnforceSystemCertificateAllowList []string `json:",omitempty"`

	ProductHiddenAPIStubs       []string `json:",omitempty"`
	ProductHiddenAPIStubsSystem []string `json:",omitempty"`
	ProductHiddenAPIStubsTest   []string `json:",omitempty"`

	ProductPublicSepolicyDirs  []string `json:",omitempty"`
	ProductPrivateSepolicyDirs []string `json:",omitempty"`

	ProductVndkVersion *string `json:",omitempty"`

	TargetFSConfigGen []string `json:",omitempty"`

	MissingUsesLibraries []string `json:",omitempty"`

	EnforceProductPartitionInterface *bool `json:",omitempty"`

	EnforceInterPartitionJavaSdkLibrary *bool    `json:",omitempty"`
	InterPartitionJavaLibraryAllowList  []string `json:",omitempty"`

	InstallExtraFlattenedApexes *bool `json:",omitempty"`

	BoardUsesRecoveryAsBoot *bool `json:",omitempty"`

	BoardKernelBinaries                []string `json:",omitempty"`
	BoardKernelModuleInterfaceVersions []string `json:",omitempty"`

	BoardMoveRecoveryResourcesToVendorBoot *bool `json:",omitempty"`

	PrebuiltHiddenApiDir *string `json:",omitempty"`

	ShippingApiLevel *string `json:",omitempty"`

	BuildBrokenClangAsFlags            bool     `json:",omitempty"`
	BuildBrokenClangCFlags             bool     `json:",omitempty"`
	BuildBrokenClangProperty           bool     `json:",omitempty"`
	BuildBrokenDepfile                 *bool    `json:",omitempty"`
	BuildBrokenEnforceSyspropOwner     bool     `json:",omitempty"`
	BuildBrokenTrebleSyspropNeverallow bool     `json:",omitempty"`
	BuildBrokenUsesSoongPython2Modules bool     `json:",omitempty"`
	BuildBrokenVendorPropertyNamespace bool     `json:",omitempty"`
	BuildBrokenInputDirModules         []string `json:",omitempty"`

	BuildDebugfsRestrictionsEnabled bool `json:",omitempty"`

	RequiresInsecureExecmemForSwiftshader bool `json:",omitempty"`

	SelinuxIgnoreNeverallows bool `json:",omitempty"`

	SepolicySplit bool `json:",omitempty"`

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

func (v *productVariables) SetDefaultConfig() {
	*v = productVariables{
		BuildNumberFile: stringPtr("build_number.txt"),

		Platform_version_name:                  stringPtr("S"),
		Platform_base_sdk_extension_version:    intPtr(30),
		Platform_sdk_version:                   intPtr(30),
		Platform_sdk_codename:                  stringPtr("S"),
		Platform_sdk_final:                     boolPtr(false),
		Platform_version_active_codenames:      []string{"S"},
		Platform_version_all_preview_codenames: []string{"S"},
		Platform_vndk_version:                  stringPtr("S"),

		HostArch:                   stringPtr("x86_64"),
		HostSecondaryArch:          stringPtr("x86"),
		DeviceName:                 stringPtr("generic_arm64"),
		DeviceProduct:              stringPtr("aosp_arm-eng"),
		DeviceArch:                 stringPtr("arm64"),
		DeviceArchVariant:          stringPtr("armv8-a"),
		DeviceCpuVariant:           stringPtr("generic"),
		DeviceAbi:                  []string{"arm64-v8a"},
		DeviceSecondaryArch:        stringPtr("arm"),
		DeviceSecondaryArchVariant: stringPtr("armv8-a"),
		DeviceSecondaryCpuVariant:  stringPtr("generic"),
		DeviceSecondaryAbi:         []string{"armeabi-v7a", "armeabi"},
		DeviceMaxPageSizeSupported: stringPtr("4096"),

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

		BootJars:     ConfiguredJarList{apexes: []string{}, jars: []string{}},
		ApexBootJars: ConfiguredJarList{apexes: []string{}, jars: []string{}},
	}

	if runtime.GOOS == "linux" {
		v.CrossHost = stringPtr("windows")
		v.CrossHostArch = stringPtr("x86")
		v.CrossHostSecondaryArch = stringPtr("x86_64")
	}
}

// ProductConfigContext requires the access to the Module to get product config properties.
type ProductConfigContext interface {
	Module() Module
}

// ProductConfigProperty contains the information for a single property (may be a struct) paired
// with the appropriate ProductConfigVariable.
type ProductConfigProperty struct {
	// The name of the product variable, e.g. "safestack", "malloc_not_svelte",
	// "board"
	Name string

	// Namespace of the variable, if this is a soong_config_module_type variable
	// e.g. "acme", "ANDROID", "vendor_name"
	Namespace string

	// Unique configuration to identify this product config property (i.e. a
	// primary key), as just using the product variable name is not sufficient.
	//
	// For product variables, this is the product variable name + optional
	// archvariant information. e.g.
	//
	// product_variables: {
	//     foo: {
	//         cflags: ["-Dfoo"],
	//     },
	// },
	//
	// FullConfig would be "foo".
	//
	// target: {
	//     android: {
	//         product_variables: {
	//             foo: {
	//                 cflags: ["-Dfoo-android"],
	//             },
	//         },
	//     },
	// },
	//
	// FullConfig would be "foo-android".
	//
	// For soong config variables, this is the namespace + product variable name
	// + value of the variable, if applicable. The value can also be
	// conditions_default.
	//
	// e.g.
	//
	// soong_config_variables: {
	//     feature1: {
	//         conditions_default: {
	//             cflags: ["-DDEFAULT1"],
	//         },
	//         cflags: ["-DFEATURE1"],
	//     },
	// }
	//
	// where feature1 is created in the "acme" namespace, so FullConfig would be
	// "acme__feature1" and "acme__feature1__conditions_default".
	//
	// e.g.
	//
	// soong_config_variables: {
	//     board: {
	//         soc_a: {
	//             cflags: ["-DSOC_A"],
	//         },
	//         soc_b: {
	//             cflags: ["-DSOC_B"],
	//         },
	//         soc_c: {},
	//         conditions_default: {
	//             cflags: ["-DSOC_DEFAULT"]
	//         },
	//     },
	// }
	//
	// where board is created in the "acme" namespace, so FullConfig would be
	// "acme__board__soc_a", "acme__board__soc_b", and
	// "acme__board__conditions_default"
	FullConfig string

	// keeps track of whether this product variable is nested under an arch variant
	OuterAxis bazel.ConfigurationAxis
}

func (p *ProductConfigProperty) AlwaysEmit() bool {
	return p.Namespace != ""
}

func (p *ProductConfigProperty) ConfigurationAxis() bazel.ConfigurationAxis {
	if p.Namespace == "" {
		return bazel.ProductVariableConfigurationAxis(p.FullConfig, p.OuterAxis)
	} else {
		// Soong config variables can be uniquely identified by the namespace
		// (e.g. acme, android) and the product variable name (e.g. board, size)
		return bazel.ProductVariableConfigurationAxis(p.Namespace+"__"+p.Name, bazel.NoConfigAxis)
	}
}

// SelectKey returns the literal string that represents this variable in a BUILD
// select statement.
func (p *ProductConfigProperty) SelectKey() string {
	if p.Namespace == "" {
		return strings.ToLower(p.FullConfig)
	}

	if p.FullConfig == bazel.ConditionsDefaultConfigKey {
		return bazel.ConditionsDefaultConfigKey
	}

	value := p.FullConfig
	if value == p.Name {
		value = ""
	}

	// e.g. acme__feature1, android__board__soc_a
	selectKey := strings.ToLower(strings.Join([]string{p.Namespace, p.Name}, "__"))
	if value != "" {
		selectKey = strings.ToLower(strings.Join([]string{selectKey, value}, "__"))
	}

	return selectKey
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
type ProductConfigProperties map[string]map[ProductConfigProperty]interface{}

// ProductVariableProperties returns a ProductConfigProperties containing only the properties which
// have been set for the given module.
func ProductVariableProperties(ctx ArchVariantContext, module Module) ProductConfigProperties {
	moduleBase := module.base()

	productConfigProperties := ProductConfigProperties{}

	if moduleBase.variableProperties != nil {
		productVariablesProperty := proptools.FieldNameForProperty("product_variables")
		productVariableValues(
			productVariablesProperty,
			moduleBase.variableProperties,
			"",
			"",
			&productConfigProperties,
			bazel.ConfigurationAxis{},
		)

		for axis, configToProps := range moduleBase.GetArchVariantProperties(ctx, moduleBase.variableProperties) {
			for config, props := range configToProps {
				// GetArchVariantProperties is creating an instance of the requested type
				// and productVariablesValues expects an interface, so no need to cast
				productVariableValues(
					productVariablesProperty,
					props,
					"",
					config,
					&productConfigProperties,
					axis)
			}
		}
	}

	if m, ok := module.(Bazelable); ok && m.namespacedVariableProps() != nil {
		for namespace, namespacedVariableProps := range m.namespacedVariableProps() {
			for _, namespacedVariableProp := range namespacedVariableProps {
				productVariableValues(
					soongconfig.SoongConfigProperty,
					namespacedVariableProp,
					namespace,
					"",
					&productConfigProperties,
					bazel.NoConfigAxis)
			}
		}
	}

	return productConfigProperties
}

func (p *ProductConfigProperties) AddProductConfigProperty(
	propertyName, namespace, productVariableName, config string, property interface{}, outerAxis bazel.ConfigurationAxis) {
	if (*p)[propertyName] == nil {
		(*p)[propertyName] = make(map[ProductConfigProperty]interface{})
	}

	productConfigProp := ProductConfigProperty{
		Namespace:  namespace,           // e.g. acme, android
		Name:       productVariableName, // e.g. size, feature1, feature2, FEATURE3, board
		FullConfig: config,              // e.g. size, feature1-x86, size__conditions_default
		OuterAxis:  outerAxis,
	}

	if existing, ok := (*p)[propertyName][productConfigProp]; ok && namespace != "" {
		switch dst := existing.(type) {
		case []string:
			if src, ok := property.([]string); ok {
				dst = append(dst, src...)
				(*p)[propertyName][productConfigProp] = dst
			}
		default:
			panic(fmt.Errorf("TODO: handle merging value %s", existing))
		}
	} else {
		(*p)[propertyName][productConfigProp] = property
	}
}

var (
	conditionsDefaultField string = proptools.FieldNameForProperty(bazel.ConditionsDefaultConfigKey)
)

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

func (productConfigProperties *ProductConfigProperties) AddProductConfigProperties(namespace, suffix string, variableValues reflect.Value, outerAxis bazel.ConfigurationAxis) {
	// variableValues can either be a product_variables or
	// soong_config_variables struct.
	//
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
	//
	// Example of soong_config_variables:
	//
	// soong_config_variables: {
	//      feature1: {
	//        	conditions_default: {
	//               ...
	//          },
	//          cflags: ...
	//      },
	//      feature2: {
	//          cflags: ...
	//        	conditions_default: {
	//               ...
	//          },
	//      },
	//      board: {
	//         soc_a: {
	//             ...
	//         },
	//         soc_a: {
	//             ...
	//         },
	//         soc_c: {},
	//         conditions_default: {
	//              ...
	//         },
	//      },
	// }
	for i := 0; i < variableValues.NumField(); i++ {
		// e.g. Platform_sdk_version, Unbundled_build, Malloc_not_svelte, etc.
		productVariableName := variableValues.Type().Field(i).Name

		variableValue := variableValues.Field(i)
		// Check if any properties were set for the module
		if variableValue.IsZero() {
			// e.g. feature1: {}, malloc_not_svelte: {}
			continue
		}

		// Unlike product variables, config variables require a few more
		// indirections to extract the struct from the reflect.Value.
		if v, ok := maybeExtractConfigVarProp(variableValue); ok {
			variableValue = v
		}

		for j := 0; j < variableValue.NumField(); j++ {
			property := variableValue.Field(j)
			// e.g. Asflags, Cflags, Enabled, etc.
			propertyName := variableValue.Type().Field(j).Name
			// config can also be "conditions_default".
			config := proptools.PropertyNameForField(propertyName)

			// If the property wasn't set, no need to pass it along
			if property.IsZero() {
				continue
			}

			if v, ok := maybeExtractConfigVarProp(property); ok {
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
				field := v
				// Iterate over fields of this struct prop.
				for k := 0; k < field.NumField(); k++ {
					// For product variables, zero values are irrelevant; however, for soong config variables,
					// empty values are relevant because there can also be a conditions default which is not
					// applied for empty variables.
					if field.Field(k).IsZero() && namespace == "" {
						continue
					}
					actualPropertyName := field.Type().Field(k).Name

					productConfigProperties.AddProductConfigProperty(
						actualPropertyName,  // e.g. cflags, static_libs
						namespace,           // e.g. acme, android
						productVariableName, // e.g. size, feature1, FEATURE2, board
						config,
						field.Field(k).Interface(), // e.g. ["-DDEFAULT"], ["foo", "bar"],
						outerAxis,
					)
				}
			} else if property.Kind() != reflect.Interface {
				// If not an interface, then this is not a conditions_default or
				// a struct prop. That is, this is a regular product variable,
				// or a bool/value config variable.
				config := productVariableName + suffix
				productConfigProperties.AddProductConfigProperty(
					propertyName,
					namespace,
					productVariableName,
					config,
					property.Interface(),
					outerAxis,
				)
			}
		}
	}
}

// productVariableValues uses reflection to convert a property struct for
// product_variables and soong_config_variables to structs that can be generated
// as select statements.
func productVariableValues(
	fieldName string, variableProps interface{}, namespace, suffix string, productConfigProperties *ProductConfigProperties, outerAxis bazel.ConfigurationAxis) {
	if suffix != "" {
		suffix = "-" + suffix
	}

	// variableValues represent the product_variables or soong_config_variables struct.
	variableValues := reflect.ValueOf(variableProps).Elem().FieldByName(fieldName)
	productConfigProperties.AddProductConfigProperties(namespace, suffix, variableValues, outerAxis)
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
