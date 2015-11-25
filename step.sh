#!/bin/bash

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -e

current_path=$(pwd)
cd $THIS_SCRIPT_DIR
bundle exec ruby "step.rb" \
	-a "${calabash_features}" \
	-b "${apk_path}"
cd $current_path
