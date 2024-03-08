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

package cc

import (
	"fmt"

	"android/soong/android"
)

// MinApiLevelForArch returns the ApiLevel for the Android version that
// first supported the architecture.
func MinApiForArch(ctx android.EarlyModuleContext,
	arch android.ArchType) android.ApiLevel {

	switch arch {
	case android.Arm, android.X86:
		return ctx.Config().MinSupportedSdkVersion()
	case android.Arm64, android.X86_64:
		return android.FirstLp64Version
	case android.Riscv64:
		apiLevel, err := android.ApiLevelFromUser(ctx, "VanillaIceCream")
		if err != nil {
			panic(err)
		}
		return apiLevel
	default:
		panic(fmt.Errorf("Unknown arch %q", arch))
	}
}

func nativeApiLevelFromUser(ctx android.BaseModuleContext,
	raw string) (android.ApiLevel, error) {

	min := MinApiForArch(ctx, ctx.Arch().ArchType)
	if raw == "minimum" {
		return min, nil
	}

	value, err := android.ApiLevelFromUser(ctx, raw)
	if err != nil {
		return android.NoneApiLevel, err
	}

	if value.LessThan(min) {
		return min, nil
	}

	return value, nil
}

func nativeApiLevelOrPanic(ctx android.BaseModuleContext,
	raw string) android.ApiLevel {
	value, err := nativeApiLevelFromUser(ctx, raw)
	if err != nil {
		panic(err.Error())
	}
	return value
}
