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

package android

import (
	"testing"
)

func TestOverrideConfiguredJarLocationFor(t *testing.T) {
	cfg := NullConfig("", "")

	cfg.productVariables = ProductVariables{
		ConfiguredJarLocationOverrides: []string{
			"platform:libfoo-old:com.android.foo:libfoo-new",
			"com.android.bar:libbar-old:platform:libbar-new",
		},
	}

	apex, jar := OverrideConfiguredJarLocationFor(cfg, "platform", "libfoo-old")
	AssertStringEquals(t, "", "com.android.foo", apex)
	AssertStringEquals(t, "", "libfoo-new", jar)

	apex, jar = OverrideConfiguredJarLocationFor(cfg, "platform", "libbar-old")
	AssertStringEquals(t, "", "platform", apex)
	AssertStringEquals(t, "", "libbar-old", jar)

	apex, jar = OverrideConfiguredJarLocationFor(cfg, "com.android.bar", "libbar-old")
	AssertStringEquals(t, "", "platform", apex)
	AssertStringEquals(t, "", "libbar-new", jar)

	apex, jar = OverrideConfiguredJarLocationFor(cfg, "platform", "libbar-old")
	AssertStringEquals(t, "", "platform", apex)
	AssertStringEquals(t, "", "libbar-old", jar)
}
