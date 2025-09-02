[![join slack](https://img.shields.io/badge/slack-join-none.svg?style=social&logo=slack)](https://join.slack.com/t/nakama-social/shared_invite/zt-143j6bzie-spuCdq79xIZJQa4DaPb0uQ)
[![join discord](https://dcbadge.vercel.app/api/server/wkvwRa3pju?style=social)](https://discord.gg/wkvwRa3pju)

# Nakama

![banner](assets/banner.svg)

Source code of the next social network for anime fans. Still on development.

New work is being done at [ssr](https://github.com/nakamauwu/nakama/tree/ssr) branch.

## Docker build

The easies way to start the server and its dependencies is by using [Docker](https://www.docker.com/).

```bash
docker-compose up --build
```

## Building

Instead of Docker, you can also install and build stuff by yourself, that way you have complete control.

So, besides having [Go](https://golang.org) installed, the server needs [CockroachDB](https://www.cockroachlabs.com) and [NATS](https://nats.io).
Also [Node.js](https://nodejs.org) and [npm](https://nodejs.org) for the front-end.

First, you need a cockroach node running.

```bash
cockroach start-single-node --insecure --listen-addr 127.0.0.1
```

Then, you need to create the database and tables.

```bash
cat schema.sql | cockroach sql --insecure
```

Then you need to start NATS server.

```bash
nats-server
```

Now, you can build and run the server.

```bash
go build ./cmd/nakama
./nakama
```

For the front-end you need to install dependencies.

```bash
cd web/app
npm i
```

Now you can either build the entire front-end, or run a dev server:

```bash
npm run build
```

or

```bash
npm run dev
```

## Database Backups

Instructions to perform a database backup and restore.<br>
Have a running S3 compatible instance, then:

```sql
BACKUP DATABASE nakama INTO 's3://${S3_BUCKET}?AWS_ACCESS_KEY_ID=${S3_ACCESS_KEY_ID}&AWS_SECRET_ACCESS_KEY=${S3_SECRET_ACCESS_KEY}&AWS_REGION=${S3_REGION}&AWS_ENDPOINT=${S3_ENDPOINT}';
```

```sql
RESTORE DATABASE nakama FROM LATEST IN 's3://${S3_BUCKET}?AWS_ACCESS_KEY_ID=${S3_ACCESS_KEY_ID}&AWS_SECRET_ACCESS_KEY=${S3_SECRET_ACCESS_KEY}&AWS_REGION=${S3_REGION}&AWS_ENDPOINT=${S3_ENDPOINT}';
```

CockroachDB follows a `YY.R.PP` **year**, **release** and **patch** versioning system. After each release, we should perform a backup before upgrading.

---

[Eva Icons](https://github.com/akveo/eva-icons) are being used in the front-end. Thank you.
