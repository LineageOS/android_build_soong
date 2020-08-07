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

	IncludeDirs() android.Paths
	SetDepsInLinkOrder([]android.Path)
	GetDepsInLinkOrder() []android.Path

	HasStaticVariant() bool
	GetStaticVariant() LinkableInterface

	NonCcVariants() bool

	StubsVersions() []string
	BuildStubs() bool
	SetBuildStubs()
	SetStubsVersions(string)
	StubsVersion() string
	HasStubsVariants() bool
	SelectedStl() string
	ApiLevel() string

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

	ToolchainLibrary() bool
	NdkPrebuiltStl() bool
	StubDecorator() bool
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
