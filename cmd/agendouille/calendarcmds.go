package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

// cmdCalendar is the operator's view of calendars, until the admin UI (M3)
// and the web app (M4) have their pages: add, list, share, unshare,
// transfer, delete. Users make their own from any CalDAV client.
func cmdCalendar(ctx context.Context, e env, args []string) error {
	const usage = "calendar add <user> <slug> [--name N] [--color C] [--timezone Z] | list [--user U] | " +
		"share <owner>~<slug> u:<user>|g:<group> read|readwrite | unshare <owner>~<slug> u:<user>|g:<group> | " +
		"transfer <owner>~<slug> <user> | delete <owner>~<slug>"
	set := flag.NewFlagSet("agendouille calendar", flag.ContinueOnError)
	loader := config.NewLoader(set)
	name := set.String("name", "", "display name (add)")
	color := set.String("color", "", "colour, #rrggbb (add)")
	zone := set.String("timezone", "", "time zone (add; default: the family_timezone setting)")
	user := set.String("user", "", "only this user's calendars (list)")
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
	op := calendar.Operator("cli")
	at := store.Time(svcNow())
	switch pos[0] {
	case "add":
		if err := want(pos, 3, "calendar add <user> <slug>"); err != nil {
			return err
		}
		p := calendar.Props{}
		if *name != "" {
			p.Name = name
		}
		if *color != "" {
			p.Color = color
		}
		if *zone != "" {
			p.Timezone = zone
		}
		i, err := svc.calendar.Create(ctx, op, pos[1], pos[2], p)
		if err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "Made %s (%s).\n", i.Ref, i.Name)
		return nil
	case "list":
		users, err := svc.dir.Users(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(e.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "CALENDAR\tNAME\tCOLOUR\tZONE\tSHARES")
		for _, u := range users {
			if *user != "" && u.Name != *user {
				continue
			}
			owned, err := svc.calendar.Owned(ctx, u.ID)
			if err != nil {
				return err
			}
			for _, i := range owned {
				shares, err := svc.calendar.Sharing().Shares(ctx, i.ID)
				if err != nil {
					return err
				}
				var s []string
				for _, sh := range shares {
					s = append(s, sh.Name+"="+sh.Access)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", i.Ref, i.Name, i.Color, i.Timezone, strings.Join(s, " "))
			}
		}
		return tw.Flush()
	case "share", "unshare":
		n := 4
		if pos[0] == "unshare" {
			n = 3
		}
		if err := want(pos, n, "calendar "+pos[0]+" <owner>~<slug> u:<user>|g:<group> [read|readwrite]"); err != nil {
			return err
		}
		ref, err := calendar.ParseRef(pos[1])
		if err != nil {
			return err
		}
		i, err := svc.calendar.Info(ctx, op, ref)
		if err != nil {
			return fmt.Errorf("%s: %w", pos[1], err)
		}
		t, err := sharing.ParseTarget(pos[2])
		if err != nil {
			return err
		}
		if pos[0] == "unshare" {
			return svc.calendar.Sharing().Unshare(ctx, "cli", i.ID, t, at)
		}
		a, err := sharing.ParseAccess(pos[3])
		if err != nil {
			return err
		}
		// The operator may share with anyone; an owner, in the web app, only
		// with the people of their groups.
		return svc.calendar.Sharing().ShareWith(ctx, "cli", i.OwnerID, false, i.ID, t, a, at)
	case "transfer":
		if err := want(pos, 3, "calendar transfer <owner>~<slug> <user>"); err != nil {
			return err
		}
		ref, err := calendar.ParseRef(pos[1])
		if err != nil {
			return err
		}
		i, err := svc.calendar.Transfer(ctx, op, ref, pos[2], true)
		if err == nil {
			fmt.Fprintf(e.stdout, "%s is now %s; %s keeps read-write.\n", ref, i.Ref, ref.Owner)
		}
		return err
	case "delete":
		if err := want(pos, 2, "calendar delete <owner>~<slug>"); err != nil {
			return err
		}
		ref, err := calendar.ParseRef(pos[1])
		if err != nil {
			return err
		}
		return svc.calendar.DeleteCalendar(ctx, op, ref)
	}
	return fmt.Errorf("usage: agendouille %s", usage)
}

// cmdImport: agendouille import <owner>~<slug> <file.ics|-> [--replace].
func cmdImport(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille import", flag.ContinueOnError)
	loader := config.NewLoader(set)
	replace := set.Bool("replace", false, "replace events whose UID is in the calendar already (default: skip them)")
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if err := want(pos, 2, "import <owner>~<slug> <file.ics|-> [--replace]"); err != nil {
		return err
	}
	ref, err := calendar.ParseRef(pos[0])
	if err != nil {
		return err
	}
	var data []byte
	if pos[1] == "-" {
		data, err = io.ReadAll(e.stdin)
	} else {
		data, err = os.ReadFile(pos[1])
	}
	if err != nil {
		return err
	}
	_, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	res, err := svc.calendar.Import(ctx, calendar.Operator("import"), ref, data, *replace)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "%s: %d added, %d replaced, %d skipped.\n", ref, res.Added, res.Replaced, res.Skipped)
	for _, msg := range res.Errors {
		fmt.Fprintln(e.stdout, "  "+msg)
	}
	return nil
}

// cmdExport: agendouille export <owner>~<slug> > file.ics.
func cmdExport(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille export", flag.ContinueOnError)
	loader := config.NewLoader(set)
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if err := want(pos, 1, "export <owner>~<slug>"); err != nil {
		return err
	}
	ref, err := calendar.ParseRef(pos[0])
	if err != nil {
		return err
	}
	_, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	data, err := svc.calendar.Export(ctx, calendar.Operator("cli"), ref)
	if err != nil {
		return err
	}
	_, err = e.stdout.Write(data)
	return err
}
