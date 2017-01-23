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
	"github.com/hashicorp/go-version"
)

// ConfigsModel ...
type ConfigsModel struct {
	WorkDir     string
	GemFilePath string
	ApkPath     string

	AndroidHome string

	CalabashAndroidVersion string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		WorkDir:     os.Getenv("work_dir"),
		GemFilePath: os.Getenv("gem_file_path"),
		ApkPath:     os.Getenv("apk_path"),

		AndroidHome: os.Getenv("android_home"),

		CalabashAndroidVersion: os.Getenv("calabash_android_version"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")
	log.Printf("- WorkDir: %s", configs.WorkDir)
	log.Printf("- GemFilePath: %s", configs.GemFilePath)
	log.Printf("- ApkPath: %s", configs.ApkPath)

	log.Printf("- AndroidHome: %s", configs.AndroidHome)

	log.Printf("- CalabashAndroidVersion: %s", configs.CalabashAndroidVersion)
}

func (configs ConfigsModel) validate() error {
	if configs.WorkDir == "" {
		return errors.New("no WorkDir parameter specified")
	}
	if exist, err := pathutil.IsDirExists(configs.WorkDir); err != nil {
		return fmt.Errorf("failed to check if WorkDir exist, error: %s", err)
	} else if !exist {
		return fmt.Errorf("WorkDir directory not exists at: %s", configs.WorkDir)
	}

	if configs.ApkPath == "" {
		return errors.New("no ApkPath parameter specified")
	}
	if exist, err := pathutil.IsPathExists(configs.ApkPath); err != nil {
		return fmt.Errorf("failed to check if apk exist, error: %s", err)
	} else if !exist {
		return fmt.Errorf("apk not exist at: %s", configs.ApkPath)
	}

	if configs.AndroidHome == "" {
		return errors.New("no ApkPath parameter specified")
	}
	if exist, err := pathutil.IsDirExists(configs.AndroidHome); err != nil {
		return fmt.Errorf("failed to check if AndroidHome exist, error: %s", err)
	} else if !exist {
		return fmt.Errorf("AndroidHome directory not exists at: %s", configs.AndroidHome)
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

func getLatestAAPT(androidHome string) (string, error) {
	// $ANDROID_HOME/build-tools/24.0.2/aapt

	pattern := filepath.Join(androidHome, "build-tools", "*", "aapt")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}

	var latestVersion *version.Version
	for _, file := range files {
		dir := filepath.Dir(file)
		base := filepath.Base(dir)

		ver, err := version.NewVersion(base)
		if err != nil {
			return "", err
		}

		if latestVersion == nil || latestVersion.LessThan(ver) {
			latestVersion = ver
		}
	}

	if latestVersion == nil {
		return "", errors.New("failed to find latest aapt version")
	}
	aapt := filepath.Join(androidHome, "build-tools", latestVersion.String(), "aapt")
	if exist, err := pathutil.IsPathExists(aapt); err != nil {
		return "", err
	} else if !exist {
		return "", fmt.Errorf("aapt not exists at: %s", aapt)
	}

	return aapt, nil
}

func ensureAPKInternetPermission(apkPth, androidHome string) error {
	aapt, err := getLatestAAPT(androidHome)
	if err != nil {
		return err
	}

	args := []string{"d", "permissions", apkPth}
	cmd := command.New(aapt, args...)

	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return err
	}

	if !strings.Contains(out, "android.permission.INTERNET") {
		return errors.New("apk has no internet permission")
	}

	return nil
}

func main() {
	configs := createConfigsModelFromEnvs()

	fmt.Println()
	configs.print()

	if err := configs.validate(); err != nil {
		registerFail("Issue with input: %s", err)
	}

	//
	// Ensure apk
	if err := ensureAPKInternetPermission(configs.ApkPath, configs.AndroidHome); err != nil {
		registerFail("Failed to ensure apk internet permission, error: %s", err)
	}
	// ---

	//
	// Determining calabash-android version
	fmt.Println()
	log.Infof("Determining calabash-android version...")

	workDir, err := pathutil.AbsPath(configs.WorkDir)
	if err != nil {
		registerFail("Failed to expand WorkDir (%s), error: %s", configs.WorkDir, err)
	}

	gemFilePath := ""
	if configs.GemFilePath != "" {
		gemFilePath, err = pathutil.AbsPath(configs.GemFilePath)
		if err != nil {
			registerFail("Failed to expand GemFilePath (%s), error: %s", configs.GemFilePath, err)
		}
	}

	useBundler := false

	if gemFilePath != "" {
		if exist, err := pathutil.IsPathExists(gemFilePath); err != nil {
			registerFail("Failed to check if Gemfile exists at (%s) exist, error: %s", gemFilePath, err)
		} else if exist {
			log.Printf("Gemfile exists at: %s", gemFilePath)

			gemfileDir := filepath.Dir(gemFilePath)
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

				useBundler = true
			} else {
				log.Warnf("Gemfile.lock doest no find with calabash-android gem at: %s", gemfileLockPth)
			}
		} else {
			log.Warnf("Gemfile doest no find with calabash-android gem at: %s", gemFilePath)
		}
	}

	if configs.CalabashAndroidVersion != "" {
		log.Donef("using calabash-android version: %s", configs.CalabashAndroidVersion)
	} else if useBundler {
		log.Donef("using calabash-android with bundler")
	} else {
		log.Donef("using calabash-android latest version")
	}
	// ---

	//
	// Intsalling calabash-android gem
	fmt.Println()
	log.Infof("Installing calabash-android gem...")

	if configs.CalabashAndroidVersion != "" {
		installed, err := rubycommand.IsGemInstalled("calabash-android", configs.CalabashAndroidVersion)
		if err != nil {
			registerFail("Failed to check if calabash-android (v%s) installed, error: %s", configs.CalabashAndroidVersion, err)
		}

		if !installed {
			installCommands, err := rubycommand.GemInstall("calabash-android", configs.CalabashAndroidVersion)
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
			log.Printf("calabash-android %s installed", configs.CalabashAndroidVersion)
		}
	} else if useBundler {
		bundleInstallCmd, err := rubycommand.New("bundle", "install", "--jobs", "20", "--retry", "5")
		if err != nil {
			registerFail("Failed to create command, error: %s", err)
		}

		bundleInstallCmd.AppendEnvs("BUNDLE_GEMFILE=" + gemFilePath)
		bundleInstallCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

		log.Printf("$ %s", bundleInstallCmd.PrintableCommandArgs())

		if err := bundleInstallCmd.Run(); err != nil {
			registerFail("bundle install failed, error: %s", err)
		}
	} else {
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

	{
		resignEnvs := []string{}

		resignArgs := []string{"calabash-android"}
		if configs.CalabashAndroidVersion != "" {
			resignArgs = append(resignArgs, fmt.Sprintf("_%s_", configs.CalabashAndroidVersion))
		} else if useBundler {
			resignArgs = append([]string{"bundle", "exec"}, resignArgs...)
			resignEnvs = append(resignEnvs, "BUNDLE_GEMFILE="+gemFilePath)
		}

		resignArgs = append(resignArgs, "resign", configs.ApkPath)

		resignCmd, err := rubycommand.NewFromSlice(resignArgs...)
		if err != nil {
			registerFail("Failed to create command, error: %s", err)
		}

		resignCmd.AppendEnvs(resignEnvs...)
		resignCmd.SetDir(workDir)
		resignCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

		log.Printf("$ %s", resignCmd.PrintableCommandArgs())
		fmt.Println()

		if err := resignCmd.Run(); err != nil {
			registerFail("Failed to run command, error: %s", err)
		}
	}
	// ---

	//
	// Run calabash-android
	fmt.Println()
	log.Infof("Running calabash-android test...")

	{
		runEnvs := []string{}

		runArgs := []string{"calabash-android"}
		if configs.CalabashAndroidVersion != "" {
			runArgs = append(runArgs, fmt.Sprintf("_%s_", configs.CalabashAndroidVersion))
		} else if useBundler {
			runArgs = append([]string{"bundle", "exec"}, runArgs...)
			runEnvs = append(runEnvs, "BUNDLE_GEMFILE="+gemFilePath)
		}

		runArgs = append(runArgs, "run", configs.ApkPath)

		runCmd, err := rubycommand.NewFromSlice(runArgs...)
		if err != nil {
			registerFail("Failed to create command, error: %s", err)
		}

		runCmd.AppendEnvs(runEnvs...)
		runCmd.SetDir(workDir)
		runCmd.SetStdout(os.Stdout).SetStderr(os.Stderr)

		log.Printf("$ %s", runCmd.PrintableCommandArgs())
		fmt.Println()

		if err := runCmd.Run(); err != nil {
			registerFail("Failed to run command, error: %s", err)
		}
	}
	// ---

	if err := exportEnvironmentWithEnvman("BITRISE_XAMARIN_TEST_RESULT", "succeeded"); err != nil {
		log.Warnf("Failed to export environment: %s, error: %s", "BITRISE_XAMARIN_TEST_RESULT", err)
	}
}
