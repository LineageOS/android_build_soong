package extensions

import (
	"strings"

	"github.com/google/blueprint/parser"

	"android/soong/bpfix/bpfix"
)

var fixSteps = bpfix.FixStepsExtension{
	Name: "partner-include-dirs",
	Steps: []bpfix.FixStep{
		{
			Name: "fixIncludeDirs",
			Fix:  fixIncludeDirs,
		},
	},
}

func init() {
	bpfix.RegisterFixStepExtension(&fixSteps)
}

type includeDirFix struct {
	libName  string
	libType  string
	variable string
	subdir   string
}

var commonIncludeDirs = []includeDirFix{
	{
		libName:  "my_header_lib",
		libType:  "header_libs",
		variable: "TARGET_OUT_HEADERS",
		subdir:   "/my_headers",
	},
}

func findHeaderLib(e parser.Expression) (*includeDirFix, bool) {
	if op, ok := e.(*parser.Operator); ok {
		if op.Operator != '+' {
			return nil, false
		}
		arg0, ok := op.Args[0].(*parser.Variable)
		arg1, ok1 := op.Args[1].(*parser.String)
		if !ok || !ok1 {
			return nil, false
		}
		for _, lib := range commonIncludeDirs {
			if arg0.Name == lib.variable && arg1.Value == lib.subdir {
				return &lib, true
			}
		}
	}
	return nil, false
}
func searchThroughOperatorList(mod *parser.Module, e parser.Expression) {
	if list, ok := e.(*parser.List); ok {
		newList := make([]parser.Expression, 0, len(list.Values))
		for _, item := range list.Values {
			if lib, found := findHeaderLib(item); found {
				if lib.libName != "" {
					addLibrary(mod, lib.libType, lib.libName)
				}
			} else {
				newList = append(newList, item)
			}
		}
		list.Values = newList
	}
	if op, ok := e.(*parser.Operator); ok {
		searchThroughOperatorList(mod, op.Args[0])
		searchThroughOperatorList(mod, op.Args[1])
	}
}
func getLiteralListProperty(mod *parser.Module, name string) (list *parser.List, found bool) {
	prop, ok := mod.GetProperty(name)
	if !ok {
		return nil, false
	}
	list, ok = prop.Value.(*parser.List)
	return list, ok
}
func addLibrary(mod *parser.Module, libType string, libName string) {
	var list, ok = getLiteralListProperty(mod, libType)
	if !ok {
		list = new(parser.List)
		prop := new(parser.Property)
		prop.Name = libType
		prop.Value = list
		mod.Properties = append(mod.Properties, prop)
	} else {
		for _, v := range list.Values {
			if stringValue, ok := v.(*parser.String); ok && stringValue.Value == libName {
				return
			}
		}
	}
	lib := new(parser.String)
	lib.Value = libName
	list.Values = append(list.Values, lib)
}
func fixIncludeDirs(f *bpfix.Fixer) error {
	tree := f.Tree()
	for _, def := range tree.Defs {
		mod, ok := def.(*parser.Module)
		if !ok {
			continue
		}
		if !strings.HasPrefix(mod.Type, "cc_") {
			continue
		}
		if prop, ok := mod.GetProperty("include_dirs"); ok {
			searchThroughOperatorList(mod, prop.Value)
		}
	}
	return nil
}
