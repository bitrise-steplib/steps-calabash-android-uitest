package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/cmdex"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-steplib/steps-calabash-android-uitest/rubycmd"
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
	log.Info("Configs:")
	log.Detail("- ApkPath: %s", configs.ApkPath)
	log.Detail("- CalabashAndroidVersion: %s", configs.CalabashAndroidVersion)
	log.Detail("- GemFilePath: %s", configs.GemFilePath)
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
	cmd := cmdex.NewCommand("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func registerFail(format string, v ...interface{}) {
	log.Error(format, v...)

	if err := exportEnvironmentWithEnvman("BITRISE_XAMARIN_TEST_RESULT", "failed"); err != nil {
		log.Warn("Failed to export environment: %s, error: %s", "BITRISE_XAMARIN_TEST_RESULT", err)
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
	log.Info("Determining calabash-android version...")

	rubyCommand, err := rubycmd.NewRubyCommandModel()
	if err != nil {
		registerFail("Failed to create ruby command, err: %s", err)
	}

	calabashAndroidVersion := ""
	useBundler := false

	if configs.GemFilePath != "" {
		if exist, err := pathutil.IsPathExists(configs.GemFilePath); err != nil {
			registerFail("Failed to check if Gemfile exists at (%s) exist, error: %s", configs.GemFilePath, err)
		} else if exist {
			log.Detail("Gemfile exists at: %s", configs.GemFilePath)

			gemfileDir := filepath.Dir(configs.GemFilePath)
			gemfileLockPth := filepath.Join(gemfileDir, "Gemfile.lock")

			if exist, err := pathutil.IsPathExists(gemfileLockPth); err != nil {
				registerFail("Failed to check if Gemfile.lock exists at (%s), error: %s", gemfileLockPth, err)
			} else if exist {
				log.Detail("Gemfile.lock exists at: %s", gemfileLockPth)

				version, err := calabashAndroidVersionFromGemfileLock(gemfileLockPth)
				if err != nil {
					registerFail("Failed to get calabash-android version from Gemfile.lock, error: %s", err)
				}

				log.Detail("calabash-android version in Gemfile.lock: %s", version)

				calabashAndroidVersion = version
				useBundler = true
			} else {
				log.Warn("Gemfile.lock doest no find with calabash-android gem at: %s", gemfileLockPth)
			}
		} else {
			log.Warn("Gemfile doest no find with calabash-android gem at: %s", configs.GemFilePath)
		}
	}

	if configs.CalabashAndroidVersion != "" {
		log.Detail("calabash-android version in configs: %s", configs.CalabashAndroidVersion)

		calabashAndroidVersion = configs.CalabashAndroidVersion
		useBundler = false
	}

	if calabashAndroidVersion == "" {
		log.Done("using calabash-android latest version")
	} else {
		log.Done("using calabash-android version: %s", calabashAndroidVersion)
	}
	// ---

	//
	// Intsalling calabash-android gem
	fmt.Println()
	log.Info("Installing calabash-android gem...")

	calabashAndroidArgs := []string{}

	// If Gemfile given with calabash-android and calabash_android_version input does not override calabash-android version
	// Run `bundle install`
	// Run calabash-android with `bundle exec`
	if useBundler {
		bundleInstallArgs := []string{"bundle", "install", "--jobs", "20", "--retry", "5"}

		// bundle install
		bundleInstallCmd, err := rubyCommand.Command(false, bundleInstallArgs)
		if err != nil {
			registerFail("Failed to create command, error: %s", err)
		}

		bundleInstallCmd.AppendEnvs([]string{"BUNDLE_GEMFILE=" + configs.GemFilePath})

		log.Detail("$ %s", cmdex.PrintableCommandArgs(false, bundleInstallArgs))

		if err := bundleInstallCmd.Run(); err != nil {
			registerFail("bundle install failed, error: %s", err)
		}
		// ---

		calabashAndroidArgs = []string{"bundle", "exec"}
	}

	calabashAndroidArgs = append(calabashAndroidArgs, "calabash-android")

	// If no need to use bundler
	if !useBundler {
		if calabashAndroidVersion != "" {
			// ... and calabash-android version detected
			// Install calabash-android detetcted version with `gem install`
			// Append version param to calabash-android command
			installed, err := rubyCommand.IsGemInstalled("calabash-android", calabashAndroidVersion)
			if err != nil {
				registerFail("Failed to check if calabash-android (v%s) installed, error: %s", calabashAndroidVersion, err)
			}

			if !installed {
				installCommands, err := rubyCommand.GemInstallCommands("calabash-android", calabashAndroidVersion)
				if err != nil {
					registerFail("Failed to create gem install commands, error: %s", err)
				}

				for _, installCommand := range installCommands {
					log.Detail("$ %s", cmdex.PrintableCommandArgs(false, installCommand.GetCmd().Args))

					if err := installCommand.Run(); err != nil {
						registerFail("command failed, error: %s", err)
					}
				}
			} else {
				log.Detail("calabash-android %s installed", calabashAndroidVersion)
			}
		} else {
			// ... and using latest version of calabash-android
			// Install calabash-android latest version with `gem install`

			installCommands, err := rubyCommand.GemInstallCommands("calabash-android", "")
			if err != nil {
				registerFail("Failed to create gem install commands, error: %s", err)
			}

			for _, installCommand := range installCommands {
				log.Detail("$ %s", cmdex.PrintableCommandArgs(false, installCommand.GetCmd().Args))

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
	log.Info("Search for debug.keystore...")

	debugKeystorePth := ""
	homeDir := pathutil.UserHomeDir()

	// $HOME/.android/debug.keystore
	androidDebugKeystorePth := filepath.Join(homeDir, ".android", "debug.keystore")
	debugKeystorePth = androidDebugKeystorePth

	if exist, err := pathutil.IsPathExists(androidDebugKeystorePth); err != nil {
		registerFail("Failed to check if debug.keystore exists at (%s), error: %s", androidDebugKeystorePth, err)
	} else if !exist {
		log.Warn("android debug keystore not exist at: %s", androidDebugKeystorePth)

		// $HOME/.local/share/Mono for Android/debug.keystore
		xamarinDebugKeystorePth := filepath.Join(homeDir, ".local", "share", "Mono for Android", "debug.keystore")

		log.Detail("checking xamarin debug keystore at: %s", xamarinDebugKeystorePth)

		if exist, err := pathutil.IsPathExists(xamarinDebugKeystorePth); err != nil {
			registerFail("Failed to check if debug.keystore exists at (%s), error: %s", xamarinDebugKeystorePth, err)
		} else if !exist {
			log.Warn("xamarin debug keystore not exist at: %s", xamarinDebugKeystorePth)
			log.Detail("generating debug keystore")

			// `keytool -genkey -v -keystore "#{debug_keystore}" -alias androiddebugkey -storepass android -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US"`
			keytoolArgs := []string{"keytool", "-genkey", "-v", "-keystore", debugKeystorePth, "-alias", "androiddebugkey", "-storepass", "android", "-keypass", "android", "-keyalg", "RSA", "-keysize", "2048", "-validity", "10000", "-dname", "CN=Android Debug,O=Android,C=US"}

			cmd, err := cmdex.NewCommandFromSlice(keytoolArgs)
			if err != nil {
				registerFail("Failed to create command, error: %s", err)
			}

			log.Detail("$ %s", cmdex.PrintableCommandArgs(false, keytoolArgs))

			if err := cmd.Run(); err != nil {
				registerFail("Failed to generate debug.keystore, error: %s", err)
			}

			log.Detail("using debug keystore: %s", debugKeystorePth)
		} else {
			log.Detail("using xamarin debug keystore: %s", xamarinDebugKeystorePth)

			debugKeystorePth = xamarinDebugKeystorePth
		}
	} else {
		log.Detail("using android debug keystore: %s", androidDebugKeystorePth)
	}
	// ---

	//
	// Resign apk with debug.keystore
	fmt.Println()
	log.Info("Resign apk with debug.keystore...")

	resignArgs := []string{"calabash-android", "resign", configs.ApkPath}
	resignCmd, err := rubyCommand.Command(useBundler, resignArgs)
	if err != nil {
		registerFail("Failed to create command, error: %s", err)
	}

	log.Detail("$ %s", cmdex.PrintableCommandArgs(false, resignArgs))
	fmt.Println()

	resignCmd.SetStdout(os.Stdout)
	resignCmd.SetStderr(os.Stderr)

	if err := resignCmd.Run(); err != nil {
		registerFail("Failed to run command, error: %s", err)
	}
	// ---

	//
	// Run calabash-android
	fmt.Println()
	log.Info("Running calabash-android test...")

	testArgs := []string{"calabash-android", "run", configs.ApkPath}
	testCmd, err := rubyCommand.Command(useBundler, testArgs)
	if err != nil {
		registerFail("Failed to create command, error: %s", err)
	}

	log.Detail("$ %s", cmdex.PrintableCommandArgs(false, testArgs))
	fmt.Println()

	testCmd.SetStdout(os.Stdout)
	testCmd.SetStderr(os.Stderr)

	if err := testCmd.Run(); err != nil {
		registerFail("Failed to run command, error: %s", err)
	}
	// ---

	if err := exportEnvironmentWithEnvman("BITRISE_XAMARIN_TEST_RESULT", "succeeded"); err != nil {
		log.Warn("Failed to export environment: %s, error: %s", "BITRISE_XAMARIN_TEST_RESULT", err)
	}
}
