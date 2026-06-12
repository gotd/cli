package main

import (
	"bufio"
	"context"
	"os"
	"strings"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

// usersToPeerList maps users to a peerListResult.
func usersToPeerList(users []tg.UserClass) peerListResult {
	ent := entitiesOf(users, nil)
	var out peerListResult
	for _, uc := range users {
		u, ok := uc.AsNotEmpty()
		if !ok {
			continue
		}
		out.Peers = append(out.Peers, describePeer(&tg.PeerUser{UserID: u.ID}, ent))
	}
	return out
}

func (a *app) newContactsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contacts",
		Short:   "Manage contacts",
		GroupID: groupChats,
		Long:    "List, search, add, delete, block and import contacts.",
	}
	cmd.AddCommand(
		a.newContactsListCmd(),
		a.newContactsSearchCmd(),
		a.newContactsAddCmd(),
		a.newContactsDeleteCmd(),
		a.newContactsBlockCmd(),
		a.newContactsUnblockCmd(),
		a.newContactsBlockedCmd(),
		a.newContactsImportCmd(),
	)
	return cmd
}

func (a *app) newContactsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     cmdList,
		Aliases: []string{"export"},
		Short:   "List your contacts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.ContactsGetContacts(ctx, 0)
				if err != nil {
					return errors.Wrap(err, "contacts.getContacts")
				}
				contacts, ok := res.(*tg.ContactsContacts)
				if !ok {
					return a.printer.Emit(peerListResult{})
				}
				return a.printer.Emit(usersToPeerList(contacts.Users))
			})
		},
	}
}

func (a *app) newContactsSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search your contacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: args[0], Limit: limit})
				if err != nil {
					return errors.Wrap(err, "contacts.search")
				}
				ent := entitiesOf(found.Users, found.Chats)
				var out peerListResult
				for _, pc := range found.MyResults {
					out.Peers = append(out.Peers, describePeer(pc, ent))
				}
				return a.printer.Emit(out)
			})
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "maximum number of results")
	return cmd
}

func (a *app) newContactsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <phone> <first-name> [last-name]",
		Short: "Add a contact by phone number",
		Long:  "Add a contact by phone number (imports it into your contact list).",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			last := ""
			if len(args) == 3 {
				last = args[2]
			}
			contact := tg.InputPhoneContact{Phone: args[0], FirstName: args[1], LastName: last}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.ContactsImportContacts(ctx, []tg.InputPhoneContact{contact})
				if err != nil {
					return errors.Wrap(err, "contacts.importContacts")
				}
				return a.printer.Emit(usersToPeerList(res.Users))
			})
		},
	}
	return cmd
}

func (a *app) newContactsImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import contacts from a CSV file (phone,first,last per line)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			contacts, err := readContactsFile(args[0])
			if err != nil {
				return err
			}
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.ContactsImportContacts(ctx, contacts)
				if err != nil {
					return errors.Wrap(err, "contacts.importContacts")
				}
				return a.printer.Emit(usersToPeerList(res.Users))
			})
		},
	}
	return cmd
}

// readContactsFile parses "phone,first,last" lines into phone contacts.
func readContactsFile(path string) ([]tg.InputPhoneContact, error) {
	f, err := os.Open(path) // #nosec G304 // user-provided path
	if err != nil {
		return nil, errors.Wrap(err, "open contacts file")
	}
	defer func() { _ = f.Close() }()

	var contacts []tg.InputPhoneContact
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		c := tg.InputPhoneContact{Phone: strings.TrimSpace(fields[0])}
		if len(fields) > 1 {
			c.FirstName = strings.TrimSpace(fields[1])
		}
		if len(fields) > 2 {
			c.LastName = strings.TrimSpace(fields[2])
		}
		contacts = append(contacts, c)
	}
	if err := sc.Err(); err != nil {
		return nil, errors.Wrap(err, "read contacts file")
	}
	if len(contacts) == 0 {
		return nil, errors.New("no contacts found in file")
	}
	return contacts, nil
}

func (a *app) newContactsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "delete <peer>",
		Short:             "Delete a contact",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				p, err := m.Resolve(ctx, args[0])
				if err != nil {
					return errors.Wrapf(err, "resolve %q", args[0])
				}
				user, ok := p.(peers.User)
				if !ok {
					return errors.New("not a user")
				}
				if _, err := api.ContactsDeleteContacts(ctx, []tg.InputUserClass{user.InputUser()}); err != nil {
					return errors.Wrap(err, "contacts.deleteContacts")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

func (a *app) newContactsBlockCmd() *cobra.Command {
	return a.blockCmd("block", "Block a peer", false)
}

func (a *app) newContactsUnblockCmd() *cobra.Command {
	return a.blockCmd("unblock", "Unblock a peer", true)
}

func (a *app) blockCmd(use, short string, unblock bool) *cobra.Command {
	return &cobra.Command{
		Use:               use + " <peer>",
		Short:             short,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: peerArgCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				m, err := a.manager(api)
				if err != nil {
					return err
				}
				p, err := resolvePeer(ctx, m, args[0])
				if err != nil {
					return err
				}
				if unblock {
					_, err = api.ContactsUnblock(ctx, &tg.ContactsUnblockRequest{ID: p})
				} else {
					_, err = api.ContactsBlock(ctx, &tg.ContactsBlockRequest{ID: p})
				}
				if err != nil {
					return errors.Wrap(err, "contacts block/unblock")
				}
				return a.printer.Emit(okResult{OK: true})
			})
		},
	}
}

func (a *app) newContactsBlockedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "blocked",
		Short: "List blocked peers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd.Context(), runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
				res, err := api.ContactsGetBlocked(ctx, &tg.ContactsGetBlockedRequest{})
				if err != nil {
					return errors.Wrap(err, "contacts.getBlocked")
				}
				switch v := res.(type) {
				case *tg.ContactsBlocked:
					return a.printer.Emit(usersToPeerList(v.Users))
				case *tg.ContactsBlockedSlice:
					return a.printer.Emit(usersToPeerList(v.Users))
				default:
					return a.printer.Emit(peerListResult{})
				}
			})
		},
	}
}
