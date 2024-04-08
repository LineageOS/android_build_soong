#!/bin/bash

aprotoc --go_out=paths=source_relative:. build_flags_src.proto build_flags_out.proto
