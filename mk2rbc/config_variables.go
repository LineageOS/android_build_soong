// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mk2rbc

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	mkparser "android/soong/androidmk/parser"
)

// Extracts the list of product config variables from a file, calling
// given registrar for each variable.
func FindConfigVariables(mkFile string, vr variableRegistrar) error {
	mkContents, err := ioutil.ReadFile(mkFile)
	if err != nil {
		return err
	}
	parser := mkparser.NewParser(mkFile, bytes.NewBuffer(mkContents))
	nodes, errs := parser.Parse()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return fmt.Errorf("cannot parse %s", mkFile)
	}
	for _, node := range nodes {
		asgn, ok := node.(*mkparser.Assignment)
		if !ok {
			continue
		}
		// We are looking for a variable called '_product_list_vars'
		// or '_product_single_value_vars'.
		if !asgn.Name.Const() {
			continue
		}
		varName := asgn.Name.Strings[0]
		var starType starlarkType
		if varName == "_product_list_vars" {
			starType = starlarkTypeList
		} else if varName == "_product_single_value_vars" {
			starType = starlarkTypeUnknown
		} else {
			continue
		}
		for _, name := range strings.Fields(asgn.Value.Dump()) {
			vr.NewVariable(name, VarClassConfig, starType)
		}

	}
	return nil
}
