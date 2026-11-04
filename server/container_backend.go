package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/pkg/errors"

	"github.com/sorenisanerd/gotty/pkg/auth"
)

// CWDResolver is an optional interface implemented by Slave backends that can
// report the active working directory of the running shell process.
type CWDResolver interface {
	ResolveCWD() (string, error)
}

// ContainerOptions configures the container runtime backend.
type ContainerOptions struct {
	Runtime      string // "docker" or "podman" (auto-detected if empty)
	DefaultImage string // e.g., "gotty-sandbox:latest"
	Mode         string // "host", "sandbox" (defaults based on user permissions)
	FallbackHost bool   // Fallback to local shell if container runtime is unavailable
}

// BuildContainerArgs constructs the CLI arguments for spawning a sandboxed user session container.
//   - Sandboxed container:
//     --security-opt label=disable -u <uid>:<gid> -v /home/<user>:/home/<user> -w /home/<user>
func BuildContainerArgs(user *auth.UserIdentity, containerName string, image string, shell string) []string {
	if image == "" {
		image = "gotty-sandbox:latest"
	}
	if shell == "" {
		shell = user.Shell
		if shell == "" {
			shell = "/bin/bash"
		}
	}

	homeDir := user.HomeDir
	if homeDir == "" {
		homeDir = filepath.Join("/home", user.Username)
	}
	if shell == "" || shell == "/bin/sh" {
		if strings.Contains(image, "sandbox") || strings.Contains(image, "gotty") {
			shell = "/bin/bash"
		}
	}

	args := []string{
		"run", "-it", "--rm",
		"--name", containerName,
		"-e", "TERM=xterm-256color",
		"-e", "GOTTY_USER=" + user.Username,
		"--security-opt", "label=disable",
		"-u", fmt.Sprintf("%d:%d", user.UID, user.GID),
		"-v", fmt.Sprintf("%s:%s", homeDir, homeDir),
	}
	if fileExists("/usr/local/bin/godl") {
		args = append(args, "-v", "/usr/local/bin/godl:/usr/local/bin/godl:ro")
	}
	args = append(args,
		"-w", homeDir,
		"-e", "HOME="+homeDir,
		"-e", "USER="+user.Username,
		image,
		shell,
	)

	return args
}

// ContainerSlave manages an interactive PTY connected to a container (or local shell fallback).
type ContainerSlave struct {
	user          *auth.UserIdentity
	containerName string
	runtime       string
	isDirectHost  bool
	cmd           *exec.Cmd
	ptyFile       *os.File
	ptyClosed     chan struct{}
}

// NewContainerSlave spawns a container (or local PTY shell fallback) for the given UserIdentity.
func NewContainerSlave(user *auth.UserIdentity, sessionID string, opts ContainerOptions) (*ContainerSlave, error) {
	if user == nil {
		return nil, errors.New("container_backend: nil user identity")
	}

	runtime := opts.Runtime
	if runtime == "" {
		if _, err := exec.LookPath("docker"); err == nil {
			runtime = "docker"
		} else if _, err := exec.LookPath("podman"); err == nil {
			runtime = "podman"
		}
	}

	containerName := fmt.Sprintf("gotty-%s-%s", user.Username, sessionID)

	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		if user.IsHostAccess {
			mode = "host"
		} else {
			mode = "sandbox"
		}
	}

	// Privileged container mode is removed: users with host access spawn native host shell
	// where they authenticate with sudo; all other requests or users without host access use sandbox container.
	if mode == "privileged" || mode == "privileged_container" {
		if user.IsHostAccess {
			mode = "host"
		} else {
			mode = "sandbox"
		}
	}

	// Security: If user does not have host access, force unprivileged sandbox container
	if !user.IsHostAccess && mode == "host" {
		mode = "sandbox"
	}

	defaultImg := opts.DefaultImage
	if defaultImg == "" || defaultImg == "alpine:latest" {
		defaultImg = "gotty-sandbox:latest"
	}

	var cmd *exec.Cmd
	var isDirectHost bool
	if mode == "sandbox" && runtime != "" && !opts.FallbackHost {
		cArgs := BuildContainerArgs(user, containerName, defaultImg, user.Shell)
		if runtime == "podman" && len(cArgs) >= 2 {
			homeDir := user.HomeDir
			if homeDir == "" {
				homeDir = filepath.Join("/home", user.Username)
			}
			shell := user.Shell
			if shell == "" {
				shell = "/bin/bash"
			}
			passwdEntry := fmt.Sprintf("%s:x:%d:%d:%s:%s:%s", user.Username, user.UID, user.GID, user.Username, homeDir, shell)
			idx := len(cArgs) - 2
			cArgs = append(cArgs[:idx], append([]string{"--passwd-entry=" + passwdEntry}, cArgs[idx:]...)...)
		}
		cmd = exec.Command(runtime, cArgs...)
	} else {
		isDirectHost = true
		// Host access: spawn interactive host shell directly with native host PTY
		shell := user.Shell
		if shell == "" || !fileExists(shell) {
			if fileExists("/bin/bash") {
				shell = "/bin/bash"
			} else {
				shell = "/bin/sh"
			}
		}
		cmd = exec.Command(shell)
		if user.HomeDir != "" && fileExists(user.HomeDir) {
			cmd.Dir = user.HomeDir
		}
		cmd.Env = append(os.Environ(),
			"TERM=xterm-256color",
			"GOTTY_USER="+user.Username,
			"GOTTY_SESSION_ID="+sessionID,
			"GOTTY_MODE="+mode,
			"USER="+user.Username,
			"LOGNAME="+user.Username,
			"SHELL="+shell,
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin",
		)
		if user.HomeDir != "" {
			cmd.Env = append(cmd.Env, "HOME="+user.HomeDir)
		}

		xdgRuntimeDir := fmt.Sprintf("/run/user/%d", user.UID)
		if fileExists(xdgRuntimeDir) {
			cmd.Env = append(cmd.Env, "XDG_RUNTIME_DIR="+xdgRuntimeDir)
			dbusBus := filepath.Join(xdgRuntimeDir, "bus")
			if fileExists(dbusBus) {
				cmd.Env = append(cmd.Env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+dbusBus)
			}
		}
		if user.UID > 0 && os.Geteuid() == 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{
					Uid:    user.UID,
					Gid:    user.GID,
					Groups: user.GroupIDs,
				},
			}
		}
	}

	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to start PTY session for %s", user.Username)
	}

	ptyClosed := make(chan struct{})
	cs := &ContainerSlave{
		user:          user,
		containerName: containerName,
		runtime:       runtime,
		isDirectHost:  isDirectHost,
		cmd:           cmd,
		ptyFile:       ptyFile,
		ptyClosed:     ptyClosed,
	}

	go func() {
		defer func() {
			_ = cs.ptyFile.Close()
			close(cs.ptyClosed)
		}()
		_ = cs.cmd.Wait()
	}()

	return cs, nil
}

func (cs *ContainerSlave) Read(p []byte) (int, error) {
	return cs.ptyFile.Read(p)
}

func (cs *ContainerSlave) Write(p []byte) (int, error) {
	return cs.ptyFile.Write(p)
}

func (cs *ContainerSlave) Close() error {
	if !cs.isDirectHost && cs.runtime != "" && cs.containerName != "" {
		_ = exec.Command(cs.runtime, "stop", "-t", "2", cs.containerName).Run()
	}
	if cs.cmd != nil && cs.cmd.Process != nil {
		_ = cs.cmd.Process.Signal(syscall.SIGHUP)
	}
	select {
	case <-cs.ptyClosed:
		return nil
	case <-time.After(3 * time.Second):
		if cs.cmd != nil && cs.cmd.Process != nil {
			_ = cs.cmd.Process.Signal(syscall.SIGKILL)
		}
		return nil
	}
}

func (cs *ContainerSlave) WindowTitleVariables() map[string]interface{} {
	pid := 0
	if cs.cmd != nil && cs.cmd.Process != nil {
		pid = cs.cmd.Process.Pid
	}
	return map[string]interface{}{
		"command":  cs.user.Shell,
		"username": cs.user.Username,
		"pid":      pid,
	}
}

func (cs *ContainerSlave) ResizeTerminal(width int, height int) error {
	window := pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	}
	return pty.Setsize(cs.ptyFile, &window)
}

// ResolveCWD resolves the active working directory of the session process via /proc/<pid>/cwd.
func (cs *ContainerSlave) ResolveCWD() (string, error) {
	if !cs.isDirectHost && cs.runtime != "" && cs.containerName != "" {
		out, err := exec.Command(cs.runtime, "inspect", "--format", "{{.State.Pid}}", cs.containerName).Output()
		if err == nil {
			pidStr := strings.TrimSpace(string(out))
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				if cwd, err := resolveProcCWD(pid); err == nil {
					return cwd, nil
				}
			}
		}
	}
	if cs.cmd != nil && cs.cmd.Process != nil && cs.cmd.Process.Pid > 0 {
		if cwd, err := resolveProcCWD(cs.cmd.Process.Pid); err == nil {
			return cwd, nil
		}
	}
	return "", errors.New("unable to resolve process CWD")
}

func resolveProcCWD(pid int) (string, error) {
	// Check if process has an active child process (e.g., subshell) first
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	if data, err := os.ReadFile(childrenPath); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if childPid, err := strconv.Atoi(fields[len(fields)-1]); err == nil && childPid > 0 {
				if childCWD, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", childPid)); err == nil && childCWD != "" {
					return childCWD, nil
				}
			}
		}
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil || cwd == "" {
		return "", errors.New("proc cwd link unreadable")
	}
	return cwd, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
