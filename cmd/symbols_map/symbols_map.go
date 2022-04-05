// Copyright 2022 Google Inc. All rights reserved.
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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"android/soong/cmd/symbols_map/symbols_map_proto"
	"android/soong/response"

	"github.com/google/blueprint/pathtools"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// This tool is used to extract a hash from an elf file or an r8 dictionary and store it as a
// textproto, or to merge multiple textprotos together.

func main() {
	var expandedArgs []string
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "@") {
			f, err := os.Open(strings.TrimPrefix(arg, "@"))
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}

			respArgs, err := response.ReadRspFile(f)
			f.Close()
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
			expandedArgs = append(expandedArgs, respArgs...)
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	flags := flag.NewFlagSet("flags", flag.ExitOnError)

	// Hide the flag package to prevent accidental references to flag instead of flags.
	flag := struct{}{}
	_ = flag

	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flags.Output(), "  %s -elf|-r8 <input file> [-write_if_changed] <output file>\n", os.Args[0])
		fmt.Fprintf(flags.Output(), "  %s -merge <output file> [-write_if_changed] [-ignore_missing_files] [-strip_prefix <prefix>] [<input file>...]\n", os.Args[0])
		fmt.Fprintln(flags.Output())

		flags.PrintDefaults()
	}

	elfFile := flags.String("elf", "", "extract identifier from an elf file")
	r8File := flags.String("r8", "", "extract identifier from an r8 dictionary")
	merge := flags.String("merge", "", "merge multiple identifier protos")

	writeIfChanged := flags.Bool("write_if_changed", false, "only write output file if it is modified")
	ignoreMissingFiles := flags.Bool("ignore_missing_files", false, "ignore missing input files in merge mode")
	stripPrefix := flags.String("strip_prefix", "", "prefix to strip off of the location field in merge mode")

	flags.Parse(expandedArgs)

	if *merge != "" {
		// If merge mode was requested perform the merge and exit early.
		err := mergeProtos(*merge, flags.Args(), *stripPrefix, *writeIfChanged, *ignoreMissingFiles)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to merge protos: %s", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if *elfFile == "" && *r8File == "" {
		fmt.Fprintf(os.Stderr, "-elf or -r8 argument is required\n")
		flags.Usage()
		os.Exit(1)
	}

	if *elfFile != "" && *r8File != "" {
		fmt.Fprintf(os.Stderr, "only one of -elf or -r8 argument is allowed\n")
		flags.Usage()
		os.Exit(1)
	}

	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(1)
	}

	output := flags.Arg(0)

	var identifier string
	var location string
	var typ symbols_map_proto.Mapping_Type
	var err error

	if *elfFile != "" {
		typ = symbols_map_proto.Mapping_ELF
		location = *elfFile
		identifier, err = elfIdentifier(*elfFile, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading elf identifier: %s\n", err)
			os.Exit(1)
		}
	} else if *r8File != "" {
		typ = symbols_map_proto.Mapping_R8
		identifier, err = r8Identifier(*r8File)
		location = *r8File
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading r8 identifier: %s\n", err)
			os.Exit(1)
		}
	} else {
		panic("shouldn't get here")
	}

	mapping := symbols_map_proto.Mapping{
		Identifier: proto.String(identifier),
		Location:   proto.String(location),
		Type:       typ.Enum(),
	}

	err = writeTextProto(output, &mapping, *writeIfChanged)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %s\n", err)
		os.Exit(1)
	}
}

// writeTextProto writes a proto to an output file as a textproto, optionally leaving the file
// unmodified if it was already up to date.
func writeTextProto(output string, message proto.Message, writeIfChanged bool) error {
	marshaller := prototext.MarshalOptions{Multiline: true}
	data, err := marshaller.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshalling textproto: %w", err)
	}

	if writeIfChanged {
		err = pathtools.WriteFileIfChanged(output, data, 0666)
	} else {
		err = ioutil.WriteFile(output, data, 0666)
	}

	if err != nil {
		return fmt.Errorf("error writing to %s: %w\n", output, err)
	}

	return nil
}

// mergeProtos merges a list of textproto files containing Mapping messages into a single textproto
// containing a Mappings message.
func mergeProtos(output string, inputs []string, stripPrefix string, writeIfChanged bool, ignoreMissingFiles bool) error {
	mappings := symbols_map_proto.Mappings{}
	for _, input := range inputs {
		mapping := symbols_map_proto.Mapping{}
		data, err := ioutil.ReadFile(input)
		if err != nil {
			if ignoreMissingFiles && os.IsNotExist(err) {
				// Merge mode is used on a list of files in the packaging directory.  If multiple
				// goals are included on the build command line, for example `dist` and `tests`,
				// then the symbols packaging rule for `dist` can run while a dependency of `tests`
				// is modifying the symbols packaging directory.  That can result in a file that
				// existed when the file list was generated being deleted as part of updating it,
				// resulting in sporadic ENOENT errors.  Ignore them if -ignore_missing_files
				// was passed on the command line.
				continue
			}
			return fmt.Errorf("failed to read %s: %w", input, err)
		}
		err = prototext.Unmarshal(data, &mapping)
		if err != nil {
			return fmt.Errorf("failed to parse textproto %s: %w", input, err)
		}
		if stripPrefix != "" && mapping.Location != nil {
			mapping.Location = proto.String(strings.TrimPrefix(*mapping.Location, stripPrefix))
		}
		mappings.Mappings = append(mappings.Mappings, &mapping)
	}

	return writeTextProto(output, &mappings, writeIfChanged)
}
