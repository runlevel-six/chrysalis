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
