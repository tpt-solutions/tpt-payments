// Command dashuser manages dashboard users: it creates them and enrolls their second factor.
//
// The password is read from standard input, so it never appears in shell history or the process
// list. The authenticator secret is printed once, as an otpauth:// URI, when a factor is enrolled.
//
//	DATABASE_URL=... DASHBOARD_TOTP_KEY=... dashuser create --merchant-email m@example.com \
//	    --email owner@example.com --name "Owner" < password.txt
//	DATABASE_URL=... DASHBOARD_TOTP_KEY=... dashuser enroll-totp --email owner@example.com
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/dashauth"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: dashuser create|enroll-totp [flags]")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL must be set")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	switch args[0] {
	case "create":
		return create(ctx, pool, args[1:], stdin, stdout)
	case "enroll-totp":
		return enroll(ctx, pool, args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func service(pool *pgxpool.Pool) (*dashauth.Service, error) {
	box, err := secretbox.New(os.Getenv("DASHBOARD_TOTP_KEY"))
	if err != nil {
		return nil, fmt.Errorf("DASHBOARD_TOTP_KEY: %w", err)
	}
	return dashauth.NewService(pool, box, clock.System{}, dashauth.DefaultIterations)
}

func create(ctx context.Context, pool *pgxpool.Pool, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	merchantEmail := fs.String("merchant-email", "", "the email of the merchant the user belongs to (required)")
	email := fs.String("email", "", "the user's sign-in email (required)")
	name := fs.String("name", "", "the user's display name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *merchantEmail == "" || *email == "" || *name == "" {
		return errors.New("--merchant-email, --email and --name are required")
	}
	password, err := readPassword(stdin)
	if err != nil {
		return err
	}
	var merchantID string
	err = pool.QueryRow(ctx, `SELECT id FROM merchants WHERE email = $1`, *merchantEmail).Scan(&merchantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no merchant with email %q", *merchantEmail)
	}
	if err != nil {
		return err
	}
	svc, err := service(pool)
	if err != nil {
		return err
	}
	user, err := svc.CreateUser(ctx, merchantID, *email, *name, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created %s for %s\n", user.ID, user.Email)
	fmt.Fprintln(stdout, "enroll a second factor with: dashuser enroll-totp --email "+user.Email)
	return nil
}

func enroll(ctx context.Context, pool *pgxpool.Pool, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("enroll-totp", flag.ContinueOnError)
	email := fs.String("email", "", "the user's sign-in email (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email is required")
	}
	var userID string
	err := pool.QueryRow(ctx, `SELECT id FROM dashboard_users WHERE email = $1`, strings.ToLower(strings.TrimSpace(*email))).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no dashboard user with email %q", *email)
	}
	if err != nil {
		return err
	}
	svc, err := service(pool)
	if err != nil {
		return err
	}
	uri, err := svc.EnrollTOTP(ctx, userID)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Add this to an authenticator app now. It is not shown again:")
	fmt.Fprintln(stdout, uri)
	fmt.Fprintln(stdout, "Enrolling again replaces the secret, so the old authenticator stops working.")
	return nil
}

// readPassword reads one line from stdin and refuses an empty one.
func readPassword(stdin io.Reader) (string, error) {
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("the password must be given on standard input")
	}
	return password, nil
}
