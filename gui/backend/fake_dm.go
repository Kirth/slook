package backend

import (
	"context"
	"slices"
	"strconv"
	"strings"
)

// OpenConversation finds the DM with one person (by PeerID) or the
// group DM with several, else makes it, named from the fake's people,
// and reports a new one as a ConversationOpened.
func (f *Fake) OpenConversation(ctx context.Context, userIDs []string) (Conversation, error) {
	ids, err := recipients(userIDs, fakeSelf)
	if err != nil {
		return Conversation{}, err
	}
	sorted := slices.Sorted(slices.Values(ids))
	key := strings.Join(sorted, ",")
	f.mu.Lock()
	if len(ids) == 1 {
		for _, id := range f.order {
			if c := f.convs[id]; c.Kind == KindDM && c.PeerID == ids[0] {
				conv := *c
				f.mu.Unlock()
				return conv, nil
			}
		}
	} else if id, ok := f.groups[key]; ok && f.convs[id] != nil {
		conv := *f.convs[id]
		f.mu.Unlock()
		return conv, nil
	}
	f.dmSeq++
	c := &Conversation{ID: "D" + strconv.Itoa(f.dmSeq), Kind: KindDM}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = fakeName(id)
	}
	if len(ids) == 1 {
		c.PeerID = ids[0]
	} else {
		c.Kind = KindGroupDM
		f.groups[key] = c.ID
	}
	c.Name = strings.Join(names, ", ")
	f.convs[c.ID], f.order = c, append(f.order, c.ID)
	f.logs[c.ID] = []Message{}
	conv := *c
	f.mu.Unlock()
	select {
	case f.events <- ConversationOpened{Conv: conv}:
		return conv, nil
	case <-ctx.Done():
		return Conversation{}, ctx.Err()
	}
}

// fakeName is a fake person's name by ID: an author, else one of the
// members known by ID only, else the ID.
func fakeName(id string) string {
	if n, ok := fakeNames.Users[id]; ok {
		return n
	}
	if n, ok := fakeMembers[id]; ok {
		return n
	}
	return id
}
