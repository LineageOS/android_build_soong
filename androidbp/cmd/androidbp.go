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
	"text/scanner"

	"github.com/google/blueprint"
	bpparser "github.com/google/blueprint/parser"
)

var recursiveSubdirRegex *regexp.Regexp = regexp.MustCompile("(.+)/\\*\\*/(.+)")

type androidMkWriter struct {
	io.Writer

	blueprint *bpparser.File
	path      string
}

type propAssignment struct {
	name, assigner, value string
}

func (a propAssignment) assignmentWithSuffix(suffix string) string {
	if suffix != "" {
		a.name = a.name + "_" + suffix
	}
	return a.name + " " + a.assigner + " " + a.value
}

func (a propAssignment) assignment() string {
	return a.assignmentWithSuffix("")
}

func (w *androidMkWriter) WriteString(s string) (int, error) {
	return io.WriteString(w.Writer, s)
}

func valueToString(value bpparser.Value) (string, error) {
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

func appendValueToValue(dest bpparser.Value, src bpparser.Value) (bpparser.Value, error) {
	if src.Type != dest.Type {
		return bpparser.Value{}, fmt.Errorf("ERROR: source and destination types don't match")
	}
	switch dest.Type {
	case bpparser.List:
		dest.ListValue = append(dest.ListValue, src.ListValue...)
		return dest, nil
	case bpparser.String:
		dest.StringValue += src.StringValue
		return dest, nil
	default:
		return bpparser.Value{}, fmt.Errorf("ERROR: unsupported append with type %s", dest.Type.String())
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
			if assignment, ok, err := translateSingleProperty(targetScopedProp); err != nil {
				return nil, err
			} else if ok {
				scopedProps = append(scopedProps, assignment.assignment())
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

var secondTargetReplacer = strings.NewReplacer("TARGET_", "TARGET_2ND_")

func translateSuffixProperties(suffixProps []*bpparser.Property,
	suffixMap map[string]string) (computedProps []string, err error) {
	for _, suffixProp := range suffixProps {
		if suffix, ok := suffixMap[suffixProp.Name.Name]; ok {
			for _, stdProp := range suffixProp.Value.MapValue {
				if assignment, ok, err := translateSingleProperty(stdProp); err != nil {
					return nil, err
				} else if ok {
					computedProps = append(computedProps, assignment.assignmentWithSuffix(suffix))
				} else {
					return nil, fmt.Errorf("Unsupported property %q", stdProp.Name.Name)
				}
			}
		} else if variant, ok := cpuVariantConditionals[suffixProp.Name.Name]; ok {
			var conditionalProps []propAssignment
			for _, stdProp := range suffixProp.Value.MapValue {
				if assignment, ok, err := translateSingleProperty(stdProp); err != nil {
					return nil, err
				} else if ok {
					conditionalProps = append(conditionalProps, assignment)
				} else {
					return nil, fmt.Errorf("Unsupported property %q", stdProp.Name.Name)
				}
			}

			appendComputedProps := func() {
				computedProps = append(computedProps, variant.conditional)
				for _, prop := range conditionalProps {
					prop.assigner = "+="
					computedProps = append(computedProps, prop.assignmentWithSuffix(variant.suffix))
				}
				computedProps = append(computedProps, "endif")
			}

			appendComputedProps()
			if variant.secondArch {
				variant.conditional = secondTargetReplacer.Replace(variant.conditional)
				variant.suffix = secondTargetReplacer.Replace(variant.suffix)
				appendComputedProps()
			}
		} else {
			return nil, fmt.Errorf("Unsupported suffix property %q", suffixProp.Name.Name)
		}
	}
	return
}

func translateSingleProperty(prop *bpparser.Property) (propAssignment, bool, error) {
	var assignment propAssignment
	if mkProp, ok := standardProperties[prop.Name.Name]; ok {
		name := mkProp.string
		val, err := valueToString(prop.Value)
		if err != nil {
			return propAssignment{}, false, err
		}
		assignment = propAssignment{name, ":=", val}
	} else if rwProp, ok := rewriteProperties[prop.Name.Name]; ok {
		val, err := valueToString(prop.Value)
		if err != nil {
			return propAssignment{}, false, err
		}
		assignment, err = rwProp.f(rwProp.string, prop, val)
		if err != nil {
			return propAssignment{}, false, err
		}
	} else {
		// Unhandled, return false with no error to tell the caller to handle it
		return propAssignment{}, false, nil
	}
	return assignment, true, nil
}

func appendAssign(name string, prop *bpparser.Property, val string) (propAssignment, error) {
	return propAssignment{name, "+=", val}, nil
}

func prependLocalPath(name string, prop *bpparser.Property, val string) (propAssignment, error) {
	return propAssignment{name, "+=", fmt.Sprintf("$(addprefix $(LOCAL_PATH)/,%s)", val)}, nil
}

func prependLocalModule(name string, prop *bpparser.Property, val string) (propAssignment, error) {
	return propAssignment{name, ":=", "$(LOCAL_MODULE)" + val}, nil
}

func versionScript(name string, prop *bpparser.Property, val string) (propAssignment, error) {
	return propAssignment{name, "+=", "-Wl,--version-script,$(LOCAL_PATH)/" + val}, nil
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
		if assignment, ok, err := translateSingleProperty(prop); err != nil {
			return err
		} else if ok {
			standardProps = append(standardProps, assignment.assignment())
		} else if suffixMap, ok := suffixProperties[prop.Name.Name]; ok {
			props, err := translateSuffixProperties(prop.Value.MapValue, suffixMap)
			if err != nil {
				return err
			}
			standardProps = append(standardProps, props...)
		} else if "target" == prop.Name.Name {
			props, err := translateTargetConditionals(prop.Value.MapValue, disabledBuilds, module.isHostRule)
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

func canUseWholeStaticLibrary(m *Module) (bool, error) {
	ret := true

	isCompatible := func(props Properties, prop *bpparser.Property) error {
		for _, p := range prop.Value.MapValue {
			if p.Name.Name == "cflags" {
				ret = false
				return nil
			}
			if prop.Name.Name == "static" {
				if p.Name.Name == "srcs" {
					ret = false
					return nil
				}
			}
		}
		return nil
	}

	err := m.IterateArchPropertiesWithName("shared", isCompatible)
	if err != nil {
		return false, err
	}
	err = m.IterateArchPropertiesWithName("static", isCompatible)
	if err != nil {
		return false, err
	}

	return ret, nil
}

func (w *androidMkWriter) mutateModule(module *Module) (modules []*Module, err error) {
	modules = []*Module{module}

	if module.bpname == "cc_library" {
		modules = []*Module{
			newModule(module.bpmod),
			newModule(module.bpmod),
		}

		ccLinkageCopy := func(props Properties, prop *bpparser.Property) error {
			for _, p := range prop.Value.MapValue {
				err := props.AppendToProp(p.Name.Name, p)
				if err != nil {
					return err
				}
			}
			props.DeleteProp(prop.Name.Name)
			return nil
		}
		deleteProp := func(props Properties, prop *bpparser.Property) error {
			props.DeleteProp(prop.Name.Name)
			return nil
		}

		if ok, err := canUseWholeStaticLibrary(module); err != nil {
			return nil, err
		} else if ok {
			err = modules[0].IterateArchPropertiesWithName("srcs", deleteProp)
			if err != nil {
				return nil, err
			}

			if nameProp, ok := modules[0].Properties().Prop("name"); !ok {
				return nil, fmt.Errorf("Can't find name property")
			} else {
				modules[0].Properties().AppendToProp("whole_static_libs", &bpparser.Property{
					Value: bpparser.Value{
						Type: bpparser.List,
						ListValue: []bpparser.Value{
							nameProp.Value.Copy(),
						},
					},
				})
			}
		}

		modules[0].bpname = "cc_library_shared"
		err := modules[0].IterateArchPropertiesWithName("shared", ccLinkageCopy)
		if err != nil {
			return nil, err
		}
		err = modules[0].IterateArchPropertiesWithName("static", deleteProp)
		if err != nil {
			return nil, err
		}

		modules[1].bpname = "cc_library_static"
		err = modules[1].IterateArchPropertiesWithName("shared", deleteProp)
		if err != nil {
			return nil, err
		}
		err = modules[1].IterateArchPropertiesWithName("static", ccLinkageCopy)
		if err != nil {
			return nil, err
		}
	}

	for _, mod := range modules {
		err := mod.translateRuleName()
		if err != nil {
			return nil, err
		}
		if mod.isHostRule || !mod.PropBool("host_supported") {
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
	comment := w.getCommentBlock(inputModule.Type.Pos)
	if translation, translated, err := getCommentTranslation(comment); err != nil {
		return err
	} else if translated {
		w.WriteString(translation)
		return nil
	}

	if ignoredModuleType[inputModule.Type.Name] {
		return nil
	}

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

func (w *androidMkWriter) handleLocalPath() error {
	w.WriteString("LOCAL_PATH := " + w.path + "\n")
	w.WriteString("LOCAL_MODULE_MAKEFILE := $(lastword $(MAKEFILE_LIST))\n\n")
	return nil
}

// Returns any block comment on the line preceding pos as a string
func (w *androidMkWriter) getCommentBlock(pos scanner.Position) string {
	var buf []byte

	comments := w.blueprint.Comments
	for i, c := range comments {
		if c.EndLine() == pos.Line-1 {
			line := pos.Line
			for j := i; j >= 0; j-- {
				c = comments[j]
				if c.EndLine() == line-1 {
					buf = append([]byte(c.Text()), buf...)
					line = c.Pos.Line
				} else {
					break
				}
			}
		}
	}

	return string(buf)
}

func getCommentTranslation(comment string) (string, bool, error) {
	lines := strings.Split(comment, "\n")

	if directive, i, err := getCommentDirective(lines); err != nil {
		return "", false, err
	} else if directive != "" {
		switch directive {
		case "ignore":
			return "", true, nil
		case "start":
			return getCommentTranslationBlock(lines[i+1:])
		case "end":
			return "", false, fmt.Errorf("Unexpected Android.mk:end translation directive")
		default:
			return "", false, fmt.Errorf("Unknown Android.mk module translation directive %q", directive)
		}
	}

	return "", false, nil
}

func getCommentTranslationBlock(lines []string) (string, bool, error) {
	var buf []byte

	for _, line := range lines {
		if directive := getLineCommentDirective(line); directive != "" {
			switch directive {
			case "end":
				return string(buf), true, nil
			default:
				return "", false, fmt.Errorf("Unexpected Android.mk translation directive %q inside start", directive)
			}
		} else {
			buf = append(buf, line...)
			buf = append(buf, '\n')
		}
	}

	return "", false, fmt.Errorf("Missing Android.mk:end translation directive")
}

func getCommentDirective(lines []string) (directive string, n int, err error) {
	for i, line := range lines {
		if directive := getLineCommentDirective(line); directive != "" {
			return strings.ToLower(directive), i, nil
		}
	}

	return "", -1, nil
}

func getLineCommentDirective(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "Android.mk:") {
		line = strings.TrimPrefix(line, "Android.mk:")
		line = strings.TrimSpace(line)
		return line
	}

	return ""
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
			// Nothing
		default:
			return fmt.Errorf("Unhandled def %v", block)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func translate(rootFile, androidBp, androidMk string) error {

	ctx := blueprint.NewContext()

	var blueprintFile *bpparser.File

	_, errs := ctx.WalkBlueprintsFiles(rootFile, func(file *bpparser.File) {
		if file.Name == androidBp {
			blueprintFile = file
		}
	})
	if len(errs) > 0 {
		return errs[0]
	}

	if blueprintFile == nil {
		return fmt.Errorf("File %q wasn't parsed from %q", androidBp, rootFile)
	}

	writer := &androidMkWriter{
		blueprint: blueprintFile,
		path:      path.Dir(androidBp),
	}

	buf := &bytes.Buffer{}

	err := writer.write(buf)
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
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Expected root Android.bp, input and output filename arguments")
		os.Exit(1)
	}

	rootFile := os.Args[1]
	androidBp, err := filepath.Rel(filepath.Dir(rootFile), os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Android.bp file %q is not relative to %q: %s\n",
			os.Args[2], rootFile, err.Error())
		os.Exit(1)
	}
	androidMk := os.Args[3]

	err = translate(rootFile, androidBp, androidMk)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error translating %s: %s\n", androidBp, err.Error())
		os.Exit(1)
	}
}
