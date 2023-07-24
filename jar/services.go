// Copyright 2023 Google Inc. All rights reserved.
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
	"android/soong/third_party/zip"
	"bufio"
	"hash/crc32"
	"sort"
	"strings"
)

const servicesPrefix = "META-INF/services/"

// Services is used to collect service files from multiple zip files and produce a list of ServiceFiles containing
// the unique lines from all the input zip entries with the same name.
type Services struct {
	services map[string]*ServiceFile
}

// ServiceFile contains the combined contents of all input zip entries with a single name.
type ServiceFile struct {
	Name       string
	FileHeader *zip.FileHeader
	Contents   []byte
	Lines      []string
}

// IsServiceFile returns true if the zip entry is in the META-INF/services/ directory.
func (Services) IsServiceFile(entry *zip.File) bool {
	return strings.HasPrefix(entry.Name, servicesPrefix)
}

// AddServiceFile adds a zip entry in the META-INF/services/ directory to the list of service files that need
// to be combined.
func (j *Services) AddServiceFile(entry *zip.File) error {
	if j.services == nil {
		j.services = map[string]*ServiceFile{}
	}

	service := entry.Name
	serviceFile := j.services[service]
	fh := entry.FileHeader
	if serviceFile == nil {
		serviceFile = &ServiceFile{
			Name:       service,
			FileHeader: &fh,
		}
		j.services[service] = serviceFile
	}

	f, err := entry.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			serviceFile.Lines = append(serviceFile.Lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// ServiceFiles returns the list of combined service files, each containing all the unique lines from the
// corresponding service files in the input zip entries.
func (j *Services) ServiceFiles() []ServiceFile {
	services := make([]ServiceFile, 0, len(j.services))

	for _, serviceFile := range j.services {
		serviceFile.Lines = dedupServicesLines(serviceFile.Lines)
		serviceFile.Lines = append(serviceFile.Lines, "")
		serviceFile.Contents = []byte(strings.Join(serviceFile.Lines, "\n"))

		serviceFile.FileHeader.UncompressedSize64 = uint64(len(serviceFile.Contents))
		serviceFile.FileHeader.CRC32 = crc32.ChecksumIEEE(serviceFile.Contents)
		if serviceFile.FileHeader.Method == zip.Store {
			serviceFile.FileHeader.CompressedSize64 = serviceFile.FileHeader.UncompressedSize64
		}

		services = append(services, *serviceFile)
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	return services
}

func dedupServicesLines(in []string) []string {
	writeIndex := 0
outer:
	for readIndex := 0; readIndex < len(in); readIndex++ {
		for compareIndex := 0; compareIndex < writeIndex; compareIndex++ {
			if interface{}(in[readIndex]) == interface{}(in[compareIndex]) {
				// The value at readIndex already exists somewhere in the output region
				// of the slice before writeIndex, skip it.
				continue outer
			}
		}
		if readIndex != writeIndex {
			in[writeIndex] = in[readIndex]
		}
		writeIndex++
	}
	return in[0:writeIndex]
}
