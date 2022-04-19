// Copyright 2021 Google Inc. All rights reserved.
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

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/google/blueprint/proptools"

	"android/soong/bpfix/bpfix"
)

type RewriteNames []RewriteName
type RewriteName struct {
	prefix string
	repl   string
}

func (r *RewriteNames) String() string {
	return ""
}

func (r *RewriteNames) Set(v string) error {
	split := strings.SplitN(v, "=", 2)
	if len(split) != 2 {
		return fmt.Errorf("Must be in the form of <prefix>=<replace>")
	}
	*r = append(*r, RewriteName{
		prefix: split[0],
		repl:   split[1],
	})
	return nil
}

func (r *RewriteNames) GoToBp(name string) string {
	ret := name
	for _, r := range *r {
		prefix := r.prefix
		if name == prefix {
			ret = r.repl
			break
		}
		prefix += "/"
		if strings.HasPrefix(name, prefix) {
			ret = r.repl + "-" + strings.TrimPrefix(name, prefix)
		}
	}
	return strings.ReplaceAll(ret, "/", "-")
}

var rewriteNames = RewriteNames{}

type Exclude map[string]bool

func (e Exclude) String() string {
	return ""
}

func (e Exclude) Set(v string) error {
	e[v] = true
	return nil
}

var excludes = make(Exclude)
var excludeDeps = make(Exclude)
var excludeSrcs = make(Exclude)

type StringList []string

func (l *StringList) String() string {
	return strings.Join(*l, " ")
}

func (l *StringList) Set(v string) error {
	*l = append(*l, strings.Fields(v)...)
	return nil
}

type GoModule struct {
	Dir string
}

type GoPackage struct {
	ExportToAndroid bool

	Dir         string
	ImportPath  string
	Name        string
	Imports     []string
	GoFiles     []string
	TestGoFiles []string
	TestImports []string

	Module *GoModule
}

func (g GoPackage) IsCommand() bool {
	return g.Name == "main"
}

func (g GoPackage) BpModuleType() string {
	if g.IsCommand() {
		return "blueprint_go_binary"
	}
	return "bootstrap_go_package"
}

func (g GoPackage) BpName() string {
	if g.IsCommand() {
		return rewriteNames.GoToBp(filepath.Base(g.ImportPath))
	}
	return rewriteNames.GoToBp(g.ImportPath)
}

func (g GoPackage) BpDeps(deps []string) []string {
	var ret []string
	for _, d := range deps {
		// Ignore stdlib dependencies
		if !strings.Contains(d, ".") {
			continue
		}
		if _, ok := excludeDeps[d]; ok {
			continue
		}
		name := rewriteNames.GoToBp(d)
		ret = append(ret, name)
	}
	return ret
}

func (g GoPackage) BpSrcs(srcs []string) []string {
	var ret []string
	prefix, err := filepath.Rel(g.Module.Dir, g.Dir)
	if err != nil {
		panic(err)
	}
	for _, f := range srcs {
		f = filepath.Join(prefix, f)
		if _, ok := excludeSrcs[f]; ok {
			continue
		}
		ret = append(ret, f)
	}
	return ret
}

// AllImports combines Imports and TestImports, as blueprint does not differentiate these.
func (g GoPackage) AllImports() []string {
	imports := append([]string(nil), g.Imports...)
	imports = append(imports, g.TestImports...)

	if len(imports) == 0 {
		return nil
	}

	// Sort and de-duplicate
	sort.Strings(imports)
	j := 0
	for i := 1; i < len(imports); i++ {
		if imports[i] == imports[j] {
			continue
		}
		j++
		imports[j] = imports[i]
	}
	return imports[:j+1]
}

var bpTemplate = template.Must(template.New("bp").Parse(`
{{.BpModuleType}} {
    name: "{{.BpName}}",
    {{- if not .IsCommand}}
    pkgPath: "{{.ImportPath}}",
    {{- end}}
    {{- if .BpDeps .AllImports}}
    deps: [
        {{- range .BpDeps .AllImports}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    {{- if .BpSrcs .GoFiles}}
    srcs: [
        {{- range .BpSrcs .GoFiles}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    {{- if .BpSrcs .TestGoFiles}}
    testSrcs: [
    	{{- range .BpSrcs .TestGoFiles}}
        "{{.}}",
       {{- end}}
    ],
    {{- end}}
}
`))

func rerunForRegen(filename string) error {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(buf))

	// Skip the first line in the file
	for i := 0; i < 2; i++ {
		if !scanner.Scan() {
			if scanner.Err() != nil {
				return scanner.Err()
			} else {
				return fmt.Errorf("unexpected EOF")
			}
		}
	}

	// Extract the old args from the file
	line := scanner.Text()
	if strings.HasPrefix(line, "// go2bp ") {
		line = strings.TrimPrefix(line, "// go2bp ")
	} else {
		return fmt.Errorf("unexpected second line: %q", line)
	}
	args := strings.Split(line, " ")
	lastArg := args[len(args)-1]
	args = args[:len(args)-1]

	// Append all current command line args except -regen <file> to the ones from the file
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-regen" || os.Args[i] == "--regen" {
			i++
		} else {
			args = append(args, os.Args[i])
		}
	}
	args = append(args, lastArg)

	cmd := os.Args[0] + " " + strings.Join(args, " ")
	// Re-exec pom2bp with the new arguments
	output, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if exitErr, _ := err.(*exec.ExitError); exitErr != nil {
		return fmt.Errorf("failed to run %s\n%s", cmd, string(exitErr.Stderr))
	} else if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, output, 0666)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `go2bp, a tool to create Android.bp files from go modules

The tool will extract the necessary information from Go files to create an Android.bp that can
compile them. This needs to be run from the same directory as the go.mod file.

Usage: %s [--rewrite <pkg-prefix>=<replace>] [-exclude <package>] [-regen <file>]

  -rewrite <pkg-prefix>=<replace>
     rewrite can be used to specify mappings between go package paths and Android.bp modules. The -rewrite
     option can be specified multiple times. When determining the Android.bp module for a given Go
     package, mappings are searched in the order they were specified. The first <pkg-prefix> matching
     either the package directly, or as the prefix '<pkg-prefix>/' will be replaced with <replace>.
     After all replacements are finished, all '/' characters are replaced with '-'.
  -exclude <package>
     Don't put the specified go package in the Android.bp file.
  -exclude-deps <package>
     Don't put the specified go package in the dependency lists.
  -exclude-srcs <module>
     Don't put the specified source files in srcs or testSrcs lists.
  -limit <package>
     If set, limit the output to the specified packages and their dependencies.
  -skip-tests
     If passed, don't write out any test srcs or dependencies to the Android.bp output.
  -regen <file>
     Read arguments from <file> and overwrite it.

`, os.Args[0])
	}

	var regen string
	var skipTests bool
	limit := StringList{}

	flag.Var(&excludes, "exclude", "Exclude go package")
	flag.Var(&excludeDeps, "exclude-dep", "Exclude go package from deps")
	flag.Var(&excludeSrcs, "exclude-src", "Exclude go file from source lists")
	flag.Var(&rewriteNames, "rewrite", "Regex(es) to rewrite artifact names")
	flag.Var(&limit, "limit", "If set, only includes the dependencies of the listed packages")
	flag.BoolVar(&skipTests, "skip-tests", false, "Whether to skip test sources")
	flag.StringVar(&regen, "regen", "", "Rewrite specified file")
	flag.Parse()

	if regen != "" {
		err := rerunForRegen(regen)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "Unused argument detected: %v\n", flag.Args())
		os.Exit(1)
	}

	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, "go.mod file not found")
		os.Exit(1)
	}

	cmd := exec.Command("go", "list", "-json", "./...")
	var stdoutb, stderrb bytes.Buffer
	cmd.Stdout = &stdoutb
	cmd.Stderr = &stderrb
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Running %q to dump the Go packages failed: %v, stderr:\n%s\n",
			cmd.String(), err, stderrb.Bytes())
		os.Exit(1)
	}
	decoder := json.NewDecoder(bytes.NewReader(stdoutb.Bytes()))

	pkgs := []*GoPackage{}
	pkgMap := map[string]*GoPackage{}
	for decoder.More() {
		pkg := GoPackage{}
		err := decoder.Decode(&pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse json: %v\n", err)
			os.Exit(1)
		}
		if len(limit) == 0 {
			pkg.ExportToAndroid = true
		}
		if skipTests {
			pkg.TestGoFiles = nil
			pkg.TestImports = nil
		}
		pkgs = append(pkgs, &pkg)
		pkgMap[pkg.ImportPath] = &pkg
	}

	buf := &bytes.Buffer{}

	fmt.Fprintln(buf, "// Automatically generated with:")
	fmt.Fprintln(buf, "// go2bp", strings.Join(proptools.ShellEscapeList(os.Args[1:]), " "))

	var mark func(string)
	mark = func(pkgName string) {
		if excludes[pkgName] {
			return
		}
		if pkg, ok := pkgMap[pkgName]; ok && !pkg.ExportToAndroid {
			pkg.ExportToAndroid = true
			for _, dep := range pkg.AllImports() {
				if !excludeDeps[dep] {
					mark(dep)
				}
			}
		}
	}

	for _, pkgName := range limit {
		mark(pkgName)
	}

	for _, pkg := range pkgs {
		if !pkg.ExportToAndroid || excludes[pkg.ImportPath] {
			continue
		}
		if len(pkg.BpSrcs(pkg.GoFiles)) == 0 && len(pkg.BpSrcs(pkg.TestGoFiles)) == 0 {
			continue
		}
		err := bpTemplate.Execute(buf, pkg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error writing", pkg.Name, err)
			os.Exit(1)
		}
	}

	out, err := bpfix.Reformat(buf.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error formatting output", err)
		os.Exit(1)
	}

	os.Stdout.WriteString(out)
}
