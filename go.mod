module android/soong

require google.golang.org/protobuf v0.0.0

require github.com/google/blueprint v0.0.0

replace google.golang.org/protobuf v0.0.0 => ../../external/golang-protobuf

replace github.com/google/blueprint v0.0.0 => ../blueprint

// Indirect deps from golang-protobuf
exclude github.com/golang/protobuf v1.5.0

replace github.com/google/go-cmp v0.5.5 => ../../external/go-cmp

// Indirect dep from go-cmp
exclude golang.org/x/xerrors v0.0.0-20191204190536-9bdfabe68543

go 1.18
