#!/bin/bash

aprotoc --go_out=paths=source_relative:. build_flags_src.proto build_flags_out.proto build_flags_common.proto build_flags_declarations.proto
