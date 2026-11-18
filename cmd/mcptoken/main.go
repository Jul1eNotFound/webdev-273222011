// Command mcptoken signs in and prints a session token for an MCP client (Slice 5).
//
//	go run ./cmd/mcptoken -email student@labwiz.test
//
// It asks for the password without echoing it, or reads one line from stdin when piped. Only the token
// goes to stdout, so MCP_TOKEN=$(go run ./cmd/mcptoken -email …) works. The token is an ordinary sign-in
// session: it lasts service.SessionTTL and grants exactly what that account may do on the web pages.
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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/config"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func main() {
	email := flag.String("email", "", "the account to sign in as")
	flag.Parse()
	if err := run(*email); err != nil {
		fmt.Fprintln(os.Stderr, "mcptoken:", err)
		os.Exit(1)
	}
}

func run(email string) error {
	if strings.TrimSpace(email) == "" {
		return errors.New("give the account with -email")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	password, err := readPassword()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return issue(ctx, service.NewAuth(store.New(pool), auth.Bcrypt{}, time.Now), email, password, os.Stdout, os.Stderr)
}

// issue signs in and writes the token to out and who it is for to info.
func issue(ctx context.Context, a *service.Auth, email, password string, out, info io.Writer) error {
	s, err := a.Login(ctx, email, password)
	if err != nil {
		return err
	}
	c, err := a.CurrentUser(ctx, s.Token)
	if err != nil {
		return err
	}
	fmt.Fprintf(info, "Signed in as %s (%s). The token works until %s.\n", c.Name, c.Role, s.ExpiresAt.Format("2 Jan 2006 15:04"))
	fmt.Fprintln(out, s.Token)
	return nil
}

// readPassword asks on stderr, so the prompt never mixes with the token on stdout. In a console the
// typing is hidden, which looks like nothing happens, so the prompt says so. Git Bash's mintty is not a
// console to Go: there the password is read as a plain line, and it shows as it is typed.
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "Password (hidden as you type, then Enter): ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	fmt.Fprint(os.Stderr, "Password (then Enter): ")
	return firstLine(os.Stdin)
}

// firstLine is the piped password: one line, without its line ending.
func firstLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
