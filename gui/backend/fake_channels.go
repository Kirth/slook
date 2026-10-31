package backend

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/gammons/slk/internal/text"
)

// SearchChannels finds the fake's channels whose name contains query,
// case- and accent-blind: the ones the user is in (Member) and the
// ones they are not.
func (f *Fake) SearchChannels(_ context.Context, query string) ([]ChannelResult, error) {
	q := text.Fold(strings.TrimSpace(strings.TrimPrefix(query, "#")))
	if q == "" {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []ChannelResult{} // empty, not nil: an answer with no match
	add := func(c *Conversation, member bool) {
		if (c.Kind == KindChannel || c.Kind == KindPrivate) && strings.Contains(text.Fold(c.Name), q) {
			out = append(out, ChannelResult{ID: c.ID, Name: c.Name, Kind: c.Kind, Member: member})
		}
	}
	for _, id := range f.order {
		add(f.convs[id], true)
	}
	ids := make([]string, 0, len(f.unjoined))
	for id := range f.unjoined {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		add(f.unjoined[id], false)
	}
	return out, nil
}

// Join moves an unjoined channel into the user's conversations with a
// little history, and reports it as a ConversationOpened. A channel the
// user is in already is described without an event.
func (f *Fake) Join(ctx context.Context, channelID string) (Conversation, error) {
	f.mu.Lock()
	if c := f.convs[channelID]; c != nil {
		conv := *c
		f.mu.Unlock()
		return conv, nil
	}
	c := f.unjoined[channelID]
	if c == nil {
		f.mu.Unlock()
		return Conversation{}, errors.New("channel_not_found")
	}
	delete(f.unjoined, channelID)
	f.convs[c.ID], f.order = c, append(f.order, c.ID)
	n := len(f.order)
	for j := range 2 {
		f.appendLocked(c.ID, fakeAuthors[(n+j)%len(fakeAuthors)], fakeLines[(n+j)%len(fakeLines)])
	}
	c.Unread = false // history from before joining is not unread
	conv := *c
	f.mu.Unlock()
	select {
	case f.events <- ConversationOpened{Conv: conv}:
		return conv, nil
	case <-ctx.Done():
		return Conversation{}, ctx.Err()
	}
}
