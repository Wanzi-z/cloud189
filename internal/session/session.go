package session

import (
	"path"
	"strings"

	"github.com/spf13/cobra"
)

var workingDirectory = ""

func Pwd() string {
	return workingDirectory
}
func Base() string {
	return path.Base(workingDirectory)
}
func SetWorkDir(path string) {
	workingDirectory = path
}
func Join(name string) string {
	if isScoped(name) {
		return name
	}
	if path.IsAbs(name) {
		return name
	}
	if isScoped(workingDirectory) {
		separator := strings.IndexByte(workingDirectory, ':')
		joined := path.Join(workingDirectory[separator+1:], name)
		if !path.IsAbs(joined) {
			joined = "/" + joined
		}
		return workingDirectory[:separator+1] + joined
	}
	return path.Join(workingDirectory, name)
}

func isScoped(name string) bool {
	separator := strings.IndexByte(name, ':')
	if separator <= 0 || (len(name) != separator+1 && name[separator+1] != '/') {
		return false
	}
	for _, char := range []byte(name[:separator]) {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
func Parse(cmd *cobra.Command, args []string) {
	for i, arg := range args {
		args[i] = Join(arg)
	}
}
