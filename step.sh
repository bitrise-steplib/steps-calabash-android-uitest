#!/bin/bash

this_script_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -e

bundle install
bundle exec ruby "${this_script_dir}/step.rb" \
	-a "${calabash_features}" \
	-b "${apk_path}"
