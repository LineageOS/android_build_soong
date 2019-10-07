// Copyright (C) 2019 The LineageOS Project
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
	"strings"
)

// Keys are bootjar name, value is whether or not
// we've marked a module as being a provider for it.
var jarMap map[string]bool

func init() {
	PreArchMutators(RegisterBootJarMutators)
}

// Note: registration function is structured this way so that it can be included
// from soong module tests.
func RegisterBootJarMutators(ctx RegisterMutatorsContext) {
	// Note: can't use Parallel() since we touch global jarMap
	ctx.TopDown("bootjar_exportednamespace", bootJarMutatorExportedNamespace)
	ctx.TopDown("bootjar_anynamespace", bootJarMutatorAnyNamespace)
}

func mutatorInit(mctx TopDownMutatorContext) {
	// Did we init already ?
	if jarMap != nil {
		return
	}

	jarMap = make(map[string]bool)
	for _, moduleName := range mctx.Config().BootJars() {
		jarMap[moduleName] = false
	}
}

// Mark modules in soong exported namespace as providing a boot jar.
func bootJarMutatorExportedNamespace(mctx TopDownMutatorContext) {
	bootJarMutator(mctx, true)
}

// Mark modules in any namespace (incl root) as providing a boot jar.
func bootJarMutatorAnyNamespace(mctx TopDownMutatorContext) {
	bootJarMutator(mctx, false)
}

func bootJarMutator(mctx TopDownMutatorContext, requireExportedNamespace bool) {
	mutatorInit(mctx)

	module, ok := mctx.Module().(Module)
	if !ok {
		// Not a proper module
		return
	}

	// Does this module produce a dex jar ?
	if _, ok := module.(interface{ DexJar() Path }); !ok {
		// No
		return
	}

	// If jarMap is empty we must be running in a test so
	// set boot jar provide to true for all modules.
	if len(jarMap) == 0 {
		module.base().commonProperties.BootJarProvider = true
		return
	}

	name := mctx.ModuleName()
	dir := mctx.ModuleDir()

	// Special treatment for hiddenapi modules - create extra
	// jarMap entries if needed.
	baseName := strings.TrimSuffix(name, "-hiddenapi")
	if name != baseName {
		_, baseIsBootJar := jarMap[baseName]
		_, alreadyExists := jarMap[name]
		if baseIsBootJar && !alreadyExists {
			// This is a hidden api module whose base name exists in the boot jar list
			// and we've not visited it before.  Create a map entry for it.
			jarMap[name] = false
		}
	}

	// Does this module match the name of a boot jar ?
	if found, exists := jarMap[name]; !exists || found {
		// No
		return
	}

	if requireExportedNamespace {
		for _, n := range mctx.Config().ExportedNamespaces() {
			if strings.HasPrefix(dir, n) {
				jarMap[name] = true
				module.base().commonProperties.BootJarProvider = true
				break
			}
		}
	} else {
		jarMap[name] = true
		module.base().commonProperties.BootJarProvider = true
	}

	return
}
