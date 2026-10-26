# relay-tui

relay-tui is the terminal dashboard for
[relay](https://github.com/kirahowe/relay): the roles and their live
conversations and the board of cards. It never asks you to approve
anything. It speaks the
[Agent Client Protocol](https://agentclientprotocol.com), and pointed at
any other ACP agent it is a plain chat client: it starts the agent as a
child process, streams its replies into the terminal's own scrollback, and
keeps a status line that shows how long the agent has been working and how
long it has been silent.

## Using it

```sh
make build
bin/relay-tui -- agentloop acp request.json
```

With relay's hub, `--open ROLE` focuses that role at the start (see
"Several conversations"). `--state-dir DIR` says where to keep the saved
view (see "Reopening where you left off").

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
input, Ctrl-C or Ctrl-D quits. `?` on an empty input lists the keys; any
key closes the list. Esc stops typing, and the next section says what
the keys do then.

Ctrl-T and Ctrl-O open the transcript: the whole session on the
terminal's alternate screen, with tool calls (Ctrl-T) or thinking
(Ctrl-O) in full. A call in full shows its arguments and the first lines
of its output. In the transcript, Ctrl-T or `t` shows or hides tool calls
and Ctrl-O or `o` thinking. Up and down, `j` and `k`, the page keys, `g`
and `G`, and the mouse wheel scroll it, and Esc or `q` goes back. It
works while the agent is busy too. The chat prints into the terminal's
own scrollback, which cannot be redrawn, so the transcript is where
blocks open and close. Nothing goes into scrollback while it is open:
what the agent writes meanwhile is printed when you leave.

The agent's stderr would garble the screen, so relay-tui keeps the last
64 KiB and prints them if the agent exits on its own. `--agent-log FILE`
writes all of it to a file. relay-tui's own notes about the connection,
such as a line it dropped as unreadable, go to the same place.

### Permission requests

relay-tui advertises no approvals, and it asks you nothing about a tool
call. An agent that still sends `session/request_permission` (relay did,
before it dropped its holds) is answered at once with a plain `cancelled`,
without a dialog, a summary, a mark or a note on screen. The call is
refused and the turn goes on. `_relay/permission/settled` is ignored, and
so is anything else about permissions or trust that an older relay sends.
Nothing in relay-tui turns a refusal into an allow: it has no key for it.

The two questions relay-tui does ask are not approvals. They are about
its own screen: whether to end a conversation (`X`, see "Several
conversations") and whether to archive a card (`X` on the board). Each is
a line of its own where you are, and the next key answers it. A `y` is
taken only from a key you pressed, not one held while connecting, and
only a moment (0.6 s) after the question came up, so a key meant for what
was there before cannot answer it. On the end question, n and Esc keep
the conversation and other keys do nothing; on the archive question any
other key keeps the card.

## Typing and moving

relay-tui has two modes. While you are typing, what you press goes into
the input, apart from the few keys below. While you are moving, letters
move between the columns and act on what is in them, and nothing you
press is typed. A card's one-line editor (a title, an item's text) is
typing too. Only you move the keys out of typing: nothing that happens
elsewhere does. If the card you are editing is archived or changed away
meanwhile, the editor stays open with your text and says so ("card 12
left the board · enter does nothing · esc to discard"); if the role you
are typing to is removed, the input keeps your text, says so, and sends
nothing; in both, what you go on typing is still typed. The status line names the mode, TYPING or MOVING, ahead of
its other hints when the line is short; on a wide screen in hub mode
the row of keys at the bottom starts with it.

There are up to three columns: the list of roles (only with relay's
hub), the conversation in focus, and the workspace pane. The panel with
the keys has its frame in the accent colour. Opening a conversation, or
starting relay-tui on one, puts you to typing in it. Esc stops typing and
keeps what you typed; `i`, or Enter on the conversation, starts again.

Typing:

| Key | What it does |
|---|---|
| `enter` | send |
| `shift+enter`, `ctrl+j` | a new line (so do `ctrl+enter` and `alt+enter`, where the terminal reports them) |
| `esc` | stop typing, to move or to get to a waiting request; what you typed stays |
| `alt+1` to `alt+9` | that role, still typing (hub) |
| `pgup`, `pgdown` | scroll the conversation (hub) |
| `ctrl+t`, `ctrl+o` | tool calls, or thinking, in full |
| `ctrl+c` | stop the turn, clear the input, or quit |
| `ctrl+d` | quit, from an empty input |
| `?` | the keys, from an empty input |

Moving:

| Key | What it does |
|---|---|
| `h`, `l`, `left`, `right` | the column to the left or the right. A column with nothing in it is skipped, and at the edge they do nothing. In an open tracker card they move between its columns instead, until esc closes it. |
| `k`, `j`, `up`, `down` | move in the list of roles, or scroll the conversation (hub) |
| `pgup`, `pgdown`, `g`, `G`, `home`, `end` | page, top and bottom of the conversation (hub) |
| `enter` | on the list, open the role and type; on the conversation, type; in the pane, what the pane says |
| `i` | type in the conversation in focus |
| `1` to `9`, `alt+1` to `alt+9` | that role, still moving (hub) |
| `X` | end this conversation, after asking (hub) |
| `R` | what is ready for you: open the view, or close it (hub) |
| `esc` | in the pane, close what is open, then go back to the conversation; on the list, back to the conversation |
| `ctrl+t`, `ctrl+o` | tool calls, or thinking, in full |
| `ctrl+c` | stop the turn, or quit |
| `ctrl+d` | quit, from an empty input |
| `?` | the keys |

In the pane its own keys work too, as its foot lists them (see "The
workspace pane" and "The board"). While the pane asks you whether to
archive a card, the next key answers or cancels it and does nothing
else. A letter that does nothing where you are shows "i to type" in the
status line. A paste while moving goes nowhere, with the same note, and
so does a paste into a card's editor: a card's text is your own words,
so a paste, which may be anyone's, does not go into it.

A held key never answers a question: a `y`, `X` or Enter typed while
connecting types or moves, and archives or ends nothing.

## The look

Colours come from the terminal's own palette, so relay-tui follows a light
theme as well as a dark one. At startup relay-tui asks the terminal for its
background and text colours and blends its tints from them: the bar
behind a selection, the block behind your own messages, a key chip, a
label pill. A terminal that does not answer gets reverse video for a
selection and no tints. Your messages are then drawn in boxes.

Once the session has read text from outside (the agent reports it in
`_meta["agentloop/labels"]`), URLs in what the agent writes (replies,
notices, tool names, arguments and output) show defanged, as
`hxxps://host[.]tld/path`, so a click cannot carry data out. The label
itself is never shown, and nothing is held or withheld because of it.
relay-tui never writes OSC 8 hyperlinks and never sends `resource_link`
blocks.

Where a thing came from is said in plain words, where it is shown: "from
the web" for an item with a web source, "from outside" for any other text
that arrived through a tool (`core.Provenance`). The words "contaminated",
"untrusted" and "private" never appear on screen.

## The workspace pane

Agents that run under relay put work in front of you as they go: what a
tool found, and the documents the latest reply cites. The workspace pane
lists it, in its items view; its other view is the board (see below),
which it shows by default. A new display brings the items into view, and
a card that changes brings the board back. On a screen 120 columns or
wider the pane stands to the right of the chat. On a narrower one, `l`
(while moving) opens it over the chat and the status line names the view
and its count, as in `l workspace (3)`.

Newest items come first. A shown item has a title, where it came from if
that was outside (see "The look"), and its sources. Below the items, each
reference in the latest reply, written `[[fact:12]]` or
`[[doc:notes/venues.md]]`, shows the title relay gives it. A reference
relay cannot find shows as `missing` in reverse video. A reference not yet
checked, or checked against an agent that cannot answer, shows as
`unchecked`. The pane never shows words from the model as a title: the
model's text is only searched for references.

Text from outside shows with its URLs defanged.

Keys, once `l` has given the pane the keys:

- Up and down, or k and j, select an item. Enter opens it; up and down,
  Page Up and Page Down scroll.
- b switches between the board and the items.
- Esc goes back from an item, then to the conversation. `h` goes to the
  conversation from anywhere in the pane.

While the pane has focus no key reaches the input box. Ctrl-C still stops
a running turn. Against an agent without relay the pane lists the
displays relay-tui saw itself.

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
the workspace items while the pane has focus. An agent without a board
leaves the pane on the items.

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
  tracker's column in an open tracker card (elsewhere they move between
  the screen's columns). When a change arrives, the cursor stays on the item,
  row or column it was on. If that one is gone, the cursor stays where it
  was and a note says so.
- x or space ticks or unticks the selected item. On a tracker it moves
  the selected cell from open to done to skipped and back to open.
- In an open card, a adds an item to a checklist or a row to a tracker,
  and A adds a column.
- e edits the selected line's text: the title, an item or a row's name.
  In the list it retitles the selected card. In an open tracker, E
  renames the selected column. A note's text cannot be edited here.
- X asks whether to archive the selected card: "Archive “title”?" with
  `y archive · any other key keeps it` in the foot. The question is
  about the card as you saw it: if it changes meanwhile (another device
  retitles it, say), the question closes with a note. The archive also
  carries the rev you saw (`seen`). relay's CLI can restore an archived
  card.
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
sends your text over the newer change; Esc drops it. While the input is
open every key goes to it, digits and `h` and `l` included; Ctrl-C closes
it, unless a turn is running, which it stops.

Card text is shown as relay sent it. relay has already made hidden
characters visible and defanged links in text from outside, which it
marks as such. relay-tui shows that text in the warning colour, and the
bottom border of a card that holds any says `from outside`. Links are
never clickable, and relay-tui fetches nothing.

## What is ready for you

`R`, while you are moving, opens a view of what is ready for you: what
relay told you, drafts it prepared in your own accounts, entries on the
agent's calendar, findings it put on the board and files it moved,
newest first; things that came at the same moment are in the order
relay recorded them, the latest first. `R`
again, Esc, `q` or `h` closes it. It covers the whole screen, so it reads
the same on a phone-width terminal (about 60 columns) as on a wide one,
and the keys row names it everywhere as `R ready`, with how many things
are in it.

Each thing is a panel of its own. Its kind and title sit in the top
border and how long ago at the right. The bottom border says where it
came from, in words relay chose, such as "from a mail by
booker@example.org" or "from the web", in the warning colour. Things of
your own say nothing. The words "contaminated", "untrusted" and "private"
never appear, whatever relay sends.

Keys: up and down, or k and j, select; g and G go to the top and bottom;
r asks relay again; `?` lists the keys, wrapped to the width, and any key
closes the list. The connection asks for the view once it is up
(`_relay/ready`), and after that relay sends what changes
(`_relay/ready/changed`), so a draft prepared while you look appears
without a key. Against an agent that is not relay the view says it has
nothing to show.

A panel shows the first few lines of a thing. When the title is too long
for the top border, it moves into the panel, wrapped, so the age stays in
the border and two titles that start alike can be told apart. When there
is more, the last row says so. Enter, or `o`, opens the selected thing
whole on a screen of its own, framed, with the same words and nothing cut.
Up and down, `j` and `k`, the page keys, `g` and `G` scroll it, and Esc,
`q` or `h` goes back to the list. `R` closes the view from either.

To answer a message, select it, or open it, and press `a`, type, and press
Enter. The reply goes to relay as yours (`_relay/reply`), and relay wakes
the role that wrote the message. Esc cancels a reply you are writing. The
reply is a one-line input that takes no paste, as a card's editor takes
none. While it is on its way a second one waits, and a refusal gives your
words back. A message relay wrote itself, or any draft, calendar entry or
finding, has nobody to answer, and `a` says so. Once relay has the reply,
the message shows "you replied" with the start of it.

A daily note numbers the things it offers: a change a mailbox shaped, to
undo, and a suggestion, to drop or to say no to its offer of help. Open
the note and it keeps its headings ("Changes to check", "Coming up"), and
the numbers are the note's own. A marker shows the picked one; the number
keys `1` to `9` pick that number and Tab, `]` and `[` step through them.
In the list, a number key opens the selected note at that item. `v`
undoes the picked change, `n` says no to the picked suggestion's offer
and keeps the suggestion, and `d` drops it for good, each with one key
and no question, through `_relay/messages/act`, the same as `relay
revert`, `relay decline` and `relay dismiss`. The wrong key for an item
says which one is right, an item done already says so, and a done item
reads "(undone)", "(dropped)" or "(offer declined)", whichever note or
command did it. One change is on its way at a time. The keys row names
the keys that work on the picked item.

A move shows what moved where, relative to the folder you granted, one
row per file, under a summary such as "41 files in notes, into 13
folders". The panel shows the summary and the first few files, and a count
of the rest; Enter lists them all. To put it back, select it and press
`u`. relay undoes it as `relay undo` does (`_relay/undo`): each file that
is as the move left it goes back exactly, and the status line says "Put
back 3 files." A file you changed since, such as one you edited after it
moved, stays, and the view names it, relative to your folder, with why.
When nothing could go back, relay refuses and the view shows its reason.
Either reason takes up to a third of the screen, so it reads whole on a
phone. Once all of it is back, the panel reads "Moved back". One undo is
on its way at a time.

## Several conversations (relay's hub)

When the agent is relay's hub, relay-tui shows every role at once. It knows
this from `_meta["relay/hub"]` in the answer to `initialize`. Against any
other agent nothing here applies, and the client works as described
above. `relay [--open ROLE]` runs it as
`relay-tui [--open ROLE] -- relay acp`.

The whole screen is drawn on the alternate screen, from state. On a
screen 73 columns or wider it is drawn as panels, each in a frame with
its title in the top border: the roles, then the conversation in focus,
then, with room, the workspace pane. The panel that has the keys has its
frame in the accent colour, and the role in focus sits on a tinted bar.
A top row names relay, the role in focus and what the other roles want.
A bottom row lists the keys for whatever has them, each key in the
accent colour. On a narrower one you see one column at a time, as on a
phone: the list of roles, the conversation, or the pane in the
conversation's place, and `h` and `l` move between them. The
conversation's first row says `h roles`. Without `--open` relay-tui
starts on the list, moving.

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

The keys are in "Typing and moving". With the hub:

- `X`, while moving, ends this conversation. The view asks first; y ends
  it, n keeps it. relay cancels a running turn before it ends.
- The mouse wheel scrolls too; `ctrl+t` and `ctrl+o` show tool calls and
  thinking in full, as in the transcript.
- `ctrl+c` cancels a running turn, including one another device started,
  and otherwise clears the input or quits. Quitting only detaches, and
  the conversations keep running. When relay runs as a service, relay-tui says so as it exits. When it does
  not (relay says so in `initialize`), the view says "relay service not
  running: conversations end when you close this" all the time.

The board is the same whichever role is in focus, with the same card
selected, and it shows with no conversation open: beside the roles on a
wide screen, or, once `l` gives the pane the keys, in the
conversation's place. Its ops work from a role that has no
conversation, and open none. With no role in focus there is no
conversation column, so `l` on the list goes straight to the board, and
esc or `h` there goes back to the list, with the cursor where it was. On
a narrow screen, where the list is the screen, the board takes its
place until then. The workspace items in the pane are the
focused conversation's own. Moving to another role gives the keys back to
its conversation. Only the conversation on screen can bring its items
into view; a display in another one waits there until you look.

relay-tui tells relay the latest turn has been seen (`_relay/seen`) once
the conversation is attached, on screen, scrolled to its end and idle,
so the unread mark clears on every device.

## Reopening where you left off

Close relay-tui and open it again against the same agent, and it looks
the way you left it. It keeps the role in focus, the list of roles and
its cursor, whether the pane had the keys, the board or the workspace
items, the selected card and whether it was open (with its line and
column), and your unsent drafts with their pastes. With a single agent
it keeps the parts that apply there. It does not keep scroll positions.
It keeps which column had the keys, not the mode: back in a
conversation you are typing, and anywhere else you are moving.

The saved view goes in a file of relay-tui's own, one per agent command
(everything after `--`), under `~/.local/state/relay-tui/`, or under
`--state-dir DIR`. Each device keeps its own, so the laptop and the phone
can look different. No environment variable chooses the path. The file
is written atomically, readable only by you, at most every half second
while the view changes, and again when relay-tui closes, so a crash or a
kill loses little. It holds your drafts as plain text.

Each part comes back once what it refers to has arrived: the focus, the
list and the drafts when relay has sent the roster, the card when it has
sent the board. Anything that has gone since falls back to the default.
A removed role is not focused and its draft is dropped, with a note. An
archived card is not selected. A role whose conversation has ended
shows none, and opening the view opens no conversation. Keys wait for
the roster (see below), so only the card can be overtaken: if you press
a key after the roster but before the board arrives, what you did
stands and the card is not put back over it. A saved draft
goes ahead of anything you typed for that role meanwhile, and its
pastes take the numbers after any you made, so neither is lost, even if
you close relay-tui before it came back. `--open ROLE` wins over the
saved focus.

Until relay-tui knows the screen your keys will land on, it handles
nothing you type: the screen says "connecting…", and your keys and
pastes wait, in order. Against a plain agent that is once its session
exists; against relay's hub, once the roles have come and the saved
view has been put back (or found nothing to put back). relay-tui then
replays them as if you had just typed them on that screen, at the times
you pressed them, so they do not stop the saved view coming back. They
type and move, and nothing else: a replayed key never archives or ends
anything, since you never saw what it would have answered: with
`--open`, or a saved role in focus, they land in that role's input
after its saved draft, and on the list they are the list's keys. Only
the card, which waits for the board, can still be cancelled by them.
If the roles fail to load, your keys go to the list, with a note.
Ctrl-C quits while connecting. If the agent exits before then, the
saved view is left as it was. A view saved against the other kind of
agent puts back what it can, and a draft with nowhere to go is not put
back, with a note.

A damaged file, one for another command, one too large or of an unknown
version (this is version 2) means the defaults, and the status line says
so. A version 1 file, from before pastes were all your own, is still
read: its drafts come back, and its placeholders such as
`[pasted 2 lines · untrusted #1]` become `[pasted 2 lines #1]`. Nothing restored counts as sent or answered: no question waiting for
y, and no queued message is saved.

## Pasting

What you paste is yours. relay-tui does not type a paste into the prompt,
though, so a long one does not fill the box. A bracketed paste is held,
and the input shows a placeholder such as `[pasted 42 lines #1]`, even
for a single line. When you send, each placeholder still in the prompt is
replaced by the pasted text, in one pass (pasted text that happens to
spell another placeholder is not expanded again). The prompt is plain
text: no resource block, no label, no note in the transcript. Delete a
placeholder from the input and that paste is not sent. Line breaks in a
paste may arrive as LF, CR or CRLF depending on the terminal; all three
count as lines and are sent as LF. A terminal that
does not do bracketed paste types the text in as keys, and relay-tui
cannot tell. relay cleans what you paste when it arrives (invisible
characters and lookalikes, reported in one plain line), so relay-tui does
not.

A draft belongs to its role, not to a conversation: its text and its
pastes stay as they are when the conversation ends, here or on another
device, or fails to open, and a message that could not be sent comes
back to it with its pastes. A paste's number is never used again in
the same draft.

A paste made while the pane has focus, the transcript is open or the
mode is moving goes nowhere, and the status line says how to type.

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
the keys. `core.State` is one per conversation: its transcript and its
own pane items. The input and its pastes are the draft's (`core.Draft`). Against a single
agent `core.Single` holds the app and the one conversation, and
`core.ReduceSingle` applies an event to both. The pane follows the same
split. `core` holds its state and the board's cards, `wire` reads relay's `_meta`, its
`_relay/items`, `_relay/resolve`, `_relay/cards` and
`_relay/cards/op` answers and its `_relay/cards/changed` notifications,
and `render` draws it all. The `agent` package makes the calls one at a
time and in order.

The two questions live in `core/authority.go`. `core.authorityOf` says
which events need a yes that is live and armed, `App.mayTake` decides
whether one is taken (a key that was not replayed, on a question that has
been the surface for `core.ArmDelay`), and `core.Provenance` says in plain
words where something came from.

`internal/agent` runs the agent process, and `internal/ui` is the Bubble
Tea program. Both are thin code around the core that carries out its
effects.

The saved view follows the same split. `core.SaveHub` and
`core.SaveSingle` derive it from state, and `core.RestoreHub` and
`core.RestoreSingle` hand a loaded one to the reducers, which put it back
part by part as the roster, the session and the board arrive.
`internal/viewfile` encodes it, with a digest of the view, and does the
file's IO in the background.

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
with replay, several sessions, a permission request and a settle notice
from a relay that still held calls (relay-tui refuses and ignores them),
and a turn queued behind scheduled work. A control file plays the
second device; see `internal/fakehub` for its commands.

`testdata/agentloop` holds ACP streams recorded from agentloop. The tests
replay them; nothing imports agentloop. The end-to-end tests run relay-tui
against `cmd/fakeagent`, which plays the scripts in `testdata/scripts`,
once in process on an emulated terminal and once as a built binary on a
real pseudo-terminal.

## License

MIT
