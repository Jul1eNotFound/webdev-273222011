// Command migrate applies (up) or reverts (down) the SQL files in migrations/.
package main

import (
	"fmt"
	"os"

	"github.com/Jul1eNotFound/webdev-273222011/internal/config"
	"github.com/Jul1eNotFound/webdev-273222011/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: go run ./cmd/migrate up|down")
	}
	switch args[0] {
	case "up":
		return migrations.Up(cfg.DatabaseURL)
	case "down":
		return migrations.Down(cfg.DatabaseURL)
	}
	return fmt.Errorf("unknown command %q, want up or down", args[0])
}
