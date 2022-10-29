module android/soong

require google.golang.org/protobuf v0.0.0

require github.com/google/blueprint v0.0.0

replace google.golang.org/protobuf v0.0.0 => ../../external/golang-protobuf

replace github.com/google/blueprint v0.0.0 => ../blueprint

// Indirect deps from golang-protobuf
exclude github.com/golang/protobuf v1.5.0

replace github.com/google/go-cmp v0.5.5 => ../../external/go-cmp

require prebuilts/bazel/common/proto/analysis_v2 v0.0.0

replace prebuilts/bazel/common/proto/analysis_v2 => ../../prebuilts/bazel/common/proto/analysis_v2

require prebuilts/bazel/common/proto/build v0.0.0 // indirect

replace prebuilts/bazel/common/proto/build => ../../prebuilts/bazel/common/proto/build

go 1.18
