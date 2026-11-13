# Try Chrysalis on your computer

In this tutorial you'll run Chrysalis on your own computer, make a board, and try the things that make it calm:
collapsing, focusing and hiding what's done. It takes about fifteen minutes. You need Docker installed; nothing else.

You'll run it in **dev mode**, where there's no sign-in and everyone is the same person. That's only for trying it out.
When you're ready to use it for real, follow one of the sign-in guides listed at the end.

## Start it

Run this in a terminal:

```sh
docker run --rm -p 127.0.0.1:8080:8080 \
  -e CHRYSALIS_BASE_URL=http://localhost:8080 \
  -e CHRYSALIS_AUTH_MODE=dev -e CHRYSALIS_DEV_MODE=true \
  -v chrysalis-try:/data \
  ghcr.io/runlevel-six/chrysalis:latest
```

The log shows a warning that dev mode signs everyone in as one fixed user, then `listening`. Open
<http://localhost:8080> in your browser. You'll see **Your boards**, with a short sentence inviting you to make your
first one.

## Make a board

1. Under the list, type a name such as `Garden`, and under **Start with** choose **Egg, Caterpillar, Chrysalis,
   Imago**.
2. Select **Create**.

The board opens with four columns: *Egg*, *Caterpillar*, *Chrysalis* and *Imago*. *Imago* is already marked as a
done column, so cards that reach it count as finished.

## Add some cards

1. Select **Add a card** at the bottom of *Egg*.
2. Type `Order seeds` and press `Enter`. The input stays open for the next one.
3. Type `Sketch the bed layout`, press `Enter`, then press `Esc` to close the input.

Each card shows only its title. That's on purpose: collapsed cards keep the board quiet.

## Open a card a little, or all the way

1. Select the arrow beside `Order seeds`. The card expands in place to show its actions.
2. Select **Add a checklist**, then add two items: `Milkweed` and `Coneflower`.
3. Tick `Milkweed`. The progress bar moves to one of two.
4. Select the arrow again. The card collapses back to its title.

The **Open card** button in the card's corner opens the full view, with room for a long description and attachments. Press
`Esc` to close it.

Reload the page. Everything is where you left it, including which cards were expanded. Chrysalis remembers your view
of each board for you, on every device you use.

## Make it quieter

1. Drag `Order seeds` into *Imago*. A small butterfly lifts off the card: the one bit of celebration Chrysalis allows
   itself. (It doesn't appear if your system asks for reduced motion.)
2. Select **View** at the top of the board, then **Hide completed cards**. The card is still there, just out of sight.
3. Open the options (`…`) on *Caterpillar* and choose **Focus on this column**. Everything else steps back. Select
   **Show all columns** in the banner, or press `Esc`, to return.
4. Press `?` to see all the keyboard shortcuts.

## Make it yours

1. Open **Board** (`…` at the top right of the board) and choose **Board settings**.
2. Under **Background**, pick the **Meadow** gradient, and under **Icon**, pick the butterfly.
3. Select the gear at the top of the page to open **Settings**, and try the **Butterfly Garden** theme in light and
   dark.
4. Select **Back to Garden** to return to your board.

## Clean up

Stop the container with `Ctrl+C`. Your board stays in the `chrysalis-try` volume; to start fresh next time, remove it:

```sh
docker volume rm chrysalis-try
```

## Next steps

- [Sign in with your identity provider (OIDC)](../how-to/sign-in-with-oidc.md), or
  [sign in through a forward-auth proxy](../how-to/sign-in-behind-a-proxy.md), to run it for your household.
- [Back up and restore](../how-to/back-up-and-restore.md) before you put anything important in it.
- [Import a board from Trello](../how-to/import-from-trello.md) if you're moving in.
