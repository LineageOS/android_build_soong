module android/soong

require (
  google.golang.org/protobuf v0.0.0
  github.com/google/blueprint v0.0.0
  prebuilts/bazel/common/proto/analysis_v2 v0.0.0
  prebuilts/bazel/common/proto/build v0.0.0 // indirect
)

replace (
  google.golang.org/protobuf v0.0.0 => ../../external/golang-protobuf
  github.com/google/blueprint v0.0.0 => ../blueprint
  github.com/google/go-cmp v0.5.5 => ../../external/go-cmp
  prebuilts/bazel/common/proto/analysis_v2 => ../../prebuilts/bazel/common/proto/analysis_v2
  prebuilts/bazel/common/proto/build => ../../prebuilts/bazel/common/proto/build
)

// Indirect deps from golang-protobuf
exclude github.com/golang/protobuf v1.5.0

go 2.0
