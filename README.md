# Go Chat (Web + Docker)

Small multi-user web chat server written in Go.

## Build image

```bash
docker build -t go-chat:latest .
```

## Run server

```bash
docker run --rm -it -p 8080:8080 --name go-chat-server go-chat:latest
```

Open the chat UI in your browser:

```text
http://localhost:8080
```

Server listens on `CHAT_PORT` (default `8080`).

To use a different port:

```bash
docker run --rm -it -p 9090:9090 -e CHAT_PORT=9090 go-chat:latest
```

Then open:

```text
http://localhost:9090
```

## Usage

- Open the page in multiple browser tabs or on multiple devices.
- Enter a display name and click **Join**.
- Send messages from each client; all connected users receive broadcasts.
- Closing or refreshing a page sends a leave event for that user.

## API endpoints

- `POST /api/join` with `{ "name": "alice" }`
- `POST /api/send` with `{ "user_id": "...", "text": "hello" }`
- `GET /api/messages?since=0`
- `POST /api/leave` with `{ "user_id": "..." }`
