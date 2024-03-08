// Copyright 2020 Google Inc. All rights reserved.
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

package dexpreopt

import (
	"encoding/json"
	"fmt"
	"strconv"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

// This comment describes the following:
//  1. the concept of class loader context (CLC) and its relation to classpath
//  2. how PackageManager constructs CLC from shared libraries and their dependencies
//  3. build-time vs. run-time CLC and why this matters for dexpreopt
//  4. manifest fixer: a tool that adds missing <uses-library> tags to the manifests
//  5. build system support for CLC
//
// 1. Class loader context
// -----------------------
//
// Java libraries and apps that have run-time dependency on other libraries should list the used
// libraries in their manifest (AndroidManifest.xml file). Each used library should be specified in
// a <uses-library> tag that has the library name and an optional attribute specifying if the
// library is optional or required. Required libraries are necessary for the library/app to run (it
// will fail at runtime if the library cannot be loaded), and optional libraries are used only if
// they are present (if not, the library/app can run without them).
//
// The libraries listed in <uses-library> tags are in the classpath of a library/app.
//
// Besides libraries, an app may also use another APK (for example in the case of split APKs), or
// anything that gets added by the app dynamically. In general, it is impossible to know at build
// time what the app may use at runtime. In the build system we focus on the known part: libraries.
//
// Class loader context (CLC) is a tree-like structure that describes class loader hierarchy. The
// build system uses CLC in a more narrow sense: it is a tree of libraries that represents
// transitive closure of all <uses-library> dependencies of a library/app. The top-level elements of
// a CLC are the direct <uses-library> dependencies specified in the manifest (aka. classpath). Each
// node of a CLC tree is a <uses-library> which may have its own <uses-library> sub-nodes.
//
// Because <uses-library> dependencies are, in general, a graph and not necessarily a tree, CLC may
// contain subtrees for the same library multiple times. In other words, CLC is the dependency graph
// "unfolded" to a tree. The duplication is only on a logical level, and the actual underlying class
// loaders are not duplicated (at runtime there is a single class loader instance for each library).
//
// Example: A has <uses-library> tags B, C and D; C has <uses-library tags> B and D;
//
//	      D has <uses-library> E; B and E have no <uses-library> dependencies. The CLC is:
//	A
//	├── B
//	├── C
//	│   ├── B
//	│   └── D
//	│       └── E
//	└── D
//	    └── E
//
// CLC defines the lookup order of libraries when resolving Java classes used by the library/app.
// The lookup order is important because libraries may contain duplicate classes, and the class is
// resolved to the first match.
//
// 2. PackageManager and "shared" libraries
// ----------------------------------------
//
// In order to load an APK at runtime, PackageManager (in frameworks/base) creates a CLC. It adds
// the libraries listed in the <uses-library> tags in the app's manifest as top-level CLC elements.
// For each of the used libraries PackageManager gets all its <uses-library> dependencies (specified
// as tags in the manifest of that library) and adds a nested CLC for each dependency. This process
// continues recursively until all leaf nodes of the constructed CLC tree are libraries that have no
// <uses-library> dependencies.
//
// PackageManager is aware only of "shared" libraries. The definition of "shared" here differs from
// its usual meaning (as in shared vs. static). In Android, Java "shared" libraries are those listed
// in /system/etc/permissions/platform.xml file. This file is installed on device. Each entry in it
// contains the name of a "shared" library, a path to its DEX jar file and a list of dependencies
// (other "shared" libraries that this one uses at runtime and specifies them in <uses-library> tags
// in its manifest).
//
// In other words, there are two sources of information that allow PackageManager to construct CLC
// at runtime: <uses-library> tags in the manifests and "shared" library dependencies in
// /system/etc/permissions/platform.xml.
//
// 3. Build-time and run-time CLC and dexpreopt
// --------------------------------------------
//
// CLC is needed not only when loading a library/app, but also when compiling it. Compilation may
// happen either on device (known as "dexopt") or during the build (known as "dexpreopt"). Since
// dexopt takes place on device, it has the same information as PackageManager (manifests and
// shared library dependencies). Dexpreopt, on the other hand, takes place on host and in a totally
// different environment, and it has to get the same information from the build system (see the
// section about build system support below).
//
// Thus, the build-time CLC used by dexpreopt and the run-time CLC used by PackageManager are
// the same thing, but computed in two different ways.
//
// It is important that build-time and run-time CLCs coincide, otherwise the AOT-compiled code
// created by dexpreopt will be rejected. In order to check the equality of build-time and
// run-time CLCs, the dex2oat compiler records build-time CLC in the *.odex files (in the
// "classpath" field of the OAT file header). To find the stored CLC, use the following command:
// `oatdump --oat-file=<FILE> | grep '^classpath = '`.
//
// Mismatch between build-time and run-time CLC is reported in logcat during boot (search with
// `logcat | grep -E 'ClassLoaderContext [a-z ]+ mismatch'`. Mismatch is bad for performance, as it
// forces the library/app to either be dexopted, or to run without any optimizations (e.g. the app's
// code may need to be extracted in memory from the APK, a very expensive operation).
//
// A <uses-library> can be either optional or required. From dexpreopt standpoint, required library
// must be present at build time (its absence is a build error). An optional library may be either
// present or absent at build time: if present, it will be added to the CLC, passed to dex2oat and
// recorded in the *.odex file; otherwise, if the library is absent, it will be skipped and not
// added to CLC. If there is a mismatch between built-time and run-time status (optional library is
// present in one case, but not the other), then the build-time and run-time CLCs won't match and
// the compiled code will be rejected. It is unknown at build time if the library will be present at
// runtime, therefore either including or excluding it may cause CLC mismatch.
//
// 4. Manifest fixer
// -----------------
//
// Sometimes <uses-library> tags are missing from the source manifest of a library/app. This may
// happen for example if one of the transitive dependencies of the library/app starts using another
// <uses-library>, and the library/app's manifest isn't updated to include it.
//
// Soong can compute some of the missing <uses-library> tags for a given library/app automatically
// as SDK libraries in the transitive dependency closure of the library/app. The closure is needed
// because a library/app may depend on a static library that may in turn depend on an SDK library,
// (possibly transitively via another library).
//
// Not all <uses-library> tags can be computed in this way, because some of the <uses-library>
// dependencies are not SDK libraries, or they are not reachable via transitive dependency closure.
// But when possible, allowing Soong to calculate the manifest entries is less prone to errors and
// simplifies maintenance. For example, consider a situation when many apps use some static library
// that adds a new <uses-library> dependency -- all the apps will have to be updated. That is
// difficult to maintain.
//
// Soong computes the libraries that need to be in the manifest as the top-level libraries in CLC.
// These libraries are passed to the manifest_fixer.
//
// All libraries added to the manifest should be "shared" libraries, so that PackageManager can look
// up their dependencies and reconstruct the nested subcontexts at runtime. There is no build check
// to ensure this, it is an assumption.
//
// 5. Build system support
// -----------------------
//
// In order to construct CLC for dexpreopt and manifest_fixer, the build system needs to know all
// <uses-library> dependencies of the dexpreopted library/app (including transitive dependencies).
// For each <uses-librarry> dependency it needs to know the following information:
//
//   - the real name of the <uses-library> (it may be different from the module name)
//   - build-time (on host) and run-time (on device) paths to the DEX jar file of the library
//   - whether this library is optional or required
//   - all <uses-library> dependencies
//
// Since the build system doesn't have access to the manifest contents (it cannot read manifests at
// the time of build rule generation), it is necessary to copy this information to the Android.bp
// and Android.mk files. For blueprints, the relevant properties are `uses_libs` and
// `optional_uses_libs`. For makefiles, relevant variables are `LOCAL_USES_LIBRARIES` and
// `LOCAL_OPTIONAL_USES_LIBRARIES`. It is preferable to avoid specifying these properties explicilty
// when they can be computed automatically by Soong (as the transitive closure of SDK library
// dependencies).
//
// Some of the Java libraries that are used as <uses-library> are not SDK libraries (they are
// defined as `java_library` rather than `java_sdk_library` in the Android.bp files). In order for
// the build system to handle them automatically like SDK libraries, it is possible to set a
// property `provides_uses_lib` or variable `LOCAL_PROVIDES_USES_LIBRARY` on the blueprint/makefile
// module of such library. This property can also be used to specify real library name in cases
// when it differs from the module name.
//
// Because the information from the manifests has to be duplicated in the Android.bp/Android.mk
// files, there is a danger that it may get out of sync. To guard against that, the build system
// generates a rule that checks the metadata in the build files against the contents of a manifest
// (verify_uses_libraries). The manifest can be available as a source file, or as part of a prebuilt
// APK. Note that reading the manifests at the Ninja stage of the build is fine, unlike the build
// rule generation phase.
//
// ClassLoaderContext is a structure that represents CLC.
type ClassLoaderContext struct {
	// The name of the library.
	Name string

	// If the library is optional or required.
	Optional bool

	// On-host build path to the library dex file (used in dex2oat argument --class-loader-context).
	Host android.Path

	// On-device install path (used in dex2oat argument --stored-class-loader-context).
	Device string

	// Nested sub-CLC for dependencies.
	Subcontexts []*ClassLoaderContext
}

// excludeLibs excludes the libraries from this ClassLoaderContext.
//
// This treats the supplied context as being immutable (as it may come from a dependency). So, it
// implements copy-on-exclusion logic. That means that if any of the excluded libraries are used
// within this context then this will return a deep copy of this without those libraries.
//
// If this ClassLoaderContext matches one of the libraries to exclude then this returns (nil, true)
// to indicate that this context should be excluded from the containing list.
//
// If any of this ClassLoaderContext's Subcontexts reference the excluded libraries then this
// returns a pointer to a copy of this without the excluded libraries and true to indicate that this
// was copied.
//
// Otherwise, this returns a pointer to this and false to indicate that this was not copied.
func (c *ClassLoaderContext) excludeLibs(excludedLibs []string) (*ClassLoaderContext, bool) {
	if android.InList(c.Name, excludedLibs) {
		return nil, true
	}

	if excludedList, modified := excludeLibsFromCLCList(c.Subcontexts, excludedLibs); modified {
		clcCopy := *c
		clcCopy.Subcontexts = excludedList
		return &clcCopy, true
	}

	return c, false
}

// ClassLoaderContextMap is a map from SDK version to CLC. There is a special entry with key
// AnySdkVersion that stores unconditional CLC that is added regardless of the target SDK version.
//
// Conditional CLC is for compatibility libraries which didn't exist prior to a certain SDK version
// (say, N), but classes in them were in the bootclasspath jars, etc., and in version N they have
// been separated into a standalone <uses-library>. Compatibility libraries should only be in the
// CLC if the library/app that uses them has `targetSdkVersion` less than N in the manifest.
//
// Currently only apps (but not libraries) use conditional CLC.
//
// Target SDK version information is unavailable to the build system at rule generation time, so
// the build system doesn't know whether conditional CLC is needed for a given app or not. So it
// generates a build rule that includes conditional CLC for all versions, extracts the target SDK
// version from the manifest, and filters the CLCs based on that version. Exact final CLC that is
// passed to dex2oat is unknown to the build system, and gets known only at Ninja stage.
type ClassLoaderContextMap map[int][]*ClassLoaderContext

// Compatibility libraries. Some are optional, and some are required: this is the default that
// affects how they are handled by the Soong logic that automatically adds implicit SDK libraries
// to the manifest_fixer, but an explicit `uses_libs`/`optional_uses_libs` can override this.
var OrgApacheHttpLegacy = "org.apache.http.legacy"
var AndroidTestBase = "android.test.base"
var AndroidTestMock = "android.test.mock"
var AndroidHidlBase = "android.hidl.base-V1.0-java"
var AndroidHidlManager = "android.hidl.manager-V1.0-java"

// Compatibility libraries grouped by version/optionality (for convenience, to avoid repeating the
// same lists in multiple places).
var OptionalCompatUsesLibs28 = []string{
	OrgApacheHttpLegacy,
}
var OptionalCompatUsesLibs30 = []string{
	AndroidTestBase,
	AndroidTestMock,
}
var CompatUsesLibs29 = []string{
	AndroidHidlManager,
	AndroidHidlBase,
}
var OptionalCompatUsesLibs = append(android.CopyOf(OptionalCompatUsesLibs28), OptionalCompatUsesLibs30...)
var CompatUsesLibs = android.CopyOf(CompatUsesLibs29)

const UnknownInstallLibraryPath = "error"

// AnySdkVersion means that the class loader context is needed regardless of the targetSdkVersion
// of the app. The numeric value affects the key order in the map and, as a result, the order of
// arguments passed to construct_context.py (high value means that the unconditional context goes
// last). We use the converntional "current" SDK level (10000), but any big number would do as well.
const AnySdkVersion int = android.FutureApiLevelInt

// Add class loader context for the given library to the map entry for the given SDK version.
func (clcMap ClassLoaderContextMap) addContext(ctx android.ModuleInstallPathContext, sdkVer int, lib string,
	optional bool, hostPath, installPath android.Path, nestedClcMap ClassLoaderContextMap) error {

	// For prebuilts, library should have the same name as the source module.
	lib = android.RemoveOptionalPrebuiltPrefix(lib)

	devicePath := UnknownInstallLibraryPath
	if installPath == nil {
		if android.InList(lib, CompatUsesLibs) || android.InList(lib, OptionalCompatUsesLibs) {
			// Assume that compatibility libraries are installed in /system/framework.
			installPath = android.PathForModuleInstall(ctx, "framework", lib+".jar")
		} else {
			// For some stub libraries the only known thing is the name of their implementation
			// library, but the library itself is unavailable (missing or part of a prebuilt). In
			// such cases we still need to add the library to <uses-library> tags in the manifest,
			// but we cannot use it for dexpreopt.
		}
	}
	if installPath != nil {
		devicePath = android.InstallPathToOnDevicePath(ctx, installPath.(android.InstallPath))
	}

	// Nested class loader context shouldn't have conditional part (it is allowed only at the top level).
	for ver, _ := range nestedClcMap {
		if ver != AnySdkVersion {
			_, clcPaths := ComputeClassLoaderContextDependencies(nestedClcMap)
			return fmt.Errorf("nested class loader context shouldn't have conditional part: %+v", clcPaths)
		}
	}
	subcontexts := nestedClcMap[AnySdkVersion]

	// Check if the library with this name is already present in unconditional top-level CLC.
	for _, clc := range clcMap[sdkVer] {
		if clc.Name != lib {
			// Ok, a different library.
		} else if clc.Host == hostPath && clc.Device == devicePath {
			// Ok, the same library with the same paths. Don't re-add it, but don't raise an error
			// either, as the same library may be reachable via different transitional dependencies.
			return nil
		} else {
			// Fail, as someone is trying to add the same library with different paths. This likely
			// indicates an error somewhere else, like trying to add a stub library.
			return fmt.Errorf("a <uses-library> named %q is already in class loader context,"+
				"but the library paths are different:\t\n", lib)
		}
	}

	clcMap[sdkVer] = append(clcMap[sdkVer], &ClassLoaderContext{
		Name:        lib,
		Optional:    optional,
		Host:        hostPath,
		Device:      devicePath,
		Subcontexts: subcontexts,
	})
	return nil
}

// Add class loader context for the given SDK version. Don't fail on unknown build/install paths, as
// libraries with unknown paths still need to be processed by manifest_fixer (which doesn't care
// about paths). For the subset of libraries that are used in dexpreopt, their build/install paths
// are validated later before CLC is used (in validateClassLoaderContext).
func (clcMap ClassLoaderContextMap) AddContext(ctx android.ModuleInstallPathContext, sdkVer int,
	lib string, optional bool, hostPath, installPath android.Path, nestedClcMap ClassLoaderContextMap) {

	err := clcMap.addContext(ctx, sdkVer, lib, optional, hostPath, installPath, nestedClcMap)
	if err != nil {
		ctx.ModuleErrorf(err.Error())
	}
}

// Merge the other class loader context map into this one, do not override existing entries.
// The implicitRootLib parameter is the name of the library for which the other class loader
// context map was constructed. If the implicitRootLib is itself a <uses-library>, it should be
// already present in the class loader context (with the other context as its subcontext) -- in
// that case do not re-add the other context. Otherwise add the other context at the top-level.
func (clcMap ClassLoaderContextMap) AddContextMap(otherClcMap ClassLoaderContextMap, implicitRootLib string) {
	if otherClcMap == nil {
		return
	}

	// If the implicit root of the merged map is already present as one of top-level subtrees, do
	// not merge it second time.
	for _, clc := range clcMap[AnySdkVersion] {
		if clc.Name == implicitRootLib {
			return
		}
	}

	for sdkVer, otherClcs := range otherClcMap {
		for _, otherClc := range otherClcs {
			alreadyHave := false
			for _, clc := range clcMap[sdkVer] {
				if clc.Name == otherClc.Name {
					alreadyHave = true
					break
				}
			}
			if !alreadyHave {
				clcMap[sdkVer] = append(clcMap[sdkVer], otherClc)
			}
		}
	}
}

// Returns top-level libraries in the CLC (conditional CLC, i.e. compatibility libraries are not
// included). This is the list of libraries that should be in the <uses-library> tags in the
// manifest. Some of them may be present in the source manifest, others are added by manifest_fixer.
// Required and optional libraries are in separate lists.
func (clcMap ClassLoaderContextMap) UsesLibs() (required []string, optional []string) {
	if clcMap != nil {
		clcs := clcMap[AnySdkVersion]
		required = make([]string, 0, len(clcs))
		optional = make([]string, 0, len(clcs))
		for _, clc := range clcs {
			if clc.Optional {
				optional = append(optional, clc.Name)
			} else {
				required = append(required, clc.Name)
			}
		}
	}
	return required, optional
}

func (clcMap ClassLoaderContextMap) Dump() string {
	jsonCLC := toJsonClassLoaderContext(clcMap)
	bytes, err := json.MarshalIndent(jsonCLC, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func (clcMap ClassLoaderContextMap) DumpForFlag() string {
	jsonCLC := toJsonClassLoaderContext(clcMap)
	bytes, err := json.Marshal(jsonCLC)
	if err != nil {
		panic(err)
	}
	return proptools.ShellEscapeIncludingSpaces(string(bytes))
}

// excludeLibsFromCLCList excludes the libraries from the ClassLoaderContext in this list.
//
// This treats the supplied list as being immutable (as it may come from a dependency). So, it
// implements copy-on-exclusion logic. That means that if any of the excluded libraries are used
// within the contexts in the list then this will return a deep copy of the list without those
// libraries.
//
// If any of the ClassLoaderContext in the list reference the excluded libraries then this returns a
// copy of this list without the excluded libraries and true to indicate that this was copied.
//
// Otherwise, this returns the list and false to indicate that this was not copied.
func excludeLibsFromCLCList(clcList []*ClassLoaderContext, excludedLibs []string) ([]*ClassLoaderContext, bool) {
	modifiedList := false
	copiedList := make([]*ClassLoaderContext, 0, len(clcList))
	for _, clc := range clcList {
		resultClc, modifiedClc := clc.excludeLibs(excludedLibs)
		if resultClc != nil {
			copiedList = append(copiedList, resultClc)
		}
		modifiedList = modifiedList || modifiedClc
	}

	if modifiedList {
		return copiedList, true
	} else {
		return clcList, false
	}
}

// ExcludeLibs excludes the libraries from the ClassLoaderContextMap.
//
// If the list o libraries is empty then this returns the ClassLoaderContextMap.
//
// This treats the ClassLoaderContextMap as being immutable (as it may come from a dependency). So,
// it implements copy-on-exclusion logic. That means that if any of the excluded libraries are used
// within the contexts in the map then this will return a deep copy of the map without those
// libraries.
//
// Otherwise, this returns the map unchanged.
func (clcMap ClassLoaderContextMap) ExcludeLibs(excludedLibs []string) ClassLoaderContextMap {
	if len(excludedLibs) == 0 {
		return clcMap
	}

	excludedClcMap := make(ClassLoaderContextMap)
	modifiedMap := false
	for sdkVersion, clcList := range clcMap {
		excludedList, modifiedList := excludeLibsFromCLCList(clcList, excludedLibs)
		if len(excludedList) != 0 {
			excludedClcMap[sdkVersion] = excludedList
		}
		modifiedMap = modifiedMap || modifiedList
	}

	if modifiedMap {
		return excludedClcMap
	} else {
		return clcMap
	}
}

// Now that the full unconditional context is known, reconstruct conditional context.
// Apply filters for individual libraries, mirroring what the PackageManager does when it
// constructs class loader context on device.
//
// TODO(b/132357300): remove "android.hidl.manager" and "android.hidl.base" for non-system apps.
func fixClassLoaderContext(clcMap ClassLoaderContextMap) {
	required, optional := clcMap.UsesLibs()
	usesLibs := append(required, optional...)

	for sdkVer, clcs := range clcMap {
		if sdkVer == AnySdkVersion {
			continue
		}
		fixedClcs := []*ClassLoaderContext{}
		for _, clc := range clcs {
			if android.InList(clc.Name, usesLibs) {
				// skip compatibility libraries that are already included in unconditional context
			} else if clc.Name == AndroidTestMock && !android.InList("android.test.runner", usesLibs) {
				// android.test.mock is only needed as a compatibility library (in conditional class
				// loader context) if android.test.runner is used, otherwise skip it
			} else {
				fixedClcs = append(fixedClcs, clc)
			}
			clcMap[sdkVer] = fixedClcs
		}
	}
}

// Return true if all build/install library paths are valid (including recursive subcontexts),
// otherwise return false. A build path is valid if it's not nil. An install path is valid if it's
// not equal to a special "error" value.
func validateClassLoaderContext(clcMap ClassLoaderContextMap) (bool, error) {
	for sdkVer, clcs := range clcMap {
		if valid, err := validateClassLoaderContextRec(sdkVer, clcs); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// Helper function for validateClassLoaderContext() that handles recursion.
func validateClassLoaderContextRec(sdkVer int, clcs []*ClassLoaderContext) (bool, error) {
	for _, clc := range clcs {
		if clc.Host == nil || clc.Device == UnknownInstallLibraryPath {
			if sdkVer == AnySdkVersion {
				// Return error if dexpreopt doesn't know paths to one of the <uses-library>
				// dependencies. In the future we may need to relax this and just disable dexpreopt.
				if clc.Host == nil {
					return false, fmt.Errorf("invalid build path for <uses-library> \"%s\"", clc.Name)
				} else {
					return false, fmt.Errorf("invalid install path for <uses-library> \"%s\"", clc.Name)
				}
			} else {
				// No error for compatibility libraries, as Soong doesn't know if they are needed
				// (this depends on the targetSdkVersion in the manifest), but the CLC is invalid.
				return false, nil
			}
		}
		if valid, err := validateClassLoaderContextRec(sdkVer, clc.Subcontexts); !valid || err != nil {
			return valid, err
		}
	}
	return true, nil
}

// Returns a slice of library names and a slice of build paths for all possible dependencies that
// the class loader context may refer to.
// Perform a depth-first preorder traversal of the class loader context tree for each SDK version.
func ComputeClassLoaderContextDependencies(clcMap ClassLoaderContextMap) (names []string, paths android.Paths) {
	for _, clcs := range clcMap {
		currentNames, currentPaths := ComputeClassLoaderContextDependenciesRec(clcs)
		names = append(names, currentNames...)
		paths = append(paths, currentPaths...)
	}
	return android.FirstUniqueStrings(names), android.FirstUniquePaths(paths)
}

// Helper function for ComputeClassLoaderContextDependencies() that handles recursion.
func ComputeClassLoaderContextDependenciesRec(clcs []*ClassLoaderContext) (names []string, paths android.Paths) {
	for _, clc := range clcs {
		subNames, subPaths := ComputeClassLoaderContextDependenciesRec(clc.Subcontexts)
		names = append(names, clc.Name)
		paths = append(paths, clc.Host)
		names = append(names, subNames...)
		paths = append(paths, subPaths...)
	}
	return names, paths
}

// Class loader contexts that come from Make via JSON dexpreopt.config. JSON CLC representation is
// the same as Soong representation except that SDK versions and paths are represented with strings.
type jsonClassLoaderContext struct {
	Name        string
	Optional    bool
	Host        string
	Device      string
	Subcontexts []*jsonClassLoaderContext
}

// A map from SDK version (represented with a JSON string) to JSON CLCs.
type jsonClassLoaderContextMap map[string][]*jsonClassLoaderContext

// Convert JSON CLC map to Soong represenation.
func fromJsonClassLoaderContext(ctx android.PathContext, jClcMap jsonClassLoaderContextMap) ClassLoaderContextMap {
	clcMap := make(ClassLoaderContextMap)
	for sdkVerStr, clcs := range jClcMap {
		sdkVer, ok := strconv.Atoi(sdkVerStr)
		if ok != nil {
			if sdkVerStr == "any" {
				sdkVer = AnySdkVersion
			} else {
				android.ReportPathErrorf(ctx, "failed to parse SDK version in dexpreopt.config: '%s'", sdkVerStr)
			}
		}
		clcMap[sdkVer] = fromJsonClassLoaderContextRec(ctx, clcs)
	}
	return clcMap
}

// Recursive helper for fromJsonClassLoaderContext.
func fromJsonClassLoaderContextRec(ctx android.PathContext, jClcs []*jsonClassLoaderContext) []*ClassLoaderContext {
	clcs := make([]*ClassLoaderContext, 0, len(jClcs))
	for _, clc := range jClcs {
		clcs = append(clcs, &ClassLoaderContext{
			Name:        clc.Name,
			Optional:    clc.Optional,
			Host:        constructPath(ctx, clc.Host),
			Device:      clc.Device,
			Subcontexts: fromJsonClassLoaderContextRec(ctx, clc.Subcontexts),
		})
	}
	return clcs
}

// Convert Soong CLC map to JSON representation for Make.
func toJsonClassLoaderContext(clcMap ClassLoaderContextMap) jsonClassLoaderContextMap {
	jClcMap := make(jsonClassLoaderContextMap)
	for sdkVer, clcs := range clcMap {
		sdkVerStr := fmt.Sprintf("%d", sdkVer)
		if sdkVer == AnySdkVersion {
			sdkVerStr = "any"
		}
		jClcMap[sdkVerStr] = toJsonClassLoaderContextRec(clcs)
	}
	return jClcMap
}

// Recursive helper for toJsonClassLoaderContext.
func toJsonClassLoaderContextRec(clcs []*ClassLoaderContext) []*jsonClassLoaderContext {
	jClcs := make([]*jsonClassLoaderContext, len(clcs))
	for i, clc := range clcs {
		var host string
		if clc.Host == nil {
			// Defer build failure to when this CLC is actually used.
			host = fmt.Sprintf("implementation-jar-for-%s-is-not-available.jar", clc.Name)
		} else {
			host = clc.Host.String()
		}
		jClcs[i] = &jsonClassLoaderContext{
			Name:        clc.Name,
			Optional:    clc.Optional,
			Host:        host,
			Device:      clc.Device,
			Subcontexts: toJsonClassLoaderContextRec(clc.Subcontexts),
		}
	}
	return jClcs
}
