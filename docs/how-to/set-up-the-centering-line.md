# Set up the centering line

The centering line is one quiet line of Scripture or a quote under each board's title. It's off until you turn it on,
and it shows entries from collections you keep. This guide turns it on, fills a collection by hand or from a file, and
shows how an administrator can import a file for someone.

## Turn it on

1. Open **Settings** (the gear at the top right).
2. Under **Centering line**, tick **Show the centering line on my boards**.
3. Choose how it behaves:
   - **Which entry:** change it from time to time, or keep the one showing now (pinned).
   - **How often it changes:** each time you open the app, every hour, every day at midnight (the default) or every
     week on Monday. "Midnight" and "Monday" are where your device is.
   - **In what order:** in order, or shuffled. Shuffled shows every entry once before any repeats.
   - **Entries come from:** the collections that feed the line.

The line shows nothing until at least one of those collections has an entry.

## Add entries by hand

1. In **Settings**, select **Collections and entries**.
2. Type a name under **Collections** and select **Add collection**. A new collection feeds your line straight away.
3. Select **Add an entry**, type the text, and optionally who said it, a reference, a translation and some tags. Select
   **Add entry**. The form stays open for the next one.
4. Use the arrows to reorder entries, **Edit** to change one, and the bin to delete it. A deleted entry can be brought
   back with **Undo**, or later from the **Trash** at the bottom of the page.

## Import a file

Write the entries as YAML or JSON (the format is in the [centering line files reference](../reference/centering-line-files.md)),
for example `morning.yaml`:

```yaml
collection: Morning
entries:
  - text: Be still, and breathe.
    attribution: A gardener
  - text: One row at a time.
```

1. In **Settings → Collections and entries**, go to **Import**.
2. Under **Into**, leave **The collection named in the file** to use the file's `collection` name (it's created if you
   don't have it), or pick one of your collections.
3. If you picked a collection and want the file to take the place of what's there, tick **Replace its entries**. The
   old entries go to the trash.
4. Choose the file under **File to import**. A message says how many entries came in.

To keep a copy, or to move a collection to another Chrysalis, open the collection and select **YAML** or **JSON** next
to **Download**.

## Import a file for someone (administrators)

`chrysalis admin quotes import` adds a file's entries to a person's collection from the command line. The person needs
to have signed in at least once. It's safe to run while the server is running; open pages show the new entries the
next time they load.

The container image has no shell to copy files into, so pass the file on standard input with `-`.

**Docker Compose:**

```sh
docker compose exec -T chrysalis /chrysalis admin quotes import \
  --user alice@example.com --collection "Morning" - < morning.yaml
```

**Kubernetes:**

```sh
kubectl -n chrysalis exec -i deploy/chrysalis -- /chrysalis admin quotes import \
  --user alice@example.com --collection "Morning" - < morning.yaml
```

- `--user` is the person's email address, as their sign-in reports it (capitals don't matter).
- `--collection` names one of their collections, and creates it if needed. Leave it out to use the file's `collection`
  name.
- Add `--replace` to move the collection's current entries to the trash first.
- With `-`, the file is read as YAML unless it starts with `{`. Give a path instead of `-` when the file is already on
  the machine running the command.

The command prints what it did, for example `created collection "Morning" for alice@example.com: 2 entries added (2 in
the collection now)`.

## Use the line

- Select the line, or press Enter or Space on it, to see all of it with its source. Select it again to fold it back.
- Hover over it or move focus to it for its controls (on a phone or tablet, open it first): **previous**, **next**,
  **pin** (keep this one until you unpin it) and **hide for today** (gone on all your devices until midnight; the toast
  offers **Undo**).
- To leave the line off one board, open **Board → Board settings** and untick **Show the centering line on this
  board**, or use **View → Hide the centering line here**. This only changes what you see.
