package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultEnvironmentVariables(t *testing.T) {
	vars := DefaultEnvironmentVariables()
	
	// Should contain PATH on all platforms
	assert.Contains(t, vars, "PATH")
	
	if runtime.GOOS == "windows" {
		assert.Contains(t, vars, "SYSTEMROOT")
		assert.Contains(t, vars, "USERNAME")
	} else {
		assert.Contains(t, vars, "HOME")
		assert.Contains(t, vars, "USER")
	}
}

func TestGetDefaultEnvironment(t *testing.T) {
	env := GetDefaultEnvironment()
	
	// Should have some environment variables
	assert.NotEmpty(t, env)
	
	// Each entry should be in KEY=VALUE format
	for _, entry := range env {
		assert.Contains(t, entry, "=")
		parts := strings.SplitN(entry, "=", 2)
		assert.Len(t, parts, 2)
		assert.NotEmpty(t, parts[0]) // Key should not be empty
	}
	
	// Should contain PATH
	var hasPath bool
	for _, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			hasPath = true
			break
		}
	}
	assert.True(t, hasPath, "Environment should contain PATH")
}

func TestNewProcessTransport_InvalidCommand(t *testing.T) {
	ctx := context.Background()
	
	// Test empty command
	_, err := NewProcessTransport(ctx, ProcessConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command is required")
}

func TestNewProcessTransport_ValidConfig(t *testing.T) {
	ctx := context.Background()
	
	config := ProcessConfig{
		Command: "echo",
		Args:    []string{"hello"},
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, pt)
	assert.Equal(t, "echo", pt.config.Command)
	assert.Equal(t, []string{"hello"}, pt.config.Args)
	assert.NotNil(t, pt.config.Env) // Should have default environment
}

func TestProcessTransport_StartAndStop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Use a command that will keep running for a while
	var cmd string
	var args []string
	
	if runtime.GOOS == "windows" {
		cmd = "ping"
		args = []string{"127.0.0.1", "-t"} // Continuous ping
	} else {
		cmd = "sleep"
		args = []string{"30"} // Sleep for 30 seconds
	}
	
	config := ProcessConfig{
		Command: cmd,
		Args:    args,
		StderrMode: StderrInherit,
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	// Initially not running
	assert.False(t, pt.IsRunning())
	assert.Equal(t, 0, pt.ProcessID())
	
	// Start the process
	err = pt.Start(ctx)
	require.NoError(t, err)
	
	// Should be running now
	assert.True(t, pt.IsRunning())
	assert.NotEqual(t, 0, pt.ProcessID())
	
	// Wait a bit
	time.Sleep(100 * time.Millisecond)
	
	// Should still be running
	assert.True(t, pt.IsRunning())
	
	// Close should terminate the process
	err = pt.Close()
	assert.NoError(t, err)
	
	// Give it time to clean up
	time.Sleep(100 * time.Millisecond)
	
	// Should no longer be running
	assert.False(t, pt.IsRunning())
}

func TestProcessTransport_StderrModes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	tests := []struct {
		name       string
		stderrMode StderrMode
		expectPipe bool
	}{
		{
			name:       "inherit",
			stderrMode: StderrInherit,
			expectPipe: false,
		},
		{
			name:       "pipe",
			stderrMode: StderrPipe,
			expectPipe: true,
		},
		{
			name:       "discard",
			stderrMode: StderrDiscard,
			expectPipe: false,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Use echo command that exits quickly
			config := ProcessConfig{
				Command:    "echo",
				Args:       []string{"test"},
				StderrMode: test.stderrMode,
			}
			
			pt, err := NewProcessTransport(ctx, config)
			require.NoError(t, err)
			
			err = pt.Start(ctx)
			require.NoError(t, err)
			defer pt.Close()
			
			if test.expectPipe {
				assert.NotNil(t, pt.Stderr())
			} else {
				assert.Nil(t, pt.Stderr())
			}
		})
	}
}

func TestProcessTransport_WorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "mcp_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	
	// Create a test file in the temp directory
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)
	
	var cmd string
	var args []string
	
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "dir", "test.txt"}
	} else {
		cmd = "ls"
		args = []string{"test.txt"}
	}
	
	config := ProcessConfig{
		Command: cmd,
		Args:    args,
		Dir:     tempDir,
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	err = pt.Start(ctx)
	require.NoError(t, err)
	defer pt.Close()
	
	// Wait for command to complete
	err = pt.Wait()
	assert.NoError(t, err)
}

func TestProcessTransport_CustomEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	customEnv := []string{
		"TEST_VAR=test_value",
		"PATH=" + os.Getenv("PATH"), // Keep PATH for command execution
	}
	
	var cmd string
	var args []string
	
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo", "%TEST_VAR%"}
	} else {
		cmd = "sh"
		args = []string{"-c", "echo $TEST_VAR"}
	}
	
	config := ProcessConfig{
		Command: cmd,
		Args:    args,
		Env:     customEnv,
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	err = pt.Start(ctx)
	require.NoError(t, err)
	defer pt.Close()
	
	// Should use custom environment
	assert.Equal(t, customEnv, pt.config.Env)
}

func TestProcessTransport_ShellMode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Use shell command with pipe or redirection
	var command string
	if runtime.GOOS == "windows" {
		command = "echo hello"
	} else {
		command = "echo hello"
	}
	
	config := ProcessConfig{
		Command:     command,
		EnableShell: true,
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	err = pt.Start(ctx)
	require.NoError(t, err)
	defer pt.Close()
	
	// Wait for command to complete
	err = pt.Wait()
	assert.NoError(t, err)
}

func TestProcessTransport_InvalidCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	config := ProcessConfig{
		Command: "nonexistent_command_12345",
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	err = pt.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start process")
}

func TestProcessTransport_DoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	config := ProcessConfig{
		Command: "echo",
		Args:    []string{"test"},
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	// First start should succeed
	err = pt.Start(ctx)
	require.NoError(t, err)
	defer pt.Close()
	
	// Second start should fail
	err = pt.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestProcessTransport_WaitOnNonStarted(t *testing.T) {
	ctx := context.Background()
	
	config := ProcessConfig{
		Command: "echo",
		Args:    []string{"test"},
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	// Wait on non-started process should fail
	err = pt.Wait()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestJoinArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "empty args",
			args:     []string{},
			expected: "",
		},
		{
			name:     "single arg no spaces",
			args:     []string{"hello"},
			expected: "hello",
		},
		{
			name:     "multiple args no spaces",
			args:     []string{"hello", "world"},
			expected: "hello world",
		},
		{
			name:     "args with spaces",
			args:     []string{"hello world", "test"},
			expected: `"hello world" test`,
		},
		{
			name:     "already quoted",
			args:     []string{`"hello world"`, "test"},
			expected: `"hello world" test`,
		},
		{
			name:     "single quoted",
			args:     []string{"'hello world'", "test"},
			expected: `'hello world' test`,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := joinArgs(test.args)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestContainsSpaces(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello", false},
		{"hello world", true},
		{"hello\tworld", true},
		{"hello\nworld", true},
		{"hello\rworld", true},
		{"", false},
		{"a", false},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("input_%s", test.input), func(t *testing.T) {
			result := containsSpaces(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestProcessTransport_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping process test in short mode")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	config := ProcessConfig{
		Command: "echo",
		Args:    []string{"test"},
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	err = pt.Start(ctx)
	require.NoError(t, err)
	defer pt.Close()
	
	// Test concurrent access to process info
	const numRoutines = 50
	results := make(chan bool, numRoutines)
	
	for i := 0; i < numRoutines; i++ {
		go func() {
			// These should not race
			_ = pt.IsRunning()
			pid := pt.ProcessID()
			stderr := pt.Stderr()
			
			results <- (pid >= 0) && (stderr == nil) // Should be consistent
		}()
	}
	
	// Collect all results
	for i := 0; i < numRoutines; i++ {
		result := <-results
		assert.True(t, result)
	}
}

// TestProcessTransport_RealMCPServer tests with a simple MCP server if available
func TestProcessTransport_RealMCPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real MCP server test in short mode")
	}
	
	// Look for a simple echo server or create one
	// This test is optional and will be skipped if no suitable server is found
	
	// Check if we have node.js available for a simple echo server
	_, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js not available for echo server test")
	}
	
	// Create a simple echo server script
	echoServer := `
const readline = require('readline');

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout,
  terminal: false
});

rl.on('line', (line) => {
  try {
    const msg = JSON.parse(line);
    if (msg.method === 'echo') {
      const response = {
        jsonrpc: '2.0',
        id: msg.id,
        result: { echo: msg.params }
      };
      console.log(JSON.stringify(response));
    }
  } catch (e) {
    // Ignore invalid JSON
  }
});
`
	
	// Write the echo server to a temporary file
	tempFile, err := os.CreateTemp("", "echo_server_*.js")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	
	_, err = tempFile.WriteString(echoServer)
	require.NoError(t, err)
	tempFile.Close()
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	config := ProcessConfig{
		Command: "node",
		Args:    []string{tempFile.Name()},
	}
	
	pt, err := NewProcessTransport(ctx, config)
	require.NoError(t, err)
	
	err = pt.Start(ctx)
	require.NoError(t, err)
	defer pt.Close()
	
	// Give the server time to start
	time.Sleep(100 * time.Millisecond)
	
	assert.True(t, pt.IsRunning())
	assert.NotEqual(t, 0, pt.ProcessID())
}

// Benchmark process creation and teardown
func BenchmarkProcessTransport_StartStop(b *testing.B) {
	ctx := context.Background()
	
	config := ProcessConfig{
		Command: "echo",
		Args:    []string{"benchmark"},
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		pt, err := NewProcessTransport(ctx, config)
		if err != nil {
			b.Fatal(err)
		}
		
		err = pt.Start(ctx)
		if err != nil {
			b.Fatal(err)
		}
		
		err = pt.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}