package backend

import "context"

// Forward posts m's permalink to toChannelID, after comment when there
// is one (slackclient.ForwardMessage), and queues the posted message
// for Run, as Send does: the socket's echo replaces it by ts and Slack's
// preview of the permalink arrives with it.
func (s *Slack) Forward(ctx context.Context, m Message, toChannelID, comment string) error {
	ts, permalink, err := s.client.ForwardMessage(ctx, m.ChannelID, m.TS, toChannelID, comment)
	if err != nil {
		return err
	}
	text := "<" + permalink + ">"
	if comment != "" {
		text = comment + "\n" + text
	}
	select {
	case s.queue <- rawMessage{channelID: toChannelID, userID: s.client.UserID(), ts: ts, text: text}:
	case <-ctx.Done():
	}
	return nil
}
