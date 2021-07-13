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
	"path/filepath"
	"strings"

	mkparser "android/soong/androidmk/parser"
)

// Implements mkparser.Scope, to be used by mkparser.Value.Value()
type localDirEval struct {
	localDir  string
	hasErrors bool
}

func (l *localDirEval) Get(name string) string {
	if name == "LOCAL_DIR" {
		return l.localDir
	}
	l.hasErrors = true
	return fmt.Sprintf("$(%s)", name)
}

func (l *localDirEval) Set(_, _ string) {
}

func (l *localDirEval) Call(_ string, _ []string) []string {
	l.hasErrors = true
	return []string{"$(call ...)"}
}

func (l *localDirEval) SetFunc(_ string, _ func([]string) []string) {
}

// UpdateProductConfigMap builds product configuration map.
// The product configuration map maps a product name (i.e., the value of the
// TARGET_PRODUCT variable) to the top-level configuration file.
// In the Android's Make-based build machinery, the equivalent of the
// product configuration map is $(PRODUCT_MAKEFILES), which is the list
// of <product>:<configuration makefile> pairs (if <product>: is missing,
// <product> is the basename of the configuration makefile).
// UpdateProductConfigMap emulates this build logic by processing the
// assignments to PRODUCT_MAKEFILES in the file passed to it.
func UpdateProductConfigMap(configMap map[string]string, configMakefile string) error {
	contents, err := ioutil.ReadFile(configMakefile)
	if err != nil {
		return err
	}
	parser := mkparser.NewParser(configMakefile, bytes.NewBuffer(contents))
	nodes, errs := parser.Parse()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return fmt.Errorf("cannot parse %s", configMakefile)
	}

	ldEval := &localDirEval{localDir: filepath.Dir(configMakefile)}

	for _, node := range nodes {
		// We are interested in assignments to 'PRODUCT_MAKEFILES'
		asgn, ok := node.(*mkparser.Assignment)
		if !ok {
			continue
		}
		if !(asgn.Name.Const() && asgn.Name.Strings[0] == "PRODUCT_MAKEFILES") {
			continue
		}

		// Resolve the references to $(LOCAL_DIR) in $(PRODUCT_MAKEFILES).
		ldEval.hasErrors = false
		value := asgn.Value.Value(ldEval)
		if ldEval.hasErrors {
			return fmt.Errorf("cannot evaluate %s", asgn.Value.Dump())
		}
		// Each item is either <product>:<configuration makefile>, or
		// just <configuration makefile>
		for _, token := range strings.Fields(value) {
			var product, config_path string
			if n := strings.Index(token, ":"); n >= 0 {
				product = token[0:n]
				config_path = token[n+1:]
			} else {
				config_path = token
				product = filepath.Base(config_path)
				product = strings.TrimSuffix(product, filepath.Ext(product))
			}
			configMap[product] = config_path
		}
	}
	return nil
}
