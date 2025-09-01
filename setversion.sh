#!/bin/bash

version=$(cat <version | grep -P 'v[\d]+' -o)

if [ "$branch" != "master" ]; then
    currentDate=$(date '+%Y-%m-%d-%H-%M')
    version="$version-pre-$currentDate"
fi

echo $version
