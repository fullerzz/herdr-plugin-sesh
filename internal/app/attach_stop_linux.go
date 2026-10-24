package app

import (
	"bytes"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

const (
	attachTermiosGet = unix.TCGETS
	attachTermiosSet = unix.TCSETS
)

func processStopped(pid int) bool {
	//nolint:gosec // The path is built from a process ID.
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the parenthesized command name, which may itself contain ") ".
	i := bytes.LastIndexByte(data, ')')
	return i >= 0 && i+2 < len(data) && data[i+2] == 'T'
}
