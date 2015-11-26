#!/bin/bash

this_script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -e

current_path=$(pwd)
cd $this_script_dir
bundle install
bundle exec ruby "step.rb" \
	-a "${calabash_features}" \
	-b "${apk_path}"
cd $current_path
