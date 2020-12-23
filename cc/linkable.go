package cc

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

// LinkableInterface is an interface for a type of module that is linkable in a C++ library.
type LinkableInterface interface {
	android.Module

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
	IsLlndk() bool
	IsLlndkPublic() bool
	IsVndk() bool
	IsVndkExt() bool
	IsVndkPrivate() bool
	HasVendorVariant() bool
	InProduct() bool

	SdkVersion() string
	AlwaysSdk() bool
	IsSdkVariant() bool

	SplitPerApiLevel() bool
}

var (
	// Dependency tag for crtbegin, an object file responsible for initialization.
	CrtBeginDepTag = dependencyTag{name: "crtbegin"}
	// Dependency tag for crtend, an object file responsible for program termination.
	CrtEndDepTag = dependencyTag{name: "crtend"}
	// Dependency tag for coverage library.
	CoverageDepTag = dependencyTag{name: "coverage"}
)

// SharedDepTag returns the dependency tag for any C++ shared libraries.
func SharedDepTag() blueprint.DependencyTag {
	return libraryDependencyTag{Kind: sharedLibraryDependency}
}

// StaticDepTag returns the dependency tag for any C++ static libraries.
func StaticDepTag() blueprint.DependencyTag {
	return libraryDependencyTag{Kind: staticLibraryDependency}
}

// HeaderDepTag returns the dependency tag for any C++ "header-only" libraries.
func HeaderDepTag() blueprint.DependencyTag {
	return libraryDependencyTag{Kind: headerLibraryDependency}
}

// SharedLibraryInfo is a provider to propagate information about a shared C++ library.
type SharedLibraryInfo struct {
	SharedLibrary           android.Path
	UnstrippedSharedLibrary android.Path

	TableOfContents       android.OptionalPath
	CoverageSharedLibrary android.OptionalPath

	StaticAnalogue *StaticLibraryInfo
}

var SharedLibraryInfoProvider = blueprint.NewProvider(SharedLibraryInfo{})

// SharedStubLibrary is a struct containing information about a stub shared library.
// Stub libraries are used for cross-APEX dependencies; when a library is to depend on a shared
// library in another APEX, it must depend on the stub version of that library.
type SharedStubLibrary struct {
	// The version of the stub (corresponding to the stable version of the shared library being
	// stubbed).
	Version           string
	SharedLibraryInfo SharedLibraryInfo
	FlagExporterInfo  FlagExporterInfo
}

// SharedLibraryStubsInfo is a provider to propagate information about all shared library stubs
// which are dependencies of a library.
// Stub libraries are used for cross-APEX dependencies; when a library is to depend on a shared
// library in another APEX, it must depend on the stub version of that library.
type SharedLibraryStubsInfo struct {
	SharedStubLibraries []SharedStubLibrary

	IsLLNDK bool
}

var SharedLibraryStubsProvider = blueprint.NewProvider(SharedLibraryStubsInfo{})

// StaticLibraryInfo is a provider to propagate information about a static C++ library.
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

// HeaderLibraryInfo is a marker provider that identifies a module as a header library.
type HeaderLibraryInfo struct {
}

// HeaderLibraryInfoProvider is a marker provider that identifies a module as a header library.
var HeaderLibraryInfoProvider = blueprint.NewProvider(HeaderLibraryInfo{})

// FlagExporterInfo is a provider to propagate transitive library information
// pertaining to exported include paths and flags.
type FlagExporterInfo struct {
	IncludeDirs       android.Paths // Include directories to be included with -I
	SystemIncludeDirs android.Paths // System include directories to be included with -isystem
	Flags             []string      // Exported raw flags.
	Deps              android.Paths
	GeneratedHeaders  android.Paths
}

var FlagExporterInfoProvider = blueprint.NewProvider(FlagExporterInfo{})
