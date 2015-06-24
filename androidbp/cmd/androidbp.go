package main

import (
	"bufio"
	"errors"
	"fmt"
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

func (m *Module) translateRuleName() {
	name := fmt.Sprintf(m.bpname)
	if translation, ok := moduleTypeToRule[m.bpname]; ok {
		name = translation
	}

	if m.isHostRule {
		if trans, ok := targetToHostModuleRule[name]; ok {
			name = trans
		} else {
			name = "NO CORRESPONDING HOST RULE" + name
		}
	} else {
		m.isHostRule = strings.Contains(name, "HOST")
	}

	m.mkname = name
}

type androidMkWriter struct {
	*bufio.Writer

	blueprint *bpparser.File
	path      string

	printedLocalPath bool

	mapScope map[string][]*bpparser.Property
}

func valueToString(value bpparser.Value) string {
	if value.Variable != "" {
		return fmt.Sprintf("$(%s)", value.Variable)
	} else if value.Expression != nil {
		if value.Expression.Operator != '+' {
			panic(fmt.Errorf("unexpected operator '%c'", value.Expression.Operator))
		}
		return fmt.Sprintf("%s%s",
			valueToString(value.Expression.Args[0]),
			valueToString(value.Expression.Args[1]))
	} else {
		switch value.Type {
		case bpparser.Bool:
			return fmt.Sprintf("%t", value.BoolValue)
		case bpparser.String:
			return fmt.Sprintf("%s", processWildcards(value.StringValue))
		case bpparser.List:
			return fmt.Sprintf("\\\n%s", listToMkString(value.ListValue))
		case bpparser.Map:
			return fmt.Sprintf("ERROR can't convert map to string")
		default:
			return fmt.Sprintf("ERROR: unsupported type %d", value.Type)
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

func listToMkString(list []bpparser.Value) string {
	lines := make([]string, 0, len(list))
	for _, tok := range list {
		lines = append(lines, fmt.Sprintf("    %s", valueToString(tok)))
	}

	return strings.Join(lines, " \\\n")
}

func translateTargetConditionals(props []*bpparser.Property,
	disabledBuilds map[string]bool, isHostRule bool) (computedProps []string) {
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
				// not found
				conditional = fmt.Sprintf(
					"ifeq(true, true) # ERROR: unsupported conditional [%s]",
					target.Name.Name)
			}
		}

		var scopedProps []string
		for _, targetScopedProp := range target.Value.MapValue {
			if mkProp, ok := standardProperties[targetScopedProp.Name.Name]; ok {
				scopedProps = append(scopedProps, fmt.Sprintf("%s += %s",
					mkProp.string, valueToString(targetScopedProp.Value)))
			} else if rwProp, ok := rewriteProperties[targetScopedProp.Name.Name]; ok {
				scopedProps = append(scopedProps, rwProp.f(rwProp.string, targetScopedProp, nil)...)
			} else if "disabled" == targetScopedProp.Name.Name {
				if targetScopedProp.Value.BoolValue {
					disabledBuilds[target.Name.Name] = true
				} else {
					delete(disabledBuilds, target.Name.Name)
				}
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
	suffixMap map[string]string) (computedProps []string) {
	for _, suffixProp := range suffixProps {
		if suffix, ok := suffixMap[suffixProp.Name.Name]; ok {
			for _, stdProp := range suffixProp.Value.MapValue {
				if mkProp, ok := standardProperties[stdProp.Name.Name]; ok {
					computedProps = append(computedProps, fmt.Sprintf("%s_%s := %s", mkProp.string, suffix, valueToString(stdProp.Value)))
				} else if rwProp, ok := rewriteProperties[stdProp.Name.Name]; ok {
					computedProps = append(computedProps, rwProp.f(rwProp.string, stdProp, &suffix)...)
				} else {
					computedProps = append(computedProps, fmt.Sprintf("# ERROR: unsupported property %s", stdProp.Name.Name))
				}
			}
		}
	}
	return
}

func prependLocalPath(name string, prop *bpparser.Property, suffix *string) (computedProps []string) {
	if suffix != nil {
		name += "_" + *suffix
	}
	return []string{
		fmt.Sprintf("%s := $(addprefix $(LOCAL_PATH)/,%s)\n", name, valueToString(prop.Value)),
	}
}

func prependLocalModule(name string, prop *bpparser.Property, suffix *string) (computedProps []string) {
	if suffix != nil {
		name += "_" + *suffix
	}
	return []string {
		fmt.Sprintf("%s := $(LOCAL_MODULE)%s\n", name, valueToString(prop.Value)),
	}
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

func (w *androidMkWriter) handleComment(comment *bpparser.Comment) {
	for _, c := range comment.Comment {
		fmt.Fprintf(w, "#%s\n", c)
	}
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

func (w *androidMkWriter) parsePropsAndWriteModule(module *Module) {
	standardProps := make([]string, 0, len(module.bpmod.Properties))
	disabledBuilds := make(map[string]bool)
	for _, prop := range module.bpmod.Properties {
		if mkProp, ok := standardProperties[prop.Name.Name]; ok {
			standardProps = append(standardProps, fmt.Sprintf("%s := %s", mkProp.string, valueToString(prop.Value)))
		} else if rwProp, ok := rewriteProperties[prop.Name.Name]; ok {
			standardProps = append(standardProps, rwProp.f(rwProp.string, prop, nil)...)
		} else if suffixMap, ok := suffixProperties[prop.Name.Name]; ok {
			suffixProps := w.lookupMap(prop.Value)
			standardProps = append(standardProps, translateSuffixProperties(suffixProps, suffixMap)...)
		} else if "target" == prop.Name.Name {
			props := w.lookupMap(prop.Value)
			standardProps = append(standardProps, translateTargetConditionals(props, disabledBuilds, module.isHostRule)...)
		} else if "host_supported" == prop.Name.Name {
		} else {
			standardProps = append(standardProps, fmt.Sprintf("# ERROR: Unsupported property %s", prop.Name.Name))
		}
	}

	w.writeModule(module.mkname, standardProps, disabledBuilds, module.isHostRule)
}

func (w *androidMkWriter) mutateModule(module *Module) (modules []*Module) {
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
		mod.translateRuleName()
		if mod.isHostRule || !modulePropBool(mod.bpmod, "host_supported") {
			continue
		}

		m := &Module{
			bpmod:      mod.bpmod,
			bpname:     mod.bpname,
			isHostRule: true,
		}
		m.translateRuleName()
		modules = append(modules, m)
	}

	return
}

func (w *androidMkWriter) handleModule(inputModule *bpparser.Module) {
	modules := w.mutateModule(newModule(inputModule))

	for _, module := range modules {
		w.parsePropsAndWriteModule(module)
	}
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

func (w *androidMkWriter) handleAssignment(assignment *bpparser.Assignment) {
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
		fmt.Fprintf(w, "%s %s %s\n", assignment.Name.Name, assigner,
			valueToString(assignment.OrigValue))
	}
}

func (w *androidMkWriter) iter() <-chan interface{} {
	ch := make(chan interface{}, len(w.blueprint.Comments)+len(w.blueprint.Defs))
	go func() {
		commIdx := 0
		defsIdx := 0
		for defsIdx < len(w.blueprint.Defs) || commIdx < len(w.blueprint.Comments) {
			if defsIdx == len(w.blueprint.Defs) {
				ch <- w.blueprint.Comments[commIdx]
				commIdx++
			} else if commIdx == len(w.blueprint.Comments) {
				ch <- w.blueprint.Defs[defsIdx]
				defsIdx++
			} else {
				commentsPos := 0
				defsPos := 0

				def := w.blueprint.Defs[defsIdx]
				switch def := def.(type) {
				case *bpparser.Module:
					defsPos = def.LbracePos.Line
				case *bpparser.Assignment:
					defsPos = def.Pos.Line
				}

				comment := w.blueprint.Comments[commIdx]
				commentsPos = comment.Pos.Line

				if commentsPos < defsPos {
					commIdx++
					ch <- comment
				} else {
					defsIdx++
					ch <- def
				}
			}
		}
		close(ch)
	}()
	return ch
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

func (w *androidMkWriter) write(androidMk string) error {
	fmt.Printf("Writing %s\n", androidMk)

	f, err := os.Create(androidMk)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	w.Writer = bufio.NewWriter(f)

	for block := range w.iter() {
		switch block := block.(type) {
		case *bpparser.Module:
			if err := w.handleLocalPath(); err != nil {
				return err
			}
			w.handleModule(block)
		case *bpparser.Assignment:
			if err := w.handleLocalPath(); err != nil {
				return err
			}
			w.handleAssignment(block)
		case bpparser.Comment:
			w.handleComment(&block)
		}
	}

	if err = w.Flush(); err != nil {
		panic(err)
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("No filename supplied")
		os.Exit(1)
	}

	androidBp := os.Args[1]
	var androidMk string
	if len(os.Args) >= 3 {
		androidMk = os.Args[2]
	} else {
		androidMk = androidBp + ".mk"
	}

	reader, err := os.Open(androidBp)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	scope := bpparser.NewScope(nil)
	blueprint, errs := bpparser.Parse(androidBp, reader, scope)
	if len(errs) > 0 {
		fmt.Println("%d errors parsing %s", len(errs), androidBp)
		fmt.Println(errs)
		os.Exit(1)
	}

	writer := &androidMkWriter{
		blueprint: blueprint,
		path:      path.Dir(androidBp),
		mapScope:  make(map[string][]*bpparser.Property),
	}

	err = writer.write(androidMk)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
