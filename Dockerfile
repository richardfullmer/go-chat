FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod main.go ./
RUN go build -o /chat-server .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /chat-server /usr/local/bin/chat-server
COPY assets ./assets

EXPOSE 8080
ENV CHAT_PORT=8080
CMD ["chat-server"]
