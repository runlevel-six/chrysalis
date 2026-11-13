# Chrysalis documentation

Chrysalis is a calm, self-hosted kanban board built to reduce visual and cognitive load: everything collapses, nothing
is lost, and what you see is what you chose to see. The pages follow [Diátaxis](https://diataxis.fr/): reference
(exactly what), how-to guides (what to do), explanation (why) and tutorials (learning by doing). The reading paths
below pick out what each kind of reader needs first; every page is listed once under [All pages](#all-pages).

## Reading paths

### Run it for your household

1. [Configuration reference](reference/configuration.md): every setting, including the two ways to sign people in.
2. [Back up and restore](how-to/back-up-and-restore.md): turn on daily backups, copy them somewhere safe, and restore
   one.
3. [Branding reference](reference/branding.md): add your own themes, icon and name.

### Move in from Trello

1. [Import a board from Trello](how-to/import-from-trello.md): export each board from Trello and bring it across.

### Add a centering line

1. [Set up the centering line](how-to/set-up-the-centering-line.md): turn it on, add entries or import a file, and
   import for someone with the admin command.
2. [Centering line files](reference/centering-line-files.md): the YAML and JSON format, its limits, and how imports
   land.

### Understand how it's built

1. [Design document](DESIGN.md): goals, the UX specification, architecture, data and security decisions.
2. [How backups work](explanation/backups.md): consistent snapshots, retention, and why the server schedules its own.

## All pages

| Page | Kind | What it covers |
| --- | --- | --- |
| [Design document](DESIGN.md) | Explanation | Why Chrysalis is built the way it is, and what it will and won't do |
| [Import a board from Trello](how-to/import-from-trello.md) | How-to | Export a board from Trello, import it, and what comes across |
| [Set up the centering line](how-to/set-up-the-centering-line.md) | How-to | Turn the line on, fill collections by hand or from a file, and import with `chrysalis admin quotes import` |
| [Centering line files](reference/centering-line-files.md) | Reference | The YAML/JSON import and export format, its limits, and additive versus replace imports |
| [Configuration](reference/configuration.md) | Reference | Every `CHRYSALIS_*` environment variable, and the `chrysalis admin` commands |
| [Branding](reference/branding.md) | Reference | `CHRYSALIS_BRANDING_DIR`: theme schema, contrast checks, icon and name |
| [Back up and restore](how-to/back-up-and-restore.md) | How-to | Daily backups, backing up by hand, and restoring with Compose or Kubernetes |
| [How backups work](explanation/backups.md) | Explanation | What a backup holds, how it stays consistent, retention, and the CronJob trade-off |
