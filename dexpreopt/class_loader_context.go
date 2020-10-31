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
	"path/filepath"
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

const AnySdkVersion int = 9999 // should go last in class loader context

// LibraryPath contains paths to the library DEX jar on host and on device.
type LibraryPath struct {
	Host   android.Path
	Device string
}

// LibraryPaths is a map from library name to on-host and on-device paths to its DEX jar.
type LibraryPaths map[string]*LibraryPath

type classLoaderContext struct {
	// Library names
	Names []string

	// The class loader context using paths in the build.
	Host android.Paths

	// The class loader context using paths as they will be on the device.
	Target []string
}

// A map of class loader contexts for each SDK version.
// A map entry for "any" version contains libraries that are unconditionally added to class loader
// context. Map entries for existing versions contains libraries that were in the default classpath
// until that API version, and should be added to class loader context if and only if the
// targetSdkVersion in the manifest or APK is less than that API version.
type classLoaderContextMap map[int]*classLoaderContext

// Add a new library path to the map, unless a path for this library already exists.
// If necessary, check that the build and install paths exist.
func (libPaths LibraryPaths) addLibraryPath(ctx android.ModuleInstallPathContext, lib string,
	hostPath, installPath android.Path, strict bool) error {

	// If missing dependencies are allowed, the build shouldn't fail when a <uses-library> is
	// not found. However, this is likely to result is disabling dexpreopt, as it won't be
	// possible to construct class loader context without on-host and on-device library paths.
	strict = strict && !ctx.Config().AllowMissingDependencies()

	if hostPath == nil && strict {
		return fmt.Errorf("unknown build path to <uses-library> '%s'", lib)
	}

	if installPath == nil {
		if android.InList(lib, CompatUsesLibs) || android.InList(lib, OptionalCompatUsesLibs) {
			// Assume that compatibility libraries are installed in /system/framework.
			installPath = android.PathForModuleInstall(ctx, "framework", lib+".jar")
		} else if strict {
			return fmt.Errorf("unknown install path to <uses-library> '%s'", lib)
		}
	}

	// Add a library only if the build and install path to it is known.
	if _, present := libPaths[lib]; !present {
		var devicePath string
		if installPath != nil {
			devicePath = android.InstallPathToOnDevicePath(ctx, installPath.(android.InstallPath))
		} else {
			// For some stub libraries the only known thing is the name of their implementation
			// library, but the library itself is unavailable (missing or part of a prebuilt). In
			// such cases we still need to add the library to <uses-library> tags in the manifest,
			// but we cannot use if for dexpreopt.
			devicePath = UnknownInstallLibraryPath
		}
		libPaths[lib] = &LibraryPath{hostPath, devicePath}
	}
	return nil
}

// Wrapper around addLibraryPath that does error reporting.
func (libPaths LibraryPaths) addLibraryPathOrReportError(ctx android.ModuleInstallPathContext, lib string,
	hostPath, installPath android.Path, strict bool) {

	err := libPaths.addLibraryPath(ctx, lib, hostPath, installPath, strict)
	if err != nil {
		android.ReportPathErrorf(ctx, err.Error())
	}
}

// Add a new library path to the map. Enforce checks that the library paths exist.
func (libPaths LibraryPaths) AddLibraryPath(ctx android.ModuleInstallPathContext, lib string, hostPath, installPath android.Path) {
	libPaths.addLibraryPathOrReportError(ctx, lib, hostPath, installPath, true)
}

// Add a new library path to the map, if the library exists (name is not nil).
// Don't enforce checks that the library paths exist. Some libraries may be missing from the build,
// but their names still need to be added to <uses-library> tags in the manifest.
func (libPaths LibraryPaths) MaybeAddLibraryPath(ctx android.ModuleInstallPathContext, lib *string, hostPath, installPath android.Path) {
	if lib != nil {
		libPaths.addLibraryPathOrReportError(ctx, *lib, hostPath, installPath, false)
	}
}

// Add library paths from the second map to the first map (do not override existing entries).
func (libPaths LibraryPaths) AddLibraryPaths(otherPaths LibraryPaths) {
	for lib, path := range otherPaths {
		if _, present := libPaths[lib]; !present {
			libPaths[lib] = path
		}
	}
}

func (m classLoaderContextMap) getValue(sdkVer int) *classLoaderContext {
	if _, ok := m[sdkVer]; !ok {
		m[sdkVer] = &classLoaderContext{}
	}
	return m[sdkVer]
}

func (clc *classLoaderContext) addLib(lib string, hostPath android.Path, targetPath string) {
	clc.Names = append(clc.Names, lib)
	clc.Host = append(clc.Host, hostPath)
	clc.Target = append(clc.Target, targetPath)
}

func (m classLoaderContextMap) addLibs(ctx android.PathContext, sdkVer int, module *ModuleConfig,
	libs ...string) (bool, error) {

	clc := m.getValue(sdkVer)
	for _, lib := range libs {
		if p, ok := module.LibraryPaths[lib]; ok && p.Host != nil && p.Device != UnknownInstallLibraryPath {
			clc.addLib(lib, p.Host, p.Device)
		} else {
			if sdkVer == AnySdkVersion {
				// Fail the build if dexpreopt doesn't know paths to one of the <uses-library>
				// dependencies. In the future we may need to relax this and just disable dexpreopt.
				return false, fmt.Errorf("dexpreopt cannot find path for <uses-library> '%s'", lib)
			} else {
				// No error for compatibility libraries, as Soong doesn't know if they are needed
				// (this depends on the targetSdkVersion in the manifest).
				return false, nil
			}
		}
	}
	return true, nil
}

func (m classLoaderContextMap) addSystemServerLibs(sdkVer int, ctx android.PathContext, module *ModuleConfig, libs ...string) {
	clc := m.getValue(sdkVer)
	for _, lib := range libs {
		clc.addLib(lib, SystemServerDexJarHostPath(ctx, lib), filepath.Join("/system/framework", lib+".jar"))
	}
}

func (m classLoaderContextMap) usesLibs() []string {
	if clc, ok := m[AnySdkVersion]; ok {
		return clc.Names
	}
	return nil
}

// genClassLoaderContext generates host and target class loader context to be passed to the dex2oat
// command for the dexpreopted module. There are three possible cases:
//
// 1. System server jars. They have a special class loader context that includes other system
//    server jars.
//
// 2. Library jars or APKs which have precise list of their <uses-library> libs. Their class loader
//    context includes build and on-device paths to these libs. In some cases it may happen that
//    the path to a <uses-library> is unknown (e.g. the dexpreopted module may depend on stubs
//    library, whose implementation library is missing from the build altogether). In such case
//    dexpreopting with the <uses-library> is impossible, and dexpreopting without it is pointless,
//    as the runtime classpath won't match and the dexpreopted code will be discarded. Therefore in
//    such cases the function returns nil, which disables dexpreopt.
//
// 3. All other library jars or APKs for which the exact <uses-library> list is unknown. They use
//    the unsafe &-classpath workaround that means empty class loader context and absence of runtime
//    check that the class loader context provided by the PackageManager agrees with the stored
//    class loader context recorded in the .odex file.
//
func genClassLoaderContext(ctx android.PathContext, global *GlobalConfig, module *ModuleConfig) (*classLoaderContextMap, error) {
	classLoaderContexts := make(classLoaderContextMap)
	systemServerJars := NonUpdatableSystemServerJars(ctx, global)

	if jarIndex := android.IndexList(module.Name, systemServerJars); jarIndex >= 0 {
		// System server jars should be dexpreopted together: class loader context of each jar
		// should include all preceding jars on the system server classpath.
		classLoaderContexts.addSystemServerLibs(AnySdkVersion, ctx, module, systemServerJars[:jarIndex]...)

	} else if module.EnforceUsesLibraries {
		// Unconditional class loader context.
		usesLibs := append(copyOf(module.UsesLibraries), module.OptionalUsesLibraries...)
		if ok, err := classLoaderContexts.addLibs(ctx, AnySdkVersion, module, usesLibs...); !ok {
			return nil, err
		}

		// Conditional class loader context for API version < 28.
		const httpLegacy = "org.apache.http.legacy"
		if ok, err := classLoaderContexts.addLibs(ctx, 28, module, httpLegacy); !ok {
			return nil, err
		}

		// Conditional class loader context for API version < 29.
		usesLibs29 := []string{
			"android.hidl.base-V1.0-java",
			"android.hidl.manager-V1.0-java",
		}
		if ok, err := classLoaderContexts.addLibs(ctx, 29, module, usesLibs29...); !ok {
			return nil, err
		}

		// Conditional class loader context for API version < 30.
		if ok, err := classLoaderContexts.addLibs(ctx, 30, module, OptionalCompatUsesLibs30...); !ok {
			return nil, err
		}

	} else {
		// Pass special class loader context to skip the classpath and collision check.
		// This will get removed once LOCAL_USES_LIBRARIES is enforced.
		// Right now LOCAL_USES_LIBRARIES is opt in, for the case where it's not specified we still default
		// to the &.
	}

	fixConditionalClassLoaderContext(classLoaderContexts)

	return &classLoaderContexts, nil
}

// Now that the full unconditional context is known, reconstruct conditional context.
// Apply filters for individual libraries, mirroring what the PackageManager does when it
// constructs class loader context on device.
//
// TODO(b/132357300):
//   - remove android.hidl.manager and android.hidl.base unless the app is a system app.
//
func fixConditionalClassLoaderContext(clcMap classLoaderContextMap) {
	usesLibs := clcMap.usesLibs()

	for sdkVer, clc := range clcMap {
		if sdkVer == AnySdkVersion {
			continue
		}
		clcMap[sdkVer] = &classLoaderContext{}
		for i, lib := range clc.Names {
			if android.InList(lib, usesLibs) {
				// skip compatibility libraries that are already included in unconditional context
			} else if lib == AndroidTestMock && !android.InList("android.test.runner", usesLibs) {
				// android.test.mock is only needed as a compatibility library (in conditional class
				// loader context) if android.test.runner is used, otherwise skip it
			} else {
				clcMap[sdkVer].addLib(lib, clc.Host[i], clc.Target[i])
			}
		}
	}
}

// Return the class loader context as a string and a slice of build paths for all dependencies.
func computeClassLoaderContext(ctx android.PathContext, clcMap classLoaderContextMap) (clcStr string, paths android.Paths) {
	for _, ver := range android.SortedIntKeys(clcMap) {
		clc := clcMap.getValue(ver)

		clcLen := len(clc.Names)
		if clcLen != len(clc.Host) || clcLen != len(clc.Target) {
			android.ReportPathErrorf(ctx, "ill-formed class loader context")
		}

		var hostClc, targetClc []string
		var hostPaths android.Paths

		for i := 0; i < clcLen; i++ {
			hostStr := "PCL[" + clc.Host[i].String() + "]"
			targetStr := "PCL[" + clc.Target[i] + "]"

			hostClc = append(hostClc, hostStr)
			targetClc = append(targetClc, targetStr)
			hostPaths = append(hostPaths, clc.Host[i])
		}

		if hostPaths != nil {
			sdkVerStr := fmt.Sprintf("%d", ver)
			if ver == AnySdkVersion {
				sdkVerStr = "any" // a special keyword that means any SDK version
			}
			clcStr += fmt.Sprintf(" --host-context-for-sdk %s %s", sdkVerStr, strings.Join(hostClc, "#"))
			clcStr += fmt.Sprintf(" --target-context-for-sdk %s %s", sdkVerStr, strings.Join(targetClc, "#"))
			paths = append(paths, hostPaths...)
		}
	}

	return clcStr, paths
}

type jsonLibraryPath struct {
	Host   string
	Device string
}

type jsonLibraryPaths map[string]jsonLibraryPath

// convert JSON map of library paths to LibraryPaths
func constructLibraryPaths(ctx android.PathContext, paths jsonLibraryPaths) LibraryPaths {
	m := LibraryPaths{}
	for lib, path := range paths {
		m[lib] = &LibraryPath{
			constructPath(ctx, path.Host),
			path.Device,
		}
	}
	return m
}
