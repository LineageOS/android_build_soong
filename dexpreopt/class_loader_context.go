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
	"fmt"
	"strconv"
	"strings"

	"android/soong/android"
)

// These libs are added as <uses-library> dependencies for apps if the targetSdkVersion in the
// app manifest is less than the specified version. This is needed because these libraries haven't
// existed prior to certain SDK version, but classes in them were in bootclasspath jars, etc.
// Some of the compatibility libraries are optional (their <uses-library> tag has "required=false"),
// so that if this library is missing this in not a build or run-time error.
var OrgApacheHttpLegacy = "org.apache.http.legacy"
var AndroidTestBase = "android.test.base"
var AndroidTestMock = "android.test.mock"
var AndroidHidlBase = "android.hidl.base-V1.0-java"
var AndroidHidlManager = "android.hidl.manager-V1.0-java"

var OptionalCompatUsesLibs28 = []string{
	OrgApacheHttpLegacy,
}
var OptionalCompatUsesLibs30 = []string{
	AndroidTestBase,
	AndroidTestMock,
}
var CompatUsesLibs29 = []string{
	AndroidHidlBase,
	AndroidHidlManager,
}
var OptionalCompatUsesLibs = append(android.CopyOf(OptionalCompatUsesLibs28), OptionalCompatUsesLibs30...)
var CompatUsesLibs = android.CopyOf(CompatUsesLibs29)

const UnknownInstallLibraryPath = "error"

// AnySdkVersion means that the class loader context is needed regardless of the targetSdkVersion
// of the app. The numeric value affects the key order in the map and, as a result, the order of
// arguments passed to construct_context.py (high value means that the unconditional context goes
// last). We use the converntional "current" SDK level (10000), but any big number would do as well.
const AnySdkVersion int = android.FutureApiLevelInt

// ClassLoaderContext is a tree of libraries used by the dexpreopted module with their dependencies.
// The context is used by dex2oat to compile the module and recorded in the AOT-compiled files, so
// that it can be checked agains the run-time class loader context on device. If there is a mismatch
// at runtime, AOT-compiled code is rejected.
type ClassLoaderContext struct {
	// The name of the library (same as the name of the module that contains it).
	Name string

	// On-host build path to the library dex file (used in dex2oat argument --class-loader-context).
	Host android.Path

	// On-device install path (used in dex2oat argument --stored-class-loader-context).
	Device string

	// Nested class loader subcontexts for dependencies.
	Subcontexts []*ClassLoaderContext
}

// ClassLoaderContextMap is a map from SDK version to a class loader context.
// There is a special entry with key AnySdkVersion that stores unconditional class loader context.
// Other entries store conditional contexts that should be added for some apps that have
// targetSdkVersion in the manifest lower than the key SDK version.
type ClassLoaderContextMap map[int][]*ClassLoaderContext

// Add class loader context for the given library to the map entry for the given SDK version.
func (clcMap ClassLoaderContextMap) addContext(ctx android.ModuleInstallPathContext, sdkVer int, lib string,
	hostPath, installPath android.Path, strict bool, nestedClcMap ClassLoaderContextMap) error {

	// If missing dependencies are allowed, the build shouldn't fail when a <uses-library> is
	// not found. However, this is likely to result is disabling dexpreopt, as it won't be
	// possible to construct class loader context without on-host and on-device library paths.
	strict = strict && !ctx.Config().AllowMissingDependencies()

	if hostPath == nil && strict {
		return fmt.Errorf("unknown build path to <uses-library> \"%s\"", lib)
	}

	devicePath := UnknownInstallLibraryPath
	if installPath == nil {
		if android.InList(lib, CompatUsesLibs) || android.InList(lib, OptionalCompatUsesLibs) {
			// Assume that compatibility libraries are installed in /system/framework.
			installPath = android.PathForModuleInstall(ctx, "framework", lib+".jar")
		} else if strict {
			return fmt.Errorf("unknown install path to <uses-library> \"%s\"", lib)
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
			clcStr, _ := ComputeClassLoaderContext(nestedClcMap)
			return fmt.Errorf("nested class loader context shouldn't have conditional part: %s", clcStr)
		}
	}
	subcontexts := nestedClcMap[AnySdkVersion]

	// If the library with this name is already present as one of the unconditional top-level
	// components, do not re-add it.
	for _, clc := range clcMap[sdkVer] {
		if clc.Name == lib {
			return nil
		}
	}

	clcMap[sdkVer] = append(clcMap[sdkVer], &ClassLoaderContext{
		Name:        lib,
		Host:        hostPath,
		Device:      devicePath,
		Subcontexts: subcontexts,
	})
	return nil
}

// Wrapper around addContext that reports errors.
func (clcMap ClassLoaderContextMap) addContextOrReportError(ctx android.ModuleInstallPathContext, sdkVer int, lib string,
	hostPath, installPath android.Path, strict bool, nestedClcMap ClassLoaderContextMap) {

	err := clcMap.addContext(ctx, sdkVer, lib, hostPath, installPath, strict, nestedClcMap)
	if err != nil {
		ctx.ModuleErrorf(err.Error())
	}
}

// Add class loader context. Fail on unknown build/install paths.
func (clcMap ClassLoaderContextMap) AddContext(ctx android.ModuleInstallPathContext, lib string,
	hostPath, installPath android.Path) {

	clcMap.addContextOrReportError(ctx, AnySdkVersion, lib, hostPath, installPath, true, nil)
}

// Add class loader context if the library exists. Don't fail on unknown build/install paths.
func (clcMap ClassLoaderContextMap) MaybeAddContext(ctx android.ModuleInstallPathContext, lib *string,
	hostPath, installPath android.Path) {

	if lib != nil {
		clcMap.addContextOrReportError(ctx, AnySdkVersion, *lib, hostPath, installPath, false, nil)
	}
}

// Add class loader context for the given SDK version. Fail on unknown build/install paths.
func (clcMap ClassLoaderContextMap) AddContextForSdk(ctx android.ModuleInstallPathContext, sdkVer int,
	lib string, hostPath, installPath android.Path, nestedClcMap ClassLoaderContextMap) {

	clcMap.addContextOrReportError(ctx, sdkVer, lib, hostPath, installPath, true, nestedClcMap)
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
func (clcMap ClassLoaderContextMap) UsesLibs() (ulibs []string) {
	if clcMap != nil {
		clcs := clcMap[AnySdkVersion]
		ulibs = make([]string, 0, len(clcs))
		for _, clc := range clcs {
			ulibs = append(ulibs, clc.Name)
		}
	}
	return ulibs
}

// Now that the full unconditional context is known, reconstruct conditional context.
// Apply filters for individual libraries, mirroring what the PackageManager does when it
// constructs class loader context on device.
//
// TODO(b/132357300): remove "android.hidl.manager" and "android.hidl.base" for non-system apps.
//
func fixClassLoaderContext(clcMap ClassLoaderContextMap) {
	usesLibs := clcMap.UsesLibs()

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

func validateClassLoaderContextRec(sdkVer int, clcs []*ClassLoaderContext) (bool, error) {
	for _, clc := range clcs {
		if clc.Host == nil || clc.Device == UnknownInstallLibraryPath {
			if sdkVer == AnySdkVersion {
				// Return error if dexpreopt doesn't know paths to one of the <uses-library>
				// dependencies. In the future we may need to relax this and just disable dexpreopt.
				return false, fmt.Errorf("invalid path for <uses-library> \"%s\"", clc.Name)
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

// Return the class loader context as a string, and a slice of build paths for all dependencies.
// Perform a depth-first preorder traversal of the class loader context tree for each SDK version.
// Return the resulting string and a slice of on-host build paths to all library dependencies.
func ComputeClassLoaderContext(clcMap ClassLoaderContextMap) (clcStr string, paths android.Paths) {
	for _, sdkVer := range android.SortedIntKeys(clcMap) { // determinisitc traversal order
		sdkVerStr := fmt.Sprintf("%d", sdkVer)
		if sdkVer == AnySdkVersion {
			sdkVerStr = "any" // a special keyword that means any SDK version
		}
		hostClc, targetClc, hostPaths := computeClassLoaderContextRec(clcMap[sdkVer])
		if hostPaths != nil {
			clcStr += fmt.Sprintf(" --host-context-for-sdk %s %s", sdkVerStr, hostClc)
			clcStr += fmt.Sprintf(" --target-context-for-sdk %s %s", sdkVerStr, targetClc)
		}
		paths = append(paths, hostPaths...)
	}
	return clcStr, android.FirstUniquePaths(paths)
}

func computeClassLoaderContextRec(clcs []*ClassLoaderContext) (string, string, android.Paths) {
	var paths android.Paths
	var clcsHost, clcsTarget []string

	for _, clc := range clcs {
		subClcHost, subClcTarget, subPaths := computeClassLoaderContextRec(clc.Subcontexts)
		if subPaths != nil {
			subClcHost = "{" + subClcHost + "}"
			subClcTarget = "{" + subClcTarget + "}"
		}

		clcsHost = append(clcsHost, "PCL["+clc.Host.String()+"]"+subClcHost)
		clcsTarget = append(clcsTarget, "PCL["+clc.Device+"]"+subClcTarget)

		paths = append(paths, clc.Host)
		paths = append(paths, subPaths...)
	}

	clcHost := strings.Join(clcsHost, "#")
	clcTarget := strings.Join(clcsTarget, "#")

	return clcHost, clcTarget, paths
}

// Paths to a <uses-library> on host and on device.
type jsonLibraryPath struct {
	Host   string
	Device string
}

// Class loader contexts that come from Make (via JSON dexpreopt.config) files have simpler
// structure than Soong class loader contexts: they are flat maps from a <uses-library> name to its
// on-host and on-device paths. There are no nested subcontexts. It is a limitation of the current
// Make implementation.
type jsonClassLoaderContext map[string]jsonLibraryPath

// A map from SDK version (represented with a JSON string) to JSON class loader context.
type jsonClassLoaderContextMap map[string]jsonClassLoaderContext

// Convert JSON class loader context map to ClassLoaderContextMap.
func fromJsonClassLoaderContext(ctx android.PathContext, jClcMap jsonClassLoaderContextMap) ClassLoaderContextMap {
	clcMap := make(ClassLoaderContextMap)
	for sdkVerStr, clc := range jClcMap {
		sdkVer, ok := strconv.Atoi(sdkVerStr)
		if ok != nil {
			if sdkVerStr == "any" {
				sdkVer = AnySdkVersion
			} else {
				android.ReportPathErrorf(ctx, "failed to parse SDK version in dexpreopt.config: '%s'", sdkVerStr)
			}
		}
		for lib, path := range clc {
			clcMap[sdkVer] = append(clcMap[sdkVer], &ClassLoaderContext{
				Name:        lib,
				Host:        constructPath(ctx, path.Host),
				Device:      path.Device,
				Subcontexts: nil,
			})
		}
	}
	return clcMap
}
