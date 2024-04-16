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
	"fmt"

	"android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

type FlagArtifact struct {
	FlagDeclaration *release_config_proto.FlagDeclaration

	// The index of the config directory where this flag was declared.
	// Flag values cannot be set in a location with a lower index.
	DeclarationIndex int

	Traces []*release_config_proto.Tracepoint

	// Assigned value
	Value *release_config_proto.Value
}

// Key is flag name.
type FlagArtifacts map[string]*FlagArtifact

func (src *FlagArtifact) Clone() *FlagArtifact {
	value := &release_config_proto.Value{}
	proto.Merge(value, src.Value)
	return &FlagArtifact{
		FlagDeclaration: src.FlagDeclaration,
		Traces:          src.Traces,
		Value:           value,
	}
}

func (src FlagArtifacts) Clone() (dst FlagArtifacts) {
	if dst == nil {
		dst = make(FlagArtifacts)
	}
	for k, v := range src {
		dst[k] = v.Clone()
	}
	return
}

func (fa *FlagArtifact) UpdateValue(flagValue FlagValue) error {
	name := *flagValue.proto.Name
	fa.Traces = append(fa.Traces, &release_config_proto.Tracepoint{Source: proto.String(flagValue.path), Value: flagValue.proto.Value})
	if fa.Value.GetObsolete() {
		return fmt.Errorf("Attempting to set obsolete flag %s. Trace=%v", name, fa.Traces)
	}
	switch val := flagValue.proto.Value.Val.(type) {
	case *release_config_proto.Value_StringValue:
		fa.Value = &release_config_proto.Value{Val: &release_config_proto.Value_StringValue{val.StringValue}}
	case *release_config_proto.Value_BoolValue:
		fa.Value = &release_config_proto.Value{Val: &release_config_proto.Value_BoolValue{val.BoolValue}}
	case *release_config_proto.Value_Obsolete:
		if !val.Obsolete {
			return fmt.Errorf("%s: Cannot set obsolete=false.  Trace=%v", name, fa.Traces)
		}
		fa.Value = &release_config_proto.Value{Val: &release_config_proto.Value_Obsolete{true}}
	default:
		return fmt.Errorf("Invalid type for flag_value: %T.  Trace=%v", val, fa.Traces)
	}
	return nil
}

func (fa *FlagArtifact) Marshal() (*release_config_proto.FlagArtifact, error) {
	return &release_config_proto.FlagArtifact{
		FlagDeclaration: fa.FlagDeclaration,
		Value:           fa.Value,
		Traces:          fa.Traces,
	}, nil
}
