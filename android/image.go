// Copyright 2019 Google Inc. All rights reserved.
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

// ImageInterface is implemented by modules that need to be split by the imageMutator.
type ImageInterface interface {
	// ImageMutatorBegin is called before any other method in the ImageInterface.
	ImageMutatorBegin(ctx BaseModuleContext)

	// CoreVariantNeeded should return true if the module needs a core variant (installed on the system image).
	CoreVariantNeeded(ctx BaseModuleContext) bool

	// RamdiskVariantNeeded should return true if the module needs a ramdisk variant (installed on the
	// ramdisk partition).
	RamdiskVariantNeeded(ctx BaseModuleContext) bool

	// RecoveryVariantNeeded should return true if the module needs a recovery variant (installed on the
	// recovery partition).
	RecoveryVariantNeeded(ctx BaseModuleContext) bool

	// ExtraImageVariations should return a list of the additional variations needed for the module.  After the
	// variants are created the SetImageVariation method will be called on each newly created variant with the
	// its variation.
	ExtraImageVariations(ctx BaseModuleContext) []string

	// SetImageVariation will be passed a newly created recovery variant of the module.  ModuleBase implements
	// SetImageVariation, most module types will not need to override it, and those that do must call the
	// overridden method.  Implementors of SetImageVariation must be careful to modify the module argument
	// and not the receiver.
	SetImageVariation(ctx BaseModuleContext, variation string, module Module)
}

const (
	// CoreVariation is the variant used for framework-private libraries, or
	// SDK libraries. (which framework-private libraries can use), which
	// will be installed to the system image.
	CoreVariation string = ""

	// RecoveryVariation means a module to be installed to recovery image.
	RecoveryVariation string = "recovery"

	// RamdiskVariation means a module to be installed to ramdisk image.
	RamdiskVariation string = "ramdisk"
)

// imageMutator creates variants for modules that implement the ImageInterface that
// allow them to build differently for each partition (recovery, core, vendor, etc.).
func imageMutator(ctx BottomUpMutatorContext) {
	if ctx.Os() != Android {
		return
	}

	if m, ok := ctx.Module().(ImageInterface); ok {
		m.ImageMutatorBegin(ctx)

		var variations []string

		if m.CoreVariantNeeded(ctx) {
			variations = append(variations, CoreVariation)
		}
		if m.RamdiskVariantNeeded(ctx) {
			variations = append(variations, RamdiskVariation)
		}
		if m.RecoveryVariantNeeded(ctx) {
			variations = append(variations, RecoveryVariation)
		}

		extraVariations := m.ExtraImageVariations(ctx)
		variations = append(variations, extraVariations...)

		if len(variations) == 0 {
			return
		}

		mod := ctx.CreateVariations(variations...)
		for i, v := range variations {
			mod[i].base().setImageVariation(v)
			m.SetImageVariation(ctx, v, mod[i])
		}
	}
}
