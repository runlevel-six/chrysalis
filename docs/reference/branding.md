# Branding

A deployment can add its own themes, icon and product name by pointing `CHRYSALIS_BRANDING_DIR` at a directory
(usually mounted read-only into the container). Everything in it is optional; a missing file just leaves the built-in
default in place. The built-in themes are listed in [Built-in themes](#built-in-themes).

| File | Purpose |
| --- | --- |
| `theme.json` | One theme, or a JSON array of themes, in the [theme schema](#theme-schema). Added to the built-in themes in everyone's **Settings → Theme**. |
| `icon.svg` or `icon.png` | Replaces the butterfly in the header and the browser tab. At most 512 KB; `icon.svg` wins if both exist. Served publicly at `/branding/icon`. |
| `display-name.txt` | Replaces "Chrysalis" in the header. One line, at most 60 characters. |

The directory is read once at startup. A directory that's configured but can't be read stops the server with an
error; problems inside individual files are logged as warnings and the rest still loads.

## Theme schema

A theme is an `id`, a `name`, and a `light` and a `dark` set of color tokens. Every token below is required in both
sets. Colors that are checked for contrast must be `#rrggbb`; `hover` may be any CSS color (it's usually translucent).

```json
{
  "id": "meadow",
  "name": "Meadow",
  "light": { "surface": "#fbfaf5", "text": "#23301c", "...": "..." },
  "dark":  { "surface": "#161b12", "text": "#eef0e6", "...": "..." }
}
```

| Field | Rules |
| --- | --- |
| `id` | Lowercase letters, digits and hyphens, up to 40 characters, starting with a letter or digit. Must not be `default` or `butterfly-garden`, or repeat another theme's id; a theme that breaks this is skipped. |
| `name` | Shown in Settings. |
| `light`, `dark` | Token sets; see the table below. |

| Token | Used for |
| --- | --- |
| `surface` | The page background |
| `surface-raised` | Cards, dialogs, menus |
| `column-bg` | Columns |
| `text` | Main text |
| `text-muted` | Secondary text: counts, hints, dates |
| `border` | Borders and dividers |
| `accent` | Buttons, links in descriptions, checklist progress |
| `on-accent` | Text on `accent` buttons |
| `focus-ring` | Keyboard focus outlines, the overdue marker |
| `hover` | Hover backgrounds for quiet buttons (any CSS color) |
| `toast-bg`, `toast-text` | The undo notification |
| `label-text` | Text on label chips |
| `label-green`, `label-lime`, `label-yellow`, `label-orange`, `label-red`, `label-pink`, `label-purple`, `label-blue`, `label-sky`, `label-gray` | The ten label, cover and column colors |

## Contrast checks

Each theme is checked against WCAG 2.2 AA when it loads, the same checks the built-in themes pass in the test suite.
A theme that fails still loads, because branding is the deployer's decision, but every failure is logged as a
warning naming the token pair and its ratio.

| Foreground | Background | Minimum |
| --- | --- | --- |
| `text`, `text-muted` | `surface`, `surface-raised`, `column-bg` | 4.5:1 |
| `focus-ring` | `surface`, `surface-raised`, `column-bg` | 3:1 |
| `accent` | `surface` | 4.5:1 |
| `on-accent` | `accent` | 4.5:1 |
| `toast-text` | `toast-bg` | 4.5:1 |
| `label-text` | each `label-*` color | 4.5:1 |
| each `label-*` color | `surface-raised` | 3:1 |

## Built-in themes

| Id | Name | Notes |
| --- | --- | --- |
| `default` | Default | Neutral greys with a blue accent. |
| `butterfly-garden` | Butterfly Garden | Pollinator-garden colors: cream surfaces, deep milkweed-green text and accent, an orange focus ring. |

Their token files are `web/src/themes/*.json` in the source tree, and are a good starting point for a new theme.

## Example

A directory for a fictional "Garden Boards" deployment:

```
branding/
  theme.json          # [{ "id": "meadow", "name": "Meadow", ... }]
  icon.svg
  display-name.txt    # Garden Boards
```

Mounted with `CHRYSALIS_BRANDING_DIR=/branding`, everyone sees "Garden Boards" and the new icon in the header, and can
pick **Meadow** in Settings.
