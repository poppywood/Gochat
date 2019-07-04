package main

import (
	"github.com/gorilla/websocket"
	"github.com/poppywood/Gotrace"
	"github.com/stretchr/objx"
	"log"
	"net/http"
)

type room struct {
	// forward is a channel that holds incoming messages
	// that should be forwarded to the other clients.
	forward chan *message
	// join is a channel for clients wishing to join the room.
	join chan *client
	// leave is a channel for clients wishing to leave the room.
	leave chan *client
	// clients holds all current clients in this room.
	clients map[*client]bool
	// tracer will receive trace information of activity
	// in the room.
	tracer trace.Tracer
}

func (r *room) run() {
	for { // run forever
		// watch the 3 channels inside the room
		select {
		case client := <-r.join:
			// joining
			r.clients[client] = true
			r.tracer.Trace("New client joined")
		case client := <-r.leave:
			// leaving
			delete(r.clients, client)
			close(client.send)
			r.tracer.Trace("Client left")
		case msg := <-r.forward:
			// forward message to all clients
			r.tracer.Trace("Message received: ", string(msg.Message))
			for client := range r.clients {
				client.send <- msg
				r.tracer.Trace(" -- sent to client")
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
	authCookie, err := req.Cookie("auth")
	if err != nil {
		log.Fatal("Failed to get auth cookie:", err)
		return
	}
	// when a request comes in via the ServeHTTP method, we get the socket by calling the upgrader.Upgrade method.
	// All being well, we then create our client and pass it into the join channel for the current room.
	client := &client{
		socket: socket,
		send:     make(chan *message, messageBufferSize),
		room:   r,
		userData: objx.MustFromBase64(authCookie.Value),
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
		forward: make(chan *message),
		join:    make(chan *client),
		leave:   make(chan *client),
		clients: make(map[*client]bool),
		//tracer:  trace.Off(), //remove to enable tracing
	}
}