package chat

import (
	"strconv"
	"strings"
	"time"

	"github.com/gammons/slk/gui/backend"
)

// ChipIcon is the small icon before a header chip's text.
type ChipIcon int

const (
	ChipNoIcon ChipIcon = iota
	ChipPeople          // member or people count
	ChipClock           // someone's local time, during their day
	ChipNight           // someone's local time, at night for them
	ChipMuted           // the conversation is muted
	ChipDND             // they have do-not-disturb on
)

// Tone is how a chip is coloured: quietly, or as something to notice.
type Tone int

const (
	ToneMuted Tone = iota
	ToneWarn       // e.g. it is night where they are
)

// Chip is one item of a conversation header's second line. Grow marks
// the one that takes the spare width and is cut short when there is
// none (a topic, a status).
type Chip struct {
	Icon ChipIcon
	Text string
	Tone Tone
	Grow bool
}

// Night hours: a local time outside [dayStart, dayEnd) is night.
const (
	dayStart = 8
	dayEnd   = 21
)

// Chips is the second line of a conversation's header:
//   - a channel: its topic (else purpose), its member count, and
//     whether it is muted;
//   - a direct message: the person's status text and title, and their
//     local time, flagged when it is night for them (their status emoji
//     goes beside the name instead);
//   - a group DM: how many people are in it.
//
// details and profile are nil until they load; what is missing is left
// out.
func Chips(c backend.Conversation, details *backend.Details, profile *backend.Profile, now time.Time) []Chip {
	var out []Chip
	text := func(s string, grow bool) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, Chip{Text: s, Grow: grow})
		}
	}
	switch c.Kind {
	case backend.KindDM:
		if profile == nil {
			break
		}
		status := StatusText(*profile, now)
		if profile.DND && (profile.DNDUntil.IsZero() || now.Before(profile.DNDUntil)) {
			out = append(out, Chip{Icon: ChipDND, Text: DNDText(profile.DNDUntil, now), Tone: ToneWarn})
		}
		text(status, true)
		text(profile.Title, status == "")
		if profile.HasTZ {
			local := now.UTC().Add(time.Duration(profile.TZOffset) * time.Second)
			chip := Chip{Icon: ChipClock, Text: local.Format("15:04")}
			if h := local.Hour(); h < dayStart || h >= dayEnd {
				chip.Icon, chip.Tone = ChipNight, ToneWarn
			}
			out = append(out, chip)
		}
	case backend.KindGroupDM:
		if c.Name != "" {
			n := len(strings.Split(c.Name, ",")) + 1 // the others and you
			out = append(out, Chip{Icon: ChipPeople, Text: strconv.Itoa(n)})
		}
	default:
		if details != nil {
			about := details.Topic
			if about == "" {
				about = details.Purpose
			}
			text(about, true)
			if details.Members > 0 {
				out = append(out, Chip{Icon: ChipPeople, Text: strconv.Itoa(details.Members)})
			}
		}
	}
	if c.Muted {
		out = append(out, Chip{Icon: ChipMuted, Text: "muted"})
	}
	return out
}

// StatusText is someone's custom status text, "" once it has expired.
func StatusText(p backend.Profile, now time.Time) string {
	if !p.StatusExpires.IsZero() && !now.Before(p.StatusExpires) {
		return ""
	}
	return p.StatusText
}

// DNDText words do-not-disturb ending at until: "do not disturb until
// 17:00", with the day when it is not today; "do not disturb" when the
// end is unknown.
func DNDText(until, now time.Time) string {
	switch {
	case until.IsZero():
		return "do not disturb"
	case SameDay(until, now):
		return "do not disturb until " + until.Format("15:04")
	}
	return "do not disturb until " + until.Format("Mon 15:04")
}
