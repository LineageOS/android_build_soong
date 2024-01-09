// Copyright 2017 Google Inc. All rights reserved.
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

package cc

import (
	"android/soong/android"
)

type kernelHeadersDecorator struct {
	*libraryDecorator
}

func (stub *kernelHeadersDecorator) link(ctx ModuleContext, flags Flags, deps PathDeps, objs Objects) android.Path {
	if ctx.Device() {
		f := &stub.libraryDecorator.flagExporter
		f.reexportSystemDirs(android.PathsForSource(ctx, ctx.DeviceConfig().DeviceKernelHeaderDirs())...)
		f.setProvider(ctx)
	}
	return stub.libraryDecorator.linkStatic(ctx, flags, deps, objs)
}

// kernel_headers retrieves the list of kernel headers directories from
// TARGET_BOARD_KERNEL_HEADERS and TARGET_PRODUCT_KERNEL_HEADERS variables in
// a makefile for compilation. See
// https://android.googlesource.com/platform/build/+/main/core/config.mk
// for more details on them.
func kernelHeadersFactory() android.Module {
	module, library := NewLibrary(android.HostAndDeviceSupported)
	library.HeaderOnly()

	stub := &kernelHeadersDecorator{
		libraryDecorator: library,
	}

	module.linker = stub

	return module.Init()
}

func init() {
	android.RegisterModuleType("kernel_headers", kernelHeadersFactory)
}
