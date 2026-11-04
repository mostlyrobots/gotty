package homedir

import (
	"os"
	"strings"
)

func Expand(path string) string {
	if strings.HasPrefix(path, "~/") {
		return os.Getenv("HOME") + path[1:]
	}
	return path
}
