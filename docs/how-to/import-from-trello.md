# Import a board from Trello

Bring a Trello board into Chrysalis, with its lists, cards, labels, checklists and comments. Each import makes a new
board; it never changes one you already have, so it's safe to try more than once.

## Before you start

- You need to be able to open the board in Trello.
- The export file can be up to 50 MB. A busy board with years of cards is usually a few megabytes.
- If Chrysalis sits behind a reverse proxy with a request size limit, the limit needs to allow the file too.

## Export the board from Trello

1. Open the board in Trello.
2. Open the board's menu (the **⋯** button at the top right).
3. Choose **Print, export, and share**, then **Export as JSON**.
4. Trello opens the export in a new tab, or downloads it. If it opens in a tab, save the page as a `.json` file.

## Import it

1. In Chrysalis, go to **Your boards** (select the app name at the top left).
2. Select **Import from Trello**, below the new board form, and choose the `.json` file.
3. When the import finishes, a short report shows what came across and what was left out.
4. Select **Open the board**. The new board is also at the end of your board list.

If the file isn't a Trello board export, or something goes wrong partway through, nothing is created and you can try
again.

## What comes across

| In Trello | In Chrysalis |
| --- | --- |
| Board name and background color | Board name and background color |
| Lists, in order | Columns, in order |
| Archived lists | Columns that start collapsed, with their cards in the board's archive |
| Card name, description, start and due dates | Card title, description, start date and due date |
| A due date marked complete | A completed card |
| Archived cards | Archived cards |
| Labels | Labels, each with the nearest Chrysalis color; labels without a name are named after their color |
| Checklists and their items, ticked or not | Checklists and items, in the same order |
| Comments | Comments under your name, each starting "Originally by" and the original author, with the original date |
| Cover colors | Cover colors |
| Attachments | Links to the attachments |

Descriptions and comments keep their formatting: headings, bold, italic, strikethrough, code, lists, links, quotes
and line breaks. Pictures in a description become links to the picture.

## What doesn't come across

The report lists anything left out from your board. It can include:

- Power-Up data and custom fields.
- People assigned to cards, and assignees and due dates on checklist items.
- Due-date reminders and card locations.
- The board's background photo and cover images. You can choose a background and covers again in Chrysalis.
- Trello's activity history (moves, edits and so on). Only comments come across.
- Attachments whose address isn't a web link.

Some things are worth knowing:

- Files you uploaded to Trello come across as links to Trello. Opening them may need you to be signed in to Trello.
- Trello's export holds only the most recent 1,000 actions, so on a long-lived board, some older comments may be
  missing.
- A start date that falls after its card's due date is left off.
- Names longer than Chrysalis allows are shortened.
