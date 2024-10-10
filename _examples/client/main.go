package main

import (
	"fmt"
	socketio "github.com/f4ai/go-socket.io"
	"github.com/f4ai/go-socket.io/engineio"
	"github.com/f4ai/go-socket.io/engineio/transport"
	"github.com/f4ai/go-socket.io/engineio/transport/websocket"
	"github.com/f4ai/go-socket.io/logger"
	"time"
)

func main() {
	//
	connect()
	select {}
}

func connect() {
	fmt.Println("Create new connection")
	var opts = &socketio.ClientOptions{

		//PingTimeout:  2000,
		//PingInterval: 10000,
		Transports:           []transport.Transport{websocket.Default},
		ReconnectionAttempts: 3,
		Reconnection:         true,
		Options: engineio.Options{
			Transports:   []transport.Transport{websocket.Default},
			PingTimeout:  10 * time.Second,
			PingInterval: 2 * time.Second,
		},
		//ReconnectionDelay: float64(20 * time.Second),
	} // Tạo client Socket.IO
	client, _ := socketio.NewClient("http://192.168.1.0:8082", opts)
	//time.Sleep(5 * time.Second)
	go manageClient(client)
}

func manageClient(client *socketio.Client) {
	// Đăng ký sự kiện "connect"
	client.OnConnect(func(conn socketio.Conn) error {
		fmt.Println("Connected to server", conn.ID())
		return nil
	})

	client.OnError(func(conn socketio.Conn, err error) {
		fmt.Println("Main Error:", err)
	})

	client.OnDisconnect(func(conn socketio.Conn, s string) {
		fmt.Println("Disconnected from server")

		defer func() {
			if err := client.Close(); err != nil {
				logger.Error("close connect:", err)
			}
		}()
		connect()
	})

	client.OnEvent("TASK_MANAGER_ASSIGN", func(s socketio.Conn, msg string) {
		fmt.Println(msg)
	})

	err := client.Connect()
	if err != nil {
		fmt.Println(err)
		time.Sleep(2 * time.Second)
		connect()
	}
}
