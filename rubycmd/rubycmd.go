package rubycmd

import (
	"errors"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/cmdex"
)

const (
	systemRubyPth = "/usr/bin/ruby"
	brewRubyPth   = "/usr/local/bin/ruby"
)

// ----------------------
// RubyCommand

// RubyInstallType ...
type RubyInstallType int8

const (
	// SystemRuby ...
	SystemRuby RubyInstallType = iota
	// BrewRuby ...
	BrewRuby
	// RVMRuby ...
	RVMRuby
	// RbenvRuby ...
	RbenvRuby
)

// RubyCommandModel ...
type RubyCommandModel struct {
	rubyInstallType RubyInstallType
}

func cmdExist(cmdSlice []string) bool {
	if len(cmdSlice) == 0 {
		return false
	}

	cmd, err := cmdex.NewCommandFromSlice(cmdSlice)
	if err != nil {
		return false
	}

	return (cmd.Run() == nil)
}

// NewRubyCommandModel ...
func NewRubyCommandModel() (RubyCommandModel, error) {
	whichRuby, err := cmdex.NewCommand("which", "ruby").RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return RubyCommandModel{}, err
	}

	command := RubyCommandModel{}

	if whichRuby == systemRubyPth {
		command.rubyInstallType = SystemRuby
	} else if whichRuby == brewRubyPth {
		command.rubyInstallType = BrewRuby
	} else if cmdExist([]string{"rvm", "-v"}) {
		command.rubyInstallType = RVMRuby
	} else if cmdExist([]string{"rbenv", "-v"}) {
		command.rubyInstallType = RbenvRuby
	} else {
		return RubyCommandModel{}, errors.New("unkown ruby installation type")
	}

	return command, nil
}

func (command RubyCommandModel) sudoNeeded(cmdSlice []string) bool {
	if command.rubyInstallType != SystemRuby {
		return false
	}

	if len(cmdSlice) < 2 {
		return false
	}

	isGemManagementCmd := (cmdSlice[0] == "gem" || cmdSlice[0] == "bundle")
	isInstallOrUnintsallCmd := (cmdSlice[1] == "install" || cmdSlice[1] == "uninstall")

	return (isGemManagementCmd && isInstallOrUnintsallCmd)
}

// Command ...
func (command RubyCommandModel) Command(useBundle bool, cmdSlice []string) (*cmdex.CommandModel, error) {
	if useBundle {
		cmdSlice = append([]string{"bundle", "exec"}, cmdSlice...)
	}

	if command.sudoNeeded(cmdSlice) {
		cmdSlice = append([]string{"sudo"}, cmdSlice...)
	}

	cmd, err := cmdex.NewCommandFromSlice(cmdSlice)
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

// GemInstallCommands ...
func (command RubyCommandModel) GemInstallCommands(gem, version string) ([]*cmdex.CommandModel, error) {
	commands := []*cmdex.CommandModel{}

	cmdSlice := []string{"gem", "install", gem}

	if version != "" {
		cmdSlice = append(cmdSlice, "-v", version)
	}

	cmdSlice = append(cmdSlice, "--no-document")

	cmd, err := command.Command(false, cmdSlice)
	if err != nil {
		return []*cmdex.CommandModel{}, err
	}

	commands = append(commands, cmd)

	if command.rubyInstallType == RbenvRuby {
		cmdSlice := []string{"rbenv", "rehash"}

		cmd, err := command.Command(false, cmdSlice)
		if err != nil {
			return []*cmdex.CommandModel{}, err
		}

		commands = append(commands, cmd)
	}

	return commands, nil
}

// IsGemInstalled ...
func (command RubyCommandModel) IsGemInstalled(gem, version string) (bool, error) {
	cmdSlice := []string{"gem", "list"}

	cmd, err := command.Command(false, cmdSlice)
	if err != nil {
		return false, err
	}

	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return false, err
	}

	regexpStr := gem + ` \((?P<versions>.*)\)`
	exp := regexp.MustCompile(regexpStr)
	matches := exp.FindStringSubmatch(out)
	if len(matches) > 1 {
		if version == "" {
			return true, nil
		}

		versionsStr := matches[1]
		versions := strings.Split(versionsStr, ", ")

		for _, v := range versions {
			if v == version {
				return true, nil
			}
		}
	}

	return false, nil
}
