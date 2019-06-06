#!/bin/bash

aprotoc --go_out=paths=source_relative:. build_error.proto
