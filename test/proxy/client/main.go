package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	serverAddress := "19.32.4.1:30999"

	fmt.Printf("Connecting to %s...\n", serverAddress)
	conn, err := net.Dial("tcp", serverAddress)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("Connected! Type something and press Enter (q to quit):")

	reader := bufio.NewReader(os.Stdin)
	serverReader := bufio.NewReader(conn)

	for {
		fmt.Print("-> ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		if text == "q" {
			break
		}

		// 发送数据给服务端
		fmt.Fprintf(conn, text+"\n")

		// 接收服务端回显的数据
		message, _ := serverReader.ReadString('\n')
		fmt.Printf("Server echoed: %s", message)
	}
}
