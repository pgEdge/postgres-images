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

	fmt.Printf("╔══════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  pgEdge Postgres Image Test Suite                                ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  Image:  %-56s║\n", truncateString(*image, 56))
	fmt.Printf("║  Flavor: %-56s║\n", *flavor)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════╝\n")
	fmt.Println()

	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Error creating Docker client: %v", err)
	}

	errorCount := 0

	// Phase 1: Test default entrypoint
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Phase 1: Default Entrypoint Test")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()

	defaultRunner := &DefaultEntrypointRunner{
		cli:   cli,
		ctx:   ctx,
		image: *image,
	}
	if err := defaultRunner.TestDefaultEntrypoint(); err != nil {
		errorCount++
		fmt.Printf("  Default entrypoint test                                ❌\n")
		log.Printf("    Error: %v", err)
	} else {
		fmt.Printf("  Default entrypoint test                                ✅\n")
	}
	fmt.Println()

	// Phase 2: Test Patroni entrypoint (standard only)
	if *flavor == "standard" {
		fmt.Println("═══════════════════════════════════════════════════════════════════")
		fmt.Println("  Phase 2: Patroni Entrypoint Test")
		fmt.Println("═══════════════════════════════════════════════════════════════════")
		fmt.Println()

		if err := defaultRunner.TestPatroniEntrypoint(); err != nil {
			errorCount++
			fmt.Printf("  Patroni entrypoint test                                ❌\n")
			log.Printf("    Error: %v", err)
		} else {
			fmt.Printf("  Patroni entrypoint test                                ✅\n")
		}
		fmt.Println()
	}

	// Phase 3: Extension tests (requires custom postgres config)
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Phase 3: Extension Tests")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()

	runner := &TestRunner{
		cli:    cli,
		ctx:    ctx,
		image:  *image,
		flavor: *flavor,
	}

	if err := runner.Start(); err != nil {
		log.Fatalf("Failed to start container: %v", err)
	}
	defer runner.Cleanup()

	tests := buildTestSuite()
	errorCount += runner.RunTests(tests)

	fmt.Println()
	fmt.Printf("╔══════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  Test Summary                                                    ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")

	// Count extension tests
	extensionTests := 0
	for _, t := range tests {
		if !t.StandardOnly || *flavor == "standard" {
			extensionTests++
		}
	}

	// Total tests: default entrypoint (1) + patroni entrypoint (1 if standard) + extension tests
	testsRun := 1 + extensionTests // default entrypoint + extensions
	if *flavor == "standard" {
		testsRun++ // patroni entrypoint
	}

	fmt.Printf("║  Tests Executed: %-48d║\n", testsRun)
	fmt.Printf("║  Errors:         %-48d║\n", errorCount)
	if errorCount == 0 {
		fmt.Printf("║  Status:         %-48s║\n", "✅ ALL TESTS PASSED")
	} else {
		fmt.Printf("║  Status:         %-48s║\n", "❌ SOME TESTS FAILED")
	}
	fmt.Printf("╚══════════════════════════════════════════════════════════════════╝\n")

	if errorCount > 0 {
		os.Exit(1)
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
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		execID, err := r.cli.ContainerExecCreate(r.ctx, containerID, container.ExecOptions{
			Cmd:          []string{"pg_isready", "-U", "postgres"},
			AttachStdout: true,
			AttachStderr: true,
		})
		if err == nil {
			execResp, err := r.cli.ContainerExecAttach(r.ctx, execID.ID, container.ExecAttachOptions{})
			if err == nil {
				execResp.Close()
				inspectResp, _ := r.cli.ContainerExecInspect(r.ctx, execID.ID)
				if inspectResp.ExitCode == 0 {
					fmt.Println("  PostgreSQL started successfully with default entrypoint!")
					return nil
				}
			}
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for PostgreSQL to be ready with default entrypoint")
}

// TestPatroniEntrypoint tests that Patroni can start and initialize
func (r *DefaultEntrypointRunner) TestPatroniEntrypoint() error {
	fmt.Println("  Starting container with Patroni entrypoint...")

	// Create a minimal patroni config for testing
	patroniConfig := `
scope: pgedge-test
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

	resp, err := r.cli.ContainerCreate(r.ctx, &container.Config{
		Image: r.image,
		Env: []string{
			"PATRONI_SCOPE=pgedge-test",
			"PATRONI_NAME=node1",
		},
		Cmd: []string{
			"sh", "-c",
			fmt.Sprintf("echo '%s' > /tmp/patroni.yml && patroni /tmp/patroni.yml", patroniConfig),
		},
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

	// Wait for Patroni REST API to respond
	fmt.Println("  Waiting for Patroni to initialize...")
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		execID, err := r.cli.ContainerExecCreate(r.ctx, containerID, container.ExecOptions{
			Cmd:          []string{"curl", "-sf", "http://127.0.0.1:8008/health"},
			AttachStdout: true,
			AttachStderr: true,
		})
		if err == nil {
			execResp, err := r.cli.ContainerExecAttach(r.ctx, execID.ID, container.ExecAttachOptions{})
			if err == nil {
				execResp.Close()
				inspectResp, _ := r.cli.ContainerExecInspect(r.ctx, execID.ID)
				if inspectResp.ExitCode == 0 {
					fmt.Println("  Patroni started and responding on REST API!")
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for Patroni to initialize")
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
	cmd := []string{
		"postgres",
		"-c", fmt.Sprintf("shared_preload_libraries=%s", sharedLibs),
		"-c", "wal_level=logical",
		"-c", "track_commit_timestamp=on",
		"-c", "max_replication_slots=10",
		"-c", "max_wal_senders=10",
		"-c", "snowflake.node=1",
		"-c", "lolor.node=1",
	}

	resp, err := r.cli.ContainerCreate(r.ctx, &container.Config{
		Image: r.image,
		Env: []string{
			"POSTGRES_PASSWORD=testpassword",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=testdb",
		},
		Cmd: cmd,
		Tty: true,
	}, &container.HostConfig{}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	r.containerID = resp.ID
	fmt.Printf("Container created: %s\n", r.containerID[:12])

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

	return nil
}

func (r *TestRunner) waitForPostgres(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		exitCode, _, err := r.exec("pg_isready -U postgres")
		if err == nil && exitCode == 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for PostgreSQL to be ready")
}

func (r *TestRunner) Cleanup() {
	fmt.Println()
	fmt.Println("Cleaning up...")

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

func (r *TestRunner) exec(cmd string) (int, string, error) {
	execID, err := r.cli.ContainerExecCreate(r.ctx, r.containerID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
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
	tests := []Test{
		// Basic PostgreSQL functionality
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

		// Spock extension (minimal + standard)
		{
			Name: "Spock extension can be created",
			Cmd:  "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS spock; SELECT 1;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				return nil
			},
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

		// LOLOR extension (minimal + standard)
		{
			Name: "LOLOR extension can be created",
			Cmd:  "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS lolor; SELECT 1;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				return nil
			},
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

		// Snowflake extension (minimal + standard)
		{
			Name: "Snowflake extension can be created",
			Cmd:  "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS snowflake; SELECT 1;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				return nil
			},
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

		// pgvector extension (standard only)
		{
			Name:         "pgvector extension can be created",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS vector; SELECT 1;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				return nil
			},
		},
		{
			Name:         "pgvector distance calculation works",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"SELECT '[1,2,3]'::vector <-> '[4,5,6]'::vector;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				// Expected: 5.196152422706632
				if !strings.HasPrefix(strings.TrimSpace(output), "5.196") {
					return fmt.Errorf("unexpected output: %s", output)
				}
				return nil
			},
		},

		// PostGIS extension (standard only)
		{
			Name:         "PostGIS extension can be created",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS postgis; SELECT 1;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				return nil
			},
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

		// pgaudit extension (standard only)
		{
			Name:         "pgaudit extension can be created",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"CREATE EXTENSION IF NOT EXISTS pgaudit; SELECT 1;\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				return nil
			},
		},

		// pgBackRest (standard only)
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

	return tests
}
