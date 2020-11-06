package cc

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

type LinkableInterface interface {
	Module() android.Module
	CcLibrary() bool
	CcLibraryInterface() bool

	OutputFile() android.OptionalPath
	CoverageFiles() android.Paths

	NonCcVariants() bool

	SelectedStl() string

	BuildStaticVariant() bool
	BuildSharedVariant() bool
	SetStatic()
	SetShared()
	Static() bool
	Shared() bool
	Toc() android.OptionalPath

	Host() bool

	InRamdisk() bool
	OnlyInRamdisk() bool

	InVendorRamdisk() bool
	OnlyInVendorRamdisk() bool

	InRecovery() bool
	OnlyInRecovery() bool

	UseSdk() bool
	UseVndk() bool
	MustUseVendorVariant() bool
	IsVndk() bool
	HasVendorVariant() bool

	SdkVersion() string
	AlwaysSdk() bool
	IsSdkVariant() bool

	SplitPerApiLevel() bool
}

var (
	CrtBeginDepTag = dependencyTag{name: "crtbegin"}
	CrtEndDepTag   = dependencyTag{name: "crtend"}
	CoverageDepTag = dependencyTag{name: "coverage"}
)

func SharedDepTag() blueprint.DependencyTag {
	return libraryDependencyTag{Kind: sharedLibraryDependency}
}

func StaticDepTag() blueprint.DependencyTag {
	return libraryDependencyTag{Kind: staticLibraryDependency}
}

func HeaderDepTag() blueprint.DependencyTag {
	return libraryDependencyTag{Kind: headerLibraryDependency}
}

type SharedLibraryInfo struct {
	SharedLibrary           android.Path
	UnstrippedSharedLibrary android.Path

	TableOfContents       android.OptionalPath
	CoverageSharedLibrary android.OptionalPath

	StaticAnalogue *StaticLibraryInfo
}

var SharedLibraryInfoProvider = blueprint.NewProvider(SharedLibraryInfo{})

type SharedLibraryImplementationStubsInfo struct {
	SharedLibraryStubsInfos []SharedLibraryStubsInfo

	IsLLNDK bool
}

var SharedLibraryImplementationStubsInfoProvider = blueprint.NewProvider(SharedLibraryImplementationStubsInfo{})

type SharedLibraryStubsInfo struct {
	Version           string
	SharedLibraryInfo SharedLibraryInfo
	FlagExporterInfo  FlagExporterInfo
}

var SharedLibraryStubsInfoProvider = blueprint.NewProvider(SharedLibraryStubsInfo{})

type StaticLibraryInfo struct {
	StaticLibrary android.Path
	Objects       Objects
	ReuseObjects  Objects

	// This isn't the actual transitive DepSet, shared library dependencies have been
	// converted into static library analogues.  It is only used to order the static
	// library dependencies that were specified for the current module.
	TransitiveStaticLibrariesForOrdering *android.DepSet
}

var StaticLibraryInfoProvider = blueprint.NewProvider(StaticLibraryInfo{})

type FlagExporterInfo struct {
	IncludeDirs       android.Paths
	SystemIncludeDirs android.Paths
	Flags             []string
	Deps              android.Paths
	GeneratedHeaders  android.Paths
}

var FlagExporterInfoProvider = blueprint.NewProvider(FlagExporterInfo{})
