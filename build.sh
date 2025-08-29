#!/bin/bash
set -xe

out_dir=$1
branch=$2

version=$(cat <version | grep -P 'v[\d]+.[\d]+.[\d]+' -o)

if [ "$branch" != "master" ]; then
    version="$version-pre"
fi

go build -ldflags "-s -w -X 'github.com/codio/go-vncproxy/cmd/govnc/main.Version=$version'" -o "$out_dir"/ ./...
