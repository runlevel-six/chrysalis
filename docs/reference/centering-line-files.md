# Centering line files

The file format for importing and exporting a centering-line collection, as YAML or JSON. The same format works in
**Settings → Centering line → Collections and entries** and with `chrysalis admin quotes import`
([configuration reference](configuration.md#commands)). Exports use it too, so an exported file imports again
unchanged.

## Shape

One file holds one collection: an optional name and a list of entries.

```yaml
collection: Morning
entries:
  - text: Be still, and breathe.
    attribution: A gardener
    tags: [calm]
  - text: |-
      Water the roots,
      not the leaves.
    reference: Notebook 3, page 12
  - text: One row at a time.
```

The same collection as JSON:

```json
{
  "collection": "Morning",
  "entries": [
    { "text": "Be still, and breathe.", "attribution": "A gardener", "tags": ["calm"] },
    { "text": "Water the roots,\nnot the leaves.", "reference": "Notebook 3, page 12" },
    { "text": "One row at a time." }
  ]
}
```

## Fields

| Field | Type | Required | Limit | Meaning |
| --- | --- | --- | --- | --- |
| `collection` | text | No | 100 characters | The collection's name. Used when the import doesn't name a collection itself. |
| `entries` | list | Yes | 2,000 per file | The entries, in the order they should appear. |
| `entries[].text` | text | Yes | 2,000 characters | The words themselves. Line breaks are kept. |
| `entries[].attribution` | text | No | 200 characters | Who said or wrote it. |
| `entries[].reference` | text | No | 200 characters | Where it's from, such as a chapter and verse or a page. |
| `entries[].translation` | text | No | 50 characters | Which translation, such as `WEB`. |
| `entries[].tags` | list of text | No | 20 tags of 40 characters each | Words to group entries by. |

- Leading and trailing spaces are trimmed. `attribution`, `reference`, `translation` and tags are one line each; line
  breaks in them become spaces.
- Empty tags and repeated tags are dropped.
- Any other field is an error, so a misspelling (`atribution`) is caught rather than quietly lost.
- The file can be up to 1 MB. A collection can hold up to 5,000 entries in all, and a person up to 100 collections.

## YAML or JSON

- In the app and with the admin command, a file ending in `.json` is read as JSON, and `.yaml` or `.yml` as YAML.
- Otherwise it's JSON if it starts with `{`, and YAML if not.
- Files are UTF-8. A byte order mark at the start is ignored.
- Only the first YAML document in a file is read.

## How an import lands

- **Which collection.** The import goes into the collection you choose. With none chosen, it goes into your own
  collection with the file's `collection` name (compared without regard to capitals), and makes that collection if you
  don't have one by that name.
- **Additive by default.** Entries are added after the ones already there. Importing the same file twice adds them
  twice.
- **Replace.** With replace (`--replace` on the command line), the collection's current entries move to the trash first.
  They can be restored from the trash until it's emptied.
- **All or nothing.** If any entry has a problem, nothing is imported, and the message names the entry by its number
  (`Entry 3: An entry needs some text.`).
- **New collections feed your line.** A collection the import creates is added to the collections your line draws from.

## Export

Each collection downloads as YAML or JSON, with its name and its entries in order. Fields that are empty are left out.
Deleted entries aren't included.
