// Copyright 2017 Google Inc. All rights reserved.
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

package jar

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"text/scanner"
	"time"
	"unicode"

	"android/soong/third_party/zip"
)

const (
	MetaDir         = "META-INF/"
	ManifestFile    = MetaDir + "MANIFEST.MF"
	ModuleInfoClass = "module-info.class"
)

var DefaultTime = time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)

var MetaDirExtra = [2]byte{0xca, 0xfe}

// EntryNamesLess tells whether <filepathA> should precede <filepathB> in
// the order of files with a .jar
func EntryNamesLess(filepathA string, filepathB string) (less bool) {
	diff := index(filepathA) - index(filepathB)
	if diff == 0 {
		return filepathA < filepathB
	}
	return diff < 0
}

// Treats trailing * as a prefix match
func patternMatch(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	} else {
		return name == pattern
	}
}

var jarOrder = []string{
	MetaDir,
	ManifestFile,
	MetaDir + "*",
	"*",
}

func index(name string) int {
	for i, pattern := range jarOrder {
		if patternMatch(pattern, name) {
			return i
		}
	}
	panic(fmt.Errorf("file %q did not match any pattern", name))
}

func MetaDirFileHeader() *zip.FileHeader {
	dirHeader := &zip.FileHeader{
		Name:  MetaDir,
		Extra: []byte{MetaDirExtra[1], MetaDirExtra[0], 0, 0},
	}
	dirHeader.SetMode(0755 | os.ModeDir)
	dirHeader.SetModTime(DefaultTime)

	return dirHeader
}

// Create a manifest zip header and contents using the provided contents if any.
func ManifestFileContents(contents []byte) (*zip.FileHeader, []byte, error) {
	b, err := manifestContents(contents)
	if err != nil {
		return nil, nil, err
	}

	fh := &zip.FileHeader{
		Name:               ManifestFile,
		Method:             zip.Store,
		UncompressedSize64: uint64(len(b)),
	}
	fh.SetMode(0644)
	fh.SetModTime(DefaultTime)

	return fh, b, nil
}

// Create manifest contents, using the provided contents if any.
func manifestContents(contents []byte) ([]byte, error) {
	manifestMarker := []byte("Manifest-Version:")
	header := append(manifestMarker, []byte(" 1.0\nCreated-By: soong_zip\n")...)

	var finalBytes []byte
	if !bytes.Contains(contents, manifestMarker) {
		finalBytes = append(append(header, contents...), byte('\n'))
	} else {
		finalBytes = contents
	}

	return finalBytes, nil
}

var javaIgnorableIdentifier = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x00, 0x08, 1},
		{0x0e, 0x1b, 1},
		{0x7f, 0x9f, 1},
	},
	LatinOffset: 3,
}

func javaIdentRune(ch rune, i int) bool {
	if unicode.IsLetter(ch) {
		return true
	}
	if unicode.IsDigit(ch) && i > 0 {
		return true
	}

	if unicode.In(ch,
		unicode.Nl, // letter number
		unicode.Sc, // currency symbol
		unicode.Pc, // connecting punctuation
	) {
		return true
	}

	if unicode.In(ch,
		unicode.Cf, // format
		unicode.Mc, // combining mark
		unicode.Mn, // non-spacing mark
		javaIgnorableIdentifier,
	) && i > 0 {
		return true
	}

	return false
}

// JavaPackage parses the package out of a java source file by looking for the package statement, or the first valid
// non-package statement, in which case it returns an empty string for the package.
func JavaPackage(r io.Reader, src string) (string, error) {
	var s scanner.Scanner
	var sErr error

	s.Init(r)
	s.Filename = src
	s.Error = func(s *scanner.Scanner, msg string) {
		sErr = fmt.Errorf("error parsing %q: %s", src, msg)
	}
	s.IsIdentRune = javaIdentRune

	var tok rune
	for {
		tok = s.Scan()
		if sErr != nil {
			return "", sErr
		}
		// If the first token is an annotation, it could be annotating a package declaration, so consume them.
		// Note that this does not support "complex" annotations with attributes, e.g. @Foo(x=y).
		if tok != '@' {
			break
		}
		tok = s.Scan()
		if tok != scanner.Ident || sErr != nil {
			return "", fmt.Errorf("expected annotation identifier, got @%v", tok)
		}
	}

	if tok == scanner.Ident {
		switch s.TokenText() {
		case "package":
		// Nothing
		case "import":
			// File has no package statement, first keyword is an import
			return "", nil
		case "class", "enum", "interface":
			// File has no package statement, first keyword is a type declaration
			return "", nil
		case "public", "protected", "private", "abstract", "static", "final", "strictfp":
			// File has no package statement, first keyword is a modifier
			return "", nil
		case "module", "open":
			// File has no package statement, first keyword is a module declaration
			return "", nil
		default:
			return "", fmt.Errorf(`expected first token of java file to be "package", got %q`, s.TokenText())
		}
	} else if tok == scanner.EOF {
		// File no package statement, it has no non-whitespace non-comment tokens
		return "", nil
	} else {
		return "", fmt.Errorf(`expected first token of java file to be "package", got %q`, s.TokenText())
	}

	var pkg string
	for {
		tok = s.Scan()
		if sErr != nil {
			return "", sErr
		}
		if tok != scanner.Ident {
			return "", fmt.Errorf(`expected "package <package>;", got "package %s%s"`, pkg, s.TokenText())
		}
		pkg += s.TokenText()

		tok = s.Scan()
		if sErr != nil {
			return "", sErr
		}
		if tok == ';' {
			return pkg, nil
		} else if tok == '.' {
			pkg += "."
		} else {
			return "", fmt.Errorf(`expected "package <package>;", got "package %s%s"`, pkg, s.TokenText())
		}
	}
}
