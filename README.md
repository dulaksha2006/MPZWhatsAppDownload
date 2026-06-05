# 🤖 WhatsApp Bot — Go + whatsmeow + Railway

A production-ready WhatsApp bot built with **Go** and **whatsmeow**, deployable on **Railway** with a beautiful web dashboard for QR code scanning.

---

## ✨ Features

- 🖥️ **Web Dashboard** — Tailwind CSS UI at your Railway URL with animated QR scanner
- 📱 **QR Code Login** — QR code with WhatsApp logo at center for easy scanning
- 🔔 **Owner Notification** — Sends a styled message to the owner when bot connects
- 💬 **Commands**:
  - `/jid` — Returns the JID (ID) of the current chat and sender
  - `/start` — Downloads and sends a movie poster as a document (with upload progress message)
- 💾 **Persistent Session** — SQLite-backed session survives restarts

---

## 📁 Project Structure

```
whatsapp-bot/
├── main.go          # Core bot logic
├── config.json      # Bot configuration
├── go.mod           # Go modules
├── go.sum           # Module checksums
├── Dockerfile       # Docker build for Railway
├── railway.toml     # Railway deployment config
├── .gitignore       # Git ignore rules
└── store/           # Auto-created SQLite session (gitignored)
```

---

## ⚙️ Configuration — `config.json`

```json
{
  "owner_number": "94771234567",
  "bot_name": "MyWhatsAppBot",
  "logo_direct_url": "https://upload.wikimedia.org/wikipedia/commons/6/6b/WhatsApp.svg"
}
```

| Field | Description |
|-------|-------------|
| `owner_number` | Your WhatsApp number in international format (no `+`) |
| `bot_name` | Display name shown in the web UI |
| `logo_direct_url` | URL of logo shown on the web page |

---

## 🚀 Deploy to Railway

### 1. Push to GitHub

```bash
git init
git add .
git commit -m "Initial commit"
git remote add origin https://github.com/YOUR_USERNAME/whatsapp-bot.git
git push -u origin main
```

### 2. Create Railway Project

1. Go to [railway.app](https://railway.app) and log in
2. Click **New Project** → **Deploy from GitHub repo**
3. Select your repository
4. Railway auto-detects the `Dockerfile` and deploys

### 3. Add Persistent Volume (Important!)

To keep your WhatsApp session across redeploys:

1. In Railway, go to your service → **Volumes**
2. Add a volume mounted at `/app/store`
3. This prevents re-scanning the QR code every time

### 4. Scan QR Code

1. Visit your Railway URL (e.g., `https://your-bot.up.railway.app`)
2. Open WhatsApp → **Settings** → **Linked Devices** → **Link a Device**
3. Scan the QR code shown on the page
4. Bot sends a confirmation message to the owner number ✅

---

## 🛠️ Local Development

### Prerequisites

- Go 1.21+
- GCC (for SQLite CGO)
- Make sure `gcc` is available: `sudo apt install build-essential` (Linux)

### Run locally

```bash
# Install dependencies
go mod tidy

# Run the bot
go run main.go
```

Visit `http://localhost:8080` to see the dashboard.

---

## 💬 Bot Commands

### `/jid`
Send in any chat to get the JID of that chat.

**Response:**
```
╔══════════════════╗
║   📌 JID Info    ║
╠══════════════════╣
║ Chat JID:
║ `1234567890@s.whatsapp.net`
║
║ Sender JID:
║ `0987654321@s.whatsapp.net`
╚══════════════════╝
```

### `/start`
Downloads `https://image.tmdb.org/t/p/original/n4ULlcixzO36t1LKqpPmphD7VGl.jpg` and sends it as a document.

First sends:
```
📤 ගොනුව උඩුගත කරමින් පවතී...
Please wait while the file is being uploaded ⏳
```

Then sends the image as `movie_poster.jpg` document.

---

## 🔗 API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Web dashboard with QR code |
| `GET /qr` | Raw QR code PNG image |
| `GET /status` | JSON status: `connected`, `waiting_qr`, `disconnected` |

---

## 🏗️ Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.21 |
| WhatsApp | [whatsmeow](https://github.com/tulir/whatsmeow) |
| Database | SQLite (via go-sqlite3) |
| Web UI | Tailwind CSS |
| Deployment | Railway + Docker |

---

## 📝 Notes

- **Session persistence**: The `store/` folder contains your WhatsApp session. Mount it as a Railway volume to avoid re-scanning QR on every deploy.
- **Owner number format**: Use international format without `+` sign (e.g., `94771234567` for Sri Lanka `+94 77 123 4567`).
- **QR expiry**: WhatsApp QR codes expire every ~20 seconds. The dashboard auto-refreshes them.

---

## 📄 License

MIT License — feel free to use and modify.
