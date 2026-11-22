// Command quickstart starts the local stack, creates a test merchant, and prints a first
// payment. It is the cross-platform entry point behind `make quickstart`, so it needs no
// shell.
//
// Run it from the repository root:
//
//	go run ./packages/tpt-pay-core/cmd/quickstart
//
// Steps: create .env from .env.example if missing; `docker compose up -d --build`; wait for
// /health; create a merchant and test keys in the running container; print curl commands.
// The test secret key is the only key it prints, and it is a sandbox key.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	composeFile = "docker-compose.yml"
	envExample  = "packages/tpt-pay-core/.env.example"
	envFile     = "packages/tpt-pay-core/.env"
	healthURL   = "http://localhost:8080/health"
	apiBase     = "http://localhost:8080/v1"
	serviceName = "tpt-pay-core"
)

var testSecretKey = regexp.MustCompile(`sk_test_[A-Za-z0-9_\-]+`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "quickstart:", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "quickstart@example.com", "merchant email for the test account")
	name := flag.String("name", "Quickstart Merchant", "merchant display name")
	timeout := flag.Duration("timeout", 3*time.Minute, "how long to wait for /health")
	flag.Parse()

	if _, err := os.Stat(composeFile); err != nil {
		return fmt.Errorf("run this from the repository root (no %s here)", composeFile)
	}

	created, err := ensureEnv(envExample, envFile)
	if err != nil {
		return err
	}
	if created {
		fmt.Println("Created", envFile, "from", envExample)
	}

	fmt.Println("Starting the stack (docker compose up -d --build)...")
	if err := stream("docker", "compose", "up", "-d", "--build"); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	fmt.Println("Waiting for", healthURL, "...")
	if err := waitHealthy(healthURL, *timeout, 2*time.Second); err != nil {
		return err
	}

	fmt.Println("Creating a test merchant and keys...")
	out, err := capture("docker", "compose", "exec", "-T", serviceName,
		"go", "run", "./cmd/bootstrap", "--email", *email, "--name", *name)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w\n%s", err, out)
	}
	key, err := extractTestSecretKey(out)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(firstPaymentSteps(key))
	return nil
}

// ensureEnv copies the example env file to dst when dst does not exist. It reports whether it
// created the file. An existing file is never overwritten.
func ensureEnv(src, dst string) (bool, error) {
	if _, err := os.Stat(dst); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("check %s: %w", dst, err)
	}
	in, err := os.Open(src)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, fmt.Errorf("create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return false, fmt.Errorf("copy %s: %w", dst, err)
	}
	return true, out.Close()
}

// waitHealthy polls url until it returns 200 or timeout passes.
func waitHealthy(url string, timeout, interval time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not become healthy within %s; check `docker compose logs %s`",
				url, timeout, serviceName)
		}
		time.Sleep(interval)
	}
}

// extractTestSecretKey finds the sandbox secret key in bootstrap output. Live keys are never
// accepted here: the quickstart must not surface a live credential.
func extractTestSecretKey(out string) (string, error) {
	key := testSecretKey.FindString(out)
	if key == "" {
		return "", errors.New("bootstrap did not print a test secret key; output was:\n" + out)
	}
	return key, nil
}

// firstPaymentSteps returns the curl commands for a first sandbox payment.
func firstPaymentSteps(key string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Your sandbox secret key (shown once; store it now):\n  %s\n\n", key)
	fmt.Fprintf(&b, "1. Create a payment intent (amount is in minor units: 1000 = 10.00 USD):\n")
	fmt.Fprintf(&b, "   curl -s -X POST %s/payment_intents \\\n", apiBase)
	fmt.Fprintf(&b, "     -H \"Authorization: Bearer %s\" -H \"Content-Type: application/json\" \\\n", key)
	fmt.Fprintf(&b, "     -d '{\"amount\":1000,\"currency\":\"usd\"}'\n\n")
	fmt.Fprintf(&b, "2. Confirm it with a sandbox card (replace PI_ID with the id from step 1):\n")
	fmt.Fprintf(&b, "   curl -s -X POST %s/payment_intents/PI_ID/confirm \\\n", apiBase)
	fmt.Fprintf(&b, "     -H \"Authorization: Bearer %s\" -H \"Content-Type: application/json\" \\\n", key)
	fmt.Fprintf(&b, "     -d '{\"payment_method\":\"pm_card_visa\"}'\n\n")
	fmt.Fprintf(&b, "The status should be \"succeeded\". To see a decline, use payment_method 4000000000000002.\n")
	fmt.Fprintf(&b, "Docs for the API: %s/openapi.json\n", strings.TrimSuffix(apiBase, "/v1"))
	return b.String()
}

// stream runs a command with its output connected to this process.
func stream(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// capture runs a command and returns its combined output.
func capture(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
