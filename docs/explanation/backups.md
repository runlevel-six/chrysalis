# How backups work

Chrysalis keeps everything in one data directory: a SQLite database and, with the local storage backend, a directory
of attachment files. A backup is a copy of both, taken together, that can be restored into a fresh instance. This page
explains the choices behind that. For the steps, see [Back up and restore](../how-to/back-up-and-restore.md).

## What a backup is

Each backup is one directory, named for the moment it was made in UTC, such as `chrysalis-20261112T030000Z`:

- `chrysalis.db`, a copy of the database;
- `attachments.tar.gz`, every attachment file;
- `manifest.json`, recording when and by which version the backup was made, how many attachment files it holds, and a
  SHA-256 checksum of the other two files.

The backup is built in a hidden `.tmp-` directory beside the others and renamed into place only after every file is
written and flushed to disk. A rename within one directory is atomic, so if the process or the machine stops partway,
what's left is a hidden temporary directory, never something that looks like a complete backup. Restores and the
retention rules ignore anything without a manifest, and leftover temporary directories are cleared a day later.

## Copying a live database

SQLite's `VACUUM INTO` writes a consistent snapshot of the database to a new file while the server keeps running.
Writers aren't blocked: the database runs in WAL mode, so the copy reads one point in time while new changes go to the
write-ahead log. The copy is also compacted, which keeps backups small. Copying the database file directly would risk
catching it halfway through a write, or missing changes that are still only in the WAL.

## Keeping the database and files in step

The database and the files can't be copied at exactly the same instant, so the order matters. When someone uploads an
attachment, Chrysalis writes the file first and then the database row that points at it. A backup copies the database
first and the files second. So every attachment the copied database knows about already had its file on disk when the
files were archived. At worst the archive holds a few files uploaded a moment after the database copy, which nothing
refers to and which do no harm.

## Retention

After each backup, old ones are pruned. By default Chrysalis keeps the newest backup from each of the 7 most recent
days that have one, plus the newest from each of the 4 most recent weeks that have one, so about a month of history in
around ten backups. Counting days that *have* backups, rather than calendar days, means a server that was off for a
fortnight doesn't come back and delete its last good copies. The newest backup is always kept.

## The built-in schedule, or a CronJob

The design runs scheduled backups as a Kubernetes CronJob, or with a scheduler inside the server for people using
Docker Compose. The example Kubernetes overlays use the CronJob. The built-in scheduler also works on Kubernetes, and
it has one advantage worth knowing about.

It comes down to the volume. SQLite needs one writer, so the data lives on a ReadWriteOnce volume, which can be mounted
by pods on only one node at a time. A CronJob's pod has to land on the same node as the server to mount it, which the
component arranges with pod affinity. That works, but it adds a second pod that can fail to schedule (for example when
the node is full), and a second process opening the database. The server already has the database open and already
runs hourly maintenance, so checking once an hour whether today's backup exists is simple and correct, and a restart
never causes a missed day or a second backup.

The CronJob's strengths are that backups show up as Kubernetes jobs you can monitor and alert on, and run at a set
time. If your storage supports ReadWriteMany, you can drop its affinity.

## What a backup protects against

Backups sit on the same volume as the data by default. They protect against mistakes, a damaged database and a bad
upgrade. They don't protect against losing the volume. For that, copy them somewhere else, which the how-to covers.

Pushing copies to S3-compatible storage is a planned option in the design but isn't built yet. With the S3 storage
backend, when it arrives, attachments will be protected by the bucket's own versioning or replication instead.

## Restoring safely

A restore is the moment you can least afford a surprise, so it's careful:

- it checks every file against the manifest's checksums, and runs `PRAGMA integrity_check` on the database, before
  touching anything;
- it refuses to overwrite existing data unless you add `--force`, and even then it moves the old data aside rather than
  deleting it;
- it moves the database's `-wal` and `-shm` files along with it, because SQLite would otherwise try to replay an old
  write-ahead log into the restored database;
- it unpacks attachments so that no path in the archive can land outside the attachments directory;
- it applies any migrations the backup is missing, so a backup from an older version restores into a newer one.

The design requires restores to be tested, not just documented. Two tests do it on every change: a Go test that fills
an instance, backs it up while it's open, restores into a fresh directory and compares the contents, and
`scripts/restore-test.sh`, which does the same against the built binary through its HTTP API and admin commands.

## An alternative: continuous replication

[Litestream](https://litestream.io/) streams every change in a SQLite database to object storage as it happens, so a
restore can recover to within seconds rather than to last night. It's an option the maintainer may choose to evaluate,
not something Chrysalis uses: it needs object storage, runs as a second process alongside the server, and covers only
the database, so attachments would still need their own backup. For a household board, a daily snapshot of both
together is simpler and enough.
