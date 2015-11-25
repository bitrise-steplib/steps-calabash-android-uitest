require 'optparse'
require 'pathname'
require 'timeout'
require_relative 'utils/logger'

# -----------------------
# --- functions
# -----------------------

def to_bool(value)
  return true if value == true || value =~ (/^(true|t|yes|y|1)$/i)
  return false if value == false || value.nil? || value == '' || value =~ (/^(false|f|no|n|0)$/i)
  fail_with_message("Invalid value for Boolean: \"#{value}\"")
end

def run_calabash_test!(feautes, apk_path)
  base_directory = File.dirname(feautes)
  Dir.chdir(base_directory) {
    puts
    puts "calabash-android resign #{apk_path} -v"
    system("calabash-android resign #{apk_path} -v")

    puts
    puts "calabash-android run #{apk_path}"
    system("calabash-android run #{apk_path} -v")
    fail_with_message('calabash-android -- failed') unless $?.success?
  }
end

# -----------------------
# --- main
# -----------------------

#
# Input validation
options = {
  features: nil,
  apk: nil
}

parser = OptionParser.new do|opts|
  opts.banner = 'Usage: step.rb [options]'
  opts.on('-a', '--feautes calabash', 'Calabash features') { |a| options[:features] = a unless a.to_s == '' }
  opts.on('-b', '--apk path', 'APK path') { |b| options[:apk] = b unless b.to_s == '' }
  opts.on('-h', '--help', 'Displays Help') do
    exit
  end
end
parser.parse!

fail_with_message('No features folder found') unless options[:features] && File.exist?(options[:features])
fail_with_message('No apk found') unless options[:apk] && File.exist?(options[:apk])

#
# Print configs
puts
puts '========== Configs =========='
puts " * features: #{options[:features]}"
puts " * apk: #{options[:apk]}"

#
# Run calabash test
puts
puts '=> run calabash test'
run_calabash_test!(options[:features], options[:apk])

puts
puts '(i) The result is: succeeded'
system('envman add --key BITRISE_XAMARIN_TEST_RESULT --value succeeded')
