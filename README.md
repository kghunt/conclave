# Conclave

A self-hosted team chat application. Organised around **Spaces** (subject-focused communities) containing **Channels**, with direct messaging, voice calls, file sharing, and role-based permissions.

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
- Push notifications (web push) for mentions and DMs — works in-browser and as an installed PWA

**Spaces & Channels**
- Public spaces (open join) or invite-only with shareable invite links
- Space discovery page for finding public communities
- Space rules — shown to users before they join or request access
- Join request flow for spaces that require admin approval
- Custom roles with names and colours — assigned per member
- Per-channel visibility and write permissions per role
- Default "everyone" role with overridable defaults
- Kick, ban, and unban members
- Threaded channels for organised discussion

**Voice**
- Voice channels with multi-party audio (powered by LiveKit)
- Direct voice calls between friends

**Direct Messages & Friends**
- One-to-one DMs (friends only)
- Friend requests and friend list
- Unread DM indicators

**Authentication**
- Google OAuth sign-in
- Username/password (local) accounts — can be used alongside or instead of Google
- Configurable registration: **open** (anyone can register), **invite-only**, or **closed**
- Registration invite codes — admins can create codes with custom uses and expiry; users can generate one 1-use/1-day code per day to invite friends
- The first user to register is always allowed through, regardless of registration mode — so local-auth-only instances can self-host without a chicken-and-egg invite problem

**Users & Presence**
- User profiles with display name, bio, and avatar (auto-generated if not set)
- Online / away / offline presence indicators
- Game status — connect the desktop companion app to automatically show what game you're playing

**Desktop Presence App**
- Optional Tauri-based system tray companion (Windows, macOS, Linux)
- Detects running game processes and reports your status in real-time
- Connect in one click from **Edit Profile → Desktop Presence App** via a `conclave://` deep link
- Game name appears below your display name in member lists across all shared spaces

**Instance Admin**
- Authentication settings — enable/disable Google OAuth and local auth independently; set registration mode
- Registration invite code management (create, list, delete)
- Ban/unban users instance-wide
- Message and inactive-space retention policies
- Max video upload size (set to 0 to disable video uploads)
- Custom theme — accent colour, background, sidebar, panel colours, etc.
- User list with search

---

## Requirements

- [Docker](https://docs.docker.com/get-docker/) with the Compose plugin (`docker compose version`)
- A Google OAuth 2.0 credential **or** local auth enabled (see [Authentication modes](#authentication-modes))

---

## Setup

### 1. Authentication

Conclave supports Google OAuth, username/password accounts, or both at once. You configure which are active from the Instance Admin panel after first login.

#### Google OAuth (optional but recommended for easy onboarding)

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

If you don't want Google OAuth at all, set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` to placeholder values (e.g. `disabled`) and disable Google auth in the admin panel after first login. The server currently requires these env vars to start even if Google auth is turned off — this will be improved in a future release.

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

# Optional: your email — grants access to the instance admin panel
# Must match the email on your Google account or your local account
INSTANCE_ADMIN_EMAIL=you@example.com

# Optional: voice channels (LiveKit)
LIVEKIT_URL=wss://your-livekit-server:7880
LIVEKIT_KEY=your-livekit-api-key
LIVEKIT_SECRET=your-livekit-api-secret

# Optional: web push notifications
VAPID_PUBLIC_KEY=
VAPID_PRIVATE_KEY=
VAPID_EMAIL=admin@yourdomain.com
```

Generate a strong JWT secret:
```bash
openssl rand -base64 32
```

Generate VAPID keys for push notifications:
```bash
npx web-push generate-vapid-keys
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

### 4. First login

The very first account registered on a fresh instance is always allowed through, regardless of the registration mode setting. Register with the email that matches `INSTANCE_ADMIN_EMAIL` to gain admin access immediately.

### 5. Stop

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
INSTANCE_ADMIN_EMAIL=you@example.com
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

## Voice channels

Voice requires a [LiveKit](https://livekit.io) server. LiveKit offers a managed cloud service (generous free tier) or you can self-host.

Set the following in `.env`:

```env
LIVEKIT_URL=wss://your-livekit-server:7880
LIVEKIT_KEY=your-api-key
LIVEKIT_SECRET=your-api-secret
```

Without these, voice channels are present in the UI but calls will not connect.

---

## Push notifications

Web push requires VAPID keys. Generate them once and store them in `.env` — they must stay the same across restarts or existing browser subscriptions will stop working.

```bash
npx web-push generate-vapid-keys
```

```env
VAPID_PUBLIC_KEY=BExamplePublicKey...
VAPID_PRIVATE_KEY=ExamplePrivateKey...
VAPID_EMAIL=admin@yourdomain.com
```

Users opt in to notifications per-browser from the bell icon in the bottom-left user bar.

---

## Desktop presence app

The optional presence companion is a small system-tray app that detects which game you're running and reports it to Conclave so other members can see your game status in real-time.

**Install**

Conclave serves the desktop app directly — no GitHub account needed. Go to **Edit Profile → Desktop Presence App** and click the download button for your platform. The button only appears once an admin has placed the binaries on the server (see below).

**Connect**

1. Download and install the app from Edit Profile
2. Click **Connect installed app** — the browser opens a `conclave://` deep link that configures the app automatically

**How it works**

- Scans running processes every 30 seconds against a list of known game executables
- Reports the current game (or clears it when no game is running) to your Conclave instance via a Bearer token — no passwords stored
- The token can be revoked at any time from Edit Profile → **Disconnect**

**Hosting the downloads on your instance**

Each release tag (`desktop-v*`) triggers a GitHub Actions build that produces canonically-named files and attaches them to the release. Copy the files for your platform(s) into `./data/downloads/` on your server:

| File | Platform |
|---|---|
| `conclave-presence-windows-x64.exe` | Windows |
| `conclave-presence-macos-x64.dmg` | macOS (Intel) |
| `conclave-presence-macos-arm64.dmg` | macOS (Apple Silicon) |
| `conclave-presence-linux-x64.AppImage` | Linux |

Conclave serves them from `/downloads/` and the Edit Profile page automatically shows a download button for the user's detected platform.

**Build from source**

```bash
cd desktop
npm install
npm run build   # produces platform installers in src-tauri/target/release/bundle/
```

Requires Rust (stable), Node 20+, and the [Tauri prerequisites](https://tauri.app/start/prerequisites/) for your platform.

---

## Authentication modes

Configure from **Instance Admin → Authentication** after first login.

| Mode | Behaviour |
|---|---|
| **Open** | Anyone can create an account |
| **Invite only** | New accounts require a valid invite code |
| **Closed** | No new registrations |

**Invite codes** can be created by admins (any uses, any expiry) from the admin panel. Regular users can generate one code per day (1-use, 24-hour expiry) from the DM sidebar — useful for letting friends in without giving them the admin panel.

---

## Instance admin

Set `INSTANCE_ADMIN_EMAIL` to your email in `.env`. After logging in with that account the **⚙ admin panel** appears in the bottom-left.

| Setting | Description |
|---|---|
| **Google auth** | Enable or disable Google OAuth sign-in |
| **Local auth** | Enable or disable username/password accounts |
| **Registration mode** | Open / invite-only / closed |
| **Registration invites** | Create and manage invite codes |
| **User list** | Search all accounts, ban/unban instance-wide |
| **Message retention** | Delete messages older than N days (`0` = keep forever) |
| **Inactive space retention** | Delete spaces with no activity for N days (`0` = never) |
| **Max video upload size** | Maximum MB for video uploads (`0` = disable video uploads, default 50 MB) |
| **Theme** | Accent colour, background, sidebar, panel colours |

Cleanup runs automatically every 24 hours and at startup.

---

## Development (without Docker)

Prerequisites: Go 1.22+, Node 20+, a running PostgreSQL and Redis instance.

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

The Go binary is built in a multi-stage Dockerfile: Node builds the SvelteKit frontend, Go compiles the backend, both land in a minimal Alpine image (~30 MB).

---

## Licence

MIT
