#!/bin/bash
set -e

profile="$1"

if [ -z "$profile" ]; then
   profile="dev"
fi

echo -e "\nRun docker-compose --profile $profile up\n"

env $(cat ".env" ".dbconf.env" | grep -Ev '^#' | xargs) docker-compose --profile $profile up