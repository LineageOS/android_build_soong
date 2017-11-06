#!/bin/bash -e

OUT=$1
shift
LIBPATH=$($@ | sed -e "s|^$PWD/||")
cp -f $LIBPATH $OUT
echo "$OUT: $LIBPATH" > ${OUT}.d
