package main

import (
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

type room struct {
	// forward is a channel that holds incoming messages
	// that should be forwarded to the other clients.
	forward chan []byte
	// join is a channel for clients wishing to join the room.
	join chan *client
	// leave is a channel for clients wishing to leave the room.
	leave chan *client
	// clients holds all current clients in this room.
	clients map[*client]bool
}

func (r *room) run() {
	for { // run forever
		// watch the 3 channels inside the room
		select {
		case client := <-r.join:
			// joining
			r.clients[client] = true
		case client := <-r.leave:
			// leaving
			delete(r.clients, client)
			close(client.send)
		case msg := <-r.forward:
			// forward message to all clients
			for client := range r.clients {
				select {
				case client.send <- msg:
					// send the message
				default:
					// failed to send
					delete(r.clients, client)
					close(client.send)
				}
			}
		}
	}
}

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)
var upgrader = &websocket.Upgrader{ReadBufferSize: socketBufferSize, WriteBufferSize: socketBufferSize}

// The ServeHTTP method means a room can now act as a handler.
func (r *room) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// In order to use web sockets, we must upgrade the HTTP connection using
	// the websocket.Upgrader type, which is reusable so we need only create one
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Fatal("ServeHTTP:", err)
		return
	}
	// when a request comes in via the ServeHTTP method, we get the socket by calling the upgrader.Upgrade method.
	// All being well, we then create our client and pass it into the join channel for the current room.
	client := &client{
		socket: socket,
		send:   make(chan []byte, messageBufferSize),
		room:   r,
	}
	r.join <- client
	defer func() { r.leave <- client }()
	//The write method for the client is then called as a Go routine, as
	// indicated by the three characters at the beginning of the line go.
	// This tells Go to run the method in a different thread or goroutine.
	go client.write()
	//we call the read method in the main thread, which will block operations
	// (keeping the connection alive) until it's time to close it.
	client.read()
}

// newRoom makes a new room that is ready to go.
func newRoom() *room {
	return &room{
		forward: make(chan []byte),
		join:    make(chan *client),
		leave:   make(chan *client),
		clients: make(map[*client]bool),
	}
}