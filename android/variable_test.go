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
	"reflect"
	"strconv"
	"testing"

	"github.com/google/blueprint/proptools"
)

type printfIntoPropertyTestCase struct {
	in  string
	val interface{}
	out string
	err bool
}

var printfIntoPropertyTestCases = []printfIntoPropertyTestCase{
	{
		in:  "%d",
		val: 0,
		out: "0",
	},
	{
		in:  "%d",
		val: 1,
		out: "1",
	},
	{
		in:  "%d",
		val: 2,
		out: "2",
	},
	{
		in:  "%d",
		val: false,
		out: "0",
	},
	{
		in:  "%d",
		val: true,
		out: "1",
	},
	{
		in:  "%d",
		val: -1,
		out: "-1",
	},

	{
		in:  "-DA=%d",
		val: 1,
		out: "-DA=1",
	},
	{
		in:  "-DA=%du",
		val: 1,
		out: "-DA=1u",
	},
	{
		in:  "-DA=%s",
		val: "abc",
		out: "-DA=abc",
	},
	{
		in:  `-DA="%s"`,
		val: "abc",
		out: `-DA="abc"`,
	},

	{
		in:  "%%",
		err: true,
	},
	{
		in:  "%d%s",
		err: true,
	},
	{
		in:  "%d,%s",
		err: true,
	},
	{
		in:  "%d",
		val: "",
		err: true,
	},
	{
		in:  "%d",
		val: 1.5,
		err: true,
	},
	{
		in:  "%f",
		val: 1.5,
		err: true,
	},
}

func TestPrintfIntoProperty(t *testing.T) {
	for _, testCase := range printfIntoPropertyTestCases {
		s := testCase.in
		v := reflect.ValueOf(&s).Elem()
		err := printfIntoProperty(v, testCase.val)
		if err != nil && !testCase.err {
			t.Errorf("unexpected error %s", err)
		} else if err == nil && testCase.err {
			t.Errorf("expected error")
		} else if err == nil && v.String() != testCase.out {
			t.Errorf("expected %q got %q", testCase.out, v.String())
		}
	}
}

type testProductVariableModule struct {
	ModuleBase
}

func (m *testProductVariableModule) GenerateAndroidBuildActions(ctx ModuleContext) {
}

var testProductVariableProperties = struct {
	Product_variables struct {
		Eng struct {
			Srcs   []string
			Cflags []string
		}
	}
}{}

func testProductVariableModuleFactoryFactory(props interface{}) func() Module {
	return func() Module {
		m := &testProductVariableModule{}
		clonedProps := proptools.CloneProperties(reflect.ValueOf(props)).Interface()
		m.AddProperties(clonedProps)

		// Set a default variableProperties, this will be used as the input to the property struct filter
		// for this test module.
		m.variableProperties = testProductVariableProperties
		InitAndroidModule(m)
		return m
	}
}

func TestProductVariables(t *testing.T) {
	ctx := NewTestContext()
	// A module type that has a srcs property but not a cflags property.
	ctx.RegisterModuleType("module1", testProductVariableModuleFactoryFactory(struct {
		Srcs []string
	}{}))
	// A module type that has a cflags property but not a srcs property.
	ctx.RegisterModuleType("module2", testProductVariableModuleFactoryFactory(struct {
		Cflags []string
	}{}))
	// A module type that does not have any properties that match product_variables.
	ctx.RegisterModuleType("module3", testProductVariableModuleFactoryFactory(struct {
		Foo []string
	}{}))
	ctx.PreDepsMutators(func(ctx RegisterMutatorsContext) {
		ctx.BottomUp("variable", variableMutator).Parallel()
	})

	// Test that a module can use one product variable even if it doesn't have all the properties
	// supported by that product variable.
	bp := `
		module1 {
			name: "foo",
			product_variables: {
				eng: {
					srcs: ["foo.c"],
				},
			},
		}
		module2 {
			name: "bar",
			product_variables: {
				eng: {
					cflags: ["-DBAR"],
				},
			},
		}

		module3 {
			name: "baz",
		}
	`

	mockFS := map[string][]byte{
		"Android.bp": []byte(bp),
	}

	ctx.MockFileSystem(mockFS)

	ctx.Register()

	config := TestConfig(buildDir, nil)
	config.TestProductVariables.Eng = proptools.BoolPtr(true)

	_, errs := ctx.ParseFileList(".", []string{"Android.bp"})
	FailIfErrored(t, errs)
	_, errs = ctx.PrepareBuildActions(config)
	FailIfErrored(t, errs)
}

func BenchmarkSliceToTypeArray(b *testing.B) {
	for _, n := range []int{1, 2, 4, 8, 100} {
		var propStructs []interface{}
		for i := 0; i < n; i++ {
			propStructs = append(propStructs, &struct {
				A *string
				B string
			}{})

		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = sliceToTypeArray(propStructs)
			}
		})
	}
}
