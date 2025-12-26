package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	// 启动 TCP Echo 服务器
	//go echo()

	proxy()
}

func echo() {
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

func proxy() {
	// 1. 获取要转发的目标地址 (从环境变量读取，更适合 K8s)
	// 在 K8s 中，我们将把它设置为 hello-service 的 DNS 地址，例如 "hello-service:80"
	targetAddr := os.Getenv("PROXY_TARGET")
	if targetAddr == "" {
		fmt.Println("Error: PROXY_TARGET env is required")
		os.Exit(1)
	}

	// 2. 监听本地端口
	localPort := ":3333"
	listener, err := net.Listen("tcp", localPort)
	if err != nil {
		fmt.Printf("Failed to listen on %s: %v\n", localPort, err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("TCP Proxy Server listening on %s, forwarding to %s\n", localPort, targetAddr)

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept: %v\n", err)
			continue
		}

		// 为每个连接开启一个 goroutine 处理
		go handleProxy(clientConn, targetAddr)
	}
}

func handleProxy(clientConn net.Conn, targetAddr string) {
	// 确保客户端连接最终会被关闭
	defer clientConn.Close()

	fmt.Printf("[New Connection] From %s\n", clientConn.RemoteAddr())

	// A. 这里可以插入你的【自定义逻辑】
	// 比如：检查 IP 白名单，或者读取前几个字节判断协议
	fmt.Println("   >>> Processing custom logic...")

	// B. 连接后端目标服务 (hello-api)
	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		fmt.Printf("Failed to connect to backend %s: %v\n", targetAddr, err)
		return
	}
	defer targetConn.Close()

	// C. 开始双向转发 (这就是"桥"的核心)
	// 我们需要两个管道：
	// 1. Client -> Target
	// 2. Target -> Client

	// 启动一个 goroutine 负责把 Target 的回复搬运给 Client
	go func() {
		_, err := io.Copy(clientConn, targetConn)
		if err != nil {
			// fmt.Printf("Copy from target to client error: %v\n", err)
		}
	}()

	// 主 goroutine 负责把 Client 的请求搬运给 Target
	_, err = io.Copy(targetConn, clientConn)
	if err != nil {
		// fmt.Printf("Copy from client to target error: %v\n", err)
	}

	fmt.Printf("[Connection Closed] %s\n", clientConn.RemoteAddr())
}
