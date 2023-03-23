#!/bin/bash
set -e

readonly service="$1"
mode="-short"
readonly env_file="$3"
tags=""

echo -e "\ncd ./internal/$service"
cd "./internal/$service"

if [[ "$2" == *"integration"* ]]; then
  mode=""
  tags="--tags=integration"
fi

echo -e "\nRun go test -v -cover $tags $mode with $env_file\n"
env $(cat "../../$env_file" | grep -Ev '^#' | xargs) go test -v -cover $tags $mode ./...