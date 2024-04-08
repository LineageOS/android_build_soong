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

	rc_proto "android/soong/cmd/release_config/release_config_proto"

	"google.golang.org/protobuf/proto"
)

// A flag artifact, with its final value and declaration/override history.
type FlagArtifact struct {
	// The flag_declaration message.
	FlagDeclaration *rc_proto.FlagDeclaration

	// The index of the config directory where this flag was declared.
	// Flag values cannot be set in a location with a lower index.
	DeclarationIndex int

	// A history of value assignments and overrides.
	Traces []*rc_proto.Tracepoint

	// The value of the flag.
	Value *rc_proto.Value

	// This flag is redacted.  Set by UpdateValue when the FlagValue proto
	// says to redact it.
	Redacted bool
}

// Key is flag name.
type FlagArtifacts map[string]*FlagArtifact

// Create a clone of the flag artifact.
//
// Returns:
//
//	*FlagArtifact: the copy of the artifact.
func (src *FlagArtifact) Clone() *FlagArtifact {
	value := &rc_proto.Value{}
	proto.Merge(value, src.Value)
	return &FlagArtifact{
		FlagDeclaration: src.FlagDeclaration,
		Traces:          src.Traces,
		Value:           value,
	}
}

// Clone FlagArtifacts.
//
// Returns:
//
//	FlagArtifacts: a copy of the source FlagArtifacts.
func (src FlagArtifacts) Clone() (dst FlagArtifacts) {
	if dst == nil {
		dst = make(FlagArtifacts)
	}
	for k, v := range src {
		dst[k] = v.Clone()
	}
	return
}

// Update the value of a flag.
//
// This appends to flagArtifact.Traces, and updates flagArtifact.Value.
//
// Args:
//
//	flagValue FlagValue: the value to assign
//
// Returns:
//
//	error: any error encountered
func (fa *FlagArtifact) UpdateValue(flagValue FlagValue) error {
	name := *flagValue.proto.Name
	fa.Traces = append(fa.Traces, &rc_proto.Tracepoint{Source: proto.String(flagValue.path), Value: flagValue.proto.Value})
	if flagValue.proto.GetRedacted() {
		fa.Redacted = true
		fmt.Printf("Redacting flag %s in %s\n", name, flagValue.path)
		return nil
	}
	if fa.Value.GetObsolete() {
		return fmt.Errorf("Attempting to set obsolete flag %s. Trace=%v", name, fa.Traces)
	}
	var newValue *rc_proto.Value
	switch val := flagValue.proto.Value.Val.(type) {
	case *rc_proto.Value_StringValue:
		newValue = &rc_proto.Value{Val: &rc_proto.Value_StringValue{val.StringValue}}
	case *rc_proto.Value_BoolValue:
		newValue = &rc_proto.Value{Val: &rc_proto.Value_BoolValue{val.BoolValue}}
	case *rc_proto.Value_Obsolete:
		if !val.Obsolete {
			return fmt.Errorf("%s: Cannot set obsolete=false.  Trace=%v", name, fa.Traces)
		}
		newValue = &rc_proto.Value{Val: &rc_proto.Value_Obsolete{true}}
	default:
		return fmt.Errorf("Invalid type for flag_value: %T.  Trace=%v", val, fa.Traces)
	}
	if proto.Equal(newValue, fa.Value) {
		warnf("%s: redundant override (set in %s)\n", flagValue.path, *fa.Traces[len(fa.Traces)-2].Source)
	}
	fa.Value = newValue
	return nil
}

// Marshal the FlagArtifact into a flag_artifact message.
func (fa *FlagArtifact) Marshal() (*rc_proto.FlagArtifact, error) {
	if fa.Redacted {
		return nil, nil
	}
	return &rc_proto.FlagArtifact{
		FlagDeclaration: fa.FlagDeclaration,
		Value:           fa.Value,
		Traces:          fa.Traces,
	}, nil
}

// Marshal the FlagArtifact without Traces.
func (fa *FlagArtifact) MarshalWithoutTraces() (*rc_proto.FlagArtifact, error) {
	if fa.Redacted {
		return nil, nil
	}
	return &rc_proto.FlagArtifact{
		FlagDeclaration: fa.FlagDeclaration,
		Value:           fa.Value,
	}, nil
}
