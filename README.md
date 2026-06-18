# Conclave

A self-hosted team chat application. Organised around **Spaces** (subject-focused communities) containing **Channels**, with direct messaging, file/image sharing, and role-based permissions.

Built with Go, SvelteKit, PostgreSQL, and Redis. Runs as a single `docker compose up`.

---

## Features

- Google OAuth sign-in
- Spaces with public/invite-only access
- Channels with real-time messaging (WebSocket)
- Direct messages
- Image uploads and inline rendering (paste or file picker)
- Edit and delete your own messages
- Role system: Owner → Admin → Member
- User profiles with avatars (auto-generated if not set)
- Instance admin panel for data retention settings

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

## Deploying publicly

1. Point a domain at your server
2. Update `.env`:
   ```env
   BASE_URL=https://chat.yourdomain.com
   ```
3. Update your Google OAuth redirect URI to `https://chat.yourdomain.com/api/auth/callback`
4. Put a reverse proxy (Caddy, nginx) in front of port 8080 to handle TLS

### Caddy example

```
chat.yourdomain.com {
    reverse_proxy localhost:8080
}
```

Caddy handles TLS automatically via Let's Encrypt.

---

## Instance admin

If you set `INSTANCE_ADMIN_EMAIL` in your `.env`, that Google account gets access to the **Instance Admin** panel (⚙ icon in the bottom-left after logging in).

From there you can configure:

| Setting | Description |
|---|---|
| **Message retention** | Delete messages older than N days (`0` = keep forever) |
| **Inactive space retention** | Delete spaces with no activity for N days (`0` = never) |

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
