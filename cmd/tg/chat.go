package main

import (
	"context"
	"fmt"
	"io"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

// chatInfo is the result of `tg chat get`.
type chatInfo struct {
	Peer     peerRef `json:"peer"`
	Verified bool    `json:"verified,omitempty"`
	Scam     bool    `json:"scam,omitempty"`
	Fake     bool    `json:"fake,omitempty"`
}

// MarshalText renders the chat info.
func (c chatInfo) MarshalText(w io.Writer) error {
	var flags string
	for name, on := range map[string]bool{"verified": c.Verified, "scam": c.Scam, "fake": c.Fake} {
		if on {
			flags += " " + name
		}
	}
	_, err := fmt.Fprintf(w, "%s id=%d%s\n", c.Peer.label(), c.Peer.ID, flags)
	return err
}

// chatFullResult is the result of `tg chat full`.
type chatFullResult struct {
	Peer              peerRef `json:"peer"`
	About             string  `json:"about,omitempty"`
	ParticipantsCount int     `json:"participants_count,omitempty"`
}

// MarshalText renders the full chat info.
func (c chatFullResult) MarshalText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "%s id=%d\n", c.Peer.label(), c.Peer.ID); err != nil {
		return err
	}
	if c.ParticipantsCount > 0 {
		if _, err := fmt.Fprintf(w, "participants: %d\n", c.ParticipantsCount); err != nil {
			return err
		}
	}
	if c.About != "" {
		if _, err := fmt.Fprintf(w, "about: %s\n", c.About); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "chat",
		Short:   "Inspect a chat",
		GroupID: groupChats,
		Long:    "Get basic or full information about a chat.",
	}
	cmd.AddCommand(a.newChatGetCmd(), a.newChatFullCmd())
	return cmd
}

func (a *app) newChatGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get <peer>",
		Short:             "Get basic chat info",
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
				return a.printer.Emit(chatInfo{
					Peer:     describeManagedPeer(p),
					Verified: p.Verified(),
					Scam:     p.Scam(),
					Fake:     p.Fake(),
				})
			})
		},
	}
	return cmd
}

func (a *app) newChatFullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "full <peer>",
		Short:             "Get full chat info (about, participant count)",
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

				res := chatFullResult{Peer: describeManagedPeer(p)}
				if err := fillChatFull(ctx, p, &res); err != nil {
					return err
				}
				return a.printer.Emit(res)
			})
		},
	}
	return cmd
}

// fillChatFull populates about/participant count by peer type.
func fillChatFull(ctx context.Context, p peers.Peer, res *chatFullResult) error {
	switch v := p.(type) {
	case peers.User:
		full, err := v.FullRaw(ctx)
		if err != nil {
			return errors.Wrap(err, "get full user")
		}
		res.About = full.About
	case peers.Channel:
		full, err := v.FullRaw(ctx)
		if err != nil {
			return errors.Wrap(err, "get full channel")
		}
		res.About = full.About
		res.ParticipantsCount = full.ParticipantsCount
	case peers.Chat:
		full, err := v.FullRaw(ctx)
		if err != nil {
			return errors.Wrap(err, "get full chat")
		}
		res.About = full.About
	}
	return nil
}
