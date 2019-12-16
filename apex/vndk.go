// Copyright (C) 2019 The Android Open Source Project
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

package apex

import (
	"path/filepath"
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/cc"

	"github.com/google/blueprint/proptools"
)

const (
	vndkApexName       = "com.android.vndk"
	vndkApexNamePrefix = vndkApexName + ".v"
)

// apex_vndk creates a special variant of apex modules which contains only VNDK libraries.
// If `vndk_version` is specified, the VNDK libraries of the specified VNDK version are gathered automatically.
// If not specified, then the "current" versions are gathered.
func vndkApexBundleFactory() android.Module {
	bundle := newApexBundle()
	bundle.vndkApex = true
	bundle.AddProperties(&bundle.vndkProperties)
	android.AddLoadHook(bundle, func(ctx android.LoadHookContext) {
		ctx.AppendProperties(&struct {
			Compile_multilib *string
		}{
			proptools.StringPtr("both"),
		})
	})
	return bundle
}

func (a *apexBundle) vndkVersion(config android.DeviceConfig) string {
	vndkVersion := proptools.StringDefault(a.vndkProperties.Vndk_version, "current")
	if vndkVersion == "current" {
		vndkVersion = config.PlatformVndkVersion()
	}
	return vndkVersion
}

type apexVndkProperties struct {
	// Indicates VNDK version of which this VNDK APEX bundles VNDK libs. Default is Platform VNDK Version.
	Vndk_version *string
}

var (
	vndkApexListKey   = android.NewOnceKey("vndkApexList")
	vndkApexListMutex sync.Mutex
)

func vndkApexList(config android.Config) map[string]string {
	return config.Once(vndkApexListKey, func() interface{} {
		return map[string]string{}
	}).(map[string]string)
}

func apexVndkMutator(mctx android.TopDownMutatorContext) {
	if ab, ok := mctx.Module().(*apexBundle); ok && ab.vndkApex {
		if ab.IsNativeBridgeSupported() {
			mctx.PropertyErrorf("native_bridge_supported", "%q doesn't support native bridge binary.", mctx.ModuleType())
		}

		vndkVersion := ab.vndkVersion(mctx.DeviceConfig())
		// Ensure VNDK APEX mount point is formatted as com.android.vndk.v###
		ab.properties.Apex_name = proptools.StringPtr(vndkApexNamePrefix + vndkVersion)

		// vndk_version should be unique
		vndkApexListMutex.Lock()
		defer vndkApexListMutex.Unlock()
		vndkApexList := vndkApexList(mctx.Config())
		if other, ok := vndkApexList[vndkVersion]; ok {
			mctx.PropertyErrorf("vndk_version", "%v is already defined in %q", vndkVersion, other)
		}
		vndkApexList[vndkVersion] = mctx.ModuleName()
	}
}

func apexVndkDepsMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*cc.Module); ok && cc.IsForVndkApex(mctx, m) {
		vndkVersion := m.VndkVersion()
		vndkApexList := vndkApexList(mctx.Config())
		if vndkApex, ok := vndkApexList[vndkVersion]; ok {
			mctx.AddReverseDependency(mctx.Module(), sharedLibTag, vndkApex)
		}
	} else if a, ok := mctx.Module().(*apexBundle); ok && a.vndkApex {
		vndkVersion := proptools.StringDefault(a.vndkProperties.Vndk_version, "current")
		mctx.AddDependency(mctx.Module(), prebuiltTag, cc.VndkLibrariesTxtModules(vndkVersion)...)
	}
}

func makeCompatSymlinks(apexName string, ctx android.ModuleContext) (symlinks []string) {
	// small helper to add symlink commands
	addSymlink := func(target, dir, linkName string) {
		outDir := filepath.Join("$(PRODUCT_OUT)", dir)
		link := filepath.Join(outDir, linkName)
		symlinks = append(symlinks, "mkdir -p "+outDir+" && rm -rf "+link+" && ln -sf "+target+" "+link)
	}

	// TODO(b/142911355): [VNDK APEX] Fix hard-coded references to /system/lib/vndk
	// When all hard-coded references are fixed, remove symbolic links
	// Note that  we should keep following symlinks for older VNDKs (<=29)
	// Since prebuilt vndk libs still depend on system/lib/vndk path
	if strings.HasPrefix(apexName, vndkApexNamePrefix) {
		// the name of vndk apex is formatted "com.android.vndk.v" + version
		vndkVersion := strings.TrimPrefix(apexName, vndkApexNamePrefix)
		if ctx.Config().Android64() {
			addSymlink("/apex/"+apexName+"/lib64", "/system/lib64", "vndk-sp-"+vndkVersion)
			addSymlink("/apex/"+apexName+"/lib64", "/system/lib64", "vndk-"+vndkVersion)
		}
		if !ctx.Config().Android64() || ctx.DeviceConfig().DeviceSecondaryArch() != "" {
			addSymlink("/apex/"+apexName+"/lib", "/system/lib", "vndk-sp-"+vndkVersion)
			addSymlink("/apex/"+apexName+"/lib", "/system/lib", "vndk-"+vndkVersion)
		}
	}
	return
}
