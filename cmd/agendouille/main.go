// Command agendouille is a self-hosted calendar server for a family: CalDAV
// for every client, a family web app, and an admin UI on the LAN.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const usage = `agendouille - the family's calendars, at home

Usage:
  agendouille <command> [flags]

Commands:
  serve     run the server
  config    print the bootstrap configuration and the settings;
            config set <key> <value>... changes settings
  admin     admin passwd [--random]: set the admin password
  user      add, enrol (a one-time sign-in link and QR), list, disable,
            enable, remove
  group     add, list, remove, member add|remove
  calendar  add, list, share, unshare, transfer, delete
  device    add (a sync credential, with DAVx5's QR), list, revoke
  import    import an .ics file into a calendar
  export    write a calendar as one .ics
  version   print the version

Run "agendouille <command> -h" for the flags of one command. Every key of
the bootstrap file is also a flag (-admin-listen) and an environment
variable (AGENDOUILLE_ADMIN_LISTEN); the file is /etc/agendouille.yaml
unless -config or AGENDOUILLE_CONFIG says otherwise.
`

// svcNow is the clock the CLI writes audit times with.
var svcNow = time.Now

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agendouille: "+err.Error())
		os.Exit(1)
	}
}

// env is the process environment, injected so tests do not touch os.Getenv.
type env struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	getenv         func(string) string
}

type command func(ctx context.Context, e env, args []string) error

var commands = map[string]command{
	"serve":    cmdServe,
	"config":   cmdConfig,
	"admin":    cmdAdmin,
	"user":     cmdUser,
	"group":    cmdGroup,
	"calendar": cmdCalendar,
	"device":   cmdDevice,
	"import":   cmdImport,
	"export":   cmdExport,
	"version":  cmdVersion,
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) error {
	e := env{stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("no command given")
	}
	name := args[0]
	switch name {
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return nil
	case "-version", "--version":
		name = "version"
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("unknown command %q", name)
	}
	return cmd(ctx, e, args[1:])
}

func cmdVersion(_ context.Context, e env, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("version takes no arguments")
	}
	fmt.Fprintln(e.stdout, "agendouille "+version)
	return nil
}

// parseFlags parses a subcommand's flags. -h prints the flags and is not an
// error; stray arguments are.
func parseFlags(set *flag.FlagSet, e env, args []string) (help bool, err error) {
	set.SetOutput(e.stderr)
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return false, err
	}
	if set.NArg() > 0 {
		return false, fmt.Errorf("%s: unexpected argument %q", set.Name(), set.Arg(0))
	}
	return false, nil
}
