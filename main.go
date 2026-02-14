package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type message struct {
	ID        int64  `json:"id"`
	Sender    string `json:"sender"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type chatServer struct {
	mu       sync.Mutex
	users    map[string]string
	messages []message
	nextID   int64
}

func newChatServer() *chatServer {
	return &chatServer{
		users:    make(map[string]string),
		messages: make([]message, 0, 128),
		nextID:   1,
	}
}

func (s *chatServer) addUser(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := randomID(16)
	s.users[id] = name
	s.appendMessageLocked("system", fmt.Sprintf("%s joined the chat", name))
	return id
}

func (s *chatServer) removeUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name, ok := s.users[userID]
	if !ok {
		return
	}
	delete(s.users, userID)
	s.appendMessageLocked("system", fmt.Sprintf("%s left the chat", name))
}

func (s *chatServer) postMessage(userID, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sender, ok := s.users[userID]
	if !ok {
		return fmt.Errorf("unknown user")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("message is empty")
	}
	s.appendMessageLocked(sender, text)
	return nil
}

func (s *chatServer) messagesAfter(lastID int64) []message {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.messages) == 0 {
		return nil
	}

	idx := 0
	for idx < len(s.messages) && s.messages[idx].ID <= lastID {
		idx++
	}
	if idx >= len(s.messages) {
		return nil
	}

	out := make([]message, len(s.messages)-idx)
	copy(out, s.messages[idx:])
	return out
}

func (s *chatServer) appendMessageLocked(sender, text string) {
	s.messages = append(s.messages, message{
		ID:        s.nextID,
		Sender:    sender,
		Text:      text,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	s.nextID++

	if len(s.messages) > 1000 {
		s.messages = s.messages[len(s.messages)-1000:]
	}
}

func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}

type joinRequest struct {
	Name string `json:"name"`
}

type joinResponse struct {
	UserID string `json:"user_id"`
}

type sendRequest struct {
	UserID string `json:"user_id"`
	Text   string `json:"text"`
}

type leaveRequest struct {
	UserID string `json:"user_id"`
}

func main() {
	port := os.Getenv("CHAT_PORT")
	if port == "" {
		port = "8080"
	}

	server := newChatServer()

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(indexHTML))
	})

	mux.HandleFunc("/api/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req joinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if len(name) > 32 {
			name = name[:32]
		}

		userID := server.addUser(name)
		writeJSON(w, http.StatusOK, joinResponse{UserID: userID})
	})

	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req sendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		req.Text = strings.TrimSpace(req.Text)
		if len(req.Text) > 500 {
			req.Text = req.Text[:500]
		}
		if err := server.postMessage(req.UserID, req.Text); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req leaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		server.removeUser(req.UserID)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sinceStr := r.URL.Query().Get("since")
		var since int64
		if sinceStr != "" {
			parsed, err := strconv.ParseInt(sinceStr, 10, 64)
			if err != nil {
				http.Error(w, "invalid since parameter", http.StatusBadRequest)
				return
			}
			since = parsed
		}
		msgs := server.messagesAfter(since)
		if msgs == nil {
			msgs = []message{}
		}
		writeJSON(w, http.StatusOK, map[string][]message{"messages": msgs})
	})

	log.Printf("web chat listening on http://0.0.0.0:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Go Chat</title>
  <style>
    :root {
      --bg: #f4efe7;
      --bg-2: #e9f1ec;
      --card: #fffefb;
      --ink: #1b2430;
      --accent: #146c5d;
      --accent-strong: #0f5a4d;
      --muted: #6a7280;
      --line: #e1d7c6;
      --message-self: #e9f7f1;
      --message-system: #eef7fb;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "Avenir Next", "Segoe UI", "Trebuchet MS", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(900px 400px at -10% -10%, #fff4d8 20%, transparent 70%),
        radial-gradient(900px 500px at 110% 0%, #deede6 20%, transparent 70%),
        linear-gradient(170deg, var(--bg), var(--bg-2));
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 16px;
    }
    .app {
      width: min(880px, 100%);
      height: min(80vh, 720px);
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 18px;
      display: grid;
      grid-template-rows: auto 1fr auto;
      overflow: hidden;
      box-shadow: 0 18px 40px rgba(35, 35, 35, 0.12), 0 2px 8px rgba(35, 35, 35, 0.06);
      backdrop-filter: blur(2px);
    }
    header {
      padding: 16px 18px;
      border-bottom: 1px solid var(--line);
      display: flex;
      justify-content: space-between;
      align-items: center;
      background: linear-gradient(180deg, #fffaf1, #fff);
      gap: 10px;
    }
    .brand {
      margin: 0;
      display: flex;
      align-items: center;
    }
    .brand img {
      display: block;
      width: clamp(170px, 35vw, 300px);
      height: auto;
    }
    #status {
      color: var(--muted);
      font-size: 12px;
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 4px 9px;
      background: #fff;
    }
    #messages {
      overflow-y: auto;
      padding: 16px;
      display: flex;
      flex-direction: column;
      gap: 10px;
      background:
        linear-gradient(180deg, rgba(255,255,255,0.84), rgba(255,255,255,0.92));
    }
    .msg {
      max-width: min(80%, 650px);
      padding: 10px 12px 11px;
      border: 1px solid var(--line);
      border-radius: 13px;
      background: #fff;
      animation: pop-in 160ms ease-out;
    }
    .msg.me {
      align-self: flex-end;
      background: var(--message-self);
      border-color: #cceadd;
    }
    .msg.system {
      background: var(--message-system);
      border-color: #d4e7f3;
    }
    .sender {
      font-size: 12px;
      color: var(--muted);
      margin-bottom: 5px;
      font-weight: 600;
      letter-spacing: 0.2px;
    }
    .sender.system { color: var(--accent); font-weight: 600; }
    form {
      padding: 12px 14px 14px;
      border-top: 1px solid var(--line);
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 8px;
      background: linear-gradient(180deg, #fff, #fefbf5);
    }
    input, button {
      font: inherit;
      border-radius: 11px;
      border: 1px solid var(--line);
      padding: 10px 12px;
      transition: all 120ms ease;
    }
    input:focus {
      outline: none;
      border-color: #8eb5ab;
      box-shadow: 0 0 0 3px rgba(20,108,93,0.14);
    }
    button {
      background: var(--accent);
      color: white;
      border-color: var(--accent);
      cursor: pointer;
      font-weight: 600;
    }
    button:hover { background: var(--accent-strong); border-color: var(--accent-strong); }
    button:active { transform: translateY(1px); }
    button:disabled { opacity: 0.6; cursor: not-allowed; }
    .overlay {
      position: absolute;
      inset: 0;
      background: rgba(244, 239, 231, 0.95);
      display: grid;
      place-items: center;
      padding: 20px;
    }
    .panel {
      width: min(420px, 100%);
      background: white;
      border: 1px solid var(--line);
      border-radius: 14px;
      padding: 20px;
      display: grid;
      gap: 11px;
      box-shadow: 0 14px 24px rgba(33, 33, 33, 0.12);
    }
    .panel h2 { margin: 0; font-size: 24px; line-height: 1; }
    .panel label { font-size: 13px; color: var(--muted); }
    .root { position: relative; width: 100%; }
    @keyframes pop-in {
      from { opacity: 0; transform: translateY(4px); }
      to { opacity: 1; transform: translateY(0); }
    }
    @media (max-width: 720px) {
      body { padding: 8px; }
      .app { height: min(90vh, 760px); border-radius: 14px; }
      #messages { padding: 12px; }
      .msg { max-width: 92%; }
      .brand img { width: clamp(145px, 48vw, 240px); }
    }
  </style>
</head>
<body>
  <div class="root app" id="appRoot">
    <header>
      <h1 class="brand">
        <img src="/assets/groundhog-title.svg" alt="Groundhog Chat" />
      </h1>
      <div id="status">Disconnected</div>
    </header>
    <main id="messages"></main>
    <form id="sendForm">
      <input id="messageInput" placeholder="Type a message" maxlength="500" autocomplete="off" />
      <button id="sendBtn" type="submit" disabled>Send</button>
    </form>
    <div class="overlay" id="joinOverlay">
      <form class="panel" id="joinForm">
        <h2>Join chat</h2>
        <label for="nameInput">Display name</label>
        <input id="nameInput" maxlength="32" required placeholder="Your name" autocomplete="nickname" />
        <button type="submit">Join</button>
      </form>
    </div>
  </div>

  <script>
    const statusEl = document.getElementById("status");
    const messagesEl = document.getElementById("messages");
    const joinOverlay = document.getElementById("joinOverlay");
    const joinForm = document.getElementById("joinForm");
    const nameInput = document.getElementById("nameInput");
    const sendForm = document.getElementById("sendForm");
    const messageInput = document.getElementById("messageInput");
    const sendBtn = document.getElementById("sendBtn");

    let userId = "";
    let currentName = "";
    let lastMessageId = 0;
    let pollTimer = null;

    function addMessage(msg) {
      const wrap = document.createElement("article");
      wrap.className = "msg";
      if (msg.sender === "system") {
        wrap.classList.add("system");
      } else if (msg.sender === currentName) {
        wrap.classList.add("me");
      }

      const sender = document.createElement("div");
      sender.className = "sender" + (msg.sender === "system" ? " system" : "");
      sender.textContent = msg.sender;

      const text = document.createElement("div");
      text.textContent = msg.text;

      wrap.appendChild(sender);
      wrap.appendChild(text);
      messagesEl.appendChild(wrap);
      messagesEl.scrollTop = messagesEl.scrollHeight;
    }

    async function pollMessages() {
      if (!userId) return;
      try {
        const res = await fetch("/api/messages?since=" + lastMessageId);
        if (!res.ok) throw new Error("poll failed");
        const data = await res.json();
        for (const msg of data.messages) {
          addMessage(msg);
          if (msg.id > lastMessageId) lastMessageId = msg.id;
        }
      } catch (err) {
        statusEl.textContent = "Connection issue... retrying";
      } finally {
        pollTimer = setTimeout(pollMessages, 1000);
      }
    }

    joinForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const name = nameInput.value.trim();
      if (!name) return;

      const res = await fetch("/api/join", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name })
      });

      if (!res.ok) {
        alert("Unable to join chat");
        return;
      }

      const data = await res.json();
      userId = data.user_id;
      currentName = name;
      joinOverlay.style.display = "none";
      sendBtn.disabled = false;
      messageInput.focus();
      statusEl.textContent = "Connected as " + name;
      pollMessages();
    });

    sendForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const text = messageInput.value.trim();
      if (!text || !userId) return;

      const res = await fetch("/api/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: userId, text })
      });

      if (res.ok) {
        messageInput.value = "";
      }
    });

    window.addEventListener("beforeunload", () => {
      if (!userId) return;
      navigator.sendBeacon("/api/leave", new Blob([JSON.stringify({ user_id: userId })], { type: "application/json" }));
      clearTimeout(pollTimer);
    });
  </script>
</body>
</html>`
