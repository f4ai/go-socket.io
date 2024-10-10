package main

import (
	"errors"
	socketio "github.com/f4ai/go-socket.io"
	"github.com/f4ai/go-socket.io/engineio/transport"
	"github.com/f4ai/go-socket.io/engineio/transport/polling"
	"github.com/f4ai/go-socket.io/engineio/transport/websocket"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Data struct {
	Text string `json:"text"`
}

type SSDoer struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

func initSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Printf("receive a signal %v to stop all process & exit", sig)
		os.Exit(-1)
	}()

}

func connectSocketIO() {
	// Simple client to talk to default-http example
	opts := &socketio.ClientOptions{
		Transports:           []transport.Transport{polling.Default, websocket.Default},
		Reconnection:         true,
		ReconnectionDelay:    float64(2 * time.Second),
		ReconnectionDelayMax: float64(5 * time.Second),
		ReconnectionAttempts: 5,
	}

	uri := "http://192.168.110.88:3000"
	//uri = "http://127.0.0.1:8000"
	client, err := socketio.NewClient(uri, opts)
	if err != nil {
		panic(err)
	}

	// Handle an incoming event
	client.OnEvent("message", func(s socketio.Conn, msg interface{}) {
		log.Println("Receive Message /reply: ", "reply", msg)
	})
	// Handle an incoming event
	client.OnConnect(func(conn socketio.Conn) error {
		log.Println("OnConnect", conn.ID())
		//panic(errors.New("断开链接"))
		return nil
	})
	// Handle an incoming event
	client.OnError(func(conn socketio.Conn, err error) {
		log.Println("OnError", conn.ID(), err)
		panic(errors.New("断开链接"))
	})

	// Handle an incoming event
	client.OnDisconnect(func(conn socketio.Conn, s string) {
		log.Println("OnDisconnect", conn.ID(), s)
		//panic(errors.New("断开链接"))
	})

	err = client.Connect()
	if err != nil {
		panic(err)
	}

	defer client.Close()
	select {}
}

func runServer() {
	for {
		initSignals()
		connectSocketIO()
	}
}

func main() {
	runServer()
}
