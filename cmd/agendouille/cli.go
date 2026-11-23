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
	"text/tabwriter"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// stringList is a repeatable flag: --group famille --group enfants.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// parseArgs parses flags that may come before, between or after the
// positional arguments: `user add lea --group famille` reads naturally.
func parseArgs(set *flag.FlagSet, e env, args []string) (help bool, positional []string, err error) {
	set.SetOutput(e.stderr)
	for {
		if err := set.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return true, nil, nil
			}
			return false, nil, err
		}
		if set.NArg() == 0 {
			return false, positional, nil
		}
		positional = append(positional, set.Arg(0))
		args = set.Args()[1:]
	}
}

// openFor loads the bootstrap configuration with the flags already parsed
// on set, and opens the database it names.
func openFor(ctx context.Context, e env, loader *config.Loader) (*config.Config, *services, error) {
	cfg, _, err := loader.Load(e.getenv)
	if err != nil {
		return nil, nil, err
	}
	svc, err := openServices(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	return cfg, svc, nil
}

func want(positional []string, n int, usage string) error {
	if len(positional) != n {
		return fmt.Errorf("usage: agendouille %s", usage)
	}
	return nil
}

// cmdAdmin: agendouille admin passwd [--random].
func cmdAdmin(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille admin", flag.ContinueOnError)
	loader := config.NewLoader(set)
	random := set.Bool("random", false, "make a random password and print it once")
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if err := want(pos, 1, "admin passwd [--random]"); err != nil || pos[0] != "passwd" {
		return errors.Join(err, fmt.Errorf("usage: agendouille admin passwd [--random]"))
	}
	cfg, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	password := ""
	if *random {
		password = auth.RandomPassword()
	} else {
		if f, ok := e.stdin.(*os.File); ok && isTerminal(f) {
			fmt.Fprint(e.stderr, "New admin password (it is shown as you type; --random avoids that): ")
		}
		line, err := bufio.NewReader(e.stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		password = strings.TrimRight(line, "\r\n")
	}
	if err := svc.auth.SetAdminPassword(ctx, "cli", password); err != nil {
		return err
	}
	if *random {
		// Printed, never logged: stdout is the operator's terminal.
		fmt.Fprintf(e.stdout, "Admin password, shown once (only its hash is kept):\n\n  %s\n\n", password)
	} else {
		fmt.Fprintln(e.stdout, "Admin password set.")
	}
	fmt.Fprintf(e.stdout, "Sign in at %s/admin\n", cfg.AdminURL())
	return nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// cmdUser: add, enrol, list, disable, enable, remove.
func cmdUser(ctx context.Context, e env, args []string) error {
	const usage = "user add <name> [--display-name N] [--color #rrggbb] [--group G]... | enrol <name> [--ttl 72h] | list | disable|enable|remove <name>"
	set := flag.NewFlagSet("agendouille user", flag.ContinueOnError)
	loader := config.NewLoader(set)
	display := set.String("display-name", "", "display name (add)")
	color := set.String("color", "", "colour, #rrggbb (add; default: the next of the palette)")
	var groups stringList
	set.Var(&groups, "group", "a group to put the user in (add; repeatable)")
	ttl := set.Duration("ttl", 0, "how long the enrolment link stays valid (enrol; default: the enrolment_link_ttl_hours setting)")
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if len(pos) == 0 {
		return fmt.Errorf("usage: agendouille %s", usage)
	}
	_, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	switch pos[0] {
	case "add":
		if err := want(pos, 2, "user add <name> [--display-name N] [--color C] [--group G]..."); err != nil {
			return err
		}
		u, err := svc.dir.AddUser(ctx, "cli", pos[1], *display, *color, groups)
		if err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "Added %s (%s).\nGive them their first sign-in with: agendouille user enrol %s\n", u.Name, u.DisplayName, u.Name)
		return nil
	case "enrol":
		if err := want(pos, 2, "user enrol <name> [--ttl 72h]"); err != nil {
			return err
		}
		base := strings.TrimRight(svc.settings.Get().PublicURL, "/")
		if base == "" {
			return errors.New("public_url is not set, so there is no address for the link: agendouille config set public_listen 0.0.0.0:8791 public_url https://agenda.example.org")
		}
		u, err := svc.dir.User(ctx, pos[1])
		if err != nil {
			return fmt.Errorf("user %q: %w", pos[1], err)
		}
		en, err := svc.auth.NewEnrolment(ctx, "cli", u.ID, *ttl)
		if err != nil {
			return err
		}
		link := base + "/enrol/" + en.Token
		code, err := webx.QRTerminal(link)
		if err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "%s\nFirst sign-in for %s, once, until %s:\n\n  %s\n\nScan it with the phone's camera, or send the link by message.\n",
			code, u.DisplayName, en.ExpiresAt.Local().Format("2006-01-02 15:04"), link)
		return nil
	case "list":
		if err := want(pos, 1, "user list"); err != nil {
			return err
		}
		users, err := svc.dir.Users(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(e.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tDISPLAY NAME\tGROUPS\tSTATE\tLAST SEEN")
		for _, u := range users {
			gs, err := svc.dir.GroupsOf(ctx, u.ID)
			if err != nil {
				return err
			}
			var names []string
			for _, g := range gs {
				names = append(names, g.Name)
			}
			state, seen := "active", "never"
			if u.Disabled {
				state = "disabled"
			}
			if u.LastSeenAt != nil {
				seen = u.LastSeenAt.Local().Format(time.DateTime)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", u.Name, u.DisplayName, strings.Join(names, ","), state, seen)
		}
		return tw.Flush()
	case "disable", "enable":
		if err := want(pos, 2, "user "+pos[0]+" <name>"); err != nil {
			return err
		}
		return svc.dir.SetDisabled(ctx, "cli", pos[1], pos[0] == "disable")
	case "remove":
		if err := want(pos, 2, "user remove <name>"); err != nil {
			return err
		}
		// A removed user's calendars are transferred or deleted first
		// (PLAN.md 6): there is no orphan calendar.
		u, err := svc.dir.User(ctx, pos[1])
		if err != nil {
			return fmt.Errorf("user %q: %w", pos[1], err)
		}
		owned, err := svc.calendar.Owned(ctx, u.ID)
		if err != nil {
			return err
		}
		if len(owned) > 0 {
			var refs []string
			for _, i := range owned {
				refs = append(refs, i.Ref.String())
			}
			return fmt.Errorf("%s still owns %s: run agendouille calendar transfer <calendar> <user>, or calendar delete <calendar>, first",
				u.Name, strings.Join(refs, ", "))
		}
		return svc.dir.Remove(ctx, "cli", pos[1])
	}
	return fmt.Errorf("usage: agendouille %s", usage)
}

// cmdGroup: add, list, remove, member add|remove.
func cmdGroup(ctx context.Context, e env, args []string) error {
	const usage = "group add <name> [--display-name N] [--color C] | list | remove <name> | member add|remove <group> <user>"
	set := flag.NewFlagSet("agendouille group", flag.ContinueOnError)
	loader := config.NewLoader(set)
	display := set.String("display-name", "", "display name (add)")
	color := set.String("color", "", "colour, #rrggbb (add)")
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if len(pos) == 0 {
		return fmt.Errorf("usage: agendouille %s", usage)
	}
	_, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	switch pos[0] {
	case "add":
		if err := want(pos, 2, "group add <name>"); err != nil {
			return err
		}
		g, err := svc.dir.AddGroup(ctx, "cli", pos[1], *display, *color)
		if err == nil {
			fmt.Fprintf(e.stdout, "Added the group %s (%s).\n", g.Name, g.DisplayName)
		}
		return err
	case "list":
		groups, err := svc.dir.Groups(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(e.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tDISPLAY NAME\tMEMBERS")
		for _, g := range groups {
			ms, err := svc.dir.Members(ctx, g.Name)
			if err != nil {
				return err
			}
			var names []string
			for _, m := range ms {
				names = append(names, m.Name)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", g.Name, g.DisplayName, strings.Join(names, ","))
		}
		return tw.Flush()
	case "remove":
		if err := want(pos, 2, "group remove <name>"); err != nil {
			return err
		}
		return svc.dir.RemoveGroup(ctx, "cli", pos[1])
	case "member":
		if err := want(pos, 4, "group member add|remove <group> <user>"); err != nil {
			return err
		}
		switch pos[1] {
		case "add":
			return svc.dir.AddMember(ctx, "cli", pos[2], pos[3])
		case "remove":
			return svc.dir.RemoveMember(ctx, "cli", pos[2], pos[3])
		}
	}
	return fmt.Errorf("usage: agendouille %s", usage)
}
