package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

type client struct {
	name string
	conn net.Conn
	out  chan string
}

type chatServer struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

func newChatServer() *chatServer {
	return &chatServer{clients: make(map[*client]struct{})}
}

func (s *chatServer) addClient(c *client) {
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
}

func (s *chatServer) removeClient(c *client) {
	s.mu.Lock()
	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)
		close(c.out)
	}
	s.mu.Unlock()
}

func (s *chatServer) broadcast(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for c := range s.clients {
		select {
		case c.out <- msg:
		default:
			// Skip slow clients instead of blocking all chat traffic.
		}
	}
}

func handleConnection(conn net.Conn, server *chatServer) {
	defer conn.Close()

	fmt.Fprintln(conn, "Welcome to Go Chat")
	fmt.Fprintln(conn, "Enter your name:")

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	name := strings.TrimSpace(scanner.Text())
	if name == "" {
		name = conn.RemoteAddr().String()
	}

	c := &client{
		name: name,
		conn: conn,
		out:  make(chan string, 16),
	}

	server.addClient(c)
	server.broadcast(fmt.Sprintf("* %s joined the chat", c.name))
	c.out <- "You are connected. Type /quit to leave."

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for msg := range c.out {
			if _, err := fmt.Fprintln(conn, msg); err != nil {
				return
			}
		}
	}()

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if text == "/quit" {
			break
		}
		server.broadcast(fmt.Sprintf("[%s] %s", c.name, text))
	}

	server.removeClient(c)
	server.broadcast(fmt.Sprintf("* %s left the chat", c.name))
	<-writerDone
}

func main() {
	port := os.Getenv("CHAT_PORT")
	if port == "" {
		port = "8080"
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", port, err)
	}
	defer ln.Close()

	log.Printf("chat server listening on :%s", port)

	server := newChatServer()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConnection(conn, server)
	}
}
