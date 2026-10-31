package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// maxTopChannels caps the frecency hint sent with a channel search.
//
// The captures show 22 ids on channels/search and the endpoint does
// not cap it, so this is slk's own bound: the hint is a ranking
// nicety, and a request whose body grows with the user's history is
// exactly the kind of shape that made the old enumeration
// recognisable.
const maxTopChannels = 25

// SearchChannels asks edgeapi's channels/search which channels match
// query: the endpoint Slack's own client uses, in place of walking
// conversations.list. Nothing is fetched until somebody types, and
// callers debounce (chat.RemoteSearch), as edge requires.
//
// The hint (top_channels) is the conversations most recently visited,
// then active, at most maxTopChannels. Archived channels, DMs and group
// DMs are left out. Member comes from the conversations slk lists; the
// response's member_channels is ignored, so one source decides
// membership.
func (s *Slack) SearchChannels(ctx context.Context, query string) ([]ChannelResult, error) {
	if query == "" {
		return nil, nil
	}
	if s.edge == nil {
		return nil, errors.New("channel search is not available")
	}
	s.mu.Lock()
	top := s.convOrder[:min(len(s.convOrder), maxTopChannels)]
	top = append([]string(nil), top...)
	s.mu.Unlock()
	chans, _, err := s.edge.ChannelsSearch(ctx, query, top)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slk-gio: channels: search %q: %v\n", query, err)
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ChannelResult, 0, len(chans))
	for _, ch := range chans {
		if ch.IsArchived || ch.IsIM || ch.IsMPIM {
			continue
		}
		r := ChannelResult{ID: ch.ID, Name: ch.Name, Kind: KindChannel}
		if ch.IsPrivate {
			r.Kind = KindPrivate
		}
		_, r.Member = s.kinds[ch.ID]
		out = append(out, r)
	}
	return out, nil
}

// Join joins a channel (conversations.join, which answers a channel the
// user is in already without error) and describes it as Conversation
// does, which also reports it as a ConversationOpened.
func (s *Slack) Join(ctx context.Context, channelID string) (Conversation, error) {
	if err := s.client.JoinChannel(ctx, channelID); err != nil {
		fmt.Fprintln(os.Stderr, "slk-gio: channels:", err)
		return Conversation{}, err
	}
	return s.Conversation(ctx, channelID)
}
