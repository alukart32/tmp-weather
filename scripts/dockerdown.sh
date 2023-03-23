#!/bin/bash
set -e

echo -e "\nRun docker-compose down\n"

env $(cat ".env" ".dbconf.env" | grep -Ev '^#' | xargs) docker-compose down