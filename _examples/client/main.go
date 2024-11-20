package main

import (
	"fmt"
	socketio "github.com/f4ai/go-socket.io"
	"github.com/f4ai/go-socket.io/engineio/transport"
	"github.com/f4ai/go-socket.io/engineio/transport/websocket"
	"github.com/f4ai/go-socket.io/logger"
	"net/http"
	"time"
)

func main() {
	//
	connect()
	select {}
}

func connect() {
	fmt.Println("Create new connection")
	headers := http.Header{}
	headers.Set("authorization", "bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOiI2NzJiOTc2ODliZTUzZWIzYzQ1MmNmNTUiLCJ0b2tlbklkIjoiNjczZDNmODUzOGVkODJkOTIxOGM1Zjk0IiwiZ3JhbnRUeXBlIjoicGFzc3dvcmQiLCJpYXQiOjE3MzIwNjcyMDUsImV4cCI6MTczMjMyNjQwNX0.7aQjKvh5wmyhyCD-ncJOerpO1wiQI8hxG9hAF9Jwpfk")
	var opts = &socketio.ClientOptions{
		Transports:    []transport.Transport{websocket.Default},
		RequestHeader: headers,
	} // Tạo client Socket.IO
	client, _ := socketio.NewClient("https://f4ai.net", opts)
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
