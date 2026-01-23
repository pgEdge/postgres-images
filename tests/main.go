package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	// postgresStabilizationPeriod is the duration to wait after PostgreSQL
	// passes readiness checks to allow background workers, extensions, and
	// internal caches to fully initialize before running tests.
	postgresStabilizationPeriod = 2 * time.Second
)

// Test represents a single test case
type Test struct {
	Name           string
	Cmd            string
	ExpectedOutput func(exitCode int, output string) error
	StandardOnly   bool // Only run on standard flavor images
}

// TestRunner manages container lifecycle and test execution
type TestRunner struct {
	cli         *client.Client
	ctx         context.Context
	containerID string
	image       string
	flavor      string
}

// DefaultEntrypointRunner tests the image with its default entrypoint
type DefaultEntrypointRunner struct {
	cli   *client.Client
	ctx   context.Context
	image string
}

func main() {
	image, flavor := parseFlags()

	printHeader(image, flavor)

	cli, ctx := setupDockerClient()
	defaultRunner := &DefaultEntrypointRunner{
		cli:   cli,
		ctx:   ctx,
		image: image,
	}

	errorCount := runEntrypointTests(defaultRunner, flavor)
	errorCount += runExtensionTests(cli, ctx, image, flavor)

	printSummary(errorCount, flavor)
	if errorCount > 0 {
		os.Exit(1)
	}
}

func parseFlags() (string, string) {
	image := flag.String("image", "", "Docker image to test (required)")
	flavor := flag.String("flavor", "", "Image flavor: minimal or standard (required)")
	flag.Parse()

	if *image == "" || *flavor == "" {
		fmt.Println("Usage: go run main.go -image <image> -flavor <minimal|standard>")
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println("  -image   Docker image to test (e.g., ghcr.io/pgedge/pgedge-postgres:17-spock5-standard)")
		fmt.Println("  -flavor  Image flavor: 'minimal' or 'standard'")
		os.Exit(1)
	}

	if *flavor != "minimal" && *flavor != "standard" {
		log.Fatalf("Invalid flavor '%s'. Must be 'minimal' or 'standard'", *flavor)
	}

	return *image, *flavor
}

func printHeader(image, flavor string) {
	fmt.Println("pgEdge Postgres Image Test Suite")
	fmt.Println()
	fmt.Printf("  Image:  %s\n", truncateString(image, 80))
	fmt.Printf("  Flavor: %s\n", flavor)
	fmt.Println()
}

func setupDockerClient() (*client.Client, context.Context) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}
	return cli, ctx
}

func runEntrypointTests(runner *DefaultEntrypointRunner, flavor string) int {
	errorCount := 0

	// Phase 1: Test default entrypoint
	printPhaseHeader("Phase 1: Default Entrypoint Test")
	if err := runner.TestDefaultEntrypoint(); err != nil {
		errorCount++
		fmt.Printf("  Default entrypoint test                                ❌\n")
		log.Printf("    Error: %v", err)
	} else {
		fmt.Printf("  Default entrypoint test                                ✅\n")
	}
	fmt.Println()

	// Phase 2: Test Patroni entrypoint (standard only)
	if flavor == "standard" {
		printPhaseHeader("Phase 2: Patroni Entrypoint Test")
		if err := runner.TestPatroniEntrypoint(); err != nil {
			errorCount++
			fmt.Printf("  Patroni entrypoint test                                ❌\n")
			log.Printf("    Error: %v", err)
		} else {
			fmt.Printf("  Patroni entrypoint test                                ✅\n")
		}
		fmt.Println()
	}

	return errorCount
}

func runExtensionTests(cli *client.Client, ctx context.Context, image, flavor string) int {
	printPhaseHeader("Phase 3: Extension Tests")

	runner := &TestRunner{
		cli:    cli,
		ctx:    ctx,
		image:  image,
		flavor: flavor,
	}

	if err := runner.Start(); err != nil {
		log.Printf("Failed to start container: %v", err)
		// Start() handles its own cleanup on error via defer, but call cleanupContainer
		// as a safety net in case the container was created but not started
		runner.cleanupContainer()
		return 1
	}
	defer runner.Cleanup()

	tests := buildTestSuite()
	return runner.RunTests(tests)
}

func printPhaseHeader(title string) {
	fmt.Printf("%s\n", title)
	fmt.Println()
}

func printSummary(errorCount int, flavor string) {
	tests := buildTestSuite()
	extensionTests := 0
	for _, t := range tests {
		if !t.StandardOnly || flavor == "standard" {
			extensionTests++
		}
	}

	testsRun := 1 + extensionTests // default entrypoint + extensions
	if flavor == "standard" {
		testsRun++ // patroni entrypoint
	}

	fmt.Println()
	fmt.Println("Test Summary")
	fmt.Printf("  Tests Executed: %d\n", testsRun)
	fmt.Printf("  Errors:         %d\n", errorCount)
	if errorCount == 0 {
		fmt.Printf("  Status:         ✅ ALL TESTS PASSED\n")
	} else {
		fmt.Printf("  Status:         ❌ SOME TESTS FAILED\n")
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TestDefaultEntrypoint tests that the image starts correctly with its default entrypoint
func (r *DefaultEntrypointRunner) TestDefaultEntrypoint() error {
	fmt.Println("  Starting container with default entrypoint...")

	// Create container with default CMD (no custom postgres args)
	resp, err := r.cli.ContainerCreate(r.ctx, &container.Config{
		Image: r.image,
		Env: []string{
			"POSTGRES_PASSWORD=testpassword",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=testdb",
		},
		// No Cmd - use default entrypoint
	}, &container.HostConfig{}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	containerID := resp.ID
	defer func() {
		r.cli.ContainerStop(r.ctx, containerID, container.StopOptions{})
		r.cli.ContainerRemove(r.ctx, containerID, container.RemoveOptions{})
	}()

	if err := r.cli.ContainerStart(r.ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %w", err)
	}

	// Wait for PostgreSQL to be ready
	fmt.Println("  Waiting for PostgreSQL to be ready...")
	return r.waitForContainerCommand(
		containerID,
		[]string{"pg_isready", "-U", "postgres"},
		60*time.Second,
		1*time.Second,
		"PostgreSQL started successfully with default entrypoint!",
		"timeout waiting for PostgreSQL to be ready with default entrypoint",
	)
}

// TestPatroniEntrypoint tests that Patroni can start and initialize
func (r *DefaultEntrypointRunner) TestPatroniEntrypoint() error {
	fmt.Println("  Starting container with Patroni entrypoint...")

	patroniConfig := createPatroniTestConfig()
	containerID, err := r.startPatroniContainer(patroniConfig)
	if err != nil {
		return err
	}
	defer r.cleanupContainer(containerID)

	if err := r.cli.ContainerStart(r.ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %w", err)
	}

	return r.waitForPatroniAPI(containerID)
}

func createPatroniTestConfig() string {
	return `scope: pgedge-test
name: node1

restapi:
  listen: 0.0.0.0:8008
  connect_address: 127.0.0.1:8008

bootstrap:
  dcs:
    ttl: 30
    loop_wait: 10
    retry_timeout: 10
    maximum_lag_on_failover: 1048576
  initdb:
    - encoding: UTF8
    - data-checksums

postgresql:
  listen: 0.0.0.0:5432
  connect_address: 127.0.0.1:5432
  data_dir: /var/lib/pgsql/data
  authentication:
    superuser:
      username: postgres
      password: testpassword
    replication:
      username: replicator
      password: testpassword
`
}

func (r *DefaultEntrypointRunner) startPatroniContainer(patroniConfig string) (string, error) {
	// Use a here-document to safely write the config file without shell interpretation
	// This prevents command injection if patroniConfig contains special characters
	// The heredoc approach avoids needing to escape quotes or other shell metacharacters
	cmd := fmt.Sprintf(`cat > /tmp/patroni.yml <<'PATRONI_EOF'
%s
PATRONI_EOF
patroni /tmp/patroni.yml`, patroniConfig)

	resp, err := r.cli.ContainerCreate(r.ctx, &container.Config{
		Image: r.image,
		Env: []string{
			"PATRONI_SCOPE=pgedge-test",
			"PATRONI_NAME=node1",
		},
		Cmd: []string{
			"sh", "-c", cmd,
		},
	}, &container.HostConfig{}, nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("error creating container: %w", err)
	}
	return resp.ID, nil
}

func (r *DefaultEntrypointRunner) cleanupContainer(containerID string) {
	r.cli.ContainerStop(r.ctx, containerID, container.StopOptions{})
	r.cli.ContainerRemove(r.ctx, containerID, container.RemoveOptions{})
}

func (r *DefaultEntrypointRunner) waitForPatroniAPI(containerID string) error {
	fmt.Println("  Waiting for Patroni to initialize...")
	return r.waitForContainerCommand(
		containerID,
		[]string{"curl", "-sf", "http://127.0.0.1:8008/health"},
		90*time.Second,
		2*time.Second,
		"Patroni started and responding on REST API!",
		"timeout waiting for Patroni to initialize",
	)
}

// waitForContainerCommand executes a command in a container repeatedly until it succeeds or times out
func (r *DefaultEntrypointRunner) waitForContainerCommand(
	containerID string,
	cmd []string,
	timeout time.Duration,
	interval time.Duration,
	successMsg string,
	timeoutMsg string,
) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		execID, err := r.cli.ContainerExecCreate(r.ctx, containerID, container.ExecOptions{
			Cmd:          cmd,
			AttachStdout: true,
			AttachStderr: true,
		})
		if err != nil {
			time.Sleep(interval)
			continue
		}

		execResp, err := r.cli.ContainerExecAttach(r.ctx, execID.ID, container.ExecAttachOptions{})
		if err != nil {
			time.Sleep(interval)
			continue
		}
		execResp.Close()

		inspectResp, err := r.cli.ContainerExecInspect(r.ctx, execID.ID)
		if err != nil {
			time.Sleep(interval)
			continue
		}
		if inspectResp.ExitCode == 0 {
			fmt.Printf("  %s\n", successMsg)
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf(timeoutMsg)
}

func (r *TestRunner) Start() error {
	fmt.Println("  Starting container with extension config...")

	// Build shared_preload_libraries based on flavor
	// These extensions require preloading before they can be used
	// Note: We only include extensions that are guaranteed to be in all images
	sharedLibs := "spock,snowflake"
	if r.flavor == "standard" {
		sharedLibs = "spock,snowflake,pgaudit"
	}

	// Build postgres command with required configuration
	// Note: We pass these as postgres arguments, which the entrypoint will handle
	cmd := []string{
		"postgres",
		"-c", fmt.Sprintf("shared_preload_libraries=%s", sharedLibs),
		"-c", "wal_level=logical",
		"-c", "track_commit_timestamp=on",
		"-c", "max_replication_slots=10",
		"-c", "max_wal_senders=10",
		"-c", "snowflake.node=1",
	}

	resp, err := r.cli.ContainerCreate(r.ctx, &container.Config{
		Image: r.image,
		Env: []string{
			"POSTGRES_PASSWORD=testpassword",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=testdb",
		},
		Cmd: cmd,
	}, &container.HostConfig{}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	r.containerID = resp.ID
	fmt.Printf("Container created: %s\n", r.containerID[:12])

	// Track if we've successfully started to avoid double cleanup
	started := false
	defer func() {
		// If Start() fails after container creation, clean up the container
		if !started && r.containerID != "" {
			r.cleanupContainer()
		}
	}()

	if err := r.cli.ContainerStart(r.ctx, r.containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %w", err)
	}
	fmt.Println("Container started")

	// Wait for PostgreSQL to be ready
	fmt.Println("Waiting for PostgreSQL to be ready...")
	if err := r.waitForPostgres(60 * time.Second); err != nil {
		return fmt.Errorf("postgres failed to start: %w", err)
	}
	fmt.Println("PostgreSQL is ready!")
	fmt.Println()

	// Mark as successfully started so defer won't clean up
	started = true
	return nil
}

func (r *TestRunner) waitForPostgres(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// First check if pg_isready succeeds
		exitCode, _, err := r.exec("pg_isready -U postgres")
		if err == nil && exitCode == 0 {
			// Then verify we can actually connect and query
			exitCode, _, err := r.exec("psql -U postgres -d testdb -t -A -c 'SELECT 1'")
			if err == nil && exitCode == 0 {
				// Give PostgreSQL a short grace period even after a successful readiness check.
				// Although pg_isready and a trivial SELECT can succeed, background workers,
				// extensions, and internal caches may still be initializing. This delay helps
				// ensure a stable database state and reduces test flakiness in subsequent
				// operations that depend on a fully-initialized instance.
				time.Sleep(postgresStabilizationPeriod)
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for PostgreSQL to be ready")
}

// cleanupContainer removes a container, attempting to stop it first if it's running
func (r *TestRunner) cleanupContainer() {
	if r.containerID == "" {
		return
	}

	// Try to stop the container first (ignore errors if it's not running)
	_ = r.cli.ContainerStop(r.ctx, r.containerID, container.StopOptions{})

	// Remove the container
	if err := r.cli.ContainerRemove(r.ctx, r.containerID, container.RemoveOptions{}); err != nil {
		log.Printf("Error removing container: %v", err)
	}
}

func (r *TestRunner) Cleanup() {
	fmt.Println()
	fmt.Println("Cleaning up...")

	if r.containerID == "" {
		return
	}

	if err := r.cli.ContainerStop(r.ctx, r.containerID, container.StopOptions{}); err != nil {
		log.Printf("Error stopping container: %v", err)
	} else {
		fmt.Println("Container stopped")
	}

	if err := r.cli.ContainerRemove(r.ctx, r.containerID, container.RemoveOptions{}); err != nil {
		log.Printf("Error removing container: %v", err)
	} else {
		fmt.Println("Container removed")
	}
}

// parseCommand safely parses a command string into command and arguments
// This prevents command injection by avoiding shell interpretation
func parseCommand(cmd string) []string {
	// Simple parser that splits on spaces while respecting single and double quotes
	// This is safe for the hardcoded commands we use in tests
	var args []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for _, char := range cmd {
		switch {
		case char == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
		case char == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case char == ' ' && !inSingleQuote && !inDoubleQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	// Add any remaining content after the loop
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	// If no arguments were parsed, return the original command as a single argument
	if len(args) == 0 {
		return []string{cmd}
	}

	return args
}

func (r *TestRunner) exec(cmd string) (int, string, error) {
	// Check if container is still running
	inspect, err := r.cli.ContainerInspect(r.ctx, r.containerID)
	if err != nil {
		return -1, "", fmt.Errorf("error inspecting container: %w", err)
	}
	if !inspect.State.Running {
		return -1, "", fmt.Errorf("container is not running (status: %s)", inspect.State.Status)
	}

	// Parse command string safely to avoid command injection
	// This prevents shell interpretation of the command string
	cmdArgs := parseCommand(cmd)

	execID, err := r.cli.ContainerExecCreate(r.ctx, r.containerID, container.ExecOptions{
		Cmd:          cmdArgs,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return -1, "", fmt.Errorf("error creating exec: %w", err)
	}

	resp, err := r.cli.ContainerExecAttach(r.ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return -1, "", fmt.Errorf("error attaching to exec: %w", err)
	}
	defer resp.Close()

	var outputBuf bytes.Buffer
	_, err = stdcopy.StdCopy(&outputBuf, &outputBuf, resp.Reader)
	if err != nil {
		return -1, "", fmt.Errorf("error copying output: %w", err)
	}

	inspectResp, err := r.cli.ContainerExecInspect(r.ctx, execID.ID)
	if err != nil {
		return -1, "", fmt.Errorf("error inspecting exec: %w", err)
	}

	return inspectResp.ExitCode, outputBuf.String(), nil
}

func (r *TestRunner) RunTests(tests []Test) int {
	errorCount := 0

	for _, test := range tests {
		// Skip standard-only tests for minimal flavor
		if test.StandardOnly && r.flavor != "standard" {
			continue
		}

		fmt.Printf("  %-55s ", test.Name)

		exitCode, output, err := r.exec(test.Cmd)
		if err != nil {
			errorCount++
			fmt.Println("❌")
			log.Printf("    Error executing command: %v", err)
			continue
		}

		if err := test.ExpectedOutput(exitCode, output); err != nil {
			errorCount++
			fmt.Println("❌")
			log.Printf("    Command: %s", test.Cmd)
			log.Printf("    Error: %v", err)
			log.Printf("    Output: %s", strings.TrimSpace(output))
		} else {
			fmt.Println("✅")
		}
	}

	return errorCount
}

func buildTestSuite() []Test {
	tests := []Test{}
	tests = append(tests, getPostgreSQLTests()...)
	tests = append(tests, getCommonExtensionTests()...)
	tests = append(tests, getStandardOnlyTests()...)
	return tests
}

func getPostgreSQLTests() []Test {
	return []Test{
		{
			Name: "PostgreSQL accepts connections",
			Cmd:  "psql -U postgres -d testdb -t -A -c 'SELECT 1'",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "1" {
					return fmt.Errorf("unexpected output: %s", output)
				}
				return nil
			},
		},
		{
			Name: "PostgreSQL version check",
			Cmd:  "psql -U postgres -d testdb -t -A -c 'SHOW server_version'",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) == "" {
					return fmt.Errorf("empty version output")
				}
				return nil
			},
		},
	}
}

func getCommonExtensionTests() []Test {
	return []Test{
		{
			Name:           "Spock extension can be created",
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS spock; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name: "Spock subscription table accessible",
			Cmd:  "psql -U postgres -d testdb -t -A -c \"SELECT count(*) FROM spock.subscription;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "0" {
					return fmt.Errorf("unexpected output: %s", output)
				}
				return nil
			},
		},
		{
			Name:           "LOLOR extension can be created",
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS lolor; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name: "LOLOR lo_create works",
			Cmd:  "psql -U postgres -d testdb -t -A -c \"SELECT lo_create(200000);\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "200000" {
					return fmt.Errorf("unexpected output: %s (expected 200000)", output)
				}
				return nil
			},
		},
		{
			Name:           "Snowflake extension can be created",
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS snowflake; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name: "Snowflake ID generation works",
			Cmd:  "psql -U postgres -d testdb -t -A -c \"SELECT snowflake.nextval() > 0;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "t" {
					return fmt.Errorf("unexpected output: %s (expected 't')", output)
				}
				return nil
			},
		},
	}
}

func getStandardOnlyTests() []Test {
	return []Test{
		{
			Name:           "system_stats extension can be created",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS system_stats; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:         "system_stats pg_sys_os_info works",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"SELECT 1 FROM pg_sys_os_info();\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "1" {
					return fmt.Errorf("unexpected output: %s (expected 1)", output)
				}
				return nil
			},
		},
		{
			Name:           "pgvector extension can be created",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS vector; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:         "pgvector distance calculation works",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"SELECT '[1,2,3]'::vector <-> '[4,5,6]'::vector;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if !strings.HasPrefix(strings.TrimSpace(output), "5.196") {
					return fmt.Errorf("unexpected output: %s", output)
				}
				return nil
			},
		},
		{
			Name:           "PostGIS extension can be created",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS postgis; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:         "PostGIS ST_Distance works",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"SELECT ST_Distance(ST_Point(1, 1), ST_Point(4, 5));\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "5" {
					return fmt.Errorf("unexpected output: %s (expected 5)", output)
				}
				return nil
			},
		},
		{
			Name:           "pgaudit extension can be created",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS pgaudit; SELECT 1;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:         "pgBackRest is installed",
			StandardOnly: true,
			Cmd:          "pgbackrest version",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if !strings.Contains(output, "pgBackRest") {
					return fmt.Errorf("unexpected output: %s", output)
				}
				return nil
			},
		},
	}
}

func expectSuccess(exitCode int, output string) error {
	if exitCode != 0 {
		return fmt.Errorf("unexpected exit code: %d", exitCode)
	}
	return nil
}
