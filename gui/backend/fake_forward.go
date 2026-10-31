package backend

import (
	"context"
	"fmt"
	"slices"

	"github.com/gammons/slk/gui/rich"
)

// Forward posts, as the user, the comment and a card holding m's body,
// as Slack's preview of a forwarded permalink shows it; the message's
// text is the permalink.
func (f *Fake) Forward(ctx context.Context, m Message, toChannelID, comment string) error {
	f.mu.Lock()
	i := slices.IndexFunc(f.logs[m.ChannelID], func(o Message) bool { return o.TS == m.TS })
	if i < 0 {
		f.mu.Unlock()
		return fmt.Errorf("message_not_found")
	}
	if _, ok := f.convs[toChannelID]; !ok {
		f.mu.Unlock()
		return fmt.Errorf("channel_not_found")
	}
	orig := cloneMessages(f.logs[m.ChannelID][i : i+1])[0]
	link, _ := f.Permalink(ctx, m.ChannelID, m.TS)
	posted := f.appendLocked(toChannelID, "You", comment)
	from := m.ChannelID
	if c := f.convs[m.ChannelID]; c != nil {
		from = c.Name
		if c.Kind == KindChannel || c.Kind == KindPrivate {
			from = "#" + from
		}
	}
	card := rich.Card{Blocks: append([]rich.Block{rich.Paragraph{Spans: []rich.Span{{Text: orig.Author, Bold: true}}}}, orig.Body...),
		Footer: "Shared from " + from}
	posted.Body = append(posted.Body, card)
	posted.Text = "<" + link + ">"
	if comment != "" {
		posted.Text = comment + "\n" + posted.Text
	}
	log := f.logs[toChannelID]
	log[len(log)-1] = posted
	f.mu.Unlock()
	select {
	case f.events <- MessagePosted{Message: posted}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
