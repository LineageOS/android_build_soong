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

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"android/soong/compliance/license_metadata_proto"
	"android/soong/response"
)

func newMultiString(flags *flag.FlagSet, name, usage string) *multiString {
	var f multiString
	flags.Var(&f, name, usage)
	return &f
}

type multiString []string

func (ms *multiString) String() string     { return strings.Join(*ms, ", ") }
func (ms *multiString) Set(s string) error { *ms = append(*ms, s); return nil }

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

	installed := flags.String("i", "", "installed target")
	sources := newMultiString(flags, "s", "source (input) file")
	dep := flags.String("d", "", "license metadata file dependency")
	outFile := flags.String("o", "", "output file")

	flags.Parse(expandedArgs)

	if len(*dep) == 0 || len(*installed) == 0 || len(*sources) == 0 {
		flags.Usage()
		if len(*dep) == 0 {
			fmt.Fprintf(os.Stderr, "source license metadata (-d flag) required\n")
		}
		if len(*sources) == 0 {
			fmt.Fprintf(os.Stderr, "source copy (-s flag required\n")
		}
		if len(*installed) == 0 {
			fmt.Fprintf(os.Stderr, "installed copy (-i flag) required\n")
		}
		os.Exit(1)
	}

	src_metadata := license_metadata_proto.LicenseMetadata{}
	err := readMetadata(*dep, &src_metadata)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(2)
	}

	metadata := src_metadata
	metadata.Built = nil
	metadata.InstallMap = nil
	metadata.Installed = []string{*installed}
	metadata.Sources = *sources
	metadata.Deps = []*license_metadata_proto.AnnotatedDependency{&license_metadata_proto.AnnotatedDependency{
		File:        proto.String(*dep),
		Annotations: []string{"static"},
	}}

	err = writeMetadata(*outFile, &metadata)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(2)
	}
}

func readMetadata(file string, metadata *license_metadata_proto.LicenseMetadata) error {
	if file == "" {
		return fmt.Errorf("source metadata file (-d) required")
	}
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading textproto %q: %w", file, err)
	}

	err = prototext.Unmarshal(buf, metadata)
	if err != nil {
		return fmt.Errorf("error unmarshalling textproto: %w", err)
	}

	return nil
}

func writeMetadata(file string, metadata *license_metadata_proto.LicenseMetadata) error {
	buf, err := prototext.MarshalOptions{Multiline: true}.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("error marshalling textproto: %w", err)
	}

	if file != "" {
		err = ioutil.WriteFile(file, buf, 0666)
		if err != nil {
			return fmt.Errorf("error writing textproto %q: %w", file, err)
		}
	} else {
		_, _ = os.Stdout.Write(buf)
	}

	return nil
}
