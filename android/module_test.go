// Copyright 2015 Google Inc. All rights reserved.
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

func TestSrcIsModule(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name       string
		args       args
		wantModule string
	}{
		{
			name: "file",
			args: args{
				s: "foo",
			},
			wantModule: "",
		},
		{
			name: "module",
			args: args{
				s: ":foo",
			},
			wantModule: "foo",
		},
		{
			name: "tag",
			args: args{
				s: ":foo{.bar}",
			},
			wantModule: "foo{.bar}",
		},
		{
			name: "extra colon",
			args: args{
				s: ":foo:bar",
			},
			wantModule: "foo:bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotModule := SrcIsModule(tt.args.s); gotModule != tt.wantModule {
				t.Errorf("SrcIsModule() = %v, want %v", gotModule, tt.wantModule)
			}
		})
	}
}

func TestSrcIsModuleWithTag(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name       string
		args       args
		wantModule string
		wantTag    string
	}{
		{
			name: "file",
			args: args{
				s: "foo",
			},
			wantModule: "",
			wantTag:    "",
		},
		{
			name: "module",
			args: args{
				s: ":foo",
			},
			wantModule: "foo",
			wantTag:    "",
		},
		{
			name: "tag",
			args: args{
				s: ":foo{.bar}",
			},
			wantModule: "foo",
			wantTag:    ".bar",
		},
		{
			name: "empty tag",
			args: args{
				s: ":foo{}",
			},
			wantModule: "foo",
			wantTag:    "",
		},
		{
			name: "extra colon",
			args: args{
				s: ":foo:bar",
			},
			wantModule: "foo:bar",
		},
		{
			name: "invalid tag",
			args: args{
				s: ":foo{.bar",
			},
			wantModule: "foo{.bar",
		},
		{
			name: "invalid tag 2",
			args: args{
				s: ":foo.bar}",
			},
			wantModule: "foo.bar}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotModule, gotTag := SrcIsModuleWithTag(tt.args.s)
			if gotModule != tt.wantModule {
				t.Errorf("SrcIsModuleWithTag() gotModule = %v, want %v", gotModule, tt.wantModule)
			}
			if gotTag != tt.wantTag {
				t.Errorf("SrcIsModuleWithTag() gotTag = %v, want %v", gotTag, tt.wantTag)
			}
		})
	}
}

type depsModule struct {
	ModuleBase
	props struct {
		Deps []string
	}
}

func (m *depsModule) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func (m *depsModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddDependency(ctx.Module(), nil, m.props.Deps...)
}

func depsModuleFactory() Module {
	m := &depsModule{}
	m.AddProperties(&m.props)
	InitAndroidModule(m)
	return m
}

func TestErrorDependsOnDisabledModule(t *testing.T) {
	ctx := NewTestContext()
	ctx.RegisterModuleType("deps", depsModuleFactory)

	bp := `
		deps {
			name: "foo",
			deps: ["bar"],
		}
		deps {
			name: "bar",
			enabled: false,
		}
	`

	config := TestConfig(buildDir, nil, bp, nil)

	ctx.Register(config)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfNoMatchingErrors(t, `module "foo": depends on disabled module "bar"`, errs)
}
