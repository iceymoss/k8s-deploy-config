package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	// 监听 3333 端口
	port := ":3333"
	listener, err := net.Listen("tcp", port)
	if err != nil {
		fmt.Printf("Failed to listen on port %s: %v\n", port, err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("TCP Echo Server listening on %s\n", port)

	for {
		// 接受连接
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		// 处理连接（开启 Goroutine）
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("New connection from %s\n", conn.RemoteAddr().String())

	// 核心逻辑：把读到的数据原样写回去 (Echo)
	// 在真实的代理场景中，这里会建立到目标服务器的连接，并进行 io.Copy 双向转发
	_, err := io.Copy(conn, conn)
	if err != nil {
		fmt.Printf("Connection error: %v\n", err)
	}
}
