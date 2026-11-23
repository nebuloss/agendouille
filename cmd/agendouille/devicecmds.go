package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// cmdDevice makes, lists and revokes sync credentials from a shell, until
// the web app's devices page (M4): the way to set up DAVx5, an iPhone or
// Thunderbird by hand.
func cmdDevice(ctx context.Context, e env, args []string) error {
	const usage = "device add <user> [--kind davx5|ios|thunderbird|other] [--label L] [--read] | list <user> | revoke <user> <id>"
	set := flag.NewFlagSet("agendouille device", flag.ContinueOnError)
	loader := config.NewLoader(set)
	kind := set.String("kind", "davx5", "davx5, ios, thunderbird or other (add)")
	label := set.String("label", "", "what the device is, as the user will see it (add)")
	read := set.Bool("read", false, "a read-only device (add)")
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if len(pos) < 2 {
		return fmt.Errorf("usage: agendouille %s", usage)
	}
	_, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	u, err := svc.dir.User(ctx, pos[1])
	if err != nil {
		return fmt.Errorf("user %q: %w", pos[1], err)
	}
	switch pos[0] {
	case "add":
		if err := want(pos, 2, "device add <user>"); err != nil {
			return err
		}
		base := strings.TrimRight(svc.settings.Get().PublicURL, "/")
		if base == "" {
			return errors.New("public_url is not set, so a device has no address to sync with: set public_listen and public_url in the configuration file (/etc/agendouille.yaml)")
		}
		cap := "readwrite"
		if *read {
			cap = "read"
		}
		if *label == "" {
			*label = *kind
		}
		d, err := svc.auth.NewDevice(ctx, u.ID, *kind, *label, cap)
		if err != nil {
			return err
		}
		pub, _ := url.Parse(base)
		fmt.Fprintf(e.stdout, "Device %d for %s (%s, %s). The secret is shown once:\n\n", d.ID, u.Name, d.Label, d.Cap)
		fmt.Fprintf(e.stdout, "  server:   %s/dav/\n  username: %s\n  password: %s\n\n", base, d.Username, d.Secret)
		if *kind == "davx5" {
			// DAVx5 opens this link with everything filled in (its manual,
			// "Integration"); the credentials are in it, so it is shown here
			// and never logged.
			link := "davx5://" + url.QueryEscape(d.Username) + ":" + url.QueryEscape(d.Secret) + "@" + pub.Host + "/dav/"
			code, err := webx.QRTerminal(link)
			if err != nil {
				return err
			}
			fmt.Fprintf(e.stdout, "%s\nScan it with the phone's camera to open DAVx5's login filled in.\n", code)
		}
		return nil
	case "list":
		ds, err := svc.auth.Devices(ctx, u.ID)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(e.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tKIND\tLABEL\tCAP\tLAST USED\tCLIENT")
		for _, d := range ds {
			used := "never"
			if d.LastUsedAt != nil {
				used = d.LastUsedAt.Local().Format(time.DateTime)
			}
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", d.ID, d.Kind, d.Label, d.Cap, used, d.Client)
		}
		return tw.Flush()
	case "revoke":
		if err := want(pos, 3, "device revoke <user> <id>"); err != nil {
			return err
		}
		id, err := strconv.ParseInt(pos[2], 10, 64)
		if err != nil {
			return fmt.Errorf("device id %q is not a number", pos[2])
		}
		return svc.auth.RevokeDevice(ctx, "cli", u.ID, id)
	}
	return fmt.Errorf("usage: agendouille %s", usage)
}
