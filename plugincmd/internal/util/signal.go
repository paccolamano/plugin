package util

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

func IsServeProcess() bool {
	if len(os.Args) < 2 {
		return true // PocketBase default is serve
	}
	return slices.Contains(os.Args[1:], "serve")
}

func SetupRestartSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		<-ch
		exe, err := os.Executable()
		if err != nil {
			os.Exit(1)
		}
		_ = syscall.Exec(exe, os.Args, os.Environ())
		os.Exit(1)
	}()
}

func SignalServe(dir string) error {
	data, err := os.ReadFile(PidFilePath(dir))
	if err != nil {
		return fmt.Errorf("server PID file not found — is the server running? (%w)", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("could not find server process (PID %d): %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("could not signal server process (PID %d): %w", pid, err)
	}
	return nil
}

func PidFilePath(dir string) string {
	return filepath.Join(dir, ".serve.pid")
}
