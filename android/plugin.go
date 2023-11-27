// Copyright 2022 Google Inc. All rights reserved.
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
	"encoding/json"
	"os"
	"strings"

	"github.com/google/blueprint"
)

func init() {
	RegisterPluginSingletonBuildComponents(InitRegistrationContext)
}

func RegisterPluginSingletonBuildComponents(ctx RegistrationContext) {
	ctx.RegisterParallelSingletonType("plugins", pluginSingletonFactory)
}

// pluginSingleton is a singleton to handle allowlisting of the final Android-<product_name>.mk file
// output.
func pluginSingletonFactory() Singleton {
	return &pluginSingleton{}
}

type pluginSingleton struct{}

var allowedPluginsByName = map[string]bool{
	"aidl-soong-rules":                       true,
	"arm_compute_library_nn_driver":          true,
	"cuttlefish-soong-rules":                 true,
	"gki-soong-rules":                        true,
	"hidl-soong-rules":                       true,
	"kernel-config-soong-rules":              true,
	"soong-angle-codegen":                    true,
	"soong-api":                              true,
	"soong-art":                              true,
	"soong-ca-certificates":                  true,
	"soong-ca-certificates-apex":             true,
	"soong-clang":                            true,
	"soong-clang-prebuilts":                  true,
	"soong-csuite":                           true,
	"soong-fluoride":                         true,
	"soong-fs_config":                        true,
	"soong-icu":                              true,
	"soong-java-config-error_prone":          true,
	"soong-libchrome":                        true,
	"soong-llvm":                             true,
	"soong-robolectric":                      true,
	"soong-rust-prebuilts":                   true,
	"soong-selinux":                          true,
	"soong-wayland-protocol-codegen":         true,
	"treble_report_app":                      true,
	"treble_report_local":                    true,
	"treble_report_module":                   true,
	"vintf-compatibility-matrix-soong-rules": true,
	"xsdc-soong-rules":                       true,
}

var internalPluginsPaths = []string{
	"vendor/google/build/soong/internal_plugins.json",
}

type pluginProvider interface {
	IsPluginFor(string) bool
}

func maybeAddInternalPluginsToAllowlist(ctx SingletonContext) {
	for _, internalPluginsPath := range internalPluginsPaths {
		if path := ExistentPathForSource(ctx, internalPluginsPath); path.Valid() {
			ctx.AddNinjaFileDeps(path.String())
			absPath := absolutePath(path.String())
			var moreAllowed map[string]bool
			data, err := os.ReadFile(absPath)
			if err != nil {
				ctx.Errorf("Failed to open internal plugins path %q %q", internalPluginsPath, err)
			}
			if err := json.Unmarshal(data, &moreAllowed); err != nil {
				ctx.Errorf("Internal plugins file %q did not parse correctly: %q", data, err)
			}
			for k, v := range moreAllowed {
				allowedPluginsByName[k] = v
			}
		}
	}
}

func (p *pluginSingleton) GenerateBuildActions(ctx SingletonContext) {
	for _, p := range ctx.DeviceConfig().BuildBrokenPluginValidation() {
		allowedPluginsByName[p] = true
	}
	maybeAddInternalPluginsToAllowlist(ctx)

	disallowedPlugins := map[string]bool{}
	ctx.VisitAllModulesBlueprint(func(module blueprint.Module) {
		if ctx.ModuleType(module) != "bootstrap_go_package" {
			return
		}

		p, ok := module.(pluginProvider)
		if !ok || !p.IsPluginFor("soong_build") {
			return
		}

		name := ctx.ModuleName(module)
		if _, ok := allowedPluginsByName[name]; ok {
			return
		}

		dir := ctx.ModuleDir(module)

		// allow use of plugins within Soong to not allowlist everything
		if strings.HasPrefix(dir, "build/soong") {
			return
		}

		// allow third party users outside of external to create new plugins, i.e. non-google paths
		// under vendor or hardware
		if !strings.HasPrefix(dir, "external/") && IsThirdPartyPath(dir) {
			return
		}
		disallowedPlugins[name] = true
	})
	if len(disallowedPlugins) > 0 {
		ctx.Errorf("New plugins are not supported; however %q were found. Please reach out to the build team or use BUILD_BROKEN_PLUGIN_VALIDATION (see Changes.md for more info).", SortedStringKeys(disallowedPlugins))
	}
}
