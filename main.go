package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/rubycommand"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
)

// ConfigsModel ...
type ConfigsModel struct {
	ApkPath                string
	CalabashAndroidVersion string
	GemFilePath            string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		ApkPath:                os.Getenv("apk_path"),
		CalabashAndroidVersion: os.Getenv("calabash_android_version"),
		GemFilePath:            os.Getenv("gem_file_path"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")
	log.Printf("- ApkPath: %s", configs.ApkPath)
	log.Printf("- CalabashAndroidVersion: %s", configs.CalabashAndroidVersion)
	log.Printf("- GemFilePath: %s", configs.GemFilePath)
}

func (configs ConfigsModel) validate() error {
	if configs.ApkPath == "" {
		return errors.New("no ApkPath parameter specified")
	}
	if exist, err := pathutil.IsPathExists(configs.ApkPath); err != nil {
		return fmt.Errorf("failed to check if apk exist, error: %s", err)
	} else if !exist {
		return fmt.Errorf("apk not exist at: %s", configs.ApkPath)
	}

	return nil
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	cmd := command.New("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func registerFail(format string, v ...interface{}) {
	log.Errorf(format, v...)

	if err := exportEnvironmentWithEnvman("BITRISE_XAMARIN_TEST_RESULT", "failed"); err != nil {
		log.Warnf("Failed to export environment: %s, error: %s", "BITRISE_XAMARIN_TEST_RESULT", err)
	}

	os.Exit(1)
}

func calabashAndroidFromGemfileLockContent(content string) string {
	relevantLines := []string{}
	lines := strings.Split(content, "\n")

	specsStart := false
	for _, line := range lines {
		if strings.Contains(line, "specs:") {
			specsStart = true
		}

		trimmed := strings.Trim(line, " ")
		if trimmed == "" {
			break
		}

		if specsStart {
			relevantLines = append(relevantLines, line)
		}
	}

	exp := regexp.MustCompile(`calabash-android \((.+)\)`)
	for _, line := range relevantLines {
		match := exp.FindStringSubmatch(line)
		if match != nil && len(match) == 2 {
			return match[1]
		}
	}

	return ""
}

func calabashAndroidVersionFromGemfileLock(gemfileLockPth string) (string, error) {
	content, err := fileutil.ReadStringFromFile(gemfileLockPth)
	if err != nil {
		return "", err
	}
	return calabashAndroidFromGemfileLockContent(content), nil
}

func main() {
	configs := createConfigsModelFromEnvs()

	fmt.Println()
	configs.print()

	if err := configs.validate(); err != nil {
		registerFail("Issue with input: %s", err)
	}

	//
	// Determining calabash-android version
	fmt.Println()
	log.Infof("Determining calabash-android version...")

	calabashAndroidVersion := ""
	useBundler := false

	if configs.GemFilePath != "" {
		if exist, err := pathutil.IsPathExists(configs.GemFilePath); err != nil {
			registerFail("Failed to check if Gemfile exists at (%s) exist, error: %s", configs.GemFilePath, err)
		} else if exist {
			log.Printf("Gemfile exists at: %s", configs.GemFilePath)

			gemfileDir := filepath.Dir(configs.GemFilePath)
			gemfileLockPth := filepath.Join(gemfileDir, "Gemfile.lock")

			if exist, err := pathutil.IsPathExists(gemfileLockPth); err != nil {
				registerFail("Failed to check if Gemfile.lock exists at (%s), error: %s", gemfileLockPth, err)
			} else if exist {
				log.Printf("Gemfile.lock exists at: %s", gemfileLockPth)

				version, err := calabashAndroidVersionFromGemfileLock(gemfileLockPth)
				if err != nil {
					registerFail("Failed to get calabash-android version from Gemfile.lock, error: %s", err)
				}

				log.Printf("calabash-android version in Gemfile.lock: %s", version)

				calabashAndroidVersion = version
				useBundler = true
			} else {
				log.Warnf("Gemfile.lock doest no find with calabash-android gem at: %s", gemfileLockPth)
			}
		} else {
			log.Warnf("Gemfile doest no find with calabash-android gem at: %s", configs.GemFilePath)
		}
	}

	if configs.CalabashAndroidVersion != "" {
		log.Printf("calabash-android version in configs: %s", configs.CalabashAndroidVersion)

		calabashAndroidVersion = configs.CalabashAndroidVersion
		useBundler = false
	}

	if calabashAndroidVersion == "" {
		log.Donef("using calabash-android latest version")
	} else {
		log.Donef("using calabash-android version: %s", calabashAndroidVersion)
	}
	// ---

	//
	// Intsalling calabash-android gem
	fmt.Println()
	log.Infof("Installing calabash-android gem...")

	// If Gemfile given with calabash-android and calabash_android_version input does not override calabash-android version
	// Run `bundle install`
	// Run calabash-android with `bundle exec`
	if useBundler {
		// bundle install
		bundleInstallCmd, err := rubycommand.New("bundle", "install", "--jobs", "20", "--retry", "5")
		if err != nil {
			registerFail("Failed to create command, error: %s", err)
		}

		bundleInstallCmd.AppendEnvs("BUNDLE_GEMFILE=" + configs.GemFilePath)
		bundleInstallCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

		log.Printf("$ %s", bundleInstallCmd.PrintableCommandArgs())

		if err := bundleInstallCmd.Run(); err != nil {
			registerFail("bundle install failed, error: %s", err)
		}
		// ---
	}

	// If no need to use bundler
	if !useBundler {
		if calabashAndroidVersion != "" {
			// ... and calabash-android version detected
			// Install calabash-android detetcted version with `gem install`
			// Append version param to calabash-android command
			installed, err := rubycommand.IsGemInstalled("calabash-android", calabashAndroidVersion)
			if err != nil {
				registerFail("Failed to check if calabash-android (v%s) installed, error: %s", calabashAndroidVersion, err)
			}

			if !installed {
				installCommands, err := rubycommand.GemInstall("calabash-android", calabashAndroidVersion)
				if err != nil {
					registerFail("Failed to create gem install commands, error: %s", err)
				}

				for _, installCommand := range installCommands {
					log.Printf("$ %s", command.PrintableCommandArgs(false, installCommand.GetCmd().Args))

					installCommand.SetStdout(os.Stdout).SetStderr(os.Stderr)

					if err := installCommand.Run(); err != nil {
						registerFail("command failed, error: %s", err)
					}
				}
			} else {
				log.Printf("calabash-android %s installed", calabashAndroidVersion)
			}
		} else {
			// ... and using latest version of calabash-android
			// Install calabash-android latest version with `gem install`

			installCommands, err := rubycommand.GemInstall("calabash-android", "")
			if err != nil {
				registerFail("Failed to create gem install commands, error: %s", err)
			}

			for _, installCommand := range installCommands {
				log.Printf("$ %s", command.PrintableCommandArgs(false, installCommand.GetCmd().Args))

				installCommand.SetStdout(os.Stdout).SetStderr(os.Stderr)

				if err := installCommand.Run(); err != nil {
					registerFail("command failed, error: %s", err)
				}
			}
		}
	}
	// ---

	//
	// Search for debug.keystore
	fmt.Println()
	log.Infof("Search for debug.keystore...")

	debugKeystorePth := ""
	homeDir := pathutil.UserHomeDir()

	// $HOME/.android/debug.keystore
	androidDebugKeystorePth := filepath.Join(homeDir, ".android", "debug.keystore")
	debugKeystorePth = androidDebugKeystorePth

	if exist, err := pathutil.IsPathExists(androidDebugKeystorePth); err != nil {
		registerFail("Failed to check if debug.keystore exists at (%s), error: %s", androidDebugKeystorePth, err)
	} else if !exist {
		log.Warnf("android debug keystore not exist at: %s", androidDebugKeystorePth)

		// $HOME/.local/share/Mono for Android/debug.keystore
		xamarinDebugKeystorePth := filepath.Join(homeDir, ".local", "share", "Mono for Android", "debug.keystore")

		log.Printf("checking xamarin debug keystore at: %s", xamarinDebugKeystorePth)

		if exist, err := pathutil.IsPathExists(xamarinDebugKeystorePth); err != nil {
			registerFail("Failed to check if debug.keystore exists at (%s), error: %s", xamarinDebugKeystorePth, err)
		} else if !exist {
			log.Warnf("xamarin debug keystore not exist at: %s", xamarinDebugKeystorePth)
			log.Printf("generating debug keystore")

			// `keytool -genkey -v -keystore "#{debug_keystore}" -alias androiddebugkey -storepass android -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US"`
			keytoolArgs := []string{"keytool", "-genkey", "-v", "-keystore", debugKeystorePth, "-alias", "androiddebugkey", "-storepass", "android", "-keypass", "android", "-keyalg", "RSA", "-keysize", "2048", "-validity", "10000", "-dname", "CN=Android Debug,O=Android,C=US"}

			cmd, err := command.NewFromSlice(keytoolArgs...)
			if err != nil {
				registerFail("Failed to create command, error: %s", err)
			}

			log.Printf("$ %s", command.PrintableCommandArgs(false, keytoolArgs))

			if err := cmd.Run(); err != nil {
				registerFail("Failed to generate debug.keystore, error: %s", err)
			}

			log.Printf("using debug keystore: %s", debugKeystorePth)
		} else {
			log.Printf("using xamarin debug keystore: %s", xamarinDebugKeystorePth)

			debugKeystorePth = xamarinDebugKeystorePth
		}
	} else {
		log.Printf("using android debug keystore: %s", androidDebugKeystorePth)
	}
	// ---

	//
	// Resign apk with debug.keystore
	fmt.Println()
	log.Infof("Resign apk with debug.keystore...")

	resignArgs := []string{"calabash-android", "resign", configs.ApkPath}
	if useBundler {
		resignArgs = append([]string{"bundle", "exec"}, resignArgs...)
	}

	resignCmd, err := rubycommand.NewFromSlice(resignArgs...)
	if err != nil {
		registerFail("Failed to create command, error: %s", err)
	}

	resignEnvs := []string{}
	if useBundler {
		resignEnvs = append(resignEnvs, "BUNDLE_GEMFILE="+configs.GemFilePath)
	}

	resignCmd.AppendEnvs(resignEnvs...)
	resignCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

	log.Printf("$ %s", resignCmd.PrintableCommandArgs())
	fmt.Println()

	if err := resignCmd.Run(); err != nil {
		registerFail("Failed to run command, error: %s", err)
	}
	// ---

	//
	// Run calabash-android
	fmt.Println()
	log.Infof("Running calabash-android test...")

	testArgs := []string{"calabash-android", "run", configs.ApkPath}
	if useBundler {
		testArgs = append([]string{"bundle", "exec"}, testArgs...)
	}

	testCmd, err := rubycommand.NewFromSlice(testArgs...)
	if err != nil {
		registerFail("Failed to create command, error: %s", err)
	}

	testEnvs := []string{}
	if useBundler {
		testEnvs = append(testEnvs, "BUNDLE_GEMFILE="+configs.GemFilePath)
	}

	testCmd.AppendEnvs(testEnvs...)
	testCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

	log.Printf("$ %s", command.PrintableCommandArgs(false, testArgs))
	fmt.Println()

	if err := testCmd.Run(); err != nil {
		registerFail("Failed to run command, error: %s", err)
	}
	// ---

	if err := exportEnvironmentWithEnvman("BITRISE_XAMARIN_TEST_RESULT", "succeeded"); err != nil {
		log.Warnf("Failed to export environment: %s, error: %s", "BITRISE_XAMARIN_TEST_RESULT", err)
	}
}
