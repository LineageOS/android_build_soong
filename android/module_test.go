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

var prepareForModuleTests = FixtureRegisterWithContext(func(ctx RegistrationContext) {
	ctx.RegisterModuleType("deps", depsModuleFactory)
})

func TestErrorDependsOnDisabledModule(t *testing.T) {
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

	emptyTestFixtureFactory.
		ExtendWithErrorHandler(FixtureExpectsAtLeastOneErrorMatchingPattern(`module "foo": depends on disabled module "bar"`)).
		RunTest(t,
			prepareForModuleTests,
			FixtureWithRootAndroidBp(bp))
}

func TestValidateCorrectBuildParams(t *testing.T) {
	config := TestConfig(t.TempDir(), nil, "", nil)
	pathContext := PathContextForTesting(config)
	bparams := convertBuildParams(BuildParams{
		// Test with Output
		Output:        PathForOutput(pathContext, "undeclared_symlink"),
		SymlinkOutput: PathForOutput(pathContext, "undeclared_symlink"),
	})

	err := validateBuildParams(bparams)
	if err != nil {
		t.Error(err)
	}

	bparams = convertBuildParams(BuildParams{
		// Test with ImplicitOutput
		ImplicitOutput: PathForOutput(pathContext, "undeclared_symlink"),
		SymlinkOutput:  PathForOutput(pathContext, "undeclared_symlink"),
	})

	err = validateBuildParams(bparams)
	if err != nil {
		t.Error(err)
	}
}

func TestValidateIncorrectBuildParams(t *testing.T) {
	config := TestConfig(t.TempDir(), nil, "", nil)
	pathContext := PathContextForTesting(config)
	params := BuildParams{
		Output:          PathForOutput(pathContext, "regular_output"),
		Outputs:         PathsForOutput(pathContext, []string{"out1", "out2"}),
		ImplicitOutput:  PathForOutput(pathContext, "implicit_output"),
		ImplicitOutputs: PathsForOutput(pathContext, []string{"i_out1", "_out2"}),
		SymlinkOutput:   PathForOutput(pathContext, "undeclared_symlink"),
	}

	bparams := convertBuildParams(params)
	err := validateBuildParams(bparams)
	if err != nil {
		FailIfNoMatchingErrors(t, "undeclared_symlink is not a declared output or implicit output", []error{err})
	} else {
		t.Errorf("Expected build params to fail validation: %+v", bparams)
	}
}

func TestDistErrorChecking(t *testing.T) {
	bp := `
		deps {
			name: "foo",
      dist: {
        dest: "../invalid-dest",
        dir: "../invalid-dir",
        suffix: "invalid/suffix",
      },
      dists: [
        {
          dest: "../invalid-dest0",
          dir: "../invalid-dir0",
          suffix: "invalid/suffix0",
        },
        {
          dest: "../invalid-dest1",
          dir: "../invalid-dir1",
          suffix: "invalid/suffix1",
        },
      ],
 		}
	`

	expectedErrs := []string{
		"\\QAndroid.bp:5:13: module \"foo\": dist.dest: Path is outside directory: ../invalid-dest\\E",
		"\\QAndroid.bp:6:12: module \"foo\": dist.dir: Path is outside directory: ../invalid-dir\\E",
		"\\QAndroid.bp:7:15: module \"foo\": dist.suffix: Suffix may not contain a '/' character.\\E",
		"\\QAndroid.bp:11:15: module \"foo\": dists[0].dest: Path is outside directory: ../invalid-dest0\\E",
		"\\QAndroid.bp:12:14: module \"foo\": dists[0].dir: Path is outside directory: ../invalid-dir0\\E",
		"\\QAndroid.bp:13:17: module \"foo\": dists[0].suffix: Suffix may not contain a '/' character.\\E",
		"\\QAndroid.bp:16:15: module \"foo\": dists[1].dest: Path is outside directory: ../invalid-dest1\\E",
		"\\QAndroid.bp:17:14: module \"foo\": dists[1].dir: Path is outside directory: ../invalid-dir1\\E",
		"\\QAndroid.bp:18:17: module \"foo\": dists[1].suffix: Suffix may not contain a '/' character.\\E",
	}

	emptyTestFixtureFactory.
		ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(expectedErrs)).
		RunTest(t,
			prepareForModuleTests,
			FixtureWithRootAndroidBp(bp))
}
