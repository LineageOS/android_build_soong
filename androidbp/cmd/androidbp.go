package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	bpparser "github.com/google/blueprint/parser"
)

type androidMkWriter struct {
	*bufio.Writer

	blueprint *bpparser.File
	path      string

	mapScope map[string][]*bpparser.Property
}

func valueToString(value bpparser.Value) string {
	if value.Variable != "" {
		return fmt.Sprintf("$(%s)", value.Variable)
	} else {
		switch value.Type {
		case bpparser.Bool:
			return fmt.Sprintf(`"%t"`, value.BoolValue)
		case bpparser.String:
			return fmt.Sprintf(`"%s"`, processWildcards(value.StringValue))
		case bpparser.List:
			return fmt.Sprintf("\\\n%s\n", listToMkString(value.ListValue))
		case bpparser.Map:
			return fmt.Sprintf("ERROR can't convert map to string")
		default:
			return fmt.Sprintf("ERROR: unsupported type %d", value.Type)
		}
	}
}

// TODO: handle non-recursive wildcards?
func processWildcards(s string) string {
	re := regexp.MustCompile("(.*)/\\*\\*/(.*)")
	submatches := re.FindAllStringSubmatch(s, -1)
	if submatches != nil && len(submatches[0]) > 2 {
		// Found a wildcard rule
		return fmt.Sprintf("$(call find-files-in-subdirs, $(LOCAL_PATH), %s, %s)",
			submatches[0][2], submatches[0][1])
	}

	return s
}

func listToMkString(list []bpparser.Value) string {
	lines := make([]string, 0, len(list))
	for _, tok := range list {
		if tok.Type == bpparser.String {
			lines = append(lines, fmt.Sprintf("\t\"%s\"", processWildcards(tok.StringValue)))
		} else {
			lines = append(lines, fmt.Sprintf("# ERROR: unsupported type %s in list",
				tok.Type.String()))
		}
	}

	return strings.Join(lines, " \\\n")
}

func translateTargetConditionals(props []*bpparser.Property,
	disabledBuilds map[string]bool, isHostRule bool) (computedProps []string) {
	for _, target := range props {
		conditionals := targetScopedPropertyConditionals
		if isHostRule {
			conditionals = hostScopedPropertyConditionals
		}

		conditional, ok := conditionals[target.Name.Name]
		if !ok {
			// not found
			conditional = fmt.Sprintf(
				"ifeq(true, true) # ERROR: unsupported conditional host [%s]",
				target.Name.Name)
		}

		var scopedProps []string
		for _, targetScopedProp := range target.Value.MapValue {
			if mkProp, ok := standardProperties[targetScopedProp.Name.Name]; ok {
				scopedProps = append(scopedProps, fmt.Sprintf("%s += %s",
					mkProp.string, valueToString(targetScopedProp.Value)))
			} else if "disabled" == targetScopedProp.Name.Name {
				if targetScopedProp.Value.BoolValue {
					disabledBuilds[target.Name.Name] = true
				} else {
					delete(disabledBuilds, target.Name.Name)
				}
			}
		}

		if len(scopedProps) > 0 {
			computedProps = append(computedProps, conditional)
			computedProps = append(computedProps, scopedProps...)
			computedProps = append(computedProps, "endif")
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
				} else {
					computedProps = append(computedProps, fmt.Sprintf("# ERROR: unsupported property %s", stdProp.Name.Name))
				}
			}
		}
	}
	return
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

func (w *androidMkWriter) handleModule(module *bpparser.Module) {
	moduleRule := fmt.Sprintf(module.Type.Name)
	if translation, ok := moduleTypeToRule[module.Type.Name]; ok {
		moduleRule = translation
	}

	isHostRule := strings.Contains(moduleRule, "HOST")
	hostSupported := false
	standardProps := make([]string, 0, len(module.Properties))
	disabledBuilds := make(map[string]bool)
	for _, prop := range module.Properties {
		if mkProp, ok := standardProperties[prop.Name.Name]; ok {
			standardProps = append(standardProps, fmt.Sprintf("%s := %s", mkProp.string, valueToString(prop.Value)))
		} else if suffixMap, ok := suffixProperties[prop.Name.Name]; ok {
			suffixProps := w.lookupMap(prop.Value)
			standardProps = append(standardProps, translateSuffixProperties(suffixProps, suffixMap)...)
		} else if "target" == prop.Name.Name {
			props := w.lookupMap(prop.Value)
			standardProps = append(standardProps, translateTargetConditionals(props, disabledBuilds, isHostRule)...)
		} else if "host_supported" == prop.Name.Name {
			hostSupported = prop.Value.BoolValue
		} else {
			standardProps = append(standardProps, fmt.Sprintf("# ERROR: Unsupported property %s", prop.Name.Name))
		}
	}

	// write out target build
	w.writeModule(moduleRule, standardProps, disabledBuilds, isHostRule)
	if hostSupported {
		hostModuleRule := "NO CORRESPONDING HOST RULE" + moduleRule
		if trans, ok := targetToHostModuleRule[moduleRule]; ok {
			hostModuleRule = trans
		}
		w.writeModule(hostModuleRule, standardProps,
			disabledBuilds, true)
	}
}

func (w *androidMkWriter) handleSubdirs(value bpparser.Value) {
	switch value.Type {
	case bpparser.String:
		fmt.Fprintf(w, "$(call all-makefiles-under, %s)\n", value.StringValue)
	case bpparser.List:
		for _, tok := range value.ListValue {
			fmt.Fprintf(w, "$(call all-makefiles-under, %s)\n", tok.StringValue)
		}
	}
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

func (w *androidMkWriter) write() {
	outFilePath := fmt.Sprintf("%s/Androidbp.mk", w.path)
	fmt.Printf("Writing %s\n", outFilePath)

	f, err := os.Create(outFilePath)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	w.Writer = bufio.NewWriter(f)

	w.WriteString("LOCAL_PATH := $(call my-dir)\n")

	for block := range w.iter() {
		switch block := block.(type) {
		case *bpparser.Module:
			w.handleModule(block)
		case *bpparser.Assignment:
			w.handleAssignment(block)
		case bpparser.Comment:
			w.handleComment(&block)
		}
	}

	if err = w.Flush(); err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("No filename supplied")
		return
	}

	reader, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	scope := bpparser.NewScope(nil)
	blueprint, errs := bpparser.Parse(os.Args[1], reader, scope)
	if len(errs) > 0 {
		fmt.Println("%d errors parsing %s", len(errs), os.Args[1])
		fmt.Println(errs)
		return
	}

	writer := &androidMkWriter{
		blueprint: blueprint,
		path:      path.Dir(os.Args[1]),
		mapScope:  make(map[string][]*bpparser.Property),
	}

	writer.write()
}
