# Back up and restore

This guide shows you how to turn on daily backups, make a backup by hand, copy backups somewhere safe, and restore
one. Each backup holds the database and every attachment, taken together. For why backups work this way, see
[How backups work](../explanation/backups.md); for every setting, see the
[configuration reference](../reference/configuration.md#backups).

The examples assume the Compose service is called `chrysalis`, and that on Kubernetes everything is in the
`chrysalis` namespace with the names from `deploy/k8s/base`. Change them to match yours.

## Turn on daily backups

**Docker Compose:** set this in your `.env` (the example `.env` already does), then run `docker compose up -d`:

```sh
CHRYSALIS_BACKUP_SCHEDULE=daily
```

**Kubernetes:** the example overlays already include the backup CronJob. If you wrote your own overlay, add it:

```yaml
components:
  - ../../components/backup-cronjob
```

It runs a backup at 03:00 UTC in a pod on the same node as Chrysalis, because the volume is ReadWriteOnce, and turns
the server's built-in schedule off. If you leave the component out, the server backs itself up once a day instead
(the base sets `CHRYSALIS_BACKUP_SCHEDULE: daily`). [How backups work](../explanation/backups.md#the-built-in-schedule-or-a-cronjob)
explains the trade-off.

Either way, backups go to `/data/backups`. By default the newest from each of the last 7 days and each of the last 4
weeks are kept, and older ones are deleted after each new backup.

## Make a backup now

You can do this while Chrysalis is running, for example before an upgrade.

```sh
# Docker Compose
docker compose exec chrysalis /chrysalis admin backup

# Kubernetes
kubectl -n chrysalis exec deploy/chrysalis -- /chrysalis admin backup
```

It prints the new backup's path, such as `/data/backups/chrysalis-20261112T030000Z`, and any old backups it pruned.

To see what backups you have:

```sh
docker compose exec chrysalis /chrysalis admin list-backups
kubectl -n chrysalis exec deploy/chrysalis -- /chrysalis admin list-backups
```

## Copy backups off the volume

Backups are written to the same volume as the data. That covers mistakes and a damaged database, but not losing the
volume itself, so copy them somewhere else regularly.

**Docker Compose:**

```sh
docker compose cp chrysalis:/data/backups ./chrysalis-backups
```

**Kubernetes:** the image has no shell or `tar`, so `kubectl cp` doesn't work with it. Use whatever already backs up
your volumes, such as volume snapshots or your cluster's backup tool, and include the `chrysalis-data` claim.

## Restore a backup

Restoring replaces the database and attachments with the backup's. Anything changed since that backup is set aside, not
lost: with `--force`, the current data is moved into `/data/pre-restore-<time>/` first.

Before it changes anything, the restore checks every file in the backup against its checksum and runs SQLite's
`PRAGMA integrity_check` on the database. If either fails, it stops and leaves your data as it was. Afterwards it applies
any database migrations the backup is missing, so you can restore an older backup with a newer version.

The server must be stopped while you restore.

### With Docker Compose

1. Stop Chrysalis:

   ```sh
   docker compose stop chrysalis
   ```

2. Pick a backup. `latest` means the newest one; otherwise use a name from the list.

   ```sh
   docker compose run --rm chrysalis admin list-backups
   ```

3. Restore it. `--force` is needed because the volume already has a database.

   ```sh
   docker compose run --rm chrysalis admin restore latest --force
   ```

   It prints where the previous data went and how many attachment files it restored.

4. Start Chrysalis again and check your boards:

   ```sh
   docker compose start chrysalis
   ```

### With Kubernetes

1. Stop Chrysalis, so nothing else is using the volume:

   ```sh
   kubectl -n chrysalis scale deploy/chrysalis --replicas=0
   kubectl -n chrysalis wait --for=delete pod -l app.kubernetes.io/name=chrysalis --timeout=120s
   ```

2. Save this as `restore.yaml`. It runs the restore once with the same settings, user and volume as the server. Change
   the image tag to the version you run, and `latest` to a backup name if you want an older one.

   ```yaml
   apiVersion: v1
   kind: Pod
   metadata:
     name: chrysalis-restore
     namespace: chrysalis
   spec:
     restartPolicy: Never
     automountServiceAccountToken: false
     securityContext:
       runAsNonRoot: true
       runAsUser: 65532
       runAsGroup: 65532
       fsGroup: 65532
       seccompProfile:
         type: RuntimeDefault
     containers:
       - name: restore
         image: ghcr.io/runlevel-six/chrysalis:latest
         args: [admin, restore, latest, --force]
         envFrom:
           - configMapRef:
               name: chrysalis-config
           - secretRef:
               name: chrysalis-secrets
               optional: true
         securityContext:
           readOnlyRootFilesystem: true
           allowPrivilegeEscalation: false
           capabilities:
             drop: [ALL]
         volumeMounts:
           - { name: data, mountPath: /data }
           - { name: tmp, mountPath: /tmp }
     volumes:
       - name: data
         persistentVolumeClaim:
           claimName: chrysalis-data
       - name: tmp
         emptyDir: {}
   ```

3. Run it and read the result:

   ```sh
   kubectl apply -f restore.yaml
   kubectl -n chrysalis wait --for=jsonpath='{.status.phase}'=Succeeded pod/chrysalis-restore --timeout=300s
   kubectl -n chrysalis logs pod/chrysalis-restore
   kubectl -n chrysalis delete pod/chrysalis-restore
   ```

   If the wait times out, the logs say why; the data is unchanged unless they say it was restored.

4. Start Chrysalis again and check your boards:

   ```sh
   kubectl -n chrysalis scale deploy/chrysalis --replicas=1
   ```

### Restore a backup from somewhere else

To restore a copy you kept elsewhere, put its directory (the one named like `chrysalis-20261112T030000Z`, with
`manifest.json` inside) back into `/data/backups` on the volume, readable by user 65532, then follow the steps above
using its name instead of `latest`.

## Clean up afterwards

When you're happy with the restored data, delete the `pre-restore-<time>` directory to free the space. The image has no
shell, so use a throwaway container on the volume. With Compose, for example (your volume's name may differ; see
`docker volume ls`):

```sh
docker run --rm -v chrysalis_chrysalis-data:/data busybox rm -rf /data/pre-restore-20261112T090000Z
```
