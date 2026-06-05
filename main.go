package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Config holds bot configuration
type Config struct {
	OwnerNumber  string `json:"owner_number"`
	BotName      string `json:"bot_name"`
	LogoDirectURL string `json:"logo_direct_url"`
}

var (
	cfg        Config
	client     *whatsmeow.Client
	qrCodeData []byte // stores latest QR code PNG bytes
	isLoggedIn bool
)

func loadConfig() error {
	f, err := os.Open("config.json")
	if err != nil {
		return fmt.Errorf("failed to open config.json: %w", err)
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(&cfg)
}

// generateQRWithLogo creates a QR code PNG with a WhatsApp logo in the center
func generateQRWithLogo(qrText string) ([]byte, error) {
	// Generate base QR code at high resolution
	qr, err := qrcode.New(qrText, qrcode.High)
	if err != nil {
		return nil, err
	}
	qr.DisableBorder = false

	qrPNG, err := qr.PNG(400)
	if err != nil {
		return nil, err
	}

	// Decode QR PNG
	qrImg, _, err := image.Decode(bytes.NewReader(qrPNG))
	if err != nil {
		return nil, err
	}

	// Create RGBA version of QR
	bounds := qrImg.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, qrImg, bounds.Min, draw.Src)

	// Draw WhatsApp logo (green circle with W shape) in center
	cx := bounds.Max.X / 2
	cy := bounds.Max.Y / 2
	r := bounds.Max.X / 8

	// Draw white background circle (slightly bigger)
	drawFilledCircle(dst, cx, cy, r+6, color.RGBA{255, 255, 255, 255})

	// Draw green circle
	whatsappGreen := color.RGBA{37, 211, 102, 255}
	drawFilledCircle(dst, cx, cy, r, whatsappGreen)

	// Draw "W" letter in white inside circle
	drawW(dst, cx, cy, r, color.RGBA{255, 255, 255, 255})

	// Encode back to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawFilledCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawW(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	thickness := r / 5
	if thickness < 3 {
		thickness = 3
	}
	// Draw a bold "W" using thick lines inside the circle
	// W has 5 strokes: two outer down, two inner up, one middle
	top := cy - r*5/8
	bot := cy + r*5/8
	mid := cy + r/8

	x1 := cx - r*5/8
	x2 := cx - r*3/10
	x3 := cx
	x4 := cx + r*3/10
	x5 := cx + r*5/8

	drawThickLine(img, x1, top, x2, bot, thickness, c)
	drawThickLine(img, x2, bot, x3, mid, thickness, c)
	drawThickLine(img, x3, mid, x4, bot, thickness, c)
	drawThickLine(img, x4, bot, x5, top, thickness, c)
}

func drawThickLine(img *image.RGBA, x0, y0, x1, y1, thickness int, c color.RGBA) {
	dx := x1 - x0
	dy := y1 - y0
	steps := abs(dx)
	if abs(dy) > steps {
		steps = abs(dy)
	}
	if steps == 0 {
		return
	}
	xInc := float64(dx) / float64(steps)
	yInc := float64(dy) / float64(steps)
	x := float64(x0)
	y := float64(y0)
	half := thickness / 2
	for i := 0; i <= steps; i++ {
		for ty := -half; ty <= half; ty++ {
			for tx := -half; tx <= half; tx++ {
				img.SetRGBA(int(x)+tx, int(y)+ty, c)
			}
		}
		x += xInc
		y += yInc
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// sendOwnerMessage sends a formatted message to the owner
func sendOwnerMessage(msg string) {
	if client == nil || !client.IsConnected() {
		return
	}
	ownerJID, err := types.ParseJID(cfg.OwnerNumber + "@s.whatsapp.net")
	if err != nil {
		fmt.Println("Invalid owner JID:", err)
		return
	}
	_, err = client.SendMessage(context.Background(), ownerJID, &waProto.Message{
		Conversation: proto.String(msg),
	})
	if err != nil {
		fmt.Println("Failed to send owner message:", err)
	}
}

// eventHandler handles all WhatsApp events
func eventHandler(evt interface{}) {
	switch v := evt.(type) {

	case *events.Connected:
		isLoggedIn = true
		fmt.Println("✅ WhatsApp Connected!")
		time.Sleep(2 * time.Second) // wait for session to stabilize
		msg := fmt.Sprintf(
			"╔══════════════════════════╗\n"+
				"║   🤖 *%s*   ║\n"+
				"╠══════════════════════════╣\n"+
				"║  ✅ Bot Connected!        ║\n"+
				"║  🕐 Time: %s  ║\n"+
				"║  🌐 Platform: Railway     ║\n"+
				"╚══════════════════════════╝\n\n"+
				"_Bot is ready and running! 🚀_",
			cfg.BotName,
			time.Now().Format("15:04:05"),
		)
		sendOwnerMessage(msg)

	case *events.Message:
		handleMessage(v)
	}
}

// handleMessage processes incoming messages
func handleMessage(evt *events.Message) {
	if evt.Info.IsFromMe {
		return
	}

	msg := evt.Message
	text := ""
	if msg.GetConversation() != "" {
		text = msg.GetConversation()
	} else if msg.GetExtendedTextMessage() != nil {
		text = msg.GetExtendedTextMessage().GetText()
	}

	text = strings.TrimSpace(text)
	sender := evt.Info.Sender
	chat := evt.Info.Chat

	fmt.Printf("📩 [%s] Message from %s: %s\n", time.Now().Format("15:04"), sender.String(), text)

	switch {
	case text == "/jid":
		handleJIDCommand(chat, sender)

	case text == "/start":
		handleStartCommand(chat)
	}
}

// handleJIDCommand sends back the JID of the chat
func handleJIDCommand(chat, sender types.JID) {
	msg := fmt.Sprintf(
		"╔══════════════════╗\n"+
			"║   📌 *JID Info*    ║\n"+
			"╠══════════════════╣\n"+
			"║ *Chat JID:*\n"+
			"║ `%s`\n"+
			"║\n"+
			"║ *Sender JID:*\n"+
			"║ `%s`\n"+
			"╚══════════════════╝",
		chat.String(),
		sender.String(),
	)
	_, err := client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String(msg),
	})
	if err != nil {
		fmt.Println("Failed to send JID message:", err)
	}
}

// handleStartCommand downloads the image and sends it as a document
func handleStartCommand(chat types.JID) {
	// Send "uploading" status message first
	statusMsg := "📤 *ගොනුව උඩුගත කරමින් පවතී...*\n_Please wait while the file is being uploaded_ ⏳"
	_, _ = client.SendMessage(context.Background(), chat, &waProto.Message{
		Conversation: proto.String(statusMsg),
	})

	// Download the image
	imageURL := "https://image.tmdb.org/t/p/original/n4ULlcixzO36t1LKqpPmphD7VGl.jpg"
	fmt.Println("📥 Downloading image from:", imageURL)

	resp, err := http.Get(imageURL)
	if err != nil {
		_, _ = client.SendMessage(context.Background(), chat, &waProto.Message{
			Conversation: proto.String("❌ Failed to download file."),
		})
		return
	}
	defer resp.Body.Close()

	imgData, err := io.ReadAll(resp.Body)
	if err != nil {
		_, _ = client.SendMessage(context.Background(), chat, &waProto.Message{
			Conversation: proto.String("❌ Failed to read file."),
		})
		return
	}

	// Upload to WhatsApp
	uploaded, err := client.Upload(context.Background(), imgData, whatsmeow.MediaDocument)
	if err != nil {
		_, _ = client.SendMessage(context.Background(), chat, &waProto.Message{
			Conversation: proto.String("❌ Failed to upload file to WhatsApp."),
		})
		return
	}

	fileName := "movie_poster.jpg"
	mimeType := "image/jpeg"
	fileSize := uint64(len(imgData))

	docMsg := &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(fileSize),
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(fileName),
			Title:         proto.String("🎬 Movie Poster"),
		},
	}

	_, err = client.SendMessage(context.Background(), chat, docMsg)
	if err != nil {
		fmt.Println("Failed to send document:", err)
		_, _ = client.SendMessage(context.Background(), chat, &waProto.Message{
			Conversation: proto.String("❌ Failed to send document."),
		})
		return
	}

	fmt.Println("✅ Document sent successfully!")
}

// startWebServer runs the HTTP server for Railway
func startWebServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	// Serve main page
	mux.HandleFunc("/", serveMainPage)

	// QR code endpoint
	mux.HandleFunc("/qr", serveQRCode)

	// Status API endpoint
	mux.HandleFunc("/status", serveStatus)

	fmt.Printf("🌐 Web server starting on port %s\n", port)
	go func() {
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			fmt.Println("Web server error:", err)
		}
	}()
}

func serveStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	status := "disconnected"
	if isLoggedIn {
		status = "connected"
	} else if len(qrCodeData) > 0 {
		status = "waiting_qr"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   status,
		"bot_name": cfg.BotName,
	})
}

func serveQRCode(w http.ResponseWriter, r *http.Request) {
	if isLoggedIn {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
		return
	}
	if len(qrCodeData) == 0 {
		http.Error(w, "QR not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(qrCodeData)
}

func serveMainPage(w http.ResponseWriter, r *http.Request) {
	html := generateHTML()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func generateHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>` + cfg.BotName + ` | WhatsApp Bot</title>
  <script src="https://cdn.tailwindcss.com"></script>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link href="https://fonts.googleapis.com/css2?family=Space+Mono:wght@400;700&family=Outfit:wght@300;400;600;700;900&display=swap" rel="stylesheet" />
  <style>
    :root {
      --wa-green: #25d366;
      --wa-dark: #075e54;
      --wa-light: #dcf8c6;
    }
    * { box-sizing: border-box; }
    body {
      font-family: 'Outfit', sans-serif;
      background: #0a0a0a;
      min-height: 100vh;
      overflow-x: hidden;
    }
    .noise-bg {
      position: fixed; inset: 0; z-index: 0; pointer-events: none;
      background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.04'/%3E%3C/svg%3E");
      opacity: 0.5;
    }
    .glow-green { box-shadow: 0 0 60px rgba(37,211,102,0.25), 0 0 120px rgba(37,211,102,0.1); }
    .glow-text { text-shadow: 0 0 30px rgba(37,211,102,0.5); }
    .qr-container {
      background: linear-gradient(135deg, rgba(37,211,102,0.08), rgba(7,94,84,0.12));
      border: 1px solid rgba(37,211,102,0.3);
      border-radius: 24px;
      padding: 2rem;
      position: relative;
      overflow: hidden;
    }
    .qr-container::before {
      content: '';
      position: absolute; inset: -2px;
      background: linear-gradient(135deg, #25d366, #075e54, transparent, #25d366);
      border-radius: 26px;
      z-index: -1;
      opacity: 0.4;
      animation: borderSpin 4s linear infinite;
    }
    @keyframes borderSpin {
      0% { transform: rotate(0deg); }
      100% { transform: rotate(360deg); }
    }
    .status-dot {
      width: 10px; height: 10px;
      border-radius: 50%;
      display: inline-block;
      animation: pulse 2s infinite;
    }
    .status-dot.connecting { background: #f59e0b; box-shadow: 0 0 8px #f59e0b; }
    .status-dot.connected { background: #25d366; box-shadow: 0 0 8px #25d366; }
    .status-dot.waiting { background: #3b82f6; box-shadow: 0 0 8px #3b82f6; }
    @keyframes pulse {
      0%, 100% { opacity: 1; transform: scale(1); }
      50% { opacity: 0.6; transform: scale(1.3); }
    }
    .cmd-card {
      background: rgba(255,255,255,0.03);
      border: 1px solid rgba(255,255,255,0.07);
      border-radius: 12px;
      padding: 1rem 1.25rem;
      transition: all 0.2s ease;
    }
    .cmd-card:hover {
      background: rgba(37,211,102,0.07);
      border-color: rgba(37,211,102,0.3);
      transform: translateX(4px);
    }
    .scan-line {
      position: absolute; left: 0; right: 0;
      height: 2px;
      background: linear-gradient(90deg, transparent, #25d366, transparent);
      animation: scan 3s linear infinite;
    }
    @keyframes scan {
      0% { top: 0; opacity: 0; }
      10% { opacity: 1; }
      90% { opacity: 1; }
      100% { top: 100%; opacity: 0; }
    }
    .slide-up {
      animation: slideUp 0.6s cubic-bezier(0.16,1,0.3,1) forwards;
      opacity: 0; transform: translateY(30px);
    }
    @keyframes slideUp {
      to { opacity: 1; transform: translateY(0); }
    }
    #qr-img { transition: opacity 0.4s ease; }
    .connected-badge {
      background: linear-gradient(135deg, #25d366, #075e54);
      border-radius: 50%;
      width: 80px; height: 80px;
      display: flex; align-items: center; justify-content: center;
      font-size: 2rem;
      animation: popIn 0.5s cubic-bezier(0.34,1.56,0.64,1);
    }
    @keyframes popIn {
      from { transform: scale(0); }
      to { transform: scale(1); }
    }
  </style>
</head>
<body class="text-white">
  <div class="noise-bg"></div>

  <div class="relative z-10 min-h-screen flex flex-col">

    <!-- Header -->
    <header class="px-6 py-5 flex items-center justify-between border-b border-white/5">
      <div class="flex items-center gap-3">
        <div class="w-9 h-9 rounded-xl flex items-center justify-center" style="background: linear-gradient(135deg, #25d366, #075e54);">
          <svg viewBox="0 0 24 24" fill="white" class="w-5 h-5">
            <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
          </svg>
        </div>
        <span class="font-bold text-lg tracking-tight">` + cfg.BotName + `</span>
      </div>
      <div class="flex items-center gap-2 text-sm" id="header-status">
        <span class="status-dot waiting" id="header-dot"></span>
        <span id="header-status-text" class="text-white/60">Initializing...</span>
      </div>
    </header>

    <!-- Main Content -->
    <main class="flex-1 flex flex-col lg:flex-row gap-8 items-start justify-center px-6 py-10 max-w-5xl mx-auto w-full">

      <!-- Left: QR Section -->
      <div class="w-full lg:w-auto slide-up" style="animation-delay:0.1s">
        <div class="qr-container max-w-sm mx-auto lg:mx-0">
          <div class="text-center mb-4">
            <p class="text-white/50 text-xs uppercase tracking-widest font-mono mb-1">Scan to Connect</p>
            <h2 class="text-xl font-bold">WhatsApp Link</h2>
          </div>

          <!-- QR Display -->
          <div class="relative rounded-2xl overflow-hidden bg-white p-3" id="qr-wrapper" style="aspect-ratio:1; max-width: 280px; margin: 0 auto;">
            <div class="scan-line" id="scan-line"></div>

            <!-- Loading state -->
            <div id="qr-loading" class="absolute inset-0 flex flex-col items-center justify-center bg-white rounded-xl">
              <div class="w-10 h-10 border-4 border-green-500 border-t-transparent rounded-full animate-spin mb-3"></div>
              <p class="text-gray-500 text-xs font-mono">Generating QR...</p>
            </div>

            <!-- QR Image -->
            <img id="qr-img" src="/qr" alt="QR Code" class="w-full h-full object-contain rounded-xl opacity-0"
              onload="this.style.opacity='1'; document.getElementById('qr-loading').style.display='none';"
              onerror="handleQRError()" />

            <!-- Connected overlay -->
            <div id="connected-overlay" class="absolute inset-0 hidden flex-col items-center justify-center bg-white rounded-xl">
              <div class="connected-badge mb-3">✅</div>
              <p class="text-gray-800 font-bold text-lg">Connected!</p>
              <p class="text-gray-500 text-sm">Bot is running</p>
            </div>
          </div>

          <p class="text-white/40 text-xs text-center mt-4 font-mono">Open WhatsApp → Linked Devices → Link a Device</p>
        </div>
      </div>

      <!-- Right: Info Panel -->
      <div class="flex-1 space-y-6 slide-up w-full" style="animation-delay:0.25s">

        <!-- Status Card -->
        <div class="rounded-2xl p-5" style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.07);">
          <h3 class="text-xs uppercase tracking-widest text-white/40 font-mono mb-3">Connection Status</h3>
          <div class="flex items-center gap-3">
            <span class="status-dot waiting" id="status-dot"></span>
            <span id="status-text" class="font-semibold text-lg">Waiting for QR scan...</span>
          </div>
          <div class="mt-3 h-1 rounded-full bg-white/5 overflow-hidden">
            <div id="progress-bar" class="h-full rounded-full transition-all duration-1000" style="width: 33%; background: linear-gradient(90deg, #25d366, #075e54);"></div>
          </div>
        </div>

        <!-- Commands -->
        <div class="rounded-2xl p-5" style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.07);">
          <h3 class="text-xs uppercase tracking-widest text-white/40 font-mono mb-4">Bot Commands</h3>
          <div class="space-y-3">
            <div class="cmd-card">
              <div class="flex items-center gap-3">
                <span class="font-mono text-green-400 font-bold text-sm">/start</span>
                <span class="text-white/30 text-xs">→</span>
                <span class="text-white/70 text-sm">Send movie poster as document</span>
              </div>
            </div>
            <div class="cmd-card">
              <div class="flex items-center gap-3">
                <span class="font-mono text-green-400 font-bold text-sm">/jid</span>
                <span class="text-white/30 text-xs">→</span>
                <span class="text-white/70 text-sm">Get current chat JID info</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Bot Info -->
        <div class="rounded-2xl p-5" style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.07);">
          <h3 class="text-xs uppercase tracking-widest text-white/40 font-mono mb-4">Bot Info</h3>
          <div class="space-y-2 text-sm">
            <div class="flex justify-between">
              <span class="text-white/40">Name</span>
              <span class="font-mono text-green-400">` + cfg.BotName + `</span>
            </div>
            <div class="flex justify-between">
              <span class="text-white/40">Platform</span>
              <span class="font-mono text-white/70">Railway ☁️</span>
            </div>
            <div class="flex justify-between">
              <span class="text-white/40">Framework</span>
              <span class="font-mono text-white/70">Go + whatsmeow</span>
            </div>
          </div>
        </div>

      </div>
    </main>

    <!-- Footer -->
    <footer class="text-center py-4 text-white/20 text-xs font-mono border-t border-white/5">
      ` + cfg.BotName + ` • Powered by whatsmeow • Deployed on Railway
    </footer>

  </div>

  <script>
    let isConnected = false;
    let qrRetries = 0;

    function handleQRError() {
      if (isConnected) return;
      qrRetries++;
      if (qrRetries < 30) {
        setTimeout(() => {
          const img = document.getElementById('qr-img');
          img.src = '/qr?t=' + Date.now();
        }, 3000);
      }
    }

    function updateStatus(status) {
      const dot = document.getElementById('status-dot');
      const headerDot = document.getElementById('header-dot');
      const text = document.getElementById('status-text');
      const headerText = document.getElementById('header-status-text');
      const progressBar = document.getElementById('progress-bar');
      const scanLine = document.getElementById('scan-line');
      const connectedOverlay = document.getElementById('connected-overlay');
      const qrImg = document.getElementById('qr-img');

      if (status === 'connected') {
        isConnected = true;
        [dot, headerDot].forEach(d => { d.className = 'status-dot connected'; });
        text.textContent = '✅ Bot Connected & Running!';
        headerText.textContent = 'Connected';
        progressBar.style.width = '100%';
        progressBar.style.background = '#25d366';
        scanLine.style.display = 'none';
        connectedOverlay.style.display = 'flex';
        qrImg.style.display = 'none';
        document.getElementById('qr-loading').style.display = 'none';
      } else if (status === 'waiting_qr') {
        [dot, headerDot].forEach(d => { d.className = 'status-dot waiting'; });
        text.textContent = 'Waiting for QR scan...';
        headerText.textContent = 'Scan QR Code';
        progressBar.style.width = '66%';
      } else {
        [dot, headerDot].forEach(d => { d.className = 'status-dot connecting'; });
        text.textContent = 'Initializing bot...';
        headerText.textContent = 'Starting...';
        progressBar.style.width = '33%';
      }
    }

    async function pollStatus() {
      try {
        const res = await fetch('/status');
        const data = await res.json();
        updateStatus(data.status);
        if (data.status === 'waiting_qr' && !isConnected) {
          const img = document.getElementById('qr-img');
          if (img.style.opacity === '0' || img.naturalWidth === 0) {
            img.src = '/qr?t=' + Date.now();
          }
        }
      } catch (e) {}
    }

    // Poll every 3 seconds
    setInterval(pollStatus, 3000);
    pollStatus();

    // Refresh QR every 30s if not connected
    setInterval(() => {
      if (!isConnected) {
        document.getElementById('qr-loading').style.display = 'flex';
        const img = document.getElementById('qr-img');
        img.style.opacity = '0';
        img.src = '/qr?t=' + Date.now();
      }
    }, 30000);
  </script>
</body>
</html>`
}

func main() {
	// Load config
	if err := loadConfig(); err != nil {
		fmt.Println("❌ Config error:", err)
		os.Exit(1)
	}
	fmt.Printf("🤖 Starting %s...\n", cfg.BotName)

	// Start web server for Railway
	startWebServer()

	// Setup logger
	logger := waLog.Stdout("Bot", "INFO", true)

	// Setup database (SQLite)
	dbPath := "store/whatsapp.db"
	os.MkdirAll("store", 0755)
	container, err := sqlstore.New("sqlite3", "file:"+dbPath+"?_foreign_keys=on", logger)
	if err != nil {
		fmt.Println("❌ DB error:", err)
		os.Exit(1)
	}

	// Get device store
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		fmt.Println("❌ Device store error:", err)
		os.Exit(1)
	}

	// Create client
	client = whatsmeow.NewClient(deviceStore, logger)
	client.AddEventHandler(eventHandler)

	// Connect or show QR
	if client.Store.ID == nil {
		// New login - generate QR
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			fmt.Println("❌ Connection error:", err)
			os.Exit(1)
		}

		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					fmt.Println("📱 QR Code received, generating image...")
					qrData, err := generateQRWithLogo(evt.Code)
					if err != nil {
						fmt.Println("QR generation error:", err)
						continue
					}
					qrCodeData = qrData
					fmt.Println("✅ QR code ready at /qr")
				} else if evt.Event == "success" {
					isLoggedIn = true
					qrCodeData = nil
					fmt.Println("✅ QR scan successful!")
				}
			}
		}()
	} else {
		// Already logged in
		err = client.Connect()
		if err != nil {
			fmt.Println("❌ Reconnect error:", err)
			os.Exit(1)
		}
		isLoggedIn = true
		fmt.Println("✅ Reconnected using existing session!")
	}

	// Wait for interrupt
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("\n👋 Shutting down...")
	client.Disconnect()
}
