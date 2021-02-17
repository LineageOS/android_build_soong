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

package android

import "fmt"

var archVariants = map[ArchType][]string{
	Arm: {
		"armv7-a",
		"armv7-a-neon",
		"armv8-a",
		"armv8-2a",
		"cortex-a7",
		"cortex-a8",
		"cortex-a9",
		"cortex-a15",
		"cortex-a53",
		"cortex-a53-a57",
		"cortex-a55",
		"cortex-a72",
		"cortex-a73",
		"cortex-a75",
		"cortex-a76",
		"krait",
		"kryo",
		"kryo385",
		"exynos-m1",
		"exynos-m2",
	},
	Arm64: {
		"armv8_a",
		"armv8_a_branchprot",
		"armv8_2a",
		"armv8-2a-dotprod",
		"cortex-a53",
		"cortex-a55",
		"cortex-a72",
		"cortex-a73",
		"cortex-a75",
		"cortex-a76",
		"kryo",
		"kryo385",
		"exynos-m1",
		"exynos-m2",
	},
	X86: {
		"amberlake",
		"atom",
		"broadwell",
		"haswell",
		"icelake",
		"ivybridge",
		"kabylake",
		"sandybridge",
		"silvermont",
		"skylake",
		"stoneyridge",
		"tigerlake",
		"whiskeylake",
		"x86_64",
	},
	X86_64: {
		"amberlake",
		"broadwell",
		"haswell",
		"icelake",
		"ivybridge",
		"kabylake",
		"sandybridge",
		"silvermont",
		"skylake",
		"stoneyridge",
		"tigerlake",
		"whiskeylake",
	},
}

var archFeatures = map[ArchType][]string{
	Arm: {
		"neon",
	},
	Arm64: {
		"dotprod",
	},
	X86: {
		"ssse3",
		"sse4",
		"sse4_1",
		"sse4_2",
		"aes_ni",
		"avx",
		"avx2",
		"avx512",
		"popcnt",
		"movbe",
	},
	X86_64: {
		"ssse3",
		"sse4",
		"sse4_1",
		"sse4_2",
		"aes_ni",
		"avx",
		"avx2",
		"avx512",
		"popcnt",
	},
}

var archFeatureMap = map[ArchType]map[string][]string{
	Arm: {
		"armv7-a-neon": {
			"neon",
		},
		"armv8-a": {
			"neon",
		},
		"armv8-2a": {
			"neon",
		},
	},
	Arm64: {
		"armv8-2a-dotprod": {
			"dotprod",
		},
	},
	X86: {
		"amberlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"atom": {
			"ssse3",
			"movbe",
		},
		"broadwell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"haswell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
			"movbe",
		},
		"icelake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"ivybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
		},
		"kabylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"sandybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"popcnt",
		},
		"silvermont": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"popcnt",
			"movbe",
		},
		"skylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"stoneyridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"avx2",
			"popcnt",
			"movbe",
		},
		"tigerlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"whiskeylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"x86_64": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"popcnt",
		},
	},
	X86_64: {
		"amberlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"broadwell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"haswell": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
		},
		"icelake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"ivybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"popcnt",
		},
		"kabylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"aes_ni",
			"popcnt",
		},
		"sandybridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"popcnt",
		},
		"silvermont": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"popcnt",
		},
		"skylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"stoneyridge": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"aes_ni",
			"avx",
			"avx2",
			"popcnt",
		},
		"tigerlake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
		"whiskeylake": {
			"ssse3",
			"sse4",
			"sse4_1",
			"sse4_2",
			"avx",
			"avx2",
			"avx512",
			"aes_ni",
			"popcnt",
		},
	},
}

var defaultArchFeatureMap = map[OsType]map[ArchType][]string{}

// RegisterDefaultArchVariantFeatures is called by files that define Toolchains to specify the
// arch features that are available for the default arch variant.  It must be called from an
// init() function.
func RegisterDefaultArchVariantFeatures(os OsType, arch ArchType, features ...string) {
	checkCalledFromInit()

	for _, feature := range features {
		if !InList(feature, archFeatures[arch]) {
			panic(fmt.Errorf("Invalid feature %q for arch %q variant \"\"", feature, arch))
		}
	}

	if defaultArchFeatureMap[os] == nil {
		defaultArchFeatureMap[os] = make(map[ArchType][]string)
	}
	defaultArchFeatureMap[os][arch] = features
}
