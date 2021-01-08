module android/soong

require github.com/golang/protobuf v0.0.0

require github.com/google/blueprint v0.0.0

replace github.com/golang/protobuf v0.0.0 => ../../external/golang-protobuf

replace github.com/google/blueprint v0.0.0 => ../blueprint

go 1.15.6
