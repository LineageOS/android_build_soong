// Copyright 2017 Google Inc. All rights reserved.
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

package androidmk

import (
	"fmt"
	"strings"

	mkparser "android/soong/androidmk/parser"

	bpparser "github.com/google/blueprint/parser"
)

func stringToStringValue(s string) bpparser.Expression {
	return &bpparser.String{
		Value: s,
	}
}

func stringListToStringValueList(list []string) []bpparser.Expression {
	valList := make([]bpparser.Expression, len(list))
	for i, l := range list {
		valList[i] = stringToStringValue(l)
	}
	return valList
}

func addValues(val1, val2 bpparser.Expression) (bpparser.Expression, error) {
	if val1 == nil {
		return val2, nil
	}

	if val1.Type() == bpparser.StringType && val2.Type() == bpparser.ListType {
		val1 = &bpparser.List{
			Values: []bpparser.Expression{val1},
		}
	} else if val2.Type() == bpparser.StringType && val1.Type() == bpparser.ListType {
		val2 = &bpparser.List{
			Values: []bpparser.Expression{val1},
		}
	} else if val1.Type() != val2.Type() {
		return nil, fmt.Errorf("cannot add mismatched types")
	}

	return &bpparser.Operator{
		Operator: '+',
		Args:     [2]bpparser.Expression{val1, val2},
	}, nil
}

func makeToStringExpression(ms *mkparser.MakeString, file *bpFile) (bpparser.Expression, error) {
	var val bpparser.Expression
	var err error

	if ms.Strings[0] != "" {
		val = stringToStringValue(ms.Strings[0])
	}

	for i, s := range ms.Strings[1:] {
		if ret, ok := ms.Variables[i].EvalFunction(file.scope); ok {
			if len(ret) > 1 {
				return nil, fmt.Errorf("Unexpected list value %s", ms.Dump())
			}
			val, err = addValues(val, stringToStringValue(ret[0]))
		} else {
			name, err := extractVariableName(ms.Variables[i].Name, file)
			if err != nil {
				return nil, err
			}
			tmp := &bpparser.Variable{
				Name:  name,
				Type_: bpparser.StringType,
			}

			if tmp.Name == "TOP" {
				if s[0] == '/' {
					s = s[1:]
				} else {
					s = "." + s
				}
			} else {
				val, err = addValues(val, tmp)
				if err != nil {
					return nil, err
				}
			}
		}

		if s != "" {
			tmp := stringToStringValue(s)
			val, err = addValues(val, tmp)
			if err != nil {
				return nil, err
			}
		}
	}

	return val, nil
}

func stringToListValue(s string) bpparser.Expression {
	list := strings.Fields(s)
	valList := make([]bpparser.Expression, len(list))
	for i, l := range list {
		valList[i] = &bpparser.String{
			Value: l,
		}
	}
	return &bpparser.List{
		Values: valList,
	}

}

func makeToListExpression(ms *mkparser.MakeString, file *bpFile) (bpparser.Expression, error) {
	fields := ms.Split(" \t")

	var listOfListValues []bpparser.Expression

	listValue := &bpparser.List{}

	for _, f := range fields {
		if len(f.Variables) == 1 && f.Strings[0] == "" && f.Strings[1] == "" {
			if ret, ok := f.Variables[0].EvalFunction(file.scope); ok {
				listValue.Values = append(listValue.Values, stringListToStringValueList(ret)...)
			} else {
				name, err := extractVariableName(f.Variables[0].Name, file)
				if err != nil {
					return nil, err
				}
				if name == "TOP" {
					listValue.Values = append(listValue.Values, &bpparser.String{
						Value: ".",
					})
				} else {
					if len(listValue.Values) > 0 {
						listOfListValues = append(listOfListValues, listValue)
					}
					listOfListValues = append(listOfListValues, &bpparser.Variable{
						Name:  name,
						Type_: bpparser.ListType,
					})
					listValue = &bpparser.List{}
				}
			}
		} else {
			s, err := makeToStringExpression(f, file)
			if err != nil {
				return nil, err
			}
			if s == nil {
				continue
			}

			listValue.Values = append(listValue.Values, s)
		}
	}

	if len(listValue.Values) > 0 {
		listOfListValues = append(listOfListValues, listValue)
	}

	if len(listOfListValues) == 0 {
		return listValue, nil
	}

	val := listOfListValues[0]
	for _, tmp := range listOfListValues[1:] {
		var err error
		val, err = addValues(val, tmp)
		if err != nil {
			return nil, err
		}
	}

	return val, nil
}

func stringToBoolValue(s string) (bpparser.Expression, error) {
	var b bool
	s = strings.TrimSpace(s)
	switch s {
	case "true":
		b = true
	case "false", "":
		b = false
	case "-frtti": // HACK for LOCAL_RTTI_VALUE
		b = true
	default:
		return nil, fmt.Errorf("unexpected bool value %s", s)
	}
	return &bpparser.Bool{
		Value: b,
	}, nil
}

func makeToBoolExpression(ms *mkparser.MakeString, file *bpFile) (bpparser.Expression, error) {
	if !ms.Const() {
		if len(ms.Variables) == 1 && ms.Strings[0] == "" && ms.Strings[1] == "" {
			name, err := extractVariableName(ms.Variables[0].Name, file)
			if err != nil {
				return nil, err
			}
			return &bpparser.Variable{
				Name:  name,
				Type_: bpparser.BoolType,
			}, nil
		} else {
			return nil, fmt.Errorf("non-const bool expression %s", ms.Dump())
		}
	}

	return stringToBoolValue(ms.Value(nil))
}

func extractVariableName(name *mkparser.MakeString, file *bpFile) (string, error) {
	if !name.Const() {
		return "", fmt.Errorf("Unsupported non-const variable name %s", name.Dump())
	}

	variableName := name.Value(nil)

	if newName, ok := file.variableRenames[variableName]; ok {
		variableName = newName
	}

	return variableName, nil
}
