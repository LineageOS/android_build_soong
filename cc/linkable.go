package cc

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

// PlatformSanitizeable is an interface for sanitizing platform modules.
type PlatformSanitizeable interface {
	LinkableInterface

	// SanitizePropDefined returns whether the Sanitizer properties struct for this module is defined.
	SanitizePropDefined() bool

	// IsDependencyRoot returns whether a module is of a type which cannot be a linkage dependency
	// of another module. For example, cc_binary and rust_binary represent dependency roots as other
	// modules cannot have linkage dependencies against these types.
	IsDependencyRoot() bool

	// IsSanitizerEnabled returns whether a sanitizer is enabled.
	IsSanitizerEnabled(t SanitizerType) bool

	// IsSanitizerExplicitlyDisabled returns whether a sanitizer has been explicitly disabled (set to false) rather
	// than left undefined.
	IsSanitizerExplicitlyDisabled(t SanitizerType) bool

	// SanitizeDep returns the value of the SanitizeDep flag, which is set if a module is a dependency of a
	// sanitized module.
	SanitizeDep() bool

	// SetSanitizer enables or disables the specified sanitizer type if it's supported, otherwise this should panic.
	SetSanitizer(t SanitizerType, b bool)

	// SetSanitizerDep returns true if the module is statically linked.
	SetSanitizeDep(b bool)

	// StaticallyLinked returns true if the module is statically linked.
	StaticallyLinked() bool

	// SetInSanitizerDir sets the module installation to the sanitizer directory.
	SetInSanitizerDir()

	// SanitizeNever returns true if this module should never be sanitized.
	SanitizeNever() bool

	// SanitizerSupported returns true if a sanitizer type is supported by this modules compiler.
	SanitizerSupported(t SanitizerType) bool

	// SanitizableDepTagChecker returns a SantizableDependencyTagChecker function type.
	SanitizableDepTagChecker() SantizableDependencyTagChecker
}

// SantizableDependencyTagChecker functions check whether or not a dependency
// tag can be sanitized. These functions should return true if the tag can be
// sanitized, otherwise they should return false. These functions should also
// handle all possible dependency tags in the dependency tree. For example,
// Rust modules can depend on both Rust and CC libraries, so the Rust module
// implementation should handle tags from both.
type SantizableDependencyTagChecker func(tag blueprint.DependencyTag) bool

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
	Header() bool
	IsPrebuilt() bool
	Toc() android.OptionalPath

	Host() bool

	InRamdisk() bool
	OnlyInRamdisk() bool

	InVendorRamdisk() bool
	OnlyInVendorRamdisk() bool

	InRecovery() bool
	OnlyInRecovery() bool

	InVendor() bool

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

	// SetPreventInstall sets the PreventInstall property to 'true' for this module.
	SetPreventInstall()
	// SetHideFromMake sets the HideFromMake property to 'true' for this module.
	SetHideFromMake()
}

var (
	// Dependency tag for crtbegin, an object file responsible for initialization.
	CrtBeginDepTag = dependencyTag{name: "crtbegin"}
	// Dependency tag for crtend, an object file responsible for program termination.
	CrtEndDepTag = dependencyTag{name: "crtend"}
	// Dependency tag for coverage library.
	CoverageDepTag = dependencyTag{name: "coverage"}
)

// GetImageVariantType returns the ImageVariantType string value for the given module
// (these are defined in cc/image.go).
func GetImageVariantType(c LinkableInterface) ImageVariantType {
	if c.Host() {
		return hostImageVariant
	} else if c.InVendor() {
		return vendorImageVariant
	} else if c.InProduct() {
		return productImageVariant
	} else if c.InRamdisk() {
		return ramdiskImageVariant
	} else if c.InVendorRamdisk() {
		return vendorRamdiskImageVariant
	} else if c.InRecovery() {
		return recoveryImageVariant
	} else {
		return coreImageVariant
	}
}

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
