# Go Chat (Docker)

Small multi-user TCP chat server written in Go.

## Build image

```bash
docker build -t go-chat:latest .
```

## Run server

```bash
docker run --rm -it -p 8080:8080 --name go-chat-server go-chat:latest
```

Server listens on `CHAT_PORT` (default `8080`).

To use a different port:

```bash
docker run --rm -it -p 9090:9090 -e CHAT_PORT=9090 go-chat:latest
```

## Connect clients

Open multiple terminals and connect with `nc`:

```bash
nc localhost 8080
```

Each client enters a name, then can chat.

Commands:
- `/quit` disconnects the current client.

Notes:
- Messages are broadcast to all connected users.
- If a username is blank, the server uses the client's remote address.
