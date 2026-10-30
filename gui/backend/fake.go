package backend

import (
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gammons/slk/gui/rich"
	"github.com/gammons/slk/internal/core"
	imgpkg "github.com/gammons/slk/internal/image"
)

// Fake is an in-memory workspace for trying the GUI without a Slack
// account: a few conversations with some history, and someone posting
// every few seconds.
type Fake struct {
	// Interval is how often a made-up message arrives; zero disables them.
	Interval time.Duration

	events chan Event

	mu    sync.Mutex
	seq   int64
	logs  map[string][]Message
	convs map[string]*Conversation // activity state, by ID
	order []string
	// bf is the pretend backfill: it walks the conversations on a
	// timer so the search window has progress to show.
	bf struct {
		cancel  context.CancelFunc
		status  BackfillStatus
		current string
	}
}

// fakeConversations are the conversations with history and live
// posts; fakeQuiet adds ones that exist but have been idle, as most of
// a real workspace's channels are.
var fakeConversations = []Conversation{
	{ID: "C1", Name: "general", Kind: KindChannel},
	{ID: "C2", Name: "random", Kind: KindChannel},
	{ID: "C3", Name: "eng-backend", Kind: KindPrivate},
	{ID: "D1", Name: "Ada Lovelace", Kind: KindDM, PeerID: "U1"},
	{ID: "D2", Name: "Grace Hopper, Linus Torvalds", Kind: KindGroupDM},
}

// fakeQuiet: name, kind, days since the last message, section.
var fakeQuiet = []struct {
	id, name string
	kind     Kind
	idleDays int
	section  string
}{
	{"C10", "incidents", KindChannel, 0, ""},
	{"C11", "release-train", KindChannel, 2, "S1"},
	{"C12", "offsite-2026", KindChannel, 300, "S1"},
	{"C13", "design-review", KindChannel, 40, ""},
	{"C14", "office-amsterdam", KindChannel, 25, ""},
	{"C15", "book-club", KindChannel, 90, ""},
	{"C16", "kubernetes-upgrades", KindPrivate, 61, ""},
	{"C17", "hackathon", KindChannel, 360, ""},
	{"D3", "Margaret Hamilton", KindDM, 45, ""}, // peer U3
}

// fakeSections mirror a Slack sidebar with one custom section.
var fakeSections = []Section{
	{ID: "S0", Name: "Starred", Type: "stars"},
	{ID: "S1", Name: "Projects", Emoji: "rocket", Type: "standard"}, // a name, as Slack sends it
	{ID: "S2", Name: "Channels", Type: "channels"},
	{ID: "S3", Name: "Direct messages", Type: "direct_messages"},
}

var fakeAuthors = []string{"Ada Lovelace", "Grace Hopper", "Linus Torvalds", "Margaret Hamilton"}

// fakeNames resolves the IDs the fake lines mention.
var fakeNames = rich.MapNames{
	Users:    map[string]string{"U1": "Ada Lovelace", "U2": "Grace Hopper", "U3": "Margaret Hamilton", "U4": "Linus Torvalds"},
	Channels: map[string]string{"C1": "general", "C3": "eng-backend"},
}

// fakeLines are mrkdwn, as Slack would send them, so the formatting
// path gets exercised.
var fakeLines = []string{
	"Morning! Anyone else seeing the deploy queue back up?",
	"Pushed a fix, CI is running :rocket:",
	"Can someone review the *cache migration* before lunch? <@U1> you touched this last",
	"That flaky test is flaky again :face_with_rolling_eyes: see <https://example.com/ci/4821|build 4821>",
	"Lunch? :pizza:",
	"Rolling back, give me five minutes.",
	"LGTM :+1:",
	"Runbook, for the record:\n- check `kubectl get pods -n api`\n- scale down the worker\n- then redeploy",
	"```\n$ make test\nok  github.com/gammons/slk/gui/rich  0.007s\n```",
	"&gt; ship it\nwords to live by, _apparently_ ~not~",
	"Moving the standup to <#C3>, &lt;3 everyone",
}

// NewFake builds a Fake whose history starts at now.
func NewFake(now time.Time) *Fake {
	f := &Fake{
		Interval: 4 * time.Second,
		events:   make(chan Event, 64),
		logs:     map[string][]Message{},
		seq:      now.Add(-2 * time.Hour).Unix(),
		convs:    map[string]*Conversation{},
	}
	for i, c := range fakeConversations {
		c := c
		f.convs[c.ID], f.order = &c, append(f.order, c.ID)
		n := 12
		if c.ID == "C1" {
			n = 80 // more than a page, to scroll back through
		}
		for j := range n {
			f.appendLocked(c.ID, fakeAuthors[(i+j)%len(fakeAuthors)], fakeLines[(i*3+j)%len(fakeLines)])
		}
		c.Visited = now.Add(-time.Duration(i+1) * time.Hour)
	}
	for _, q := range fakeQuiet {
		c := &Conversation{ID: q.id, Name: q.name, Kind: q.kind, Section: q.section,
			Latest: now.Add(-time.Duration(q.idleDays) * 24 * time.Hour)}
		if q.kind == KindDM {
			c.PeerID = "U3"
		}
		f.convs[q.id], f.order = c, append(f.order, q.id)
		m := f.appendLocked(q.id, fakeAuthors[len(f.order)%len(fakeAuthors)], fakeLines[len(f.order)%len(fakeLines)])
		// Its one message is from today; an idle conversation's newest
		// is that many days back.
		c.Latest = m.Time.Add(-time.Duration(q.idleDays) * 24 * time.Hour)
	}
	// A message with attachments, for the previewer.
	m := f.appendLocked("C1", "Grace Hopper", "Photos from the offsite, and my notes")
	log := f.logs["C1"]
	log[len(log)-1].Body = append(m.Body,
		rich.Image{FileID: "FAKE1", URL: "fake://sunset.png", FileName: "sunset.png", Title: "Sunset", Alt: "sunset.png", W: 640, H: 400},
		rich.Image{FileID: "FAKE2", URL: "fake://harbour.png", FileName: "harbour.png", Title: "Harbour", Alt: "harbour.png", W: 640, H: 400},
		rich.File{FileID: "FAKE3", URL: "fake://notes.txt", Name: "Offsite notes", FileName: "offsite-notes.txt", Mime: "text/plain", Size: 812},
	)
	// Threads: a reply count to click, and replies to read.
	log = f.logs["C1"]
	// Their parents are in the newest page, where a window opens.
	parent := log[len(log)-11].TS
	f.replyLocked("C1", parent, "Linus Torvalds", "Which queue? The deploy one has been fine for me", false)
	f.replyLocked("C1", parent, "Ada Lovelace", "The *worker* queue. <@U2> restarted it, looks better now", false)
	f.replyLocked("C1", parent, "Grace Hopper", "Yes, back to normal :+1:", false)
	f.replyLocked("C1", log[len(log)-6].TS, "Margaret Hamilton", "Following up in <#C3>", true)
	// Reactions, one the user is in and one custom, and custom emoji in
	// a line of text.
	f.appendLocked("C1", "Linus Torvalds", "Merged :partyparrot: and tagged, :shipit: when ready")
	feed := f.pageLocked("C1", "")
	newest, earlier := feed[len(feed)-1].TS, feed[len(feed)-4].TS
	f.reactLocked("C1", newest, "partyparrot", "U1", true)
	f.reactLocked("C1", newest, "partyparrot", "U2", true)
	for _, u := range []string{"U1", "U2", fakeSelf} {
		f.reactLocked("C1", earlier, "+1", u, true)
	}
	f.reactLocked("C1", earlier, "tada", "U3", true)
	// Seeding history marked everything unread; start read instead.
	for _, c := range f.convs {
		c.Unread = false
	}
	// Something to see in each part of the list.
	f.convs["C10"].Unread, f.convs["C10"].Mentions = true, 2
	f.convs["C3"].Starred = true
	f.convs["C2"].Muted = true
	f.convs["D1"].Unread, f.convs["D1"].Mentions = true, 1
	return f
}

// appendLocked records a message with the next ts. Callers hold mu, or
// own f before it is shared.
func (f *Fake) appendLocked(channelID, author, text string) Message {
	m := f.nextLocked(channelID, author, text)
	f.logs[channelID] = append(f.logs[channelID], m)
	if c := f.convs[channelID]; c != nil {
		c.Latest = m.Time
		if !m.FromSelf {
			c.Unread = true
		}
	}
	return m
}

// replyLocked records a reply in the thread under threadTS and counts
// it on the parent. A broadcast reply is also in the channel's history.
func (f *Fake) replyLocked(channelID, threadTS, author, text string, broadcast bool) Message {
	m := f.nextLocked(channelID, author, text)
	m.ThreadTS, m.InThreadOnly = threadTS, !broadcast
	f.logs[channelID] = append(f.logs[channelID], m)
	for i := range f.logs[channelID] {
		if p := &f.logs[channelID][i]; p.TS == threadTS {
			p.ThreadTS = threadTS
			p.ReplyCount++
		}
	}
	return m
}

// nextLocked builds a message with the next ts without recording it, as
// for an ephemeral reply that history does not keep.
func (f *Fake) nextLocked(channelID, author, text string) Message {
	f.seq += 7
	return Message{
		FromSelf:  author == "You",
		ChannelID: channelID,
		TS:        strconv.FormatInt(f.seq, 10) + ".000100",
		Author:    author,
		AuthorID:  author,
		Body:      rich.FromMrkdwn(text, fakeNames),
		Text:      text,
		Time:      time.Unix(f.seq, 0),
	}
}

func (f *Fake) Workspace() string { return "Demo" }
func (f *Fake) Self() string      { return fakeSelf }

// Images is a fetcher whose "downloads" are pictures made up on the
// spot, so custom emoji and the fake's images show without a network.
func (f *Fake) Images() *imgpkg.Fetcher { return fakeFetcher() }

var fakeFetcher = sync.OnceValue(func() *imgpkg.Fetcher {
	dir, err := os.MkdirTemp("", "slk-gio-fake-images-")
	if err != nil {
		return nil
	}
	store, err := imgpkg.NewCache(dir, 20)
	if err != nil {
		return nil
	}
	return imgpkg.NewFetcher(store, &http.Client{Transport: fakePictures{}})
})

// fakePictures answers every request with a PNG: a disc in a color
// taken from the URL, so each picture is recognisably its own.
type fakePictures struct{}

func (fakePictures) RoundTrip(req *http.Request) (*http.Response, error) {
	sum := sha1.Sum([]byte(req.URL.String()))
	const n = 64
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			dx, dy := x-n/2, y-n/2
			if dx*dx+dy*dy > (n/2-2)*(n/2-2) {
				continue
			}
			img.Set(x, y, color.NRGBA{sum[0] | 0x40, uint8(x * 255 / n), sum[2] | 0x40, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Request: req,
		Header: http.Header{"Content-Type": {"image/png"}},
		Body:   io.NopCloser(&buf), ContentLength: int64(buf.Len()),
	}, nil
}

// fakeTitles gives the made-up people something to say about themselves.
var fakeTitles = map[string]string{
	"Ada Lovelace":      "Analyst, Engines Division",
	"Grace Hopper":      "Rear Admiral, Compilers",
	"Linus Torvalds":    "Kernel maintainer",
	"Margaret Hamilton": "Director of Software Engineering",
}

func (f *Fake) Profile(_ context.Context, id string) (Profile, error) {
	name := id
	if n, ok := fakeNames.Users[id]; ok {
		name = n
	} else if n, ok := fakeMembers[id]; ok {
		name = n
	}
	p := Profile{
		ID:       id,
		Name:     name,
		RealName: name,
		Handle:   strings.ToLower(strings.ReplaceAll(name, " ", ".")),
		Title:    fakeTitles[name],
		TZLabel:  "Central European Time",
		TZOffset: 3600,
		HasTZ:    true,
	}
	if name == "Ada Lovelace" {
		p.StatusEmoji, p.StatusText = ":palm_tree:", "Out until Monday"
	}
	return p, nil
}

func (f *Fake) Conversations(context.Context) ([]Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Conversation, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, *f.convs[id])
	}
	return out, nil
}

// fakeDetails are the fake conversations' topics and sizes.
var fakeDetails = map[string]Details{
	"C1": {Topic: "Company-wide announcements and work-based matters", Members: 13,
		Purpose: "This channel is for workspace-wide communication and announcements. All members are in this channel. Ask <@U3> before posting, and read <https://example.com/handbook|the handbook>.",
		Created: time.Date(2019, 3, 14, 9, 26, 0, 0, time.Local), CreatorID: "U3", Creator: "Margaret Hamilton"},
	"C2": {Purpose: "Non-work banter and water cooler conversation", Members: 41,
		Created: time.Date(2019, 3, 14, 9, 27, 0, 0, time.Local), CreatorID: "U3", Creator: "Margaret Hamilton"},
	"C3": {Topic: "Deploys, migrations and the occasional fire 🔥", Members: 9,
		Created: time.Date(2023, 11, 2, 16, 4, 0, 0, time.Local), CreatorID: "U4", Creator: "Linus Torvalds"},
	"C10": {Topic: "SEV2 open: checkout latency · runbook in the bookmarks", Members: 23},
}

func (f *Fake) Details(_ context.Context, channelID string) (Details, error) {
	d := fakeDetails[channelID]
	d.TopicBody, d.PurposeBody = rich.FromMrkdwn(d.Topic, fakeNames), rich.FromMrkdwn(d.Purpose, fakeNames)
	return d, nil
}

// fakeMembers are people the fake knows only by ID until asked, so the
// members panel has names to resolve as it shows them.
var fakeMembers = map[string]string{
	"U5": "Katherine Johnson", "U6": "Barbara Liskov", "U7": "Edsger Dijkstra",
	"U8": "Frances Allen", "U9": "Ken Thompson", "U10": "Radia Perlman",
	"U11": "Dennis Ritchie", "U12": "Hedy Lamarr",
}

// Members is everyone the fake knows for a channel (the people of
// People, then fakeMembers by ID only), the two in a group DM, or the
// peer of a DM.
func (f *Fake) Members(ctx context.Context, channelID string) (Members, error) {
	known, _ := f.People(ctx)
	var m Members
	switch {
	case channelID == "D1":
		m.People = []Person{known[0], {ID: fakeSelf, Name: "You"}}
	case channelID == "D2":
		m.People = []Person{known[1], known[3], {ID: fakeSelf, Name: "You"}}
	default:
		m.People = append(m.People, Person{ID: fakeSelf, Name: "You"})
		for _, p := range known {
			if !p.Bot {
				m.People = append(m.People, p)
			}
		}
		for i := 5; i <= 12; i++ {
			m.People = append(m.People, Person{ID: fmt.Sprintf("U%d", i)})
		}
	}
	m.Total = len(m.People)
	return m, nil
}

// SetNotify keeps the level in memory and echoes it.
func (f *Fake) SetNotify(ctx context.Context, channelID string, level NotifyLevel) error {
	f.mu.Lock()
	if c := f.convs[channelID]; c != nil {
		c.Notify = level
	}
	f.mu.Unlock()
	select {
	case f.events <- NotifyChanged{ChannelID: channelID, Level: level}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *Fake) WebURL(string) string { return "" }

func (f *Fake) Sections() []Section { return append([]Section(nil), fakeSections...) }

// MarkRead clears the conversation's unread state and echoes it, as
// Slack's channel_marked does.
func (f *Fake) MarkRead(ctx context.Context, channelID, ts string) error {
	f.mu.Lock()
	if c := f.convs[channelID]; c != nil {
		c.Unread, c.Mentions = false, 0
	}
	f.mu.Unlock()
	select {
	case f.events <- ReadChanged{ChannelID: channelID, LastRead: ts}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Download makes up the fake attachments: a picture for an image, a
// few lines for anything else.
func (f *Fake) Download(_ context.Context, a Attachment) (string, error) {
	p := AttachmentPath(a)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	if strings.HasSuffix(a.Name, ".png") {
		img := image.NewRGBA(image.Rect(0, 0, 640, 400))
		for y := range 400 {
			for x := range 640 {
				img.Set(x, y, color.RGBA{uint8(x * 255 / 640), uint8(80 + y*100/400), uint8(200 - y*150/400), 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return "", err
		}
		return p, os.WriteFile(p, buf.Bytes(), 0o600)
	}
	return p, os.WriteFile(p, []byte("Offsite notes\n\n- ship the GUI\n- fewer meetings\n"), 0o600)
}

// CustomEmoji: the fake workspace has none.
func (f *Fake) CustomEmoji(context.Context) (map[string]string, error) {
	return map[string]string{
		"partyparrot": "https://emoji.fake/partyparrot.png",
		"shipit":      "alias:partyparrot",
	}, nil
}

// fakeSelf is the user's ID in the fake, as its messages' AuthorID.
const fakeSelf = "You"

// reactLocked adds or removes user's reaction on the message with ts.
// It reports whether anything changed.
func (f *Fake) reactLocked(channelID, ts, name, user string, add bool) bool {
	for i := range f.logs[channelID] {
		m := &f.logs[channelID][i]
		if m.TS != ts {
			continue
		}
		j := slices.IndexFunc(m.Reactions, func(r Reaction) bool { return r.Name == name })
		switch {
		case add && j < 0:
			m.Reactions = append(m.Reactions, Reaction{Name: name, Count: 1, Users: []string{user}, Mine: user == fakeSelf})
		case add:
			r := &m.Reactions[j]
			if slices.Contains(r.Users, user) {
				return false
			}
			r.Users = append(r.Users, user)
			r.Count++
			r.Mine = r.Mine || user == fakeSelf
		case j < 0:
			return false
		default:
			r := &m.Reactions[j]
			k := slices.Index(r.Users, user)
			if k < 0 {
				return false
			}
			r.Users = slices.Delete(slices.Clone(r.Users), k, k+1)
			r.Count--
			if user == fakeSelf {
				r.Mine = false
			}
			if r.Count == 0 {
				m.Reactions = slices.Delete(m.Reactions, j, j+1)
			}
		}
		return true
	}
	return false
}

// Edit replaces a message's text in the log and reports the new
// version, as Slack's message_changed does.
func (f *Fake) Edit(ctx context.Context, m Message, text string) error {
	f.mu.Lock()
	var edited Message
	found := false
	for i := range f.logs[m.ChannelID] {
		if p := &f.logs[m.ChannelID][i]; p.TS == m.TS {
			p.Text, p.Body, p.Edited = text, rich.FromMrkdwn(text, fakeNames), true
			edited, found = cloneMessages([]Message{*p})[0], true
		}
	}
	f.mu.Unlock()
	if !found {
		return fmt.Errorf("message_not_found")
	}
	select {
	case f.events <- MessagePosted{Message: edited}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Delete removes a message from the log and reports it.
func (f *Fake) Delete(ctx context.Context, channelID, ts string) error {
	f.mu.Lock()
	n := len(f.logs[channelID])
	f.logs[channelID] = slices.DeleteFunc(f.logs[channelID], func(m Message) bool { return m.TS == ts })
	gone := len(f.logs[channelID]) < n
	f.mu.Unlock()
	if !gone {
		return fmt.Errorf("message_not_found")
	}
	select {
	case f.events <- MessageDeleted{ChannelID: channelID, TS: ts}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Permalink is a made-up link in Slack's archives form.
func (f *Fake) Permalink(_ context.Context, channelID, ts string) (string, error) {
	return "https://demo.slack.com/archives/" + channelID + "/p" + strings.ReplaceAll(ts, ".", ""), nil
}

// React toggles the user's reaction and reports it, as Slack's echo.
func (f *Fake) React(ctx context.Context, channelID, ts, name string, add bool) error {
	f.mu.Lock()
	f.reactLocked(channelID, ts, name, fakeSelf, add)
	f.mu.Unlock()
	select {
	case f.events <- ReactionChanged{ChannelID: channelID, TS: ts, Name: name, UserID: fakeSelf, Added: add, Self: true}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Presence: Ada is around, everyone else away.
func (f *Fake) Presence(_ context.Context, userID string) (bool, error) {
	return userID == "U1", nil
}

func (f *Fake) Visited(channelID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.convs[channelID]; c != nil {
		c.Visited = time.Now()
	}
}

// fakePage is how many messages History and Older return at most.
const fakePage = 30

// History is the conversation's newest page, oldest first.
func (f *Fake) History(_ context.Context, channelID string) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.logs[channelID]; !ok {
		return nil, fmt.Errorf("no conversation %s", channelID)
	}
	return f.pageLocked(channelID, ""), nil
}

// fakeOlderDelay makes the "loading older" row visible, as a request
// to Slack would.
const fakeOlderDelay = 400 * time.Millisecond

// Older is the page before beforeTS, oldest first; empty at the start.
func (f *Fake) Older(ctx context.Context, channelID, beforeTS string) ([]Message, error) {
	select {
	case <-time.After(fakeOlderDelay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pageLocked(channelID, beforeTS), nil
}

// pageLocked is the newest fakePage channel messages before beforeTS
// ("" for the newest of all), leaving out replies only in a thread.
// The fake's ts all have the same width, so they order as strings.
func (f *Fake) pageLocked(channelID, beforeTS string) []Message {
	var out []Message
	for _, m := range f.logs[channelID] {
		if !m.InThreadOnly && (beforeTS == "" || m.TS < beforeTS) {
			out = append(out, m)
		}
	}
	if len(out) > fakePage {
		out = out[len(out)-fakePage:]
	}
	return cloneMessages(out)
}

// cloneMessages copies msgs deep enough that the fake changing its own
// reactions later does not reach a window holding the copy.
func cloneMessages(msgs []Message) []Message {
	out := slices.Clone(msgs)
	for i := range out {
		rs := slices.Clone(out[i].Reactions)
		for j := range rs {
			rs[j].Users = slices.Clone(rs[j].Users)
		}
		out[i].Reactions = rs
	}
	return out
}

// Thread is the parent and its replies, oldest first.
func (f *Fake) Thread(_ context.Context, channelID, threadTS string) ([]Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	log, ok := f.logs[channelID]
	if !ok {
		return nil, fmt.Errorf("no conversation %s", channelID)
	}
	var out []Message
	for _, m := range log {
		if m.TS == threadTS || m.ThreadTS == threadTS {
			out = append(out, m)
		}
	}
	return cloneMessages(out), nil
}

// SendReply posts as You in the thread.
func (f *Fake) SendReply(ctx context.Context, channelID, threadTS, text string, broadcast bool) error {
	f.mu.Lock()
	m := f.replyLocked(channelID, threadTS, "You", text, broadcast)
	f.mu.Unlock()
	select {
	case f.events <- MessagePosted{Message: m}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// MarkThreadRead: the fake keeps no thread read state.
func (f *Fake) MarkThreadRead(context.Context, string, string, string) error { return nil }

// fakeCommands are the slash commands the fake offers.
var fakeCommands = []Command{
	{Name: "/me", Usage: "your message", Desc: "Displays action text"},
	{Name: "/msg", Usage: "@user [your message]", Desc: "Send a message to someone"},
	{Name: "/open", Usage: "[channel]", Desc: "Open a channel"},
	{Name: "/remind", Usage: "[@someone or #channel] [what] [when]", Desc: "Set a reminder"},
	{Name: "/shrug", Usage: "your text", Desc: "Appends ¯\\_(ツ)_/¯ to your message"},
	{Name: "/standup", Usage: "[notes]", Desc: "Post your standup notes", App: "Standup Bot"},
}

// People is the fake's people, the authors of its messages.
func (f *Fake) People(context.Context) ([]Person, error) {
	return []Person{
		{ID: "U1", Name: "Ada Lovelace", RealName: "Augusta Ada King", Handle: "ada.lovelace"},
		{ID: "U2", Name: "Grace Hopper", Handle: "grace.hopper"},
		{ID: "U3", Name: "Margaret Hamilton", Handle: "margaret.hamilton"},
		{ID: "U4", Name: "Linus Torvalds", Handle: "linus.torvalds"},
		{ID: "B1", Name: "Standup Bot", Handle: "standup", Bot: true},
	}, nil
}

func (f *Fake) Commands(context.Context) ([]Command, error) {
	return append([]Command(nil), fakeCommands...), nil
}

// RunCommand acts out a few commands: /shrug and /me post as You,
// /remind and /standup answer with an ephemeral reply, anything else is
// refused as Slack would.
func (f *Fake) RunCommand(ctx context.Context, channelID, threadTS, name, args string) error {
	post := func(text string) Message {
		if threadTS != "" {
			return f.replyLocked(channelID, threadTS, "You", text, false)
		}
		return f.appendLocked(channelID, "You", text)
	}
	var ev Event
	f.mu.Lock()
	switch name {
	case "/shrug":
		ev = MessagePosted{Message: post(strings.TrimSpace(args + ` ¯\_(ツ)_/¯`))}
	case "/me":
		m := post(args)
		rich.Italicize(m.Body) // as Slack's me_message
		ev = MessagePosted{Message: m}
	case "/remind", "/standup":
		m := f.nextLocked(channelID, "Slackbot", "Got it! I'll remind you: *"+args+"*")
		if name == "/standup" {
			m = f.nextLocked(channelID, "Standup Bot", "Thanks, your notes are saved: "+args)
		}
		m.Ephemeral = true
		m.ThreadTS, m.InThreadOnly = threadTS, threadTS != ""
		ev = MessagePosted{Message: m}
	default:
		f.mu.Unlock()
		return &core.CommandError{Code: "unknown_command"}
	}
	f.mu.Unlock()
	select {
	case f.events <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *Fake) Send(ctx context.Context, channelID, text string) error {
	f.mu.Lock()
	m := f.appendLocked(channelID, "You", text)
	f.mu.Unlock()
	select {
	case f.events <- MessagePosted{Message: m}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *Fake) Run(ctx context.Context, emit func(Event)) error {
	emit(ConnectionChanged{Connected: true})
	emit(PresenceChanged{UserID: "U1", Active: true})
	emit(PresenceChanged{UserID: "U3", Active: false})
	var tick <-chan time.Time
	if f.Interval > 0 {
		t := time.NewTicker(f.Interval)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-f.events:
			emit(ev)
		case <-tick:
			c := fakeConversations[rand.IntN(len(fakeConversations))]
			f.mu.Lock()
			m := f.appendLocked(c.ID, fakeAuthors[rand.IntN(len(fakeAuthors))], fakeLines[rand.IntN(len(fakeLines))])
			f.mu.Unlock()
			emit(MessagePosted{Message: m})
		}
	}
}

// Search scans the fake's history for the query's words, newest first.
func (f *Fake) Search(_ context.Context, query string, limit int, order SearchOrder) ([]SearchHit, error) {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var hits []SearchHit
	score := map[string]int{} // by ts: how often the terms appear, for Relevance
	for _, id := range f.order {
		for _, m := range f.logs[id] {
			text := strings.ToLower(rich.PlainBlocks(m.Body))
			n := 0
			for _, t := range terms {
				c := strings.Count(text, t)
				if c == 0 {
					n = 0
					break
				}
				n += c
			}
			if n > 0 {
				hits = append(hits, SearchHit{Message: cloneMessages([]Message{m})[0]})
				score[m.ChannelID+m.TS] = n
			}
		}
	}
	slices.SortFunc(hits, func(a, b SearchHit) int {
		if order == Relevance {
			if d := score[b.Message.ChannelID+b.Message.TS] - score[a.Message.ChannelID+a.Message.TS]; d != 0 {
				return d
			}
		}
		return b.Message.Time.Compare(a.Message.Time)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// SyncStatus: the fake holds every conversation whole.
func (f *Fake) SyncStatus(context.Context) (SyncStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := SyncStatus{Backfill: f.bf.status}
	if f.bf.current != "" {
		st.Active = []string{"backfilling " + f.bf.current}
	}
	for _, id := range f.order {
		c := f.convs[id]
		cov := Coverage{ID: id, Name: c.Name, Kind: c.Kind, Messages: len(f.logs[id]), Complete: true, Live: true}
		if n := len(f.logs[id]); n > 0 {
			cov.Oldest = f.logs[id][0].Time
			cov.Spans = []TimeSpan{{Oldest: cov.Oldest, Latest: f.logs[id][n-1].Time}}
		}
		// Two idle conversations show the other states a real cache has:
		// one only partly back toward the horizon, one with a gap.
		now := time.Now()
		day := 24 * time.Hour
		switch id {
		case "C12":
			cov.Complete = false
			cov.Oldest = now.Add(-30 * day)
			cov.Spans = []TimeSpan{{Oldest: cov.Oldest, Latest: now}}
		case "C16":
			cov.Complete, cov.Holes = false, 1
			cov.Oldest = now.Add(-120 * day)
			cov.Spans = []TimeSpan{{Oldest: cov.Oldest, Latest: now.Add(-50 * day)}, {Oldest: now.Add(-20 * day), Latest: now}}
		}
		st.Messages += cov.Messages
		st.Conversations = append(st.Conversations, cov)
	}
	return st, nil
}

// StartBackfill pretends: one conversation every 400 ms, then a few
// threads, then done.
func (f *Fake) StartBackfill(horizon time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bf.cancel != nil {
		return errors.New("a backfill is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.bf.cancel = cancel
	f.bf.status = BackfillStatus{State: BackfillRunning, Horizon: horizon, Phase: "conversations", Total: len(f.order), StartedAt: time.Now()}
	order := slices.Clone(f.order)
	go func() {
		step := func(update func()) bool {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(400 * time.Millisecond):
			}
			f.mu.Lock()
			update()
			f.mu.Unlock()
			select {
			case f.events <- SyncChanged{}:
			default:
			}
			return true
		}
		for i, id := range order {
			if !step(func() {
				f.bf.current = f.convs[id].Name
				f.bf.status.Current = f.convs[id].Name
				f.bf.status.Done = i
				f.bf.status.Requests++
				f.bf.status.Fetched += 28
			}) {
				break
			}
		}
		if ctx.Err() == nil {
			step(func() { f.bf.status.Phase, f.bf.status.Done, f.bf.status.Total = "threads", 0, 3 })
			for i := 0; i < 3 && ctx.Err() == nil; i++ {
				step(func() { f.bf.status.Current = "general · thread"; f.bf.status.Done = i + 1; f.bf.status.Requests++ })
			}
		}
		f.mu.Lock()
		f.bf.cancel = nil
		f.bf.current, f.bf.status.Current = "", ""
		if ctx.Err() != nil {
			f.bf.status.State = BackfillOff
		} else {
			f.bf.status.State = BackfillDone
		}
		f.mu.Unlock()
		select {
		case f.events <- SyncChanged{}:
		default:
		}
	}()
	return nil
}

// StopBackfill ends the pretend run.
func (f *Fake) StopBackfill() {
	f.mu.Lock()
	cancel := f.bf.cancel
	f.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
