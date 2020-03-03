// Copyright 2018 Google Inc. All rights reserved.
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

package makedeps

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"android/soong/androidmk/parser"
)

type Deps struct {
	Output string
	Inputs []string
}

func Parse(filename string, r io.Reader) (*Deps, error) {
	p := parser.NewParser(filename, r)
	nodes, errs := p.Parse()

	if len(errs) == 1 {
		return nil, errs[0]
	} else if len(errs) > 1 {
		return nil, fmt.Errorf("many errors: %v", errs)
	}

	pos := func(node parser.Node) string {
		return p.Unpack(node.Pos()).String() + ": "
	}

	ret := &Deps{}

	for _, node := range nodes {
		switch x := node.(type) {
		case *parser.Comment:
			// Do nothing
		case *parser.Rule:
			if x.Recipe != "" {
				return nil, fmt.Errorf("%sunexpected recipe in rule: %v", pos(node), x)
			}

			if !x.Target.Const() {
				return nil, fmt.Errorf("%sunsupported variable expansion: %v", pos(node), x.Target.Dump())
			}
			outputs := x.Target.Words()
			if len(outputs) == 0 {
				return nil, fmt.Errorf("%smissing output: %v", pos(node), x)
			}
			ret.Output = outputs[0].Value(nil)

			if !x.Prerequisites.Const() {
				return nil, fmt.Errorf("%sunsupported variable expansion: %v", pos(node), x.Prerequisites.Dump())
			}
			for _, input := range x.Prerequisites.Words() {
				ret.Inputs = append(ret.Inputs, input.Value(nil))
			}
		default:
			return nil, fmt.Errorf("%sunexpected line: %#v", pos(node), node)
		}
	}

	return ret, nil
}

func (d *Deps) Print() []byte {
	// We don't really have to escape every \, but it's simpler,
	// and ninja will handle it.
	replacer := strings.NewReplacer(" ", "\\ ",
		":", "\\:",
		"#", "\\#",
		"$", "$$",
		"\\", "\\\\")

	b := &bytes.Buffer{}
	fmt.Fprintf(b, "%s:", replacer.Replace(d.Output))
	for _, input := range d.Inputs {
		fmt.Fprintf(b, " %s", replacer.Replace(input))
	}
	fmt.Fprintln(b)
	return b.Bytes()
}
