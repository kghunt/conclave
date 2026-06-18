# Conclave

A self-hosted team chat application. Organised around **Spaces** (subject-focused communities) containing **Channels**, with direct messaging, file/image sharing, and role-based permissions.

Built with Go, SvelteKit, PostgreSQL, and Redis. Runs as a single `docker compose up`.

---

## Features

**Messaging**
- Real-time messaging via WebSocket (text channels and threads)
- Reply to messages with quoted context
- Edit and delete your own messages (admins can delete any)
- Emoji reactions — click to toggle, notifies the original author
- Inline image rendering (paste, drag-and-drop, or file picker)
- Video clip uploads (mp4, webm, mov) with configurable size cap
- @mention notifications with sound alerts and channel highlights

**Spaces & Channels**
- Public spaces (open join) or invite-only with shareable invite links
- Space discovery page for finding public communities
- Join request flow for spaces that require approval
- Custom roles with names and colours — assigned per member
- Per-channel visibility and write permissions per role
- Default "everyone" role with overridable defaults
- Kick, ban, and unban members

**Direct Messages**
- One-to-one DMs (friends only)
- Friend requests and friend list

**Users**
- Google OAuth sign-in
- User profiles with avatars (auto-generated if not set)
- Online/away presence indicators

**Instance Admin**
- Message and inactive-space retention policies
- Max video upload size (set to 0 to disable video uploads)
- Ban/unban users instance-wide
- Custom theme (accent colour, background, sidebar, etc.)

---

## Requirements

- [Docker](https://docs.docker.com/get-docker/) with the Compose plugin (`docker compose version`)
- A Google Cloud project with OAuth 2.0 credentials

---

## Setup

### 1. Google OAuth credentials

1. Go to [console.cloud.google.com](https://console.cloud.google.com)
2. Create a project (or use an existing one)
3. Navigate to **APIs & Services → Credentials → Create Credentials → OAuth 2.0 Client ID**
4. Application type: **Web application**
5. Add to **Authorised redirect URIs**:
   ```
   http://localhost:8080/api/auth/callback
   ```
   (Replace `localhost:8080` with your domain if deploying publicly)
6. Add to **Authorised JavaScript origins**:
   ```
   http://localhost:8080
   ```
7. Copy the **Client ID** and **Client Secret**

### 2. Environment file

Copy the example and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:

```env
# Required
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-google-client-secret
JWT_SECRET=a-long-random-string-at-least-32-chars

# URL the app is reachable at (no trailing slash)
BASE_URL=http://localhost:8080

# Optional: your Google account email — grants access to the instance admin panel
INSTANCE_ADMIN_EMAIL=you@gmail.com
```

Generate a strong JWT secret:
```bash
openssl rand -base64 32
```

### 3. Run

```bash
docker compose up --build
```

Open [http://localhost:8080](http://localhost:8080).

The first `--build` compiles the Go binary and the SvelteKit frontend inside Docker — subsequent starts are faster:

```bash
docker compose up
```

### 4. Stop

```bash
docker compose down          # stop containers, keep data
docker compose down -v       # stop and delete all data (destructive)
```

---

## Production deployment

There are two ways to run Conclave in production. Using pre-built images is recommended — it avoids needing build tools on the server and makes updates a one-liner.

### Option A: Pre-built images (recommended)

Every push to `main` and every `v*` tag publishes a Docker image to GitHub Container Registry. To deploy on a server:

**1. Copy the production compose and env template**

```bash
curl -O https://raw.githubusercontent.com/kghunt/conclave/main/docker-compose.prod.yml
curl -O https://raw.githubusercontent.com/kghunt/conclave/main/.env.prod.example
cp .env.prod.example .env
```

**2. Fill in `.env`**

```env
# Set the same password in both lines
POSTGRES_PASSWORD=a-strong-random-password
DATABASE_URL=postgres://conclave:a-strong-random-password@postgres:5432/conclave?sslmode=disable

JWT_SECRET=a-long-random-string-at-least-32-chars
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
BASE_URL=https://chat.yourdomain.com
INSTANCE_ADMIN_EMAIL=you@gmail.com   # optional
```

Generate secrets:
```bash
openssl rand -base64 32   # run twice — once for each secret
```

**3. Update your Google OAuth redirect URI**

In Google Cloud Console → OAuth credentials, add:
```
https://chat.yourdomain.com/api/auth/callback
```

**4. Start**

```bash
docker compose -f docker-compose.prod.yml up -d
```

**Updating to the latest release**

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

**Pinning to a specific version**

```bash
TAG=v1.2.3 docker compose -f docker-compose.prod.yml up -d
```

---

### Option B: Build from source

Use the standard `docker-compose.yml` if you want to build the image on the server itself (requires Docker with Buildx):

```bash
cp .env.example .env   # fill in values
docker compose up --build -d
```

---

### Reverse proxy

Conclave listens on port 8080. Put a reverse proxy in front to handle TLS.

> **Important:** WebSocket support must be enabled on your proxy — the real-time chat requires a persistent `/ws` connection.

#### Nginx Proxy Manager

1. In NPM, create a new **Proxy Host**:
   - **Domain**: `chat.yourdomain.com`
   - **Scheme**: `http`
   - **Forward Hostname/IP**: your server's IP (or `172.17.0.1` if NPM is in Docker on the same host)
   - **Forward Port**: `8080`
   - **Websockets Support**: ✅ enable this — required for chat
2. On the **SSL** tab, request a Let's Encrypt certificate
3. Set `BASE_URL=https://chat.yourdomain.com` in your `.env` and restart the app

> If NPM and Conclave are both running in Docker on the same host, the easiest approach is to put them on a shared Docker network rather than routing through the host IP. Alternatively, exposing port `8080` to the host (the default) and pointing NPM at the host IP works fine.

#### Caddy

```
chat.yourdomain.com {
    reverse_proxy localhost:8080
}
```

Caddy handles TLS and WebSocket proxying automatically.

---

## Instance admin

If you set `INSTANCE_ADMIN_EMAIL` in your `.env`, that Google account gets access to the **Instance Admin** panel (⚙ icon in the bottom-left after logging in).

From there you can configure:

| Setting | Description |
|---|---|
| **Message retention** | Delete messages older than N days (`0` = keep forever) |
| **Inactive space retention** | Delete spaces with no activity for N days (`0` = never) |
| **Max video upload size** | Maximum MB for video uploads (`0` = disable video uploads, default 50MB) |

Cleanup runs automatically every 24 hours and at startup.

---

## Development (without Docker)

Prerequisites: Go 1.25+, Node 20+, a running PostgreSQL and Redis instance.

```bash
# Backend
cd backend
cp ../.env.example ../.env   # fill in DATABASE_URL, REDIS_URL, etc.
go run ./cmd/server

# Frontend (in a separate terminal)
cd frontend
npm install
npm run dev
```

The Vite dev server proxies `/api` and `/ws` to the Go backend on port 8080 and supports hot-reload.

---

## Architecture

```
docker compose
├── app      — Go binary (API + WebSocket + serves built frontend)
├── postgres — persistent data
└── redis    — WebSocket pub/sub (future: multi-instance scaling)
```

The Go binary is built in a multi-stage Dockerfile: Node builds the SvelteKit frontend, Go compiles the backend, both land in a minimal Alpine image (~30MB).

---

## Licence

MIT
