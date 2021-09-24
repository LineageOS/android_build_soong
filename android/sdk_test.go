// Copyright (C) 2021 The Android Open Source Project
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

import "testing"

type testSdkRegisterable struct {
	name string
}

func (t *testSdkRegisterable) SdkPropertyName() string {
	return t.name
}

var _ sdkRegisterable = &testSdkRegisterable{}

func TestSdkRegistry(t *testing.T) {
	alpha := &testSdkRegisterable{"alpha"}
	beta := &testSdkRegisterable{"beta"}
	betaDup := &testSdkRegisterable{"beta"}

	// Make sure that an empty registry is empty.
	emptyRegistry := &sdkRegistry{}
	AssertDeepEquals(t, "emptyRegistry should be empty", ([]sdkRegisterable)(nil), emptyRegistry.registeredObjects())

	// Add beta to the empty registry to create another registry, check that it contains beta and make
	// sure that it does not affect the creating registry.
	registry1 := emptyRegistry.copyAndAppend(beta)
	AssertDeepEquals(t, "emptyRegistry should still be empty", ([]sdkRegisterable)(nil), emptyRegistry.registeredObjects())
	AssertDeepEquals(t, "registry1 should contain beta", []sdkRegisterable{beta}, registry1.registeredObjects())

	// Add alpha to the registry containing beta to create another registry, check that it contains
	// alpha,beta (in order) and make sure that it does not affect the creating registry.
	registry2 := registry1.copyAndAppend(alpha)
	AssertDeepEquals(t, "registry1 should still contain beta", []sdkRegisterable{beta}, registry1.registeredObjects())
	AssertDeepEquals(t, "registry2 should contain alpha,beta", []sdkRegisterable{alpha, beta}, registry2.registeredObjects())

	AssertPanicMessageContains(t, "duplicate beta should be detected", `"beta" already exists in ["alpha" "beta"]`, func() {
		registry2.copyAndAppend(betaDup)
	})
}
