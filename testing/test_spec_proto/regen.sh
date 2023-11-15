#!/bin/bash

aprotoc --go_out=paths=source_relative:. test_spec.proto
