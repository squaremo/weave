#!/bin/bash

. ./config.sh

boldly echo Sanity checks
if ! bash ./sanity_check.sh; then
    boldly echo ...failed
    exit 1
fi
boldly echo ...ok

for t in *_test.sh; do
    echo
    timidly echo "---= Running $t =---"
    . $t
done
