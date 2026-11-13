# Chrysalis documentation

Chrysalis is a calm, self-hosted kanban board built to reduce visual and cognitive load: everything collapses, nothing
is lost, and what you see is what you chose to see. The pages follow [Diátaxis](https://diataxis.fr/): reference
(exactly what), how-to guides (what to do), explanation (why) and tutorials (learning by doing). The reading paths
below pick out what each kind of reader needs first; every page is listed once under [All pages](#all-pages).

## Reading paths

### Try it

1. [Try Chrysalis on your computer](tutorials/try-chrysalis.md): run it in a minute with Docker and make your first
   board.
2. [Keyboard shortcuts](reference/keyboard-shortcuts.md).

### Run it for your household

1. Choose how people sign in: [with your identity provider (OIDC)](how-to/sign-in-with-oidc.md), or
   [through a forward-auth proxy](how-to/sign-in-behind-a-proxy.md).
2. [Configuration reference](reference/configuration.md): every setting.
3. [Back up and restore](how-to/back-up-and-restore.md): turn on daily backups, copy them somewhere safe, and restore
   one.
4. [Branding reference](reference/branding.md): add your own themes, icon and name.

### Move in from Trello

1. [Import a board from Trello](how-to/import-from-trello.md): export each board from Trello and bring it across.

### Add a centering line

1. [Set up the centering line](how-to/set-up-the-centering-line.md): turn it on, add entries or import a file, and
   import for someone with the admin command.
2. [Centering line files](reference/centering-line-files.md): the YAML and JSON format, its limits, and how imports
   land.

### Understand how it's built

1. [Design document](DESIGN.md): goals, the UX specification, architecture, data and security decisions.
2. [Decision records](adr/): choices made along the way and why, such as
   [building the calendar on the board's drag library](adr/0001-calendar-on-the-boards-drag-library.md).
3. [How sync and offline work](explanation/sync-and-offline.md): live changes across devices, and read-only offline.
4. [How backups work](explanation/backups.md): consistent snapshots, retention, and the CronJob trade-off.

## All pages

| Page | Kind | What it covers |
| --- | --- | --- |
| [Try Chrysalis on your computer](tutorials/try-chrysalis.md) | Tutorial | Run it locally and make a first board |
| [Sign in with OIDC](how-to/sign-in-with-oidc.md) | How-to | Register a client and configure OIDC sign-in |
| [Sign in through a forward-auth proxy](how-to/sign-in-behind-a-proxy.md) | How-to | Header mode, safely |
| [Keyboard shortcuts](reference/keyboard-shortcuts.md) | Reference | Every shortcut, and moving cards from the keyboard |
| [How sync and offline work](explanation/sync-and-offline.md) | Explanation | Live sync, conflicts, and why offline is read-only |
| [ADR-0001: Calendar on the board's drag library](adr/0001-calendar-on-the-boards-drag-library.md) | Decision record | Why the calendar is built in the app rather than on a calendar library |
| [Design document](DESIGN.md) | Explanation | Why Chrysalis is built the way it is, and what it will and won't do |
| [Import a board from Trello](how-to/import-from-trello.md) | How-to | Export a board from Trello, import it, and what comes across |
| [Set up the centering line](how-to/set-up-the-centering-line.md) | How-to | Turn the line on, fill collections by hand or from a file, and import with `chrysalis admin quotes import` |
| [Centering line files](reference/centering-line-files.md) | Reference | The YAML/JSON import and export format, its limits, and additive versus replace imports |
| [Configuration](reference/configuration.md) | Reference | Every `CHRYSALIS_*` environment variable, and the `chrysalis admin` commands |
| [Branding](reference/branding.md) | Reference | `CHRYSALIS_BRANDING_DIR`: theme schema, contrast checks, icon and name |
| [Back up and restore](how-to/back-up-and-restore.md) | How-to | Daily backups, backing up by hand, and restoring with Compose or Kubernetes |
| [How backups work](explanation/backups.md) | Explanation | What a backup holds, how it stays consistent, retention, and the CronJob trade-off |
