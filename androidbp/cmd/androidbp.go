package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	bpparser "github.com/google/blueprint/parser"
)

type androidMkWriter struct {
	*bufio.Writer

	file *bpparser.File
	path string
}

func (w *androidMkWriter) valueToString(value bpparser.Value) string {
	if value.Variable != "" {
		return fmt.Sprintf("$(%s)", value.Variable)
	} else {
		switch value.Type {
		case bpparser.Bool:
			return fmt.Sprintf(`"%t"`, value.BoolValue)
		case bpparser.String:
			return fmt.Sprintf(`"%s"`, value.StringValue)
		case bpparser.List:
			return fmt.Sprintf("\\\n%s\n", w.listToMkString(value.ListValue))
		case bpparser.Map:
			w.errorf("maps not supported in assignment")
			return "ERROR: unsupported type map in assignment"
		}
	}

	return ""
}

func (w *androidMkWriter) listToMkString(list []bpparser.Value) string {
	lines := make([]string, 0, len(list))
	for _, tok := range list {
		lines = append(lines, fmt.Sprintf("\t\"%s\"", tok.StringValue))
	}

	return strings.Join(lines, " \\\n")
}

func (w *androidMkWriter) errorf(format string, values ...interface{}) {
	s := fmt.Sprintf(format, values)
	w.WriteString("# ANDROIDBP ERROR:\n")
	for _, line := range strings.Split(s, "\n") {
		fmt.Fprintf(w, "# %s\n", line)
	}
}

func (w *androidMkWriter) handleComment(comment *bpparser.Comment) {
	for _, c := range comment.Comment {
		mkComment := strings.Replace(c, "//", "#", 1)
		// TODO: handle /* comments?
		fmt.Fprintf(w, "%s\n", mkComment)
	}
}

func (w *androidMkWriter) handleModule(module *bpparser.Module) {
	if moduleName, ok := moduleTypes[module.Type.Name]; ok {
		w.WriteString("include $(CLEAR_VARS)\n")
		standardProps := make([]string, 0, len(module.Properties))
		//condProps := make([]string, len(module.Properties))
		for _, prop := range module.Properties {
			if mkProp, ok := standardProperties[prop.Name.Name]; ok {
				standardProps = append(standardProps, fmt.Sprintf("%s := %s", mkProp.string,
					w.valueToString(prop.Value)))
			}
		}

		mkModule := strings.Join(standardProps, "\n")
		w.WriteString(mkModule)

		fmt.Fprintf(w, "include $(%s)\n\n", moduleName)
	} else {
		w.errorf("Unsupported module %s", module.Type.Name)
	}
}

func (w *androidMkWriter) handleAssignment(assignment *bpparser.Assignment) {
	assigner := ":="
	if assignment.Assigner != "=" {
		assigner = assignment.Assigner
	}
	fmt.Fprintf(w, "%s %s %s\n", assignment.Name.Name, assigner,
		w.valueToString(assignment.OrigValue))
}

func (w *androidMkWriter) iter() <-chan interface{} {
	ch := make(chan interface{}, len(w.file.Comments)+len(w.file.Defs))
	go func() {
		commIdx := 0
		defsIdx := 0
		for defsIdx < len(w.file.Defs) || commIdx < len(w.file.Comments) {
			if defsIdx == len(w.file.Defs) {
				ch <- w.file.Comments[commIdx]
				commIdx++
			} else if commIdx == len(w.file.Comments) {
				ch <- w.file.Defs[defsIdx]
				defsIdx++
			} else {
				commentsPos := 0
				defsPos := 0

				def := w.file.Defs[defsIdx]
				switch def := def.(type) {
				case *bpparser.Module:
					defsPos = def.LbracePos.Line
				case *bpparser.Assignment:
					defsPos = def.Pos.Line
				}

				comment := w.file.Comments[commIdx]
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
	outFilePath := fmt.Sprintf("%s/Android.mk.out", w.path)
	fmt.Printf("Writing %s\n", outFilePath)

	f, err := os.Create(outFilePath)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

	w.Writer = bufio.NewWriter(f)

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
	file, errs := bpparser.Parse(os.Args[1], reader, scope)
	if len(errs) > 0 {
		fmt.Println("%d errors parsing %s", len(errs), os.Args[1])
		fmt.Println(errs)
		return
	}

	writer := &androidMkWriter{
		file: file,
		path: path.Dir(os.Args[1]),
	}

	writer.write()
}
