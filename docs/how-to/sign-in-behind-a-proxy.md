# Sign in through a forward-auth proxy

This guide sets Chrysalis up behind a reverse proxy that already authenticates people, such as one using a
forward-auth endpoint. The proxy signs people in and passes who they are in request headers; Chrysalis trusts those
headers, but **only** from the proxy's own addresses.

Use this mode when your proxy already protects your other apps and you'd rather not register another OIDC client. If
you can, prefer [OIDC](sign-in-with-oidc.md): it doesn't depend on the network being laid out correctly.

## What has to be true

Header mode is only safe if nothing but the proxy can reach Chrysalis. Before you start, make sure that:

- Chrysalis's port isn't published anywhere the proxy isn't;
- you know the proxy's address (or the narrowest range that contains it);
- the proxy **removes** any identity headers a client sends, and sets its own after authenticating.

## 1. Configure Chrysalis

```sh
CHRYSALIS_BASE_URL=https://board.example.home
CHRYSALIS_AUTH_MODE=header
CHRYSALIS_TRUSTED_PROXY_CIDRS=172.30.0.2/32
```

By default Chrysalis reads `Remote-User` (a stable identifier), `Remote-Email`, `Remote-Name` and `Remote-Groups`. If
your proxy uses other names, set the `CHRYSALIS_HEADER_*` variables in the
[configuration reference](../reference/configuration.md#sign-in).

A request with identity headers from any other address is refused, and so is one from the proxy without them.

## 2. Configure the proxy

Have the proxy check every request with your authentication service and copy the identity headers to Chrysalis.
`deploy/compose/compose.forward-auth.example.yaml` shows a complete Compose setup, with the proxy on a fixed address in
a private network and a matching proxy configuration in its comments.

## 3. On Kubernetes

Start from `deploy/k8s/overlays/example-header`:

- set `CHRYSALIS_TRUSTED_PROXY_CIDRS` to your ingress controller pods' range (often the cluster's pod CIDR);
- keep the base NetworkPolicy, which only admits traffic from the ingress controller's namespace; check that its
  selector matches your controller;
- add your authentication annotations to the Ingress.

## 4. Check it

Open `https://board.example.home`. Your proxy asks you to sign in, then **Your boards** appears with your name at the
top. To confirm the protection works, request Chrysalis directly from somewhere that isn't the proxy, with a made-up
`Remote-User` header: it must answer `401`.
