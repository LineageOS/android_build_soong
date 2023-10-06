// Copyright 2022 Google Inc. All rights reserved.
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

type ApiSurface int

// TODO(b/246656800): Reconcile with android.SdkKind
const (
	// API surface provided by platform and mainline modules to other mainline modules
	ModuleLibApi ApiSurface = iota
	PublicApi               // Aka NDK
	VendorApi               // Aka LLNDK
)

func (a ApiSurface) String() string {
	switch a {
	case ModuleLibApi:
		return "module-libapi"
	case PublicApi:
		return "publicapi"
	case VendorApi:
		return "vendorapi"
	default:
		return "invalid"
	}
}
