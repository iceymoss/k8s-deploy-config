package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	// 监听容器内的 4444 端口 (UDP)
	port := ":4444"
	addr, err := net.ResolveUDPAddr("udp", port)
	if err != nil {
		fmt.Println("Error resolving address:", err)
		os.Exit(1)
	}

	// 建立 UDP 监听
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error listening:", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Printf("UDP Echo Server listening on %s\n", port)
	buffer := make([]byte, 1024)

	for {
		// 读取数据
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error reading:", err)
			continue
		}

		fmt.Printf("Received %d bytes from %s: %s\n", n, remoteAddr, string(buffer[:n]))

		// 原样写回 (Echo)
		_, err = conn.WriteToUDP(buffer[:n], remoteAddr)
		if err != nil {
			fmt.Println("Error writing back:", err)
		}
	}
}
