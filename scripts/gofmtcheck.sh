#!/usr/bin/env bash

# Check gofmt
echo "  GOFMT"
gofmt_files=$(gofmt -s -d `find . -name '*.go' | grep -v vendor`)
if [[ -n ${gofmt_files} ]]; then
    echo "${gofmt_files}"
    echo
    echo "You can use the command: \`make fmt\` to reformat code."
    exit 1
fi

exit 0
