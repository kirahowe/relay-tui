# relay-tui

relay-tui is a terminal chat client for agents that speak the
[Agent Client Protocol](https://agentclientprotocol.com). It starts the agent
as a child process, streams its replies into the terminal's own scrollback,
and keeps a status line that shows how long the agent has been working and
how long it has been silent.

## Using it

```sh
make build
bin/relay-tui -- agentloop acp request.json
```

With relay's hub, `--open ROLE` focuses that role at the start (see
"Several conversations").

Everything after `--` is the agent's command. It runs in the current
directory, which is also the session's working directory.

When the session starts, relay-tui prints a banner with the agent's name and
version, and the model if the agent names one in its session options. The
banner's title is `--title` if given, otherwise the title the agent reports
about itself, otherwise its name. The input sits in a box at the bottom,
with a status line under it showing what the agent is doing, for how long,
and how much of its context window is in use and what the session has
cost when the agent reports those. A message you send stays in its box,
so your turns stand apart from the agent's. Tool calls made one after
another share one line that names the tools, and the agent's thinking
shows as one faint line, `∴ thought`.

Enter sends. Ctrl-J starts a new line in any terminal; Shift-Enter,
Ctrl-Enter and Alt-Enter do too where the terminal reports them. Pressing
Enter while the agent is working queues the text, and everything queued
goes out as one message when the turn ends. Ctrl-C cancels the turn and
puts any queued text back in the input. With nothing running and an empty
input, Ctrl-C or Ctrl-D quits. `?` on an empty input, or Ctrl-/, lists
the keys; any key closes the list.

Ctrl-T and Ctrl-O open the transcript: the whole session on the
terminal's alternate screen, with tool calls (Ctrl-T) or thinking
(Ctrl-O) in full. A call in full shows its arguments and the first lines
of its output. In the transcript, Ctrl-T or `t` shows or hides tool calls
and Ctrl-O or `o` thinking. Up and down, `j` and `k`, the page keys, `g`
and `G`, and the mouse wheel scroll it, and Esc or `q` goes back. It
works while the agent is busy too. The chat prints into the terminal's
own scrollback, which cannot be redrawn, so the transcript is where
blocks open and close. Nothing goes into scrollback while it is open:
what the agent writes meanwhile is printed when you leave. A permission
request closes the transcript, so the request is always on screen, and
the session's labels lead its footer as they lead the status line.

The agent's stderr would garble the screen, so relay-tui keeps the last
64 KiB and prints them if the agent exits on its own. `--agent-log FILE`
writes all of it to a file. relay-tui's own notes about the connection,
such as a line it dropped as unreadable, go to the same place.

When the agent asks permission for a tool call, the request replaces the
input box. It shows the tool's name and its input, indented and cut to ten
lines, and the options the agent offered. Keys: `y` picks the first
allow_once option, `n` the first reject_once, up/down and enter pick between
them (the highlight starts on a reject, so a bare enter refuses). Options
of any other kind, such as allow_always, are not offered: relay-tui never
grants standing permission. `v`
puts the whole input into the transcript, and Esc or Ctrl-C answers
cancelled. Keys that would approve do nothing for the first 600 ms, so a
`y` typed for the input cannot allow a call that appeared mid-word. Other
keys are ignored while a request waits, so no prompt can go out. Requests
queue and are answered in order; ending the turn cancels the rest.

Nothing the agent wrote in a request is dropped. Control characters, bidi
and other format characters, and any other code point that does not print
show as `\uXXXX` escapes, so the input reads back as JSON to exactly what
will run. Numbers show as the agent wrote them: relay-tui reads the input
from the request's own params, not from the ACP SDK's decoding, which
would round `9007199254740993` to `9007199254740992`. Wrapping adds
line breaks; nothing is cut except in the collapsed box, where `v`
shows the rest.

If the agent reports session labels in `_meta["agentloop/labels"]` on tool
calls, usage updates and session_info_update, the status line shows them
as pills: ` private ` and ` contaminated `, each in its own colour on a
tint. Where the terminal has not said what its colours are, they show as
`[private]` in bold and ` contaminated ` in reverse video. Labels only
accumulate during a session.

## The look

Colours come from the terminal's own palette, so relay-tui follows a light
theme as well as a dark one. At startup relay-tui asks the terminal for its
background and text colours and blends its tints from them: the bar
behind a selection, the block behind your own messages, a key chip, a
label pill. A terminal that does not answer gets reverse video for a
selection and no tints. Your messages are then drawn in boxes.

While the session is contaminated, URLs in what the agent writes (replies,
notices, tool names, arguments and output) show defanged, as
`hxxps://host[.]tld/path`, so a click cannot carry data out. relay-tui never
writes OSC 8 hyperlinks. The request box shows its input exactly, but
with its URLs defanged once the session is contaminated: a click on one,
to look at it, would send what it carries before you approve anything.
A paste while a request waits is dropped. relay-tui never sends
`resource_link` blocks.

## The workspace pane

Agents that run under relay put work in front of you as they go: what a
tool found, the calls waiting for your approval, and the documents the
latest reply cites. The workspace pane lists it, in its items view; its
other view is the board (see below), which it shows by default. A new
display or proposal brings the items into view, and a card that changes
brings the board back. On a screen 120 columns or wider the pane stands
to the right of the chat. On a narrower one, Ctrl-X opens it over the
chat and the status line names the view and its count, as in
`ctrl+x workspace (3)`.

Newest items come first. A shown item has a title, its labels and its
sources. A proposal is a call held for approval, with relay's preview of
what it will do and its status: waiting, allowed, rejected or cancelled.
Below the items, each reference in the latest reply, written `[[fact:12]]`
or `[[doc:notes/venues.md]]`, shows the title relay gives it. A reference
relay cannot find shows as `missing` in reverse video. A reference not yet
checked, or checked against an agent that cannot answer, shows as
`unchecked`. The pane never shows words from the model as a title: the
model's text is only searched for references.

Text from a contaminated item sits under an `untrusted` mark in reverse
video, with its URLs defanged. A preview's URLs are always defanged,
since a preview is what a service said. relay labels a preview's text
apart from the session (`relay/labels` on the preview block), and a
request or proposal is shown by both sets of labels. Arguments the
operator fixed in relay's settings are listed under "set by your
settings". A link to a domain that no approved or trusted item has shown
so far in this run carries `[new domain]`.

Keys, once Ctrl-X has given the pane focus:

- Up and down, or k and j, select an item. Enter opens it; up and down,
  Page Up and Page Down scroll.
- t starts a trust of the selected item.
- b switches between the board and the items.
- Esc goes back from an item, then returns to the input. Ctrl-X does the
  same from anywhere.

While the pane has focus no key reaches the input box. Ctrl-C still stops
a running turn. A permission request takes the keys back from the pane,
and you answer it as before: the pane only highlights the waiting
proposal and shows its preview, here and in the request box. It adds no
second way to approve.

Trusting needs relay chat, which lets relay-tui ask relay to do it. Press t
on a contaminated shown item. The pane shows the text that would be
trusted, with its path, version and hash, and says "Trust this? It clears
contaminated from what it says now." After a short delay, y confirms;
any other key cancels. relay-tui then sends relay the version and hash it
listed, so a document that changed in between is refused. Where relay does
not take trust from this client, the pane shows the `relay trust` command
to run in a terminal and does nothing else. Against an agent without relay
the pane lists the displays and requests relay-tui saw itself, and offers no
trust.

## The board

relay keeps cards: small structured things you work through over days,
such as a checklist or a tracker of accounts by month. Roles make them.
The pane shows the board unless something else has come up; b in the
focused pane switches to it. The board belongs to the connection, not to
a conversation: relay-tui asks relay for it with `_relay/cards` once the
connection is up (against relay's hub, right after `initialize`; against
a single agent, once its session exists), so the board is there before
any conversation opens, and the pane's header can count the cards from
either view ("board (2) · workspace (0)"). After that relay sends
`_relay/cards/changed` whenever a card changes, here, in a terminal or by
a role, and the board redraws from what relay sent. An archived card
leaves the board. Against an agent without relay the board says "This
agent has no board."

When a card is added or changes, the pane switches back to the board
and selects that card, so a card a role mentions is in view. It stays on
the workspace items while the pane has focus or a proposal is waiting
for your answer. An agent without a board leaves the pane on the items.

Each card is a panel of its own, with a blank row between cards. The
title sits in the top border in the colour of its kind, and how far along
the card is sits at the right. The selected card's frame is in the accent
colour while the pane has the keys. Enter opens a card, and the selected
line is then a tinted bar. A checklist shows a green ✓ and struck-through
text for a done item and ○ for an open one. The list shows the first six
items. A tracker is a grid: the column names across the top, then one row
per line, with ● for done, × for skipped and ○ for open. A note keeps the
line breaks it was written with. A `#` heading and `**bold**` are bold,
and a list line's marker is faint. A card of a kind relay-tui does not know
shows the text form relay sends with every card.

Keys on the board:

- Up and down, or k and j, select a card, or, in an open card, its title
  and then each item or row. Left and right, or h and l, select a
  tracker's column. When a change arrives, the cursor stays on the item,
  row or column it was on. If that one is gone, the cursor stays where it
  was and a note says so.
- x or space ticks or unticks the selected item. On a tracker it moves
  the selected cell from open to done to skipped and back to open.
- In an open card, a adds an item to a checklist or a row to a tracker,
  and A adds a column.
- e edits the selected line's text: the title, an item or a row's name.
  In the list it retitles the selected card. In an open tracker, E
  renames the selected column. A note's text cannot be edited here.
- X archives the selected card, after you confirm with y. relay's CLI can
  restore it.
- t trusts the selected card when it holds outside text. The pane shows
  the whole card and asks "Trust …? It clears outside text from what it
  says now." After a short delay, y confirms, and any other key cancels.
  relay-tui sends relay the card's rev and the digest relay sent with it
  (`_relay/cards/trust`), so a card that changed in the meantime is not
  trusted. If the card changes while you are being asked, the question
  closes, as it does when you leave it: ctrl+x, another role, the list of
  roles or the list of keys. relay takes trust only from `relay chat`, which runs with
  approvals. Elsewhere the pane names the `relay trust card <id>` command
  to run in a terminal.
- r asks relay for the board again; b goes back to the workspace items. An agent with
  no board is not asked again.

Adding and editing use a one-line input at the foot of the pane: Enter
sends, Esc cancels. An edit starts from the current text, unless that text
came from outside. What you send counts as your own text, so relay-tui
does not offer outside words for you to send back unchanged. Every edit carries the
revision you were looking at when you opened it, so relay refuses it if
someone changed that text in the meantime. A paste into the input goes
nowhere, as anywhere else in the pane.

relay-tui never changes a card itself. Each key sends relay an op with
`_relay/cards/op` and waits for the card relay sends back. Until then the
card is marked `sending…`, and a key for another op is dropped with a
note saying so. When relay refuses an op, the pane shows "Refused." with
relay's reason, until relay takes your next op or you close the input.
If the op carried text, the input opens again with what you typed.
After a refused edit it carries the card's current revision, so Enter
sends your text over the newer change; Esc drops it. Leaving
the pane with ctrl+x keeps an open input as it was.

Card text is shown as relay sent it. relay has already made hidden
characters visible and defanged links in text from outside, which it
marks as such. relay-tui shows that text in the warning colour, and the
bottom border of a card that holds any says `outside text`. Links are
never clickable, and relay-tui fetches nothing.

## Several conversations (relay's hub)

When the agent is relay's hub, relay-tui shows every role at once. It knows
this from `_meta["relay/hub"]` in the answer to `initialize`. Against any
other agent nothing here applies, and the client works as described
above. `relay chat [role]` runs it as
`relay-tui [--open ROLE] -- relay acp --approvals`.

The whole screen is drawn on the alternate screen, from state. On a
screen 73 columns or wider it is drawn as panels, each in a frame with
its title in the top border: the roles, then the conversation in focus,
then, with room, the workspace pane. The panel that has the keys has its
frame in the accent colour, and the role in focus sits on a tinted bar.
A top row names relay, the role in focus and what the other roles want.
A bottom row lists the keys for whatever has them, each key in the
accent colour. On a narrower one you see either
the list of roles or the conversation, as on a phone: the conversation's
first row says `ctrl+g roles`. Without `--open` relay-tui starts on the
list.

If relay had to drop the oldest history from a replay, the conversation
starts with "relay: earlier history is not shown".

Each role has at most one live conversation. A role with one is
attached with its history: relay replays what was said, including your
own prompts (shown in their boxes), and prompts sent from another device
arrive as they happen. A role with none shows "No conversation"; typing
a message, or pressing enter on an empty input, starts one. Moving
between roles never starts a conversation. Closing relay-tui does not end
a conversation when relay runs as a service. Every role keeps its own
unsent text while you are elsewhere.

What the sidebar shows beside a role (the `?` list explains them too):

- `▶` a turn is running, `…` a prompt waits for the role's scheduled work
  to finish (the status line says "waiting for the role's scheduled
  work"), `~` the role's scheduled work is running
- `●` the latest reply is unread
- `?2` two permission requests wait, in reverse video; `!1` one of the
  role's calls is held for you

A request is bounded: its title takes three rows and each option two, and
`v` puts all of it in the conversation. When the window cannot show the
role's name, the request, its options and the status line with three
rows of conversation left to scroll through, the view says "Window too
small" and the keys that allow do nothing (esc still cancels).

A request from a conversation in the background shows as `?N` beside its
role and is answered after switching to it. Switching starts the 600 ms
delay again, so a key typed for the switch cannot approve it. When
another device answers first, the dialog closes and the transcript notes
"answered elsewhere".

Keys, besides the usual ones:

- ctrl+n and ctrl+p, the next and previous role
- alt+1 to alt+9, that role
- ctrl+l, the role you were in before
- ctrl+g, the list of roles: up and down move, enter opens, esc goes
  back, and ctrl+x gives the board the keys
- ctrl+s, end this conversation. The view asks first; y ends it, n
  keeps it. relay cancels a running turn before it ends.
- page up and page down, or the mouse wheel, scroll; ctrl+t and ctrl+o
  show tool calls and thinking in full, as in the transcript
- ctrl+c cancels a running turn, including one another device started,
  and otherwise clears the input or quits. Quitting only detaches: it
  answers no permission request, in any conversation (relay holds an
  unanswered one and tells you), and the conversations keep running.
  When relay runs as a service, relay-tui says so as it exits. When it does
  not (relay says so in `initialize`), the view says "relay service not
  running: conversations end when you close this" all the time.

The board is the same whichever role is in focus, with the same card
selected, and it shows with no conversation open: beside the roles on a
wide screen, or, once ctrl+x gives the pane the keys, in the
conversation's place. Its ops and trust work from a role that has no
conversation, and open none. With no role in focus the list has the
keys, and ctrl+x there gives them to the board; esc or ctrl+x on the
board gives them back to the list, with the cursor where it was. On a
narrow screen, where the list is the screen, the board takes its place
until then. The workspace items in the pane are the
focused conversation's own. Moving to another role gives the keys back to
its conversation. Only the conversation on screen can bring its items
into view; a display in another one waits there until you look.

relay-tui tells relay the latest turn has been seen (`_relay/seen`) once
the conversation is attached, on screen, scrolled to its end and idle,
so the unread mark clears on every device.

## Pasting marks the conversation untrusted

A pasted email or web page is text an outsider may have written, and
pasting it is the usual way that text reaches the agent. So relay-tui does
not type a paste into the prompt. A bracketed paste is held, and the input
shows a placeholder such as `[pasted 42 lines · untrusted #1]`, even for a
single line: a short paste can carry an injected instruction as well as a
long one. When you send, the prompt is the text you typed, with each
placeholder replaced by a reference like `[paste:1]`, plus one embedded
`resource` block per paste (text/plain, uri `paste:1`). agentloop labels a
session `contaminated` when it receives a resource block, which is what
makes egress need your approval from then on. Delete a placeholder from
the input and that paste is not sent. A terminal that does not do
bracketed paste types the text in as keys, and relay-tui cannot tell.

A paste is always untrusted when it arrives. To send one block as trusted
text, paste it and then press Ctrl-Y: it marks the latest paste still in
the input, and its placeholder changes to `[pasted 42 lines · trusted #1]`.
At send it goes into the prompt as plain text, and the transcript notes
that it was not labelled. Press Ctrl-Y again to trust the one before it.
With no untrusted paste in the input, Ctrl-Y does nothing and the status
line says so. Nothing is armed ahead of a paste, so no paste can inherit
a trust meant for another. If a placeholder still reads `untrusted` when
you send, it goes as an attachment whatever was marked. A paste made while
a permission request waits, the pane has focus or the transcript is open
goes nowhere.

If the agent does not advertise `promptCapabilities.embeddedContext`, a
paste cannot be labelled. It goes in the prompt as plain text and the
transcript warns that it was not labelled untrusted.

`go test` also drives a real agentloop binary against a fake Messages API on
localhost, when one exists at `../agentloop/bin/agentloop` (or the path in
`-agentloop`); without it those tests skip.

## How it is built

The state of a chat, and every rule for changing it, lives in
`internal/core`. `core.Reduce` takes the state and one event (a chunk of
reply, a tool call, a key press, a clock tick) and returns the new state
and a list of effects, such as "send this prompt" or "print these entries".
It reads no clock and does no IO, so property tests can check it over long
random sessions. `internal/wire` turns ACP messages into those events, and
`internal/render` turns the state into terminal lines. The renderer uses
the terminal's own colours and tints blended from them (see The look).
The terminal's colours are its one input that is not part of the state.

The client's state comes in two parts. `core.App` is one per
connection: relay's board, which view the pane shows and whether it has
the keys. `core.State` is one per conversation: its transcript, input,
pastes, permission requests and its own pane items. Against a single
agent `core.Single` holds the app and the one conversation, and
`core.ReduceSingle` applies an event to both. The pane follows the same
split. `core` holds its state, the trust rules and the board's cards, `wire` reads relay's `_meta`, its
`_relay/items`, `_relay/resolve`, `_relay/trust`, `_relay/cards` and
`_relay/cards/op` answers and its `_relay/cards/changed` notifications,
and `render` draws it all. The `agent` package makes the calls one at a
time and in order.

`internal/agent` runs the agent process, and `internal/ui` is the Bubble
Tea program. Both are thin code around the core that carries out its
effects.

Hub mode adds one level above the conversation. `core.Hub` holds the
app, relay's roster as it last sent it, one `State` per conversation (marked `Shared`, so it
takes updates in any phase, since a replay has no end marker and another
device may be driving the turn) and which role is in focus.
`core.ReduceHub` reduces events for one conversation together with the
app and wraps the effects with the session they belong to. Every event
from the agent about a conversation carries its session id
(`core.Routed`); the board's answers and notifications carry none and go
to the app. `render.HubView` draws the whole
screen from the hub alone, and the roster is replaced whole each time
relay sends it, so applying one twice changes nothing.

## Reading JSON

relay-tui reads everything the agent sends, and every recording another
process wrote, by the one dialect that relay and agentloop also use
(relay#58). `internal/strictjson` implements it with
`encoding/json/v2`. A message is one JSON object in valid UTF-8, with
nothing but whitespace after it. A name may not appear twice in an
object at any depth, and names match exactly, case included. Nesting
stops at 128 levels, a number literal at 1000 characters, and an
exponent at 9999 either way (by value, so `1e+0009999` passes). Unknown
members are ignored. `testdata/strict-json/cases.jsonl` is a copy of the
table of hostile lines shared by the three repos (the source is relay's
`testdata/strict-json`), and the tests run every case through relay-tui's
reader.

Each line from the agent goes through `wire.Screen` before the ACP SDK
sees it. A line that breaks the dialect is dropped. The SDK never sees
it, and relay-tui notes it, with a count, where it keeps the agent's stderr,
not on the screen. The SDK decodes with `encoding/json` v1, which
matches names without regard to case, so the screen also removes any
member that v1 would read under a different name, such as `"Method"`
where the message has `method`. Names that differ only in case and name
no field the SDK reads are unknown to v1 as to the dialect, and pass as
written. If v1 would put one of them in
a field and which one it took cannot be seen, the line is dropped. The
screen does this first for the SDK's own
envelope, the step where the SDK reads `method` and `id`, so the member
goes even when the params that follow cannot be decoded. A line the SDK
cannot read as an envelope at all, such as one whose `method` is not a
string, is dropped. After that the SDK reads each line the way relay and
agentloop read it. Lines that a JSON encoder wrote pass
through byte for byte. Answers to relay's extension methods and its
`_meta` values are read by the strict reader directly.

## Building

Go 1.27 or later.

```sh
make build    # bin/relay-tui
make check    # gofmt, vet, and every test with the race detector
make update   # move every dependency to its latest version, then check
```

The property tests run 100 cases each by default. For a longer run:

```sh
go test ./... -rapid.checks=5000
```

`cmd/fakehub` stands in for relay's hub in the tests: the roster, attach
with replay, several sessions, a permission request another device can
settle, and a turn queued behind scheduled work. A control file plays the
second device; see `internal/fakehub` for its commands.

`testdata/agentloop` holds ACP streams recorded from agentloop. The tests
replay them; nothing imports agentloop. The end-to-end tests run relay-tui
against `cmd/fakeagent`, which plays the scripts in `testdata/scripts`,
once in process on an emulated terminal and once as a built binary on a
real pseudo-terminal.

## License

MIT
