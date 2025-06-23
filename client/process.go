package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/rubys/mcp-go-sdk/transport"
)

// ProcessConfig configures process spawning for MCP servers
type ProcessConfig struct {
	// Command is the executable to run
	Command string

	// Args are command line arguments to pass to the executable
	Args []string

	// Env specifies the environment variables for the process
	// If nil, a default safe environment will be used
	Env []string

	// Dir specifies the working directory for the process
	// If empty, the current working directory is used
	Dir string

	// StderrMode controls how stderr is handled
	StderrMode StderrMode

	// EnableShell determines whether to run the command through a shell
	EnableShell bool
}

// StderrMode controls how the child process's stderr is handled
type StderrMode int

const (
	// StderrInherit pipes stderr to the parent process's stderr (default)
	StderrInherit StderrMode = iota

	// StderrPipe creates a pipe for stderr that can be read separately
	StderrPipe

	// StderrDiscard discards all stderr output
	StderrDiscard

	// StderrToStdout redirects stderr to stdout
	StderrToStdout
)

// ProcessTransport wraps a stdio transport with process management
type ProcessTransport struct {
	// Embedded stdio transport
	*transport.StdioTransport

	// Process management
	cmd    *exec.Cmd
	stderr io.ReadCloser
	mu     sync.RWMutex

	// Configuration
	config ProcessConfig
}

// DefaultEnvironmentVariables returns a list of safe environment variables to inherit
func DefaultEnvironmentVariables() []string {
	if runtime.GOOS == "windows" {
		return []string{
			"APPDATA",
			"HOMEDRIVE", 
			"HOMEPATH",
			"LOCALAPPDATA",
			"PATH",
			"PROCESSOR_ARCHITECTURE",
			"SYSTEMDRIVE",
			"SYSTEMROOT",
			"TEMP",
			"USERNAME",
			"USERPROFILE",
		}
	}

	// Unix-like systems
	return []string{
		"HOME",
		"LOGNAME",
		"PATH", 
		"SHELL",
		"TERM",
		"USER",
	}
}

// GetDefaultEnvironment returns a default environment with only safe variables
func GetDefaultEnvironment() []string {
	var env []string
	defaultVars := DefaultEnvironmentVariables()
	
	for _, key := range defaultVars {
		if value := os.Getenv(key); value != "" {
			// Skip functions (security risk on some systems)
			if len(value) > 2 && value[:2] == "()" {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	return env
}

// NewProcessTransport creates a new process transport that spawns a server process
func NewProcessTransport(ctx context.Context, config ProcessConfig) (*ProcessTransport, error) {
	if config.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	// Set default environment if not provided
	if config.Env == nil {
		config.Env = GetDefaultEnvironment()
	}

	pt := &ProcessTransport{
		config: config,
	}

	return pt, nil
}

// Start spawns the server process and creates the stdio transport
func (pt *ProcessTransport) Start(ctx context.Context) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.cmd != nil {
		return fmt.Errorf("process already started")
	}

	// Create command
	var cmd *exec.Cmd
	if pt.config.EnableShell {
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "cmd", "/c", pt.config.Command+" "+joinArgs(pt.config.Args))
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", pt.config.Command+" "+joinArgs(pt.config.Args))
		}
	} else {
		cmd = exec.CommandContext(ctx, pt.config.Command, pt.config.Args...)
	}

	// Set environment
	cmd.Env = pt.config.Env

	// Set working directory
	if pt.config.Dir != "" {
		cmd.Dir = pt.config.Dir
	}

	// Configure stdin/stdout pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Configure stderr based on mode
	switch pt.config.StderrMode {
	case StderrInherit:
		cmd.Stderr = os.Stderr
	case StderrPipe:
		stderr, err := cmd.StderrPipe()
		if err != nil {
			stdin.Close()
			stdout.Close()
			return fmt.Errorf("failed to create stderr pipe: %w", err)
		}
		pt.stderr = stderr
	case StderrDiscard:
		cmd.Stderr = nil
	case StderrToStdout:
		cmd.Stderr = cmd.Stdout
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		if pt.stderr != nil {
			pt.stderr.Close()
		}
		return fmt.Errorf("failed to start process: %w", err)
	}

	pt.cmd = cmd

	// Create stdio transport with the process pipes
	stdioConfig := transport.StdioConfig{
		Reader: stdout,
		Writer: stdin,
	}

	stdioTransport, err := transport.NewStdioTransport(ctx, stdioConfig)
	if err != nil {
		pt.cleanup()
		return fmt.Errorf("failed to create stdio transport: %w", err)
	}

	pt.StdioTransport = stdioTransport

	// StdioTransport starts automatically in its constructor
	return nil
}

// Stderr returns the stderr reader if StderrPipe mode was used
func (pt *ProcessTransport) Stderr() io.ReadCloser {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.stderr
}

// ProcessID returns the process ID of the spawned process
func (pt *ProcessTransport) ProcessID() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if pt.cmd != nil && pt.cmd.Process != nil {
		return pt.cmd.Process.Pid
	}
	return 0
}

// IsRunning returns true if the process is currently running
func (pt *ProcessTransport) IsRunning() bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	
	if pt.cmd == nil || pt.cmd.Process == nil {
		return false
	}

	// Check if process is still running
	if pt.cmd.ProcessState != nil {
		// Process has exited
		return false
	}

	if runtime.GOOS == "windows" {
		// On Windows, if ProcessState is nil, the process is still running
		return true
	}

	// On Unix-like systems, check if process is still alive
	// We can't use Signal(nil) as it's not supported on all systems
	// Instead, just check if we have a valid process and ProcessState is nil
	return pt.cmd.Process.Pid > 0
}

// Wait waits for the process to complete and returns its exit status
func (pt *ProcessTransport) Wait() error {
	pt.mu.RLock()
	cmd := pt.cmd
	pt.mu.RUnlock()

	if cmd == nil {
		return fmt.Errorf("process not started")
	}

	return cmd.Wait()
}

// Close terminates the process and cleans up resources
func (pt *ProcessTransport) Close() error {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	var err error

	// Close the stdio transport first
	if pt.StdioTransport != nil {
		if closeErr := pt.StdioTransport.Close(); closeErr != nil {
			err = closeErr
		}
	}

	// Clean up process resources
	pt.cleanup()

	return err
}

// cleanup terminates the process and closes pipes (must be called with mutex held)
func (pt *ProcessTransport) cleanup() {
	if pt.cmd != nil && pt.cmd.Process != nil {
		// Try to terminate gracefully first
		if runtime.GOOS == "windows" {
			pt.cmd.Process.Kill() // Windows doesn't have SIGTERM
		} else {
			pt.cmd.Process.Signal(os.Interrupt)
		}

		// Give the process a chance to exit gracefully
		done := make(chan error, 1)
		go func() {
			done <- pt.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill after timeout
			pt.cmd.Process.Kill()
			<-done // Wait for Wait() to complete
		}
	}

	if pt.stderr != nil {
		pt.stderr.Close()
		pt.stderr = nil
	}

	pt.cmd = nil
}

// joinArgs joins command arguments with proper quoting
func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}

	var result string
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		
		// Simple quoting - wrap in quotes if contains spaces
		if len(arg) > 0 && (arg[0] == '"' || arg[0] == '\'') {
			result += arg
		} else if containsSpaces(arg) {
			result += `"` + arg + `"`
		} else {
			result += arg
		}
	}
	
	return result
}

// containsSpaces checks if a string contains spaces or special characters
func containsSpaces(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}