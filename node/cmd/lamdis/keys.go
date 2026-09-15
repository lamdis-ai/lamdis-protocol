package main

// `lamdis keys` is the operator's side of letting other people use your
// inference without being able to drain you.
//
//	lamdis keys mint sam@example.com -limit 2
//	lamdis keys list
//	lamdis keys revoke <hash>
//
// Each minted key carries a hard credit cap that never refills, so the worst
// an approved account can cost you is the number you chose. The key is theirs
// to paste into Settings; their prompts go from their machine to OpenRouter
// and never through anything of yours.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

func cmdKeys(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return keysUsage()
	}
	p := &agent.Provisioner{ManagementKey: os.Getenv("LAMDIS_OPENROUTER_MANAGEMENT_KEY")}
	sub, rest := args[0], args[1:]
	switch sub {
	case "mint":
		fs := flag.NewFlagSet("keys mint", flag.ContinueOnError)
		limit := fs.Float64("limit", 2, "hard credit cap in dollars; never refills")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		label := strings.TrimSpace(strings.Join(fs.Args(), " "))
		if label == "" {
			return fmt.Errorf("say who it is for: lamdis keys mint sam@example.com -limit 2")
		}
		k, err := p.Mint(ctx, label, *limit)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", k.Secret)
		fmt.Fprintf(os.Stderr, "\nminted for %s · cap $%.2f · hash %s\n", k.Name, k.Limit, k.Hash)
		fmt.Fprintf(os.Stderr, "The cap does not refill. Revoke with: lamdis keys revoke %s\n", k.Hash)
		fmt.Fprintf(os.Stderr, "Send them the line above and tell them to paste it into Settings → OpenRouter key.\n")
		return nil
	case "list":
		keys, err := p.List(ctx)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			fmt.Println("no keys")
			return nil
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].Spent() > keys[j].Spent() })
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "WHO\tSPENT\tCAP\tLEFT\tSTATE\tHASH")
		var spent, capped float64
		for _, k := range keys {
			state := "live"
			if k.Disabled {
				state = "disabled"
			} else if k.Limit > 0 && k.Left() <= 0 {
				state = "spent out"
			}
			fmt.Fprintf(w, "%s\t$%.2f\t$%.2f\t$%.2f\t%s\t%s\n", k.Name, k.Spent(), k.Limit, k.Left(), state, k.Hash)
			spent += k.Spent()
			capped += k.Limit
		}
		w.Flush()
		fmt.Printf("\n%d keys · $%.2f spent · $%.2f is the most they can ever cost you\n", len(keys), spent, capped)
		return nil
	case "revoke":
		if len(rest) != 1 {
			return fmt.Errorf("usage: lamdis keys revoke <hash>")
		}
		if err := p.Revoke(ctx, rest[0]); err != nil {
			return err
		}
		fmt.Println("revoked")
		return nil
	case "limit":
		fs := flag.NewFlagSet("keys limit", flag.ContinueOnError)
		to := fs.Float64("to", 0, "new cap in dollars")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if len(fs.Args()) != 1 || *to <= 0 {
			return fmt.Errorf("usage: lamdis keys limit <hash> -to 5")
		}
		if err := p.SetLimit(ctx, fs.Args()[0], *to); err != nil {
			return err
		}
		fmt.Printf("cap is now $%.2f\n", *to)
		return nil
	}
	return keysUsage()
}

func keysUsage() error {
	fmt.Fprint(os.Stderr, `usage: lamdis keys <command>

  mint <who> [-limit 2]     mint a key with a hard cap that never refills;
                            prints the key on stdout, the details on stderr
  list                      every key, what it has spent, what is left
  limit <hash> -to 5        raise or lower a cap
  revoke <hash>             kill a key now

Set LAMDIS_OPENROUTER_MANAGEMENT_KEY first. Create one at
openrouter.ai/settings/management-keys; it is not an ordinary API key and it
should never leave the machine you approve people from.
`)
	return fmt.Errorf("missing or unknown command")
}
