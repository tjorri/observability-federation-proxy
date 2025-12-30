//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	testClient     *http.Client
	proxyBaseURL   string
	proxyCmd       *exec.Cmd
	e2eDir         string
	generatedConfig string
)

func TestMain(m *testing.M) {
	// Determine e2e directory
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "Failed to determine e2e directory")
		os.Exit(1)
	}
	e2eDir = filepath.Dir(filename)

	// Override with env var if set
	if envDir := os.Getenv("E2E_DIR"); envDir != "" {
		e2eDir = envDir
	}

	// Setup
	if err := setupTestEnvironment(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup test environment: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown
	teardownTestEnvironment()

	os.Exit(code)
}

func setupTestEnvironment() error {
	kubeconfigPath := filepath.Join(e2eDir, "kubeconfig")

	// Check if kubeconfig exists (cluster should be set up via make e2e-setup)
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kubeconfig not found at %s - run 'make e2e-setup' first", kubeconfigPath)
	}

	// Generate config with correct kubeconfig path
	if err := generateConfig(kubeconfigPath); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	// Start proxy in background
	if err := startProxy(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	// Setup HTTP client
	testClient = &http.Client{Timeout: 30 * time.Second}
	proxyBaseURL = "http://localhost:18080"

	// Wait for proxy to be ready
	if err := waitForProxyReady(); err != nil {
		stopProxy()
		return fmt.Errorf("proxy failed to become ready: %w", err)
	}

	return nil
}

func generateConfig(kubeconfigPath string) error {
	// Read template config
	templatePath := filepath.Join(e2eDir, "testdata", "config.yaml")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read config template: %w", err)
	}

	// Replace placeholder with actual kubeconfig path
	configContent := strings.Replace(string(content), `path: ""`, fmt.Sprintf(`path: "%s"`, kubeconfigPath), 1)

	// Write generated config
	generatedConfig = filepath.Join(e2eDir, "testdata", "config-generated.yaml")
	if err := os.WriteFile(generatedConfig, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write generated config: %w", err)
	}

	fmt.Printf("==> Generated config with kubeconfig: %s\n", kubeconfigPath)
	return nil
}

func startProxy() error {
	projectRoot := filepath.Dir(e2eDir)
	configPath := generatedConfig

	// Build the proxy first
	buildCmd := exec.Command("go", "build", "-o", filepath.Join(projectRoot, "bin", "e2e-proxy"), "./cmd/proxy")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build proxy: %w", err)
	}

	// Start the proxy
	proxyCmd = exec.Command(filepath.Join(projectRoot, "bin", "e2e-proxy"), "--config", configPath)
	proxyCmd.Dir = projectRoot
	proxyCmd.Stdout = os.Stdout
	proxyCmd.Stderr = os.Stderr

	if err := proxyCmd.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	fmt.Printf("==> Started proxy (PID: %d)\n", proxyCmd.Process.Pid)
	return nil
}

func stopProxy() {
	if proxyCmd != nil && proxyCmd.Process != nil {
		fmt.Printf("==> Stopping proxy (PID: %d)\n", proxyCmd.Process.Pid)
		proxyCmd.Process.Signal(syscall.SIGTERM)
		// Wait for graceful shutdown
		done := make(chan error, 1)
		go func() {
			done <- proxyCmd.Wait()
		}()
		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill if graceful shutdown takes too long
			proxyCmd.Process.Kill()
		}
	}
}

func waitForProxyReady() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("==> Waiting for proxy to be ready...")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for proxy to be ready")
		case <-ticker.C:
			resp, err := testClient.Get(proxyBaseURL + "/healthz")
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Println("==> Proxy is ready")
				return nil
			}
		}
	}
}

func teardownTestEnvironment() {
	stopProxy()
	// Clean up generated config
	if generatedConfig != "" {
		os.Remove(generatedConfig)
	}
}
