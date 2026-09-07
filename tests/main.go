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
	StandardOnly   bool // Only run on standard-or-later flavors (standard, coldfront)
	ColdfrontOnly  bool // Only run on the coldfront flavor
}

// includesStandard reports whether a flavor ships everything standard does.
// coldfront is chained FROM standard, so it is a superset.
func includesStandard(flavor string) bool {
	return flavor == "standard" || flavor == "coldfront"
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

	// Derive the expected spock major version from the image tag (e.g.
	// "...-spock6.0.0-beta1-standard" -> "6"). Used to assert that the image
	// actually ships the spock version its tag advertises.
	spockMajor := spockMajorFromImage(image)

	printHeader(image, flavor)

	cli, ctx := setupDockerClient()
	defaultRunner := &DefaultEntrypointRunner{
		cli:   cli,
		ctx:   ctx,
		image: image,
	}

	errorCount := runEntrypointTests(defaultRunner, flavor)
	errorCount += runExtensionTests(cli, ctx, image, flavor, spockMajor)

	printSummary(errorCount, flavor, spockMajor)
	if errorCount > 0 {
		os.Exit(1)
	}
}

// spockMajorFromImage extracts the spock major version from an image reference's
// tag, e.g. "ghcr.io/pgedge/pgedge-postgres:16.14-spock6.0.0-beta1-standard-1"
// or "...:16-spock6-standard" both yield "6". It returns "" when no spock
// version can be determined, in which case the version assertion is skipped.
func spockMajorFromImage(image string) string {
	// Isolate the tag: the portion after the final ':'. Registry ports (e.g.
	// "127.0.0.1:5000/...") use earlier colons, so the last one starts the tag.
	tag := image
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		tag = image[idx+1:]
	}

	idx := strings.Index(tag, "spock")
	if idx == -1 {
		return ""
	}

	var major strings.Builder
	for _, c := range tag[idx+len("spock"):] {
		if c < '0' || c > '9' {
			break
		}
		major.WriteRune(c)
	}
	return major.String()
}

func parseFlags() (string, string) {
	image := flag.String("image", "", "Docker image to test (required)")
	flavor := flag.String("flavor", "", "Image flavor: minimal, standard or coldfront (required)")
	flag.Parse()

	if *image == "" || *flavor == "" {
		fmt.Println("Usage: go run main.go -image <image> -flavor <minimal|standard|coldfront>")
		fmt.Println()
		fmt.Println("Arguments:")
		fmt.Println("  -image   Docker image to test (e.g., ghcr.io/pgedge/pgedge-postgres:17-spock5-standard)")
		fmt.Println("  -flavor  Image flavor: 'minimal', 'standard' or 'coldfront'")
		os.Exit(1)
	}

	if *flavor != "minimal" && !includesStandard(*flavor) {
		log.Fatalf("Invalid flavor '%s'. Must be 'minimal', 'standard' or 'coldfront'", *flavor)
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

	// Phase 2: Test Patroni entrypoint (standard and the flavors chained from it)
	if includesStandard(flavor) {
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

func runExtensionTests(cli *client.Client, ctx context.Context, image, flavor, spockMajor string) int {
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

	tests := buildTestSuite(spockMajor)
	return runner.RunTests(tests)
}

func printPhaseHeader(title string) {
	fmt.Printf("%s\n", title)
	fmt.Println()
}

func printSummary(errorCount int, flavor, spockMajor string) {
	tests := buildTestSuite(spockMajor)
	extensionTests := 0
	for _, t := range tests {
		if t.StandardOnly && !includesStandard(flavor) {
			continue
		}
		if t.ColdfrontOnly && flavor != "coldfront" {
			continue
		}
		extensionTests++
	}

	testsRun := 1 + extensionTests // default entrypoint + extensions
	if includesStandard(flavor) {
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
	if includesStandard(r.flavor) {
		sharedLibs = "spock,snowflake,pgaudit,supautils,pg_cron,pg_tokenizer"
	}
	// pg_duckdb and coldfront install hooks at postmaster start. The image's own
	// entrypoint already passes them, but the -c built below is appended after it
	// and would otherwise replace the value.
	if r.flavor == "coldfront" {
		sharedLibs += ",pg_duckdb,coldfront"
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
	if r.flavor == "standard" {
		// pg_cron only ever installs into the one database this names,
		// and refuses CREATE EXTENSION anywhere else.
		cmd = append(cmd, "-c", "cron.database_name=testdb", "-c", "cron.use_background_workers=on")
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

type commandParser struct {
	args          []string
	current       strings.Builder
	inSingleQuote bool
	inDoubleQuote bool
}

func (p *commandParser) flush() {
	if p.current.Len() > 0 {
		p.args = append(p.args, p.current.String())
		p.current.Reset()
	}
}

func (p *commandParser) processChar(char rune) {
	switch char {
	case '\'':
		if !p.inDoubleQuote {
			p.inSingleQuote = !p.inSingleQuote
		} else {
			p.current.WriteRune(char)
		}
	case '"':
		if !p.inSingleQuote {
			p.inDoubleQuote = !p.inDoubleQuote
		} else {
			p.current.WriteRune(char)
		}
	case ' ':
		if p.inSingleQuote || p.inDoubleQuote {
			p.current.WriteRune(char)
		} else {
			p.flush()
		}
	default:
		p.current.WriteRune(char)
	}
}

// parseCommand safely parses a command string into command and arguments.
// This prevents command injection by avoiding shell interpretation.
func parseCommand(cmd string) []string {
	p := &commandParser{}
	for _, char := range cmd {
		p.processChar(char)
	}
	p.flush()
	if len(p.args) == 0 {
		return nil
	}
	return p.args
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
	if len(cmdArgs) == 0 {
		return -1, "", fmt.Errorf("empty command")
	}

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
		if test.StandardOnly && !includesStandard(r.flavor) {
			continue
		}
		if test.ColdfrontOnly && r.flavor != "coldfront" {
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

func buildTestSuite(spockMajor string) []Test {
	tests := []Test{}
	tests = append(tests, getPostgreSQLTests()...)
	tests = append(tests, getCommonExtensionTests()...)
	// Runs after getCommonExtensionTests, which creates the spock extension.
	tests = append(tests, getSpockVersionTests(spockMajor)...)
	tests = append(tests, getStandardOnlyTests()...)
	tests = append(tests, getColdfrontTests()...)
	return tests
}

// getSpockVersionTests asserts that the spock extension installed in the image
// matches the major version advertised by the image tag. This distinguishes,
// for example, a spock6 image from a spock5 image, a mismatch would otherwise
// pass every other test unnoticed. Returns no tests when the expected major
// version could not be derived from the image tag.
func getSpockVersionTests(spockMajor string) []Test {
	if spockMajor == "" {
		return nil
	}
	return []Test{
		{
			Name: fmt.Sprintf("Spock extension major version is %s", spockMajor),
			Cmd:  "psql -U postgres -d testdb -t -A -c \"SELECT extversion FROM pg_extension WHERE extname = 'spock';\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				version := strings.TrimSpace(output)
				if version == "" {
					return fmt.Errorf("spock extension is not installed")
				}
				if !strings.HasPrefix(version, spockMajor+".") {
					return fmt.Errorf("spock version %q does not match expected major version %s", version, spockMajor)
				}
				return nil
			},
		},
	}
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

// rpmExtensionDir is where pgedge-coldfront-duckdb-extensions installs the
// DuckDB extension binaries. duckdb.extension_directory points here and
// duckdb.autoinstall_known_extensions is off, so a successful load proves the
// extensions are read from the read-only package path rather than fetched.
const rpmExtensionDir = "/usr/lib/pgedge/coldfront/duckdb-extensions"

func expectTrimmed(want string) func(int, string) error {
	return func(exitCode int, output string) error {
		if exitCode != 0 {
			return fmt.Errorf("unexpected exit code: %d", exitCode)
		}
		if got := strings.TrimSpace(output); got != want {
			return fmt.Errorf("expected %q, got %q", want, got)
		}
		return nil
	}
}

func getColdfrontTests() []Test {
	loadAll := `SELECT duckdb.load_extension('iceberg');` +
		`SELECT duckdb.load_extension('avro');` +
		`SELECT duckdb.load_extension('azure');` +
		`SELECT duckdb.load_extension('postgres_scanner');` +
		`SELECT * FROM duckdb.query('SELECT count(*) FROM duckdb_extensions() ` +
		`WHERE loaded AND install_path LIKE ''` + rpmExtensionDir + `%''');`

	return []Test{
		{
			Name:           "pg_duckdb extension can be created",
			ColdfrontOnly:  true,
			Cmd:            `psql -U postgres -d testdb -t -A -c "CREATE EXTENSION IF NOT EXISTS pg_duckdb; SELECT 1;"`,
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "coldfront extension can be created",
			ColdfrontOnly:  true,
			Cmd:            `psql -U postgres -d testdb -t -A -c "CREATE EXTENSION IF NOT EXISTS coldfront CASCADE; SELECT 1;"`,
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "DuckDB executes a query",
			ColdfrontOnly:  true,
			Cmd:            `psql -U postgres -d testdb -t -A -c "SELECT * FROM duckdb.query('SELECT 42');"`,
			ExpectedOutput: expectTrimmed("42"),
		},
		{
			Name:           "duckdb.extension_directory points at the package path",
			ColdfrontOnly:  true,
			Cmd:            `psql -U postgres -d testdb -t -A -c "SHOW duckdb.extension_directory;"`,
			ExpectedOutput: expectTrimmed(rpmExtensionDir),
		},
		{
			Name:           "DuckDB extension autoinstall is disabled",
			ColdfrontOnly:  true,
			Cmd:            `psql -U postgres -d testdb -t -A -c "SHOW duckdb.autoinstall_known_extensions;"`,
			ExpectedOutput: expectTrimmed("off"),
		},
		{
			Name:          "all four DuckDB extensions load from the package path",
			ColdfrontOnly: true,
			Cmd:           `psql -U postgres -d testdb -t -A -c "` + loadAll + `"`,
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				fields := strings.Fields(strings.TrimSpace(output))
				if len(fields) == 0 || fields[len(fields)-1] != "4" {
					return fmt.Errorf("expected 4 extensions loaded from %s, got: %s", rpmExtensionDir, output)
				}
				return nil
			},
		},
	}
}

func getStandardOnlyTests() []Test {
	tests := append(getSystemStatsAndVectorTests(), getPostGISAuditBackrestTests()...)
	tests = append(tests, getSupautilsTests()...)
	return append(tests, getExtensionCustomScriptsTests()...)
}

func getSupautilsTests() []Test {
	return []Test{
		{
			// supautils is a shared_preload_libraries-only module (no CREATE EXTENSION,
			// no SQL functions/views). Its only SQL-visible evidence of a successful
			// load is the GUCs it registers in _PG_init, so we assert those exist in
			// pg_settings. This query never raises, returning 't' only when the
			// library was actually preloaded and initialized.
			Name:         "supautils is preloaded",
			StandardOnly: true,
			Cmd:          "psql -U postgres -d testdb -t -A -c \"SELECT EXISTS (SELECT 1 FROM pg_settings WHERE name LIKE 'supautils.%');\"",
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
		{
			// A registered GUC is readable via SHOW. If supautils were not loaded this
			// errors with 'unrecognized configuration parameter', so a clean exit
			// confirms the library registered its settings.
			Name:           "supautils GUC is accessible",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"SHOW supautils.reserved_roles;\"",
			ExpectedOutput: expectSuccess,
		},
	}
}

func getSystemStatsAndVectorTests() []Test {
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
	}
}

func getPostGISAuditBackrestTests() []Test {
	return []Test{
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

// expectFailureContaining returns an ExpectedOutput func for a command that
// must fail (a non-zero exit from psql -c means the statement errored), with
// the error text containing want. Used for every negative case below: a
// plain non-zero exit code alone would also pass for the wrong reason (a
// typo'd role name, a connection failure), so the actual error text is
// checked too.
func expectFailureContaining(want string) func(exitCode int, output string) error {
	return func(exitCode int, output string) error {
		if exitCode == 0 {
			return fmt.Errorf("expected failure, got success: %s", output)
		}
		if !strings.Contains(output, want) {
			return fmt.Errorf("expected output containing %q, got: %s", want, output)
		}
		return nil
	}
}

// getExtensionCustomScriptsTests exercises supautils' gate and the
// extension-custom-scripts this repo ships, not just that the library
// loads: a non-superuser role installing an allowlisted extension through
// the gate, the same role refused a non-allowlisted one, and each
// extension's after-create.sql granting the database's own owner exactly
// the access it documents, in whichever schema the extension actually
// landed in, not a hardcoded one.
func getExtensionCustomScriptsTests() []Test {
	return []Test{
		{
			// Made testdb's actual owner, not just given CREATE on it: the
			// scripts under test grant to pg_database_owner, a predefined
			// role whose membership tracks whoever currently owns the
			// database, and testdb starts out owned by postgres, a
			// superuser that bypasses every ACL check regardless of what
			// gets granted. Without this, checking pg_database_owner's
			// access would really be checking postgres's, which proves
			// nothing.
			Name:           "create the non-superuser role the gate tests connect as",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE ROLE gate_test_role LOGIN NOSUPERUSER; ALTER DATABASE testdb OWNER TO gate_test_role; ALTER ROLE gate_test_role SET session_preload_libraries = 'supautils';\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "configure supautils.privileged_role",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"ALTER SYSTEM SET supautils.privileged_role = 'gate_test_role';\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "configure supautils.superuser",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"ALTER SYSTEM SET supautils.superuser = 'postgres';\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "configure supautils.privileged_extensions",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"ALTER SYSTEM SET supautils.privileged_extensions = 'address_standardizer, address_standardizer_data_us, pg_cron, pg_tokenizer, vchord_bm25, postgis, postgis_tiger_geocoder, postgis_topology';\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "configure supautils.extension_custom_scripts_path",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"ALTER SYSTEM SET supautils.extension_custom_scripts_path = '/etc/pgedge/extension-custom-scripts';\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "reload for the new supautils settings to take effect",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"SELECT pg_reload_conf();\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// dblink is untrusted and not on the allowlist configured above:
			// the gate must refuse it for a non-superuser role the same way
			// Postgres core would refuse any untrusted extension.
			Name:           "gate refuses a non-allowlisted extension",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION dblink;\"",
			ExpectedOutput: expectFailureContaining("Must be superuser"),
		},
		{
			// address_standardizer is a dependency address_standardizer_data_us
			// needs installed first; both are on the allowlist configured above.
			Name:           "gate allows an allowlisted extension",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION address_standardizer; CREATE EXTENSION address_standardizer_data_us;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// Confirms after-create.sql actually ran and granted access,
			// not just that the extension installed: gate_test_role has no
			// grant of its own on these tables, only what the script gave
			// pg_database_owner, which gate_test_role belongs to by owning
			// testdb (see the role-creation step above). Runs the real
			// query, not a has_table_privilege check: that check only
			// covers the table-level grant, missing a separate, real bug
			// this exact test caught once already, a table grant with no
			// matching schema USAGE, which fails at query time despite
			// has_table_privilege reporting true.
			Name:           "address_standardizer_data_us after-create.sql granted access",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT count(*) FROM us_lex;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// Reproduces the case the schema lookup in after-create.sql
			// exists for: an explicit SCHEMA clause lands the tables
			// somewhere other than public, and the grant must still land
			// on the actual schema, not a hardcoded one.
			Name:           "address_standardizer_data_us after-create.sql follows an explicit SCHEMA clause",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE SCHEMA relocated_gis; DROP EXTENSION address_standardizer_data_us; DROP EXTENSION address_standardizer;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "reinstall address_standardizer_data_us into the relocated schema",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION address_standardizer SCHEMA relocated_gis; CREATE EXTENSION address_standardizer_data_us SCHEMA relocated_gis;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// Real query again, for the same reason as the default-schema
			// case above, and specifically the one where a missing schema
			// USAGE grant would actually surface: public grants USAGE to
			// PUBLIC by default, so the default-schema case would have
			// passed even without it, this relocated schema has no such
			// default and only passes if the script's own USAGE grant
			// worked.
			Name:           "address_standardizer_data_us after-create.sql found the relocated schema",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT count(*) FROM relocated_gis.us_lex;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "gate installs pg_cron",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION pg_cron;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// pg_cron's after-create.sql grants USAGE + SELECT only, no
			// ownership: schedule() writes to cron.job through pg_cron's
			// own internal code, not a caller-privileged INSERT, so
			// SELECT is enough for the database's owner to schedule its
			// own jobs directly.
			Name:           "pg_cron after-create.sql lets the owner schedule its own job",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT cron.schedule('probe', '* * * * *', 'SELECT 1');\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:         "pg_cron after-create.sql lets the owner list its own job",
			StandardOnly: true,
			Cmd:          "psql -U gate_test_role -d testdb -t -A -c \"SELECT jobname FROM cron.job WHERE jobname = 'probe';\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "probe" {
					return fmt.Errorf("expected to see the scheduled job, got: %s", output)
				}
				return nil
			},
		},
		{
			Name:         "pg_cron after-create.sql lets the owner unschedule its own job",
			StandardOnly: true,
			Cmd:          "psql -U gate_test_role -d testdb -t -A -c \"SELECT cron.unschedule('probe');\"",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if strings.TrimSpace(output) != "t" {
					return fmt.Errorf("expected the job to be unscheduled, got: %s", output)
				}
				return nil
			},
		},
		{
			// The thing SELECT-only deliberately does not allow: a raw
			// write against cron.job. No ownership, no write path.
			Name:           "pg_cron after-create.sql refuses a raw write to cron.job",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"INSERT INTO cron.job (schedule, command, nodename, nodeport, database, username) VALUES ('* * * * *', 'SELECT 1', 'localhost', 5432, 'testdb', 'postgres');\"",
			ExpectedOutput: expectFailureContaining("permission denied for table job"),
		},
		{
			Name:           "gate installs pg_tokenizer",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION pg_tokenizer;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// Real query against a real table in tokenizer_catalog, the
			// same reasoning as address_standardizer_data_us above: a
			// table grant with no matching schema USAGE fails here even
			// though has_table_privilege would report true.
			Name:           "pg_tokenizer after-create.sql granted access to tokenizer_catalog",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT count(*) FROM tokenizer_catalog.tokenizer;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "gate installs vchord_bm25",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION vchord_bm25;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// bm25_catalog holds only the bm25vector type and its support
			// functions, no tables: declaring a column of that type is
			// the real thing the database's owner needs USAGE on the
			// schema for.
			Name:           "vchord_bm25 after-create.sql granted access to bm25_catalog",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE TABLE bm25_probe(id int, v bm25_catalog.bm25vector);\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// The earlier, unrelated common extension test already
			// installed postgis as postgres directly, before the gate
			// was configured. Drop it so the gate genuinely installs it
			// below, the same reset pattern used for
			// address_standardizer_data_us above.
			Name:           "reset: drop postgis installed by the earlier common test",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"DROP EXTENSION IF EXISTS postgis CASCADE;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// postgis_tiger_geocoder and postgis_topology both depend on
			// postgis; installed here as the gated role, the same as any
			// other allowlisted extension.
			Name:           "gate installs postgis",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION postgis;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "gate installs postgis_tiger_geocoder",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION postgis_tiger_geocoder CASCADE;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// Real query against a real table in tiger, created at
			// CREATE EXTENSION time, the same reasoning as
			// address_standardizer_data_us and pg_tokenizer above.
			Name:           "postgis_tiger_geocoder after-create.sql granted access to tiger",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT count(*) FROM tiger.county;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "gate installs postgis_topology",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE EXTENSION postgis_topology;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// CreateTopology() INSERTs into topology.topology and
			// topology.layer, which needs real ownership of both tables,
			// not just a grant: unlike pg_cron, postgis_topology's own
			// functions run as the caller through ordinary ACL-checked
			// DML, and RenameTopoGeometryColumn() additionally runs
			// ALTER TABLE ... DISABLE/ENABLE TRIGGER on topology.layer,
			// which only an owner or superuser can do.
			Name:           "postgis_topology after-create.sql lets the owner create a topology",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT topology.CreateTopology('probe_topo', 4326);\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// AddTopoGeometryColumn only needs INSERT, which a plain
			// grant already covers, so this alone would pass even
			// without ownership. Included for lifecycle completeness,
			// the real proof is the rename step below.
			Name:           "postgis_topology after-create.sql lets the owner register a layer",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"CREATE TABLE probe_feat(id serial primary key); SELECT topology.AddTopoGeometryColumn('probe_topo', 'public', 'probe_feat', 'g', 'POLYGON');\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// This is the one call in the whole lifecycle that a plain
			// grant cannot satisfy: RenameTopoGeometryColumn() runs
			// ALTER TABLE topology.layer DISABLE/ENABLE TRIGGER, which
			// needs real ownership. Confirmed directly: a role with
			// full DML and even the TRIGGER privilege on both tables
			// still gets "must be owner of table layer" here, so this
			// is the test that actually proves the ownership handoff
			// is doing something, not just that CreateTopology's INSERT
			// happens to work.
			Name:           "postgis_topology after-create.sql lets the owner rename a layer column",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT topology.RenameTopoGeometryColumn('probe_feat', 'g', 'g2');\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "postgis_topology after-create.sql lets the owner drop a topology",
			StandardOnly:   true,
			Cmd:            "psql -U gate_test_role -d testdb -t -A -c \"SELECT topology.DropTopology('probe_topo');\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// A role the database's owner creates itself, not the
			// owner and not a member of pg_database_owner: none of the
			// grants above reach it through membership at all, so this
			// is what actually proves the PUBLIC grant, not just that
			// pg_database_owner has access.
			Name:           "create a third-party role the owner does not control access through",
			StandardOnly:   true,
			Cmd:            "psql -U postgres -d testdb -t -A -c \"CREATE ROLE reporting_role LOGIN NOSUPERUSER;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// pg_database_owner's own grant carries no GRANT OPTION,
			// so the owner has no way to pass this on by hand either;
			// confirms that silent no-op rather than assuming it.
			Name:         "owner re-granting schema access by hand is a silent no-op",
			StandardOnly: true,
			Cmd:          "psql -U gate_test_role -d testdb -t -A -c \"GRANT USAGE ON SCHEMA tiger TO reporting_role;\" 2>&1",
			ExpectedOutput: func(exitCode int, output string) error {
				if exitCode != 0 {
					return fmt.Errorf("unexpected exit code: %d", exitCode)
				}
				if !strings.Contains(output, "no privileges were granted") {
					return fmt.Errorf("expected a no-op warning, got: %s", output)
				}
				return nil
			},
		},
		{
			// Reads relocated_gis.us_lex, not the bare table name: an
			// earlier test in this suite already relocated
			// address_standardizer_data_us there via an explicit
			// SCHEMA clause.
			Name:           "third-party role reaches address_standardizer_data_us via PUBLIC",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"SELECT count(*) FROM relocated_gis.us_lex;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "third-party role reaches postgis_tiger_geocoder via PUBLIC",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"SELECT normalize_address('1 Devonshire Pl, Boston, MA 02109');\"",
			ExpectedOutput: expectSuccess,
		},
		{
			Name:           "third-party role reaches vchord_bm25/pg_tokenizer via PUBLIC",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"SELECT pg_typeof('{1:1}'::bm25_catalog.bm25vector); SELECT count(*) FROM tokenizer_catalog.tokenizer;\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// The write side stays pg_database_owner-only regardless
			// of the PUBLIC read grant above.
			Name:           "third-party role still refused a tokenizer_catalog write",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"INSERT INTO tokenizer_catalog.tokenizer(name) VALUES ('probe');\"",
			ExpectedOutput: expectFailureContaining("permission denied for table tokenizer"),
		},
		{
			// Schema USAGE makes every function in tokenizer_catalog
			// resolvable, and Postgres grants EXECUTE on new functions
			// to PUBLIC by default, so without this REVOKE a
			// third-party role could reach config-parsing/model-loading
			// functions that were never meant to be public, none of
			// them SECURITY DEFINER, but still real work running before
			// any table-ACL check fires. Confirmed cleanly refused at
			// the function call itself now, not at some later step.
			Name:           "third-party role refused the tokenizer_catalog config-management functions",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"SELECT tokenizer_catalog.create_tokenizer('probe', 'x');\"",
			ExpectedOutput: expectFailureContaining("permission denied for function create_tokenizer"),
		},
		{
			// The three functions the read-only use case actually
			// needs stay PUBLIC-executable: tokenize() and
			// apply_text_analyzer() to process text against an
			// existing configuration, list_preload_models() to see
			// what's available.
			Name:           "third-party role keeps the read-only tokenizer_catalog functions",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"SELECT tokenizer_catalog.list_preload_models();\"",
			ExpectedOutput: expectSuccess,
		},
		{
			// pg_cron is deliberately not part of this fix: cron.job is
			// already scoped per username by its own row-level security
			// policy, a PUBLIC grant here would not change what a
			// third-party role can see or do with it.
			Name:           "third-party role still has no path into pg_cron",
			StandardOnly:   true,
			Cmd:            "psql -U reporting_role -d testdb -t -A -c \"SELECT cron.schedule('probe', '* * * * *', 'SELECT 1');\"",
			ExpectedOutput: expectFailureContaining("permission denied for schema cron"),
		},
	}
}
