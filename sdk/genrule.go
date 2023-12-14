// Copyright 2023 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package sdk

import (
	"android/soong/android"
	"android/soong/genrule"
)

func init() {
	registerGenRuleBuildComponents(android.InitRegistrationContext)
}

func registerGenRuleBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("sdk_genrule", SdkGenruleFactory)
}

// sdk_genrule_host is a genrule that can depend on sdk and sdk_snapshot module types
//
// What this means is that it's a genrule with only the "common_os" variant.
// sdk modules have 3 variants: host, android, and common_os. The common_os one depends
// on the host/device ones and packages their result into a final snapshot zip.
// Genrules probably want access to this snapshot zip when they depend on an sdk module,
// which means they want to depend on the common_os variant and not the host/android
// variants.
func SdkGenruleFactory() android.Module {
	module := genrule.NewGenRule()

	android.InitCommonOSAndroidMultiTargetsArchModule(module, android.NeitherHostNorDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)

	return module
}
