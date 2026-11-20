package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/gotd/cli/internal/peercache"
)

func TestRunInvite(t *testing.T) {
	const targetID, userID, channelHash, userHash = 42, 86, 101, 102
	chatKey := peers.Key{Prefix: "chats_", ID: targetID}
	channelKey := peers.Key{Prefix: "channel_", ID: targetID}
	user := &tg.InputUser{UserID: userID, AccessHash: userHash}
	channel := &tg.InputChannel{ChannelID: targetID, AccessHash: channelHash}
	channelInfo := &tg.Channel{
		ID: targetID, AccessHash: channelHash, Title: "Test team", Megagroup: true, Photo: &tg.ChatPhotoEmpty{},
	}
	getChat := &tg.MessagesGetChatsRequest{ID: []int64{targetID}}
	chatResult := &tg.MessagesChats{
		Chats: []tg.ChatClass{&tg.Chat{ID: targetID, Title: "Test group", Photo: &tg.ChatPhotoEmpty{}}},
	}
	addChatUser := &tg.MessagesAddChatUserRequest{ChatID: targetID, UserID: user}
	getChannel := &tg.ChannelsGetChannelsRequest{ID: []tg.InputChannelClass{channel}}
	channelResult := &tg.MessagesChats{Chats: []tg.ChatClass{channelInfo}}
	inviteToChannel := &tg.ChannelsInviteToChannelRequest{Channel: channel, Users: []tg.InputUserClass{user}}

	for _, tt := range []struct {
		name, target         string
		cache                []peers.Key
		lookup, lookupResult bin.Encoder
		invite               bin.Encoder
		skipped              bool
		wantErr              string
	}{
		{
			name: "private basic group", target: "id:42", cache: []peers.Key{chatKey},
			lookup: getChat, lookupResult: chatResult, invite: addChatUser,
		},
		{
			name: "private supergroup", target: "id:42", cache: []peers.Key{channelKey},
			lookup: getChannel, lookupResult: channelResult, invite: inviteToChannel,
		},
		{
			name: "public username", target: "@example_team",
			lookup: &tg.ContactsResolveUsernameRequest{Username: "example_team"},
			lookupResult: &tg.ContactsResolvedPeer{
				Peer: &tg.PeerChannel{ChannelID: targetID}, Chats: []tg.ChatClass{channelInfo},
			},
			invite: inviteToChannel,
		},
		{
			name: "uncached ID", target: "id:42", wantErr: "not in cache",
		},
		{
			name: "ambiguous ID", target: "id:42", cache: []peers.Key{chatKey, channelKey},
			wantErr: "ambiguous",
		},
		{
			name: "basic group skips invitee", target: "id:42", cache: []peers.Key{chatKey},
			lookup: getChat, lookupResult: chatResult, invite: addChatUser,
			skipped: true, wantErr: "Telegram did not invite 1 user",
		},
		{
			name: "supergroup skips invitee", target: "id:42", cache: []peers.Key{channelKey},
			lookup: getChannel, lookupResult: channelResult, invite: inviteToChannel,
			skipped: true, wantErr: "Telegram did not invite 1 user",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := peercache.Open(filepath.Join(t.TempDir(), "peers.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save(ctx, peers.Key{Prefix: "users_", ID: userID}, peers.Value{AccessHash: userHash}); err != nil {
				t.Fatal(err)
			}
			for _, key := range tt.cache {
				var value peers.Value
				if key == channelKey {
					value.AccessHash = channelHash
				}
				if err := store.Save(ctx, key, value); err != nil {
					t.Fatal(err)
				}
			}

			api, mock := newTestAPI(t)
			mock.ExpectCall(&tg.UsersGetUsersRequest{ID: []tg.InputUserClass{user}}).
				ThenResult(&tg.UserClassVector{Elems: []tg.UserClass{&tg.User{ID: userID, AccessHash: userHash}}})
			if tt.lookup != nil {
				mock.ExpectCall(tt.lookup).ThenResult(tt.lookupResult)
			}
			if tt.invite != nil {
				result := &tg.MessagesInvitedUsers{Updates: &tg.UpdatesTooLong{}}
				if tt.skipped {
					result.MissingInvitees = []tg.MissingInvitee{{UserID: userID}}
				}
				mock.ExpectCall(tt.invite).ThenResult(result)
			}

			m := &peerManager{Manager: peers.Options{Storage: store}.Build(api), store: store}
			err = runInvite(ctx, api, m, tt.target, []string{"id:86"})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("runInvite error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
