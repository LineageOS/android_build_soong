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

package bp2build

const (
	// The default `load` preamble for every generated queryview BUILD file.
	soongModuleLoad = `package(default_visibility = ["//visibility:public"])
load("//build/bazel/queryview_rules:soong_module.bzl", "soong_module")

`

	// A macro call in the BUILD file representing a Soong module, with space
	// for expanding more attributes.
	soongModuleTarget = `soong_module(
    name = "%s",
    module_name = "%s",
    module_type = "%s",
    module_variant = "%s",
    module_deps = %s,
%s)`

	bazelTarget = `%s(
    name = "%s",
%s)`

	// A simple provider to mark and differentiate Soong module rule shims from
	// regular Bazel rules. Every Soong module rule shim returns a
	// SoongModuleInfo provider, and can only depend on rules returning
	// SoongModuleInfo in the `module_deps` attribute.
	providersBzl = `SoongModuleInfo = provider(
    fields = {
        "name": "Name of module",
        "type": "Type of module",
        "variant": "Variant of module",
    },
)
`

	// The soong_module rule implementation in a .bzl file.
	soongModuleBzl = `
%s

load("//build/bazel/queryview_rules:providers.bzl", "SoongModuleInfo")

def _generic_soong_module_impl(ctx):
    return [
        SoongModuleInfo(
            name = ctx.attr.module_name,
            type = ctx.attr.module_type,
            variant = ctx.attr.module_variant,
        ),
    ]

generic_soong_module = rule(
    implementation = _generic_soong_module_impl,
    attrs = {
        "module_name": attr.string(mandatory = True),
        "module_type": attr.string(mandatory = True),
        "module_variant": attr.string(),
        "module_deps": attr.label_list(providers = [SoongModuleInfo]),
    },
)

soong_module_rule_map = {
%s}

_SUPPORTED_TYPES = ["bool", "int", "string"]

def _is_supported_type(value):
    if type(value) in _SUPPORTED_TYPES:
        return True
    elif type(value) == "list":
        supported = True
        for v in value:
            supported = supported and type(v) in _SUPPORTED_TYPES
        return supported
    else:
        return False

# soong_module is a macro that supports arbitrary kwargs, and uses module_type to
# expand to the right underlying shim.
def soong_module(name, module_type, **kwargs):
    soong_module_rule = soong_module_rule_map.get(module_type)

    if soong_module_rule == None:
        # This module type does not have an existing rule to map to, so use the
        # generic_soong_module rule instead.
        generic_soong_module(
            name = name,
            module_type = module_type,
            module_name = kwargs.pop("module_name", ""),
            module_variant = kwargs.pop("module_variant", ""),
            module_deps = kwargs.pop("module_deps", []),
        )
    else:
        supported_kwargs = dict()
        for key, value in kwargs.items():
            if _is_supported_type(value):
                supported_kwargs[key] = value
        soong_module_rule(
            name = name,
            **supported_kwargs,
        )
`

	// A rule shim for representing a Soong module type and its properties.
	moduleRuleShim = `
def _%[1]s_impl(ctx):
    return [SoongModuleInfo()]

%[1]s = rule(
    implementation = _%[1]s_impl,
    attrs = %[2]s
)
`
)
