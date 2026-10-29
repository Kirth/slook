# Threads in slk-gio

## Problem

The Gio frontend showed a thread only as "N replies" under its parent. You
could not read the replies, answer them, or follow a search hit on a reply
past its parent.

## What was already there

- The cache stores replies. `cache.DB.GetThreadReplies` reads them.
- `msgsync.Syncer.Thread` fetches a whole thread with one
  `conversations.replies` call. It caches the thread, prunes replies Slack
  no longer has, and upserts the parent with its current `reply_count`.
- `slackclient.Client.SendReply` posts with `thread_ts` and can set
  `reply_broadcast`. `Client.MarkThread` marks a thread read.
  `CommandRequest.ThreadTS` runs a slash command in a thread.

## Design

**A thread gets a window of its own.** That fits the GUI's model of one
window per conversation, and lets the compositor place and tile it. A side
panel would crowd a 600dp chat window.

### Backend

Three methods are added to `backend.Backend`, and `RunCommand` gains a
thread ts.

- **`Thread(ctx, channelID, threadTS)`** returns the parent and replies,
  oldest first. It has the same contract as `History`: answer from the
  cache, then fetch the thread in the background. The fetch's changes
  arrive through `Run` as a `HistoryUpdated`. A thread the cache holds
  nothing of is fetched before returning. Each open costs one replies
  call and never starts a sweep.
- **`SendReply(ctx, channelID, threadTS, text, broadcast)`** queues the
  posted reply for `Run` right away, as `Send` does.
- **`MarkThreadRead(ctx, channelID, threadTS, ts)`** marks the thread read.

The `HistoryUpdated` from a thread fetch carries the parent with Slack's
current reply count, and sets `ThreadTS`. The channel window applies it
with `Timeline.Refresh`, so the channel's count corrects itself.
`Refresh` only updates rows the window already shows. Without that, an
old parent fetched through a thread would be inserted far above the
loaded history.

### State

**`chat.Thread`** is one thread's messages: the parent, its replies,
broadcasts and ephemeral replies. It shares a sorted message list,
`sortedMsgs`, with `chat.Timeline`.

**Each timeline and thread counts a reply ts once.** Without that, the
optimistic copy of a sent reply and the socket's echo of it, which share a
ts, would count twice on the parent.

### Windows

The chat window's message rendering moved into `msgView`. Its composer,
with the slash-command popup, moved into `composeBox`. The chat window and
the thread window embed both.

A thread opens three ways:

- by clicking the "N replies" caption under a message
- by the "Reply" action that appears when the pointer is over a message
- by the "replied to a thread" caption on a broadcast reply

**The thread window** shows the parent, a divider with the reply count,
then the replies. Its composer has an "Also send to #channel" checkbox,
which resets after each send. While the window has the keyboard, it marks
the thread read.

**The hub routes events to thread windows** by channel and parent ts. The
rule is a pure function, `threadTargets`, so it is tested without windows.

### Search

A search hit on a reply that lives only in its thread opens two windows.
The channel scrolls to the parent, and the thread window opens on top,
scrolled to the reply and lit.

## Out of scope

- Slack's Threads view (every thread the user follows)
- Thread subscriptions
- Unread state per thread in the conversation list
