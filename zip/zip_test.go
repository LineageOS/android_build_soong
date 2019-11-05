// Copyright 2018 Google Inc. All rights reserved.
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

package zip

import (
	"bytes"
	"hash/crc32"
	"io"
	"os"
	"reflect"
	"syscall"
	"testing"

	"android/soong/third_party/zip"

	"github.com/google/blueprint/pathtools"
)

var (
	fileA        = []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	fileB        = []byte("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	fileC        = []byte("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC")
	fileEmpty    = []byte("")
	fileManifest = []byte("Manifest-Version: 1.0\nCreated-By: soong_zip\n\n")

	fileCustomManifest  = []byte("Custom manifest: true\n")
	customManifestAfter = []byte("Manifest-Version: 1.0\nCreated-By: soong_zip\nCustom manifest: true\n\n")
)

var mockFs = pathtools.MockFs(map[string][]byte{
	"a/a/a":               fileA,
	"a/a/b":               fileB,
	"a/a/c -> ../../c":    nil,
	"dangling -> missing": nil,
	"a/a/d -> b":          nil,
	"c":                   fileC,
	"l_nl":                []byte("a/a/a\na/a/b\nc\n"),
	"l_sp":                []byte("a/a/a a/a/b c"),
	"l2":                  []byte("missing\n"),
	"manifest.txt":        fileCustomManifest,
})

func fh(name string, contents []byte, method uint16) zip.FileHeader {
	return zip.FileHeader{
		Name:               name,
		Method:             method,
		CRC32:              crc32.ChecksumIEEE(contents),
		UncompressedSize64: uint64(len(contents)),
		ExternalAttrs:      0,
	}
}

func fhManifest(contents []byte) zip.FileHeader {
	return zip.FileHeader{
		Name:               "META-INF/MANIFEST.MF",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(contents),
		UncompressedSize64: uint64(len(contents)),
		ExternalAttrs:      (syscall.S_IFREG | 0700) << 16,
	}
}

func fhLink(name string, to string) zip.FileHeader {
	return zip.FileHeader{
		Name:               name,
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE([]byte(to)),
		UncompressedSize64: uint64(len(to)),
		ExternalAttrs:      (syscall.S_IFLNK | 0777) << 16,
	}
}

func fhDir(name string) zip.FileHeader {
	return zip.FileHeader{
		Name:               name,
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(nil),
		UncompressedSize64: 0,
		ExternalAttrs:      (syscall.S_IFDIR|0700)<<16 | 0x10,
	}
}

func fileArgsBuilder() *FileArgsBuilder {
	return &FileArgsBuilder{
		fs: mockFs,
	}
}

func TestZip(t *testing.T) {
	testCases := []struct {
		name               string
		args               *FileArgsBuilder
		compressionLevel   int
		emulateJar         bool
		nonDeflatedFiles   map[string]bool
		dirEntries         bool
		manifest           string
		storeSymlinks      bool
		ignoreMissingFiles bool

		files []zip.FileHeader
		err   error
	}{
		{
			name: "empty args",
			args: fileArgsBuilder(),

			files: []zip.FileHeader{},
		},
		{
			name: "files",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b").
				File("c"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
				fh("c", fileC, zip.Deflate),
			},
		},
		{
			name: "files glob",
			args: fileArgsBuilder().
				SourcePrefixToStrip("a").
				File("a/**/*"),
			compressionLevel: 9,
			storeSymlinks:    true,

			files: []zip.FileHeader{
				fh("a/a", fileA, zip.Deflate),
				fh("a/b", fileB, zip.Deflate),
				fhLink("a/c", "../../c"),
				fhLink("a/d", "b"),
			},
		},
		{
			name: "dir",
			args: fileArgsBuilder().
				SourcePrefixToStrip("a").
				Dir("a"),
			compressionLevel: 9,
			storeSymlinks:    true,

			files: []zip.FileHeader{
				fh("a/a", fileA, zip.Deflate),
				fh("a/b", fileB, zip.Deflate),
				fhLink("a/c", "../../c"),
				fhLink("a/d", "b"),
			},
		},
		{
			name: "stored files",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b").
				File("c"),
			compressionLevel: 0,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Store),
				fh("a/a/b", fileB, zip.Store),
				fh("c", fileC, zip.Store),
			},
		},
		{
			name: "symlinks in zip",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b").
				File("a/a/c").
				File("a/a/d"),
			compressionLevel: 9,
			storeSymlinks:    true,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
				fhLink("a/a/c", "../../c"),
				fhLink("a/a/d", "b"),
			},
		},
		{
			name: "follow symlinks",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b").
				File("a/a/c").
				File("a/a/d"),
			compressionLevel: 9,
			storeSymlinks:    false,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
				fh("a/a/c", fileC, zip.Deflate),
				fh("a/a/d", fileB, zip.Deflate),
			},
		},
		{
			name: "dangling symlinks",
			args: fileArgsBuilder().
				File("dangling"),
			compressionLevel: 9,
			storeSymlinks:    true,

			files: []zip.FileHeader{
				fhLink("dangling", "missing"),
			},
		},
		{
			name: "list",
			args: fileArgsBuilder().
				List("l_nl"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
				fh("c", fileC, zip.Deflate),
			},
		},
		{
			name: "list",
			args: fileArgsBuilder().
				List("l_sp"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
				fh("c", fileC, zip.Deflate),
			},
		},
		{
			name: "prefix in zip",
			args: fileArgsBuilder().
				PathPrefixInZip("foo").
				File("a/a/a").
				File("a/a/b").
				File("c"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("foo/a/a/a", fileA, zip.Deflate),
				fh("foo/a/a/b", fileB, zip.Deflate),
				fh("foo/c", fileC, zip.Deflate),
			},
		},
		{
			name: "relative root",
			args: fileArgsBuilder().
				SourcePrefixToStrip("a").
				File("a/a/a").
				File("a/a/b"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("a/a", fileA, zip.Deflate),
				fh("a/b", fileB, zip.Deflate),
			},
		},
		{
			name: "multiple relative root",
			args: fileArgsBuilder().
				SourcePrefixToStrip("a").
				File("a/a/a").
				SourcePrefixToStrip("a/a").
				File("a/a/b"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("a/a", fileA, zip.Deflate),
				fh("b", fileB, zip.Deflate),
			},
		},
		{
			name: "emulate jar",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b"),
			compressionLevel: 9,
			emulateJar:       true,

			files: []zip.FileHeader{
				fhDir("META-INF/"),
				fhManifest(fileManifest),
				fhDir("a/"),
				fhDir("a/a/"),
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
			},
		},
		{
			name: "emulate jar with manifest",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b"),
			compressionLevel: 9,
			emulateJar:       true,
			manifest:         "manifest.txt",

			files: []zip.FileHeader{
				fhDir("META-INF/"),
				fhManifest(customManifestAfter),
				fhDir("a/"),
				fhDir("a/a/"),
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
			},
		},
		{
			name: "dir entries",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b"),
			compressionLevel: 9,
			dirEntries:       true,

			files: []zip.FileHeader{
				fhDir("a/"),
				fhDir("a/a/"),
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
			},
		},
		{
			name: "junk paths",
			args: fileArgsBuilder().
				JunkPaths(true).
				File("a/a/a").
				File("a/a/b"),
			compressionLevel: 9,

			files: []zip.FileHeader{
				fh("a", fileA, zip.Deflate),
				fh("b", fileB, zip.Deflate),
			},
		},
		{
			name: "non deflated files",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b"),
			compressionLevel: 9,
			nonDeflatedFiles: map[string]bool{"a/a/a": true},

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Store),
				fh("a/a/b", fileB, zip.Deflate),
			},
		},
		{
			name: "ignore missing files",
			args: fileArgsBuilder().
				File("a/a/a").
				File("a/a/b").
				File("missing"),
			compressionLevel:   9,
			ignoreMissingFiles: true,

			files: []zip.FileHeader{
				fh("a/a/a", fileA, zip.Deflate),
				fh("a/a/b", fileB, zip.Deflate),
			},
		},

		// errors
		{
			name: "error missing file",
			args: fileArgsBuilder().
				File("missing"),
			err: os.ErrNotExist,
		},
		{
			name: "error missing dir",
			args: fileArgsBuilder().
				Dir("missing"),
			err: os.ErrNotExist,
		},
		{
			name: "error missing file in list",
			args: fileArgsBuilder().
				List("l2"),
			err: os.ErrNotExist,
		},
		{
			name: "error incorrect relative root",
			args: fileArgsBuilder().
				SourcePrefixToStrip("b").
				File("a/a/a"),
			err: IncorrectRelativeRootError{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if test.args.Error() != nil {
				t.Fatal(test.args.Error())
			}

			args := ZipArgs{}
			args.FileArgs = test.args.FileArgs()
			args.CompressionLevel = test.compressionLevel
			args.EmulateJar = test.emulateJar
			args.AddDirectoryEntriesToZip = test.dirEntries
			args.NonDeflatedFiles = test.nonDeflatedFiles
			args.ManifestSourcePath = test.manifest
			args.StoreSymlinks = test.storeSymlinks
			args.IgnoreMissingFiles = test.ignoreMissingFiles
			args.Filesystem = mockFs
			args.Stderr = &bytes.Buffer{}

			buf := &bytes.Buffer{}
			err := ZipTo(args, buf)

			if (err != nil) != (test.err != nil) {
				t.Fatalf("want error %v, got %v", test.err, err)
			} else if test.err != nil {
				if os.IsNotExist(test.err) {
					if !os.IsNotExist(test.err) {
						t.Fatalf("want error %v, got %v", test.err, err)
					}
				} else if _, wantRelativeRootErr := test.err.(IncorrectRelativeRootError); wantRelativeRootErr {
					if _, gotRelativeRootErr := err.(IncorrectRelativeRootError); !gotRelativeRootErr {
						t.Fatalf("want error %v, got %v", test.err, err)
					}
				} else {
					t.Fatalf("want error %v, got %v", test.err, err)
				}
				return
			}

			br := bytes.NewReader(buf.Bytes())
			zr, err := zip.NewReader(br, int64(br.Len()))
			if err != nil {
				t.Fatal(err)
			}

			var files []zip.FileHeader
			for _, f := range zr.File {
				r, err := f.Open()
				if err != nil {
					t.Fatalf("error when opening %s: %s", f.Name, err)
				}

				crc := crc32.NewIEEE()
				len, err := io.Copy(crc, r)
				r.Close()
				if err != nil {
					t.Fatalf("error when reading %s: %s", f.Name, err)
				}

				if uint64(len) != f.UncompressedSize64 {
					t.Errorf("incorrect length for %s, want %d got %d", f.Name, f.UncompressedSize64, len)
				}

				if crc.Sum32() != f.CRC32 {
					t.Errorf("incorrect crc for %s, want %x got %x", f.Name, f.CRC32, crc)
				}

				files = append(files, f.FileHeader)
			}

			if len(files) != len(test.files) {
				t.Fatalf("want %d files, got %d", len(test.files), len(files))
			}

			for i := range files {
				want := test.files[i]
				got := files[i]

				if want.Name != got.Name {
					t.Errorf("incorrect file %d want %q got %q", i, want.Name, got.Name)
					continue
				}

				if want.UncompressedSize64 != got.UncompressedSize64 {
					t.Errorf("incorrect file %s length want %v got %v", want.Name,
						want.UncompressedSize64, got.UncompressedSize64)
				}

				if want.ExternalAttrs != got.ExternalAttrs {
					t.Errorf("incorrect file %s attrs want %x got %x", want.Name,
						want.ExternalAttrs, got.ExternalAttrs)
				}

				if want.CRC32 != got.CRC32 {
					t.Errorf("incorrect file %s crc want %v got %v", want.Name,
						want.CRC32, got.CRC32)
				}

				if want.Method != got.Method {
					t.Errorf("incorrect file %s method want %v got %v", want.Name,
						want.Method, got.Method)
				}
			}
		})
	}
}

func TestReadRespFile(t *testing.T) {
	testCases := []struct {
		name, in string
		out      []string
	}{
		{
			name: "single quoting test case 1",
			in:   `./cmd '"'-C`,
			out:  []string{"./cmd", `"-C`},
		},
		{
			name: "single quoting test case 2",
			in:   `./cmd '-C`,
			out:  []string{"./cmd", `-C`},
		},
		{
			name: "single quoting test case 3",
			in:   `./cmd '\"'-C`,
			out:  []string{"./cmd", `\"-C`},
		},
		{
			name: "single quoting test case 4",
			in:   `./cmd '\\'-C`,
			out:  []string{"./cmd", `\\-C`},
		},
		{
			name: "none quoting test case 1",
			in:   `./cmd \'-C`,
			out:  []string{"./cmd", `'-C`},
		},
		{
			name: "none quoting test case 2",
			in:   `./cmd \\-C`,
			out:  []string{"./cmd", `\-C`},
		},
		{
			name: "none quoting test case 3",
			in:   `./cmd \"-C`,
			out:  []string{"./cmd", `"-C`},
		},
		{
			name: "double quoting test case 1",
			in:   `./cmd "'"-C`,
			out:  []string{"./cmd", `'-C`},
		},
		{
			name: "double quoting test case 2",
			in:   `./cmd "\\"-C`,
			out:  []string{"./cmd", `\-C`},
		},
		{
			name: "double quoting test case 3",
			in:   `./cmd "\""-C`,
			out:  []string{"./cmd", `"-C`},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := ReadRespFile([]byte(testCase.in))
			if !reflect.DeepEqual(got, testCase.out) {
				t.Errorf("expected %q got %q", testCase.out, got)
			}
		})
	}
}

func TestSrcJar(t *testing.T) {
	mockFs := pathtools.MockFs(map[string][]byte{
		"wrong_package.java":       []byte("package foo;"),
		"foo/correct_package.java": []byte("package foo;"),
		"src/no_package.java":      nil,
		"src2/parse_error.java":    []byte("error"),
	})

	want := []string{
		"foo/",
		"foo/wrong_package.java",
		"foo/correct_package.java",
		"no_package.java",
		"src2/",
		"src2/parse_error.java",
	}

	args := ZipArgs{}
	args.FileArgs = NewFileArgsBuilder().File("**/*.java").FileArgs()

	args.SrcJar = true
	args.AddDirectoryEntriesToZip = true
	args.Filesystem = mockFs
	args.Stderr = &bytes.Buffer{}

	buf := &bytes.Buffer{}
	err := ZipTo(args, buf)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	br := bytes.NewReader(buf.Bytes())
	zr, err := zip.NewReader(br, int64(br.Len()))
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatalf("error when opening %s: %s", f.Name, err)
		}

		crc := crc32.NewIEEE()
		len, err := io.Copy(crc, r)
		r.Close()
		if err != nil {
			t.Fatalf("error when reading %s: %s", f.Name, err)
		}

		if uint64(len) != f.UncompressedSize64 {
			t.Errorf("incorrect length for %s, want %d got %d", f.Name, f.UncompressedSize64, len)
		}

		if crc.Sum32() != f.CRC32 {
			t.Errorf("incorrect crc for %s, want %x got %x", f.Name, f.CRC32, crc)
		}

		got = append(got, f.Name)
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("want files %q, got %q", want, got)
	}
}
