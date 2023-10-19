// Copyright 2023 Google Inc. All rights reserved.
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
//
// Note: If you want to know how to use orderfile for your binary or shared
// library, you can go look at the README in toolchains/pgo-profiles/orderfiles

package cc

import (
	"fmt"

	"android/soong/android"
)

// Order files are text files containing symbols representing functions names.
// Linkers (lld) uses order files to layout functions in a specific order.
// These binaries with ordered symbols will reduce page faults and improve a program's launch time
// due to the efficient loading of symbols during a program’s cold-start.
var (
	// Add flags to ignore warnings about symbols not be found
	// or not allowed to be ordered
	orderfileOtherFlags = []string{
		"-Wl,--no-warn-symbol-ordering",
	}

	// Add folder projects for orderfiles
	globalOrderfileProjects = []string{
		"toolchain/pgo-profiles/orderfiles",
		"vendor/google_data/pgo_profile/orderfiles",
	}
)

var orderfileProjectsConfigKey = android.NewOnceKey("OrderfileProjects")

const orderfileProfileFlag = "-forder-file-instrumentation"
const orderfileUseFormat = "-Wl,--symbol-ordering-file=%s"

func getOrderfileProjects(config android.DeviceConfig) []string {
	return config.OnceStringSlice(orderfileProjectsConfigKey, func() []string {
		return globalOrderfileProjects
	})
}

func recordMissingOrderfile(ctx BaseModuleContext, missing string) {
	getNamedMapForConfig(ctx.Config(), modulesMissingProfileFileKey).Store(missing, true)
}

type OrderfileProperties struct {
	Orderfile struct {
		Instrumentation *bool
		Order_file_path *string `android:"arch_variant"`
		Load_order_file *bool   `android:"arch_variant"`
		// Additional compiler flags to use when building this module
		// for orderfile profiling.
		Cflags []string `android:"arch_variant"`
	} `android:"arch_variant"`

	ShouldProfileModule bool `blueprint:"mutated"`
	OrderfileLoad       bool `blueprint:"mutated"`
	OrderfileInstrLink  bool `blueprint:"mutated"`
}

type orderfile struct {
	Properties OrderfileProperties
}

func (props *OrderfileProperties) shouldInstrument() bool {
	return Bool(props.Orderfile.Instrumentation)
}

// ShouldLoadOrderfile returns true if we need to load the order file rather than
// profile the binary or shared library
func (props *OrderfileProperties) shouldLoadOrderfile() bool {
	return Bool(props.Orderfile.Load_order_file) && props.Orderfile.Order_file_path != nil
}

// orderfileEnabled returns true for binaries and shared libraries
// if instrument flag is set to true
func (orderfile *orderfile) orderfileEnabled() bool {
	return orderfile != nil && orderfile.Properties.shouldInstrument()
}

// orderfileLinkEnabled returns true for binaries and shared libraries
// if you should instrument dependencies
func (orderfile *orderfile) orderfileLinkEnabled() bool {
	return orderfile != nil && orderfile.Properties.OrderfileInstrLink
}

func (orderfile *orderfile) props() []interface{} {
	return []interface{}{&orderfile.Properties}
}

// Get the path to the order file by checking it is valid and not empty
func (props *OrderfileProperties) getOrderfile(ctx BaseModuleContext) android.OptionalPath {
	orderFile := *props.Orderfile.Order_file_path

	// Test if the order file is present in any of the Orderfile projects
	for _, profileProject := range getOrderfileProjects(ctx.DeviceConfig()) {
		path := android.ExistentPathForSource(ctx, profileProject, orderFile)
		if path.Valid() {
			return path
		}
	}

	// Record that this module's order file is absent
	missing := *props.Orderfile.Order_file_path + ":" + ctx.ModuleDir() + "/Android.bp:" + ctx.ModuleName()
	recordMissingOrderfile(ctx, missing)

	return android.OptionalPath{}
}

func (props *OrderfileProperties) addInstrumentationProfileGatherFlags(ctx ModuleContext, flags Flags) Flags {
	flags.Local.CFlags = append(flags.Local.CFlags, orderfileProfileFlag)
	flags.Local.CFlags = append(flags.Local.CFlags, "-mllvm -enable-order-file-instrumentation")
	flags.Local.CFlags = append(flags.Local.CFlags, props.Orderfile.Cflags...)
	flags.Local.LdFlags = append(flags.Local.LdFlags, orderfileProfileFlag)
	return flags
}

func (props *OrderfileProperties) loadOrderfileFlags(ctx ModuleContext, file string) []string {
	flags := []string{fmt.Sprintf(orderfileUseFormat, file)}
	flags = append(flags, orderfileOtherFlags...)
	return flags
}

func (props *OrderfileProperties) addLoadFlags(ctx ModuleContext, flags Flags) Flags {
	orderFile := props.getOrderfile(ctx)
	orderFilePath := orderFile.Path()
	loadFlags := props.loadOrderfileFlags(ctx, orderFilePath.String())

	flags.Local.LdFlags = append(flags.Local.LdFlags, loadFlags...)

	// Update CFlagsDeps and LdFlagsDeps so the module is rebuilt
	// if orderfile gets updated
	flags.CFlagsDeps = append(flags.CFlagsDeps, orderFilePath)
	flags.LdFlagsDeps = append(flags.LdFlagsDeps, orderFilePath)
	return flags
}

func (orderfile *orderfile) begin(ctx BaseModuleContext) {
	// Currently, we are not enabling orderfiles for host
	if ctx.Host() {
		return
	}

	// Currently, we are not enabling orderfiles to begin from static libraries
	if ctx.static() && !ctx.staticBinary() {
		return
	}

	if ctx.DeviceConfig().ClangCoverageEnabled() {
		return
	}

	// Checking if orderfile is enabled for this module
	if !orderfile.orderfileEnabled() {
		return
	}

	orderfile.Properties.OrderfileLoad = orderfile.Properties.shouldLoadOrderfile()
	orderfile.Properties.ShouldProfileModule = !orderfile.Properties.shouldLoadOrderfile()
	orderfile.Properties.OrderfileInstrLink = orderfile.orderfileEnabled() && !orderfile.Properties.shouldLoadOrderfile()
}

func (orderfile *orderfile) flags(ctx ModuleContext, flags Flags) Flags {
	props := orderfile.Properties
	// Add flags to load the orderfile using the path in its Android.bp
	if orderfile.Properties.OrderfileLoad {
		flags = props.addLoadFlags(ctx, flags)
		return flags
	}

	// Add flags to profile this module
	if props.ShouldProfileModule {
		flags = props.addInstrumentationProfileGatherFlags(ctx, flags)
		return flags
	}

	return flags
}

// Propagate profile orderfile flags down from binaries and shared libraries
// We do not allow propagation for load flags because the orderfile is specific
// to the module (binary / shared library)
func orderfileDepsMutator(mctx android.TopDownMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if !m.orderfile.orderfileLinkEnabled() {
			return
		}
		mctx.WalkDeps(func(dep android.
			Module, parent android.Module) bool {
			tag := mctx.OtherModuleDependencyTag(dep)
			libTag, isLibTag := tag.(libraryDependencyTag)

			// Do not recurse down non-static dependencies
			if isLibTag {
				if !libTag.static() {
					return false
				}
			} else {
				if tag != objDepTag && tag != reuseObjTag {
					return false
				}
			}

			if dep, ok := dep.(*Module); ok {
				if m.orderfile.Properties.OrderfileInstrLink {
					dep.orderfile.Properties.OrderfileInstrLink = true
				}
			}

			return true
		})
	}
}

// Create orderfile variants for modules that need them
func orderfileMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok && m.orderfile != nil {
		if !m.static() && m.orderfile.orderfileEnabled() {
			mctx.SetDependencyVariation("orderfile")
			return
		}

		variationNames := []string{""}
		if m.orderfile.Properties.OrderfileInstrLink {
			variationNames = append(variationNames, "orderfile")
		}

		if len(variationNames) > 1 {
			modules := mctx.CreateVariations(variationNames...)
			for i, name := range variationNames {
				if name == "" {
					continue
				}
				variation := modules[i].(*Module)
				variation.Properties.PreventInstall = true
				variation.Properties.HideFromMake = true
				variation.orderfile.Properties.ShouldProfileModule = true
				variation.orderfile.Properties.OrderfileLoad = false
			}
		}
	}
}
