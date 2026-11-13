# Get reminders on your phone

This guide sets up reminders so they arrive as notifications on your phone or computer, even when Chrysalis isn't
open. Reminders always show under the bell at the top of the app; notifications are an extra, turned on separately on
each device you want them on.

## 1. Turn reminders on

1. Open **Settings** (the gear at the top of the page).
2. Under **Reminders**, tick **Remind me about cards I choose**.

Nothing reminds you yet: reminders are chosen card by card.

## 2. Turn notifications on for a device

Do this on each phone, tablet or computer you want notifications on.

**On iPhone and iPad**, notifications only work for apps on the Home Screen:

1. Open Chrysalis in Safari.
2. Tap the Share button, then **Add to Home Screen**.
3. Open Chrysalis from the new Home Screen icon, sign in if asked, and go to **Settings → Reminders**.

**On Android, and on computers**, you can stay in the browser.

Then, under **On this device**:

1. Select **Send reminders to this device**.
2. When the browser asks whether Chrysalis may show notifications, choose **Allow**.
3. Select **Send a test**. A notification should appear within a few seconds.

If the page says notifications are blocked, they were turned down for this site earlier. Allow them in the browser's
site settings (on iPhone: **Settings → Notifications → Chrysalis**), then come back.

## 3. Choose a reminder on a card

1. Expand a card and select **Dates…**.
2. Under **Remind me**, choose when: when it's due, 1 hour, 1 day, 2 days or 1 week before, or at a time you choose.
3. Select **Save**.

A due date without a time reminds you at 9 in the morning. When the due date moves, so does the reminder.

## 4. Set quiet hours (optional)

Under **Settings → Reminders**, tick **Quiet hours** and choose the times. Nothing is sent to your devices between them;
reminders that come due then wait under the bell.

## When a reminder arrives

Selecting the notification opens the card. In the app, under the bell, each reminder can be opened, snoozed (for an
hour, until this evening, until tomorrow morning, or until a time you pick) or marked done.

## Stopping

- **On one device:** **Settings → Reminders → Stop on this device**, or turn notifications off for Chrysalis in the
  device's settings.
- **Everywhere:** untick **Remind me about cards I choose**. Card reminders you've chosen are kept, but nothing goes
  off until you turn it back on.

## For whoever runs the server

Push needs the server to reach the browsers' push services over HTTPS (port 443). The Kubernetes NetworkPolicy in
`deploy/k8s/base` allows that. The messages are encrypted for each device, so the push services that carry them can't
read them. To turn push off for everyone, set `CHRYSALIS_PUSH=off` (see the
[configuration reference](../reference/configuration.md#push-notifications)).
