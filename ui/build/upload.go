// Copyright 2020 Google Inc. All rights reserved.
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

package build

// This file contains the functionality to upload data from one location to
// another.

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"

	upload_proto "android/soong/ui/metrics/upload_proto"
)

const (
	uploadPbFilename = ".uploader.pb"
)

// UploadMetrics uploads a set of metrics files to a server for analysis. An
// uploader full path is required to be specified in order to upload the set
// of metrics files. This is accomplished by defining the ANDROID_ENABLE_METRICS_UPLOAD
// environment variable.
func UploadMetrics(ctx Context, config Config, buildStartedMilli int64, files ...string) {
	uploader := config.MetricsUploaderApp()
	// No metrics to upload if the path to the uploader was not specified.
	if uploader == "" {
		return
	}

	// Some files may not exist. For example, build errors protobuf file
	// may not exist since the build was successful.
	var metricsFiles []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			metricsFiles = append(metricsFiles, f)
		}
	}

	if len(metricsFiles) == 0 {
		return
	}

	// For platform builds, the branch and target name is hardcoded to specific
	// values for later extraction of the metrics in the data metrics pipeline.
	data, err := proto.Marshal(&upload_proto.Upload{
		CreationTimestampMs:   proto.Uint64(uint64(buildStartedMilli)),
		CompletionTimestampMs: proto.Uint64(uint64(time.Now().UnixNano() / int64(time.Millisecond))),
		BranchName:            proto.String("developer-metrics"),
		TargetName:            proto.String("platform-build-systems-metrics"),
		MetricsFiles:          metricsFiles,
	})
	if err != nil {
		ctx.Fatalf("failed to marshal metrics upload proto buffer message: %v\n", err)
	}

	pbFile := filepath.Join(config.OutDir(), uploadPbFilename)
	if err := ioutil.WriteFile(pbFile, data, 0644); err != nil {
		ctx.Fatalf("failed to write the marshaled metrics upload protobuf to %q: %v\n", pbFile, err)
	}
	// Remove the upload file as it's not longer needed after it has been processed by the uploader.
	defer os.Remove(pbFile)

	Command(ctx, config, "upload metrics", uploader, "--upload-metrics", pbFile).RunAndStreamOrFatal()
}
