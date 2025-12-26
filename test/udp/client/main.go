package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	// ⚠️ 配置目标地址 (K8s Node IP : NodePort)
	// 请确保这个 IP 是你的 K8s 节点 IP，端口是 Traefik 暴露的 30998
	serverAddr := "10.4.4.15:30998"

	fmt.Printf("Connecting to UDP server at %s...\n", serverAddr)

	// net.Dial 在 UDP 中不会产生握手网络流量，它只是在本地建立映射
	conn, err := net.Dial("udp", serverAddr)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	defer conn.Close()

	// 准备标准输入读取器
	inputReader := bufio.NewReader(os.Stdin)

	// 准备接收缓冲区 (UDP包通常不超过 1500 字节，这里给 1024 足够了)
	buffer := make([]byte, 1024)

	for {
		fmt.Print("-> ")
		// 读取用户输入
		text, _ := inputReader.ReadString('\n')
		text = strings.TrimSpace(text)

		if text == "q" {
			fmt.Println("Exiting...")
			break
		}
		if text == "" {
			continue
		}

		// 1. 发送数据 (自动加换行符，方便服务端日志查看)
		_, err = fmt.Fprintf(conn, text+"\n")
		if err != nil {
			fmt.Println("Write error:", err)
			continue
		}

		// 设置读取超时 (UDP 是不可靠的，万一丢包了，不能一直死等)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		// 2. 接收数据 (更推荐用 Read 而不是 ReadString)
		n, err := conn.Read(buffer)
		if err != nil {
			// 处理超时
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Println("Timeout: No response from server (Packet lost?)")
				continue
			}
			fmt.Println("Read error:", err)
			continue
		}

		// 打印回显结果
		fmt.Printf("Server echoed: %s", string(buffer[:n]))
	}
}
