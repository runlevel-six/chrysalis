# Sign in with your identity provider (OIDC)

This guide sets Chrysalis up to sign people in through an OpenID Connect provider, such as the single sign-on your
household already uses. This is the recommended mode: Chrysalis checks every sign-in itself, so its security doesn't
depend on how your network is laid out.

You need:

- an OIDC provider you can register a client with;
- the address people will use, served over HTTPS, for example `https://board.example.home`.

## 1. Register Chrysalis with your provider

Create a confidential client (one with a secret) in your provider with:

| Setting | Value |
| --- | --- |
| Redirect URI | `https://board.example.home/auth/callback` |
| Scopes | `openid`, `profile`, `email` (add `groups` if you'll limit sign-in by group) |
| Grant type | Authorization code |

Note the client ID and secret, and your provider's issuer URL (the address its discovery document is under, without
`/.well-known/openid-configuration`).

## 2. Configure Chrysalis

Set these, as described in the [configuration reference](../reference/configuration.md#sign-in):

```sh
CHRYSALIS_BASE_URL=https://board.example.home
CHRYSALIS_AUTH_MODE=oidc
CHRYSALIS_OIDC_ISSUER=https://auth.example.com
CHRYSALIS_OIDC_CLIENT_ID=chrysalis
CHRYSALIS_OIDC_CLIENT_SECRET=...
# Optional: only these groups may sign in.
CHRYSALIS_OIDC_ALLOWED_GROUPS=family
```

**Docker Compose:** copy `deploy/compose/.env.example` to `.env`, fill it in, and run `docker compose up -d`. Keep
`.env` out of version control.

**Kubernetes:** start from `deploy/k8s/overlays/example-oidc`. Put the secret in a Secret rather than the ConfigMap:

```sh
kubectl -n chrysalis create secret generic chrysalis-secrets \
  --from-literal=CHRYSALIS_OIDC_CLIENT_SECRET=...
```

## 3. Put it behind HTTPS

Chrysalis's session cookies only work over HTTPS. Terminate TLS in your reverse proxy or ingress and forward to port
8080. The Compose file publishes the port on `127.0.0.1` only, so just the proxy on the same host can reach it.

## 4. Check it

Open `https://board.example.home`. You're sent to your provider, then back to **Your boards**. If you set
`CHRYSALIS_OIDC_ALLOWED_GROUPS`, someone outside those groups is told calmly that their account doesn't have access, and to ask the person who runs it.

If sign-in fails, the server log (JSON on standard error) says why: usually a redirect URI that doesn't match exactly,
or an issuer URL with a trailing path.
