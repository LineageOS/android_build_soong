module android/soong

require google.golang.org/protobuf v0.0.0

require github.com/google/blueprint v0.0.0

replace google.golang.org/protobuf v0.0.0 => ../../external/golang-protobuf

replace github.com/google/blueprint v0.0.0 => ../blueprint

go 1.15
