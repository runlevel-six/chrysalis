# How sync and offline work

Chrysalis is meant to be open on a phone, a laptop and a tablet at once, and to never make you wonder which one is
right. This page explains how changes travel between them, and what happens when one loses its connection.

## The server is the one copy

Everything you make (boards, cards, checklists, descriptions) lives on the server. A device shows a copy, changes it
immediately when you act so it feels instant, and sends the change. If the server refuses or can't be reached, the
device puts things back and says so calmly, with a way to try again or undo.

Your **view** of a board, meaning which cards are expanded, which columns are collapsed, focus and hide-completed, is
saved too, a moment after you change it. It's yours alone: two people on the same board each see it their own way. For
the view, the last change wins; it isn't content, so a rare collision costs nothing.

## Changes arrive on their own

Each open page keeps a quiet connection to the server (server-sent events). When anything changes on a board you're
looking at, the server sends a short notice, such as "a card on this board changed". The page then asks for the
current state through the normal API, so the notice itself never carries content, and what you can see is always
decided in one place.

A few details keep this calm:

- **Your own changes don't echo.** Each tab marks its requests, and skips notices it caused.
- **Nothing moves under your hands.** While you're dragging, or a change of yours is still being saved, notices wait
  until you're done.
- **Gaps are filled.** If the connection drops, the browser reconnects and says which notice it saw last. The server
  replays what was missed, or, after a long gap or a server restart, tells the page to refresh everything.

Descriptions are the one place two people's edits can genuinely clash. Each save says which version it was based on;
if someone else saved first, Chrysalis asks whether to keep yours, use theirs, or see both side by side, rather than
silently dropping either.

## Offline: read-only, on purpose

When the server can't be reached, Chrysalis switches to the last copy this device saw:

- the app itself is kept by a small service worker, so it still opens;
- every board and card you've looked at in the last 30 days was saved on the device as you went, so it opens too;
- a banner says **Offline — viewing last synced copy**, and editing is put away until you're back.

Editing offline was left out deliberately. Merging changes made in two places while apart is a large source of
confusing bugs, and it isn't needed for the app's purpose. One exception: a description you were typing when the
connection dropped is kept on the device and sent when it comes back, so nothing you wrote is lost.

The offline copy stays on that device. Signing out clears it.
