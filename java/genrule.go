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

package java

import (
	"android/soong/android"
	"android/soong/genrule"
)

func init() {
	RegisterGenRuleBuildComponents(android.InitRegistrationContext)
}

func RegisterGenRuleBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_genrule", GenRuleFactory)
	ctx.RegisterModuleType("java_genrule_host", GenRuleFactoryHost)
}

// java_genrule is a genrule that can depend on other java_* objects.
//
// By default a java_genrule has a single variant that will run against the device variant of its dependencies and
// produce an output that can be used as an input to a device java rule.
//
// Specifying `host_supported: true` will produce two variants, one that uses device dependencies and one that uses
// host dependencies.  Each variant will run the command.
//
// Use a java_genrule instead of a genrule when it needs to depend on or be depended on by other java modules, unless
// the dependency is for a generated source file.
//
// Examples:
//
// Use a java_genrule to package generated java resources:
//
//	java_genrule {
//	    name: "generated_resources",
//	    tools: [
//	        "generator",
//	        "soong_zip",
//	    ],
//	    srcs: ["generator_inputs/**/*"],
//	    out: ["generated_android_icu4j_resources.jar"],
//	    cmd: "$(location generator) $(in) -o $(genDir) " +
//	        "&& $(location soong_zip) -o $(out) -C $(genDir)/res -D $(genDir)/res",
//	}
//
//	java_library {
//	    name: "lib_with_generated_resources",
//	    srcs: ["src/**/*.java"],
//	    static_libs: ["generated_resources"],
//	}
func GenRuleFactory() android.Module {
	module := genrule.NewGenRule()

	android.InitAndroidArchModule(module, android.HostAndDeviceSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)

	return module
}

// java_genrule_host is a genrule that can depend on other java_* objects.
//
// A java_genrule_host has a single variant that will run against the host variant of its dependencies and
// produce an output that can be used as an input to a host java rule.
func GenRuleFactoryHost() android.Module {
	module := genrule.NewGenRule()

	android.InitAndroidArchModule(module, android.HostSupported, android.MultilibCommon)
	android.InitDefaultableModule(module)

	return module
}
