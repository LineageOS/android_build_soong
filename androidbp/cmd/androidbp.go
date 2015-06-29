package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	bpparser "github.com/google/blueprint/parser"
)

var recursiveSubdirRegex *regexp.Regexp = regexp.MustCompile("(.+)/\\*\\*/(.+)")

type Module struct {
	bpmod      *bpparser.Module
	bpname     string
	mkname     string
	isHostRule bool
}

func newModule(mod *bpparser.Module) *Module {
	return &Module{
		bpmod:  mod,
		bpname: mod.Type.Name,
	}
}

func (m *Module) translateRuleName() error {
	var name string
	if translation, ok := moduleTypeToRule[m.bpname]; ok {
		name = translation
	} else {
		return fmt.Errorf("Unknown module type %q", m.bpname)
	}

	if m.isHostRule {
		if trans, ok := targetToHostModuleRule[name]; ok {
			name = trans
		} else {
			return fmt.Errorf("No corresponding host rule for %q", name)
		}
	} else {
		m.isHostRule = strings.Contains(name, "HOST")
	}

	m.mkname = name

	return nil
}

type androidMkWriter struct {
	io.Writer

	blueprint *bpparser.File
	path      string

	printedLocalPath bool

	mapScope map[string][]*bpparser.Property
}

func (w *androidMkWriter) WriteString(s string) (int, error) {
	return io.WriteString(w.Writer, s)
}

func valueToString(value bpparser.Value) (string, error) {
	if value.Variable != "" {
		return fmt.Sprintf("$(%s)", value.Variable), nil
	} else if value.Expression != nil {
		if value.Expression.Operator != '+' {
			return "", fmt.Errorf("unexpected operator '%c'", value.Expression.Operator)
		}
		val1, err := valueToString(value.Expression.Args[0])
		if err != nil {
			return "", err
		}
		val2, err := valueToString(value.Expression.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s%s", val1, val2), nil
	} else {
		switch value.Type {
		case bpparser.Bool:
			return fmt.Sprintf("%t", value.BoolValue), nil
		case bpparser.String:
			return fmt.Sprintf("%s", processWildcards(value.StringValue)), nil
		case bpparser.List:
			val, err := listToMkString(value.ListValue)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("\\\n%s", val), nil
		case bpparser.Map:
			return "", fmt.Errorf("Can't convert map to string")
		default:
			return "", fmt.Errorf("ERROR: unsupported type %d", value.Type)
		}
	}
}

func getTopOfAndroidTree(wd string) (string, error) {
	if !filepath.IsAbs(wd) {
		return "", errors.New("path must be absolute: " + wd)
	}

	topfile := "build/soong/bootstrap.bash"

	for "/" != wd {
		expected := filepath.Join(wd, topfile)

		if _, err := os.Stat(expected); err == nil {
			// Found the top
			return wd, nil
		}

		wd = filepath.Join(wd, "..")
	}

	return "", errors.New("couldn't find top of tree from " + wd)
}

// TODO: handle non-recursive wildcards?
func processWildcards(s string) string {
	submatches := recursiveSubdirRegex.FindStringSubmatch(s)
	if len(submatches) > 2 {
		// Found a wildcard rule
		return fmt.Sprintf("$(call find-files-in-subdirs, $(LOCAL_PATH), %s, %s)",
			submatches[2], submatches[1])
	}

	return s
}

func listToMkString(list []bpparser.Value) (string, error) {
	lines := make([]string, 0, len(list))
	for _, tok := range list {
		val, err := valueToString(tok)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("    %s", val))
	}

	return strings.Join(lines, " \\\n"), nil
}

func translateTargetConditionals(props []*bpparser.Property,
	disabledBuilds map[string]bool, isHostRule bool) (computedProps []string, err error) {
	for _, target := range props {
		conditionals := targetScopedPropertyConditionals
		altConditionals := hostScopedPropertyConditionals
		if isHostRule {
			conditionals, altConditionals = altConditionals, conditionals
		}

		conditional, ok := conditionals[target.Name.Name]
		if !ok {
			if _, ok := altConditionals[target.Name.Name]; ok {
				// This is only for the other build type
				continue
			} else {
				return nil, fmt.Errorf("Unsupported conditional %q", target.Name.Name)
			}
		}

		var scopedProps []string
		for _, targetScopedProp := range target.Value.MapValue {
			if mkProp, ok := standardProperties[targetScopedProp.Name.Name]; ok {
				val, err := valueToString(targetScopedProp.Value)
				if err != nil {
					return nil, err
				}
				scopedProps = append(scopedProps, fmt.Sprintf("%s += %s",
					mkProp.string, val))
			} else if rwProp, ok := rewriteProperties[targetScopedProp.Name.Name]; ok {
				props, err := rwProp.f(rwProp.string, targetScopedProp, nil)
				if err != nil {
					return nil, err
				}
				scopedProps = append(scopedProps, props...)
			} else if "disabled" == targetScopedProp.Name.Name {
				if targetScopedProp.Value.BoolValue {
					disabledBuilds[target.Name.Name] = true
				} else {
					delete(disabledBuilds, target.Name.Name)
				}
			} else {
				return nil, fmt.Errorf("Unsupported target property %q", targetScopedProp.Name.Name)
			}
		}

		if len(scopedProps) > 0 {
			if conditional != "" {
				computedProps = append(computedProps, conditional)
				computedProps = append(computedProps, scopedProps...)
				computedProps = append(computedProps, "endif")
			} else {
				computedProps = append(computedProps, scopedProps...)
			}
		}
	}

	return
}

func translateSuffixProperties(suffixProps []*bpparser.Property,
	suffixMap map[string]string) (computedProps []string, err error) {
	for _, suffixProp := range suffixProps {
		if suffix, ok := suffixMap[suffixProp.Name.Name]; ok {
			for _, stdProp := range suffixProp.Value.MapValue {
				if mkProp, ok := standardProperties[stdProp.Name.Name]; ok {
					val, err := valueToString(stdProp.Value)
					if err != nil {
						return nil, err
					}
					computedProps = append(computedProps, fmt.Sprintf("%s_%s := %s", mkProp.string, suffix, val))
				} else if rwProp, ok := rewriteProperties[stdProp.Name.Name]; ok {
					props, err := rwProp.f(rwProp.string, stdProp, &suffix)
					if err != nil {
						return nil, err
					}
					computedProps = append(computedProps, props...)
				} else {
					return nil, fmt.Errorf("Unsupported property %q", stdProp.Name.Name)
				}
			}
		} else {
			return nil, fmt.Errorf("Unsupported suffix property %q", suffixProp.Name.Name)
		}
	}
	return
}

func prependLocalPath(name string, prop *bpparser.Property, suffix *string) ([]string, error) {
	if suffix != nil {
		name += "_" + *suffix
	}
	val, err := valueToString(prop.Value)
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("%s := $(addprefix $(LOCAL_PATH)/,%s)\n", name, val),
	}, nil
}

func prependLocalModule(name string, prop *bpparser.Property, suffix *string) ([]string, error) {
	if suffix != nil {
		name += "_" + *suffix
	}
	val, err := valueToString(prop.Value)
	if err != nil {
		return nil, err
	}
	return []string{
		fmt.Sprintf("%s := $(LOCAL_MODULE)%s\n", name, val),
	}, nil
}

func modulePropBool(module *bpparser.Module, name string) bool {
	for _, prop := range module.Properties {
		if name == prop.Name.Name {
			return prop.Value.BoolValue
		}
	}
	return false
}

func (w *androidMkWriter) lookupMap(parent bpparser.Value) (mapValue []*bpparser.Property) {
	if parent.Variable != "" {
		mapValue = w.mapScope[parent.Variable]
	} else {
		mapValue = parent.MapValue
	}
	return
}

func (w *androidMkWriter) writeModule(moduleRule string, props []string,
	disabledBuilds map[string]bool, isHostRule bool) {
	disabledConditionals := disabledTargetConditionals
	if isHostRule {
		disabledConditionals = disabledHostConditionals
	}
	for build, _ := range disabledBuilds {
		if conditional, ok := disabledConditionals[build]; ok {
			fmt.Fprintf(w, "%s\n", conditional)
			defer fmt.Fprintf(w, "endif\n")
		}
	}

	fmt.Fprintf(w, "include $(CLEAR_VARS)\n")
	fmt.Fprintf(w, "%s\n", strings.Join(props, "\n"))
	fmt.Fprintf(w, "include $(%s)\n\n", moduleRule)
}

func (w *androidMkWriter) parsePropsAndWriteModule(module *Module) error {
	standardProps := make([]string, 0, len(module.bpmod.Properties))
	disabledBuilds := make(map[string]bool)
	for _, prop := range module.bpmod.Properties {
		if mkProp, ok := standardProperties[prop.Name.Name]; ok {
			val, err := valueToString(prop.Value)
			if err != nil {
				return err
			}
			standardProps = append(standardProps, fmt.Sprintf("%s := %s", mkProp.string, val))
		} else if rwProp, ok := rewriteProperties[prop.Name.Name]; ok {
			props, err := rwProp.f(rwProp.string, prop, nil)
			if err != nil {
				return err
			}
			standardProps = append(standardProps, props...)
		} else if suffixMap, ok := suffixProperties[prop.Name.Name]; ok {
			suffixProps := w.lookupMap(prop.Value)
			props, err := translateSuffixProperties(suffixProps, suffixMap)
			if err != nil {
				return err
			}
			standardProps = append(standardProps, props...)
		} else if "target" == prop.Name.Name {
			suffixProps := w.lookupMap(prop.Value)
			props, err := translateTargetConditionals(suffixProps, disabledBuilds, module.isHostRule)
			if err != nil {
				return err
			}
			standardProps = append(standardProps, props...)
		} else if _, ok := ignoredProperties[prop.Name.Name]; ok {
		} else {
			return fmt.Errorf("Unsupported property %q", prop.Name.Name)
		}
	}

	w.writeModule(module.mkname, standardProps, disabledBuilds, module.isHostRule)

	return nil
}

func (w *androidMkWriter) mutateModule(module *Module) (modules []*Module, err error) {
	modules = []*Module{module}

	if module.bpname == "cc_library" {
		modules = []*Module{
			newModule(module.bpmod),
			newModule(module.bpmod),
		}
		modules[0].bpname = "cc_library_shared"
		modules[1].bpname = "cc_library_static"
	}

	for _, mod := range modules {
		err := mod.translateRuleName()
		if err != nil {
			return nil, err
		}
		if mod.isHostRule || !modulePropBool(mod.bpmod, "host_supported") {
			continue
		}

		m := &Module{
			bpmod:      mod.bpmod,
			bpname:     mod.bpname,
			isHostRule: true,
		}
		err = m.translateRuleName()
		if err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}

	return
}

func (w *androidMkWriter) handleModule(inputModule *bpparser.Module) error {
	modules, err := w.mutateModule(newModule(inputModule))
	if err != nil {
		return err
	}

	for _, module := range modules {
		err := w.parsePropsAndWriteModule(module)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *androidMkWriter) handleSubdirs(value bpparser.Value) {
	subdirs := make([]string, 0, len(value.ListValue))
	for _, tok := range value.ListValue {
		subdirs = append(subdirs, tok.StringValue)
	}
	// The current makefile may be generated to outside the source tree (such as the out directory), with a different structure.
	fmt.Fprintf(w, "# Uncomment the following line if you really want to include subdir Android.mks.\n")
	fmt.Fprintf(w, "# include $(wildcard $(addsuffix $(LOCAL_PATH)/%s/, Android.mk))\n", strings.Join(subdirs, " "))
}

func (w *androidMkWriter) handleAssignment(assignment *bpparser.Assignment) error {
	if "subdirs" == assignment.Name.Name {
		w.handleSubdirs(assignment.OrigValue)
	} else if assignment.OrigValue.Type == bpparser.Map {
		// maps may be assigned in Soong, but can only be translated to .mk
		// in the context of the module
		w.mapScope[assignment.Name.Name] = assignment.OrigValue.MapValue
	} else {
		assigner := ":="
		if assignment.Assigner != "=" {
			assigner = assignment.Assigner
		}
		val, err := valueToString(assignment.OrigValue)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s %s %s\n", assignment.Name.Name, assigner, val)
	}

	return nil
}

func (w *androidMkWriter) handleLocalPath() error {
	if w.printedLocalPath {
		return nil
	}
	w.printedLocalPath = true

	localPath, err := filepath.Abs(w.path)
	if err != nil {
		return err
	}

	top, err := getTopOfAndroidTree(localPath)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(top, localPath)
	if err != nil {
		return err
	}

	w.WriteString("LOCAL_PATH := " + rel + "\n")
	w.WriteString("LOCAL_MODULE_MAKEFILE := $(lastword $(MAKEFILE_LIST))\n\n")
	return nil
}

func (w *androidMkWriter) write(writer io.Writer) (err error) {
	w.Writer = writer

	if err = w.handleLocalPath(); err != nil {
		return err
	}

	for _, block := range w.blueprint.Defs {
		switch block := block.(type) {
		case *bpparser.Module:
			err = w.handleModule(block)
		case *bpparser.Assignment:
			err = w.handleAssignment(block)
		default:
			return fmt.Errorf("Unhandled def %v", block)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func translate(androidBp, androidMk string) error {
	reader, err := os.Open(androidBp)
	if err != nil {
		return err
	}

	scope := bpparser.NewScope(nil)
	blueprint, errs := bpparser.Parse(androidBp, reader, scope)
	if len(errs) > 0 {
		return errs[0]
	}

	writer := &androidMkWriter{
		blueprint: blueprint,
		path:      path.Dir(androidBp),
		mapScope:  make(map[string][]*bpparser.Property),
	}

	buf := &bytes.Buffer{}

	err = writer.write(buf)
	if err != nil {
		os.Remove(androidMk)
		return err
	}

	f, err := os.Create(androidMk)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(buf.Bytes())

	return err
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Expected input and output filename arguments")
		os.Exit(1)
	}

	androidBp := os.Args[1]
	androidMk := os.Args[2]

	err := translate(androidBp, androidMk)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error translating %s: %s\n", androidBp, err.Error())
		os.Exit(1)
	}
}
