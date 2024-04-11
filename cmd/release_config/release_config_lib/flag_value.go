// Copyright 2024 Google Inc. All rights reserved.
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

package release_config_lib

import (
	"android/soong/cmd/release_config/release_config_proto"
)

type FlagValue struct {
	// The path providing this value.
	path string

	// Protobuf
	proto release_config_proto.FlagValue
}

func FlagValueFactory(protoPath string) (fv *FlagValue) {
	fv = &FlagValue{path: protoPath}
	if protoPath != "" {
		LoadTextproto(protoPath, &fv.proto)
	}
	return fv
}

func MarshalValue(value *release_config_proto.Value) string {
	switch val := value.Val.(type) {
	case *release_config_proto.Value_UnspecifiedValue:
		// Value was never set.
		return ""
	case *release_config_proto.Value_StringValue:
		return val.StringValue
	case *release_config_proto.Value_BoolValue:
		if val.BoolValue {
			return "true"
		}
		// False ==> empty string
		return ""
	case *release_config_proto.Value_Obsolete:
		return " #OBSOLETE"
	default:
		// Flagged as error elsewhere, so return empty string here.
		return ""
	}
}
