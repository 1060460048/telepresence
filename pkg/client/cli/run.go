package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/datawire/telepresence2/v2/pkg/client"
)

func runAsRoot(exe string, args []string) error {
	if os.Geteuid() != 0 {
		if err := exec.Command("sudo", "-n", "true").Run(); err != nil {
			fmt.Printf("Need root privileges to run %q\n", client.ShellString(exe, args))
			if err = exec.Command("sudo", "true").Run(); err != nil {
				return err
			}
		}
		args = append([]string{"-n", "-E", exe}, args...)
		exe = "sudo"
	}
	return start(exe, args, false, nil, nil, nil)
}

func start(exe string, args []string, wait bool, stdin io.Reader, stdout, stderr io.Writer, env ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if !wait {
		// Process must live in a process group of its own to prevent
		// getting affected by <ctrl-c> in the terminal
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	var err error
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("%s: %v", client.ShellString(exe, args), err)
	}
	if !wait {
		_ = cmd.Process.Release()
		return nil
	}

	// Ensure that SIGINT and SIGTERM are propagated to the child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		if sig == nil {
			return
		}
		_ = cmd.Process.Signal(sig)
	}()
	s, err := cmd.Process.Wait()
	if err != nil {
		return fmt.Errorf("%s: %v", client.ShellString(exe, args), err)
	}

	sigCh <- nil
	exitCode := s.ExitCode()
	if exitCode != 0 {
		return fmt.Errorf("%s %s: exited with %d", exe, strings.Join(args, " "), exitCode)
	}
	return nil
}
