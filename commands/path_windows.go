//go:build windows
// +build windows

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/git-lfs/git-lfs/v3/subprocess"
	"golang.org/x/sys/windows/registry"
)

var (
	winBashPrefix string
	winBashMu     sync.Mutex
	winBashRe     *regexp.Regexp
)

func osLineEnding() string {
	return "\r\n"
}

// cleanRootPath replaces the windows root path prefix with a unix path prefix:
// "/". Git Bash (provided with Git For Windows) expands a path like "/foo" to
// the actual Windows directory, but with forward slashes. You can see this
// for yourself:
//
//	$ git /foo
//	git: 'C:/Program Files/Git/foo' is not a git command. See 'git --help'.
//
// You can check the path with `pwd -W`:
//
//	$ cd /
//	$ pwd
//	/
//	$ pwd -W
//	c:/Program Files/Git
func cleanRootPath(pattern string) string {
	winBashMu.Lock()
	defer winBashMu.Unlock()

	// check if path starts with windows drive letter
	if !winPathHasDrive(pattern) {
		return pattern
	}

	if len(winBashPrefix) < 1 {
		// cmd.Path is something like C:\Program Files\Git\usr\bin\pwd.exe
		cmd, err := subprocess.ExecCommand("pwd")
		if err != nil {
			return pattern
		}
		winBashPrefix = strings.Replace(filepath.Dir(filepath.Dir(filepath.Dir(cmd.Path))), `\`, "/", -1) + "/"
	}

	return strings.Replace(pattern, winBashPrefix, "/", 1)
}

func winPathHasDrive(pattern string) bool {
	if winBashRe == nil {
		winBashRe = regexp.MustCompile(`\A\w{1}:[/\/]`)
	}

	return winBashRe.MatchString(pattern)
}

const (
	FILE_ATTRIBUTE_RECALL_ON_OPEN        = 0x00040000
	FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS = 0x00400000
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

const (
	SYNCROOTS_PATH = "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Explorer\\SyncRootManager"
	PROVIDER_NAME  = "AnchorpointGitCloudProvider"
)

func isSyncRoot(path string) bool {
	// Ensure the path ends with a backslash
	if !strings.HasSuffix(path, `\`) {
		path += `\`
	}

	// Open the registry key
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, SYNCROOTS_PATH, registry.READ)
	if err != nil {
		return false
	}
	defer key.Close()

	// Enumerate subkeys
	subkeys, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return false
	}

	for _, subkey := range subkeys {
		if strings.Contains(subkey, PROVIDER_NAME) {
			fullSubKeyPath := fmt.Sprintf(`%s\%s\UserSyncRoots`, SYNCROOTS_PATH, subkey)
			subKey, err := registry.OpenKey(registry.LOCAL_MACHINE, fullSubKeyPath, registry.READ)
			if err == nil {
				defer subKey.Close()

				// Enumerate values
				values, err := subKey.ReadValueNames(-1)
				if err == nil {
					for _, valueName := range values {
						val, _, err := subKey.GetStringValue(valueName)
						if err == nil && val == path {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func isPlaceholderFile(path string) bool {
	if !cfg.IsSyncRoot {
		return false
	}

	exists := fileExists(path)
	if !exists {
		// assume it's a placeholder if the file doesn't exist (the parent folder might be virtual)
		// any call to the smudge filer will still download the file
		return true
	}

	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}

	attributes, err := syscall.GetFileAttributes(pointer)
	if err != nil {
		return false
	}

	if attributes&FILE_ATTRIBUTE_RECALL_ON_OPEN != 0 {
		return true
	}

	if attributes&FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS != 0 {
		return true
	}

	return false
}
