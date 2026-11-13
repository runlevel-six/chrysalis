# Configuration

Chrysalis is configured entirely by environment variables, all starting with `CHRYSALIS_`. The server checks them at
startup and refuses to start, listing every problem, if any are invalid or inconsistent.

## Server

| Variable | Default | Meaning |
| --- | --- | --- |
| `CHRYSALIS_BASE_URL` | — (required) | The address people use, such as `https://board.example.home`, with no trailing path. Used for sign-in redirects, secure cookies and cross-site request checks. |
| `CHRYSALIS_LISTEN_ADDR` | `:8080` | Where the HTTP server listens. |
| `CHRYSALIS_DATA_DIR` | `/data` | The SQLite database (`chrysalis.db`) and local attachments (`attachments/`). The only path the server writes to. |
| `CHRYSALIS_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. Logs are JSON on standard error. |
| `CHRYSALIS_BRANDING_DIR` | — | Optional extra themes, icon and display name; see [Branding](branding.md). |

## Sign-in

`CHRYSALIS_AUTH_MODE` chooses how people are identified.

| Mode | Use |
| --- | --- |
| `oidc` (default) | Chrysalis signs people in itself through an OpenID Connect provider. Recommended: it doesn't depend on network layout for security. |
| `header` | A reverse proxy authenticates people and passes their identity in headers. Chrysalis trusts those headers only from `CHRYSALIS_TRUSTED_PROXY_CIDRS`. |
| `dev` | Everyone is one fixed user. For local development and tests only; also requires `CHRYSALIS_DEV_MODE=true`. |

| Variable | Default | Meaning |
| --- | --- | --- |
| `CHRYSALIS_OIDC_ISSUER` | — | The provider's issuer URL. Required in `oidc` mode. |
| `CHRYSALIS_OIDC_CLIENT_ID` | — | Required in `oidc` mode. |
| `CHRYSALIS_OIDC_CLIENT_SECRET` | — | Required in `oidc` mode. Keep it in a secret, not in a committed file. |
| `CHRYSALIS_OIDC_REDIRECT_URL` | `<base URL>/auth/callback` | The callback registered with the provider. |
| `CHRYSALIS_OIDC_SCOPES` | `openid profile email` | Space-separated. |
| `CHRYSALIS_OIDC_ALLOWED_GROUPS` | — | Comma-separated. If set, only members of at least one of these groups (from the ID token's `groups` claim) can sign in. |
| `CHRYSALIS_TRUSTED_PROXY_CIDRS` | — | Comma-separated addresses or CIDR ranges of the proxy that sets identity headers. Required in `header` mode; requests from anywhere else are refused. |
| `CHRYSALIS_HEADER_USER` | `Remote-User` | Header carrying the user's stable identifier. |
| `CHRYSALIS_HEADER_EMAIL` | `Remote-Email` | Header carrying the email address. |
| `CHRYSALIS_HEADER_NAME` | `Remote-Name` | Header carrying the display name. |
| `CHRYSALIS_HEADER_GROUPS` | `Remote-Groups` | Header carrying groups (read, not currently used for access). |
| `CHRYSALIS_SESSION_TTL` | `720h` | How long an idle `oidc` session lasts. Each use extends it. |
| `CHRYSALIS_DEV_MODE` | `false` | Must be `true` to allow `dev` mode. |

## Content

| Variable | Default | Meaning |
| --- | --- | --- |
| `CHRYSALIS_MAX_UPLOAD_MB` | `25` | Largest single attachment, 1 to 1024. |
| `CHRYSALIS_STORAGE_BACKEND` | `local` | Where attachments are kept. Only `local` (under the data directory) is available today. |
| `CHRYSALIS_TRASH_RETENTION_DAYS` | `30` | How long deleted boards, columns, cards, labels, checklists and attachments can be restored before they're permanently removed. |

## Push notifications

Reminders always appear in the app. With push on, people can also have them sent to their phones and computers as
notifications; see [Get reminders on your phone](../how-to/get-reminders-on-your-phone.md).

| Variable | Default | Meaning |
| --- | --- | --- |
| `CHRYSALIS_PUSH` | `on` | `off` turns push notifications off for everyone; reminders still show in the app. Push needs outbound HTTPS to the browsers' push services. |
| `CHRYSALIS_VAPID_PRIVATE_KEY` | — | The server's push identity, as base64url. If unset, the server makes one on first start and keeps it in the database (so it's in backups). Changing it means everyone has to turn notifications on again. |
| `CHRYSALIS_VAPID_SUBJECT` | `<base URL>` | How push services can contact whoever runs the server: a `mailto:` or `https:` address. |

## Backups

See [Back up and restore](../how-to/back-up-and-restore.md) for how to use these, and
[How backups work](../explanation/backups.md) for why they work the way they do.

| Variable | Default | Meaning |
| --- | --- | --- |
| `CHRYSALIS_BACKUP_SCHEDULE` | `off` | `daily`: the server backs itself up once per UTC day, checking hourly, and prunes old backups. `off`: backups happen only when you run `chrysalis admin backup` (by hand or from a Kubernetes CronJob). The Compose example and the Kubernetes base set `daily`. |
| `CHRYSALIS_BACKUP_DIR` | `<data dir>/backups` | Where backups are written, one directory each, named like `chrysalis-20261112T030000Z`. |
| `CHRYSALIS_BACKUP_KEEP_DAILY` | `7` | After each backup, keep the newest backup from each of this many most recent days that have one. At least 1. |
| `CHRYSALIS_BACKUP_KEEP_WEEKLY` | `4` | Also keep the newest backup from each of this many most recent ISO weeks (Monday to Sunday, UTC) that have one. 0 or more. |

## Commands

The same binary runs the server and a few maintenance commands. In the container image it's `/chrysalis`.

| Command | What it does |
| --- | --- |
| `chrysalis` or `chrysalis serve` | Runs the server. |
| `chrysalis admin backup` | Makes one backup now, prints its path, then prunes old backups using the settings above. Safe while the server is running. |
| `chrysalis admin list-backups` | Lists the backups in `CHRYSALIS_BACKUP_DIR`, newest first. |
| `chrysalis admin restore <backup> [--force]` | Restores a backup into `CHRYSALIS_DATA_DIR`. `<backup>` is a name from `list-backups`, a path to a backup directory, or `latest`. Refuses if there's already a database or attachments, unless `--force`, which moves them into `pre-restore-<time>/` in the data directory rather than deleting them. Checks the backup's checksums and runs `PRAGMA integrity_check` before changing anything, and applies migrations afterwards. Stop the server first. |
| `chrysalis admin quotes import --user <email> [--collection <name>] [--replace] <file>` | Adds the entries in a YAML or JSON [centering line file](centering-line-files.md) to a person's collection, creating it if needed. `<file>` is a path, or `-` to read standard input. `--collection` defaults to the name in the file; `--replace` moves the collection's current entries to the trash first. The person must have signed in once. Safe while the server is running. See [Set up the centering line](../how-to/set-up-the-centering-line.md#import-a-file-for-someone-administrators). |

Commands read the same `CHRYSALIS_*` variables as the server, so run them with the server's environment.
