package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

func main() {
	// Запускаем сервер на порту 8080
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}
	defer listener.Close()
	
	fmt.Println("=== Сервер запущен на порту 8080 ===")
	fmt.Println("Откройте новое окно PowerShell и введите: telnet localhost 8080")
	fmt.Println("Или просто напишите текст ниже, он появится здесь:")
	fmt.Println()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		
		// Обрабатываем клиента в отдельной горутине
		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	
	clientAddr := conn.RemoteAddr().String()
	fmt.Printf("Клиент подключился: %s\n", clientAddr)
	
	reader := bufio.NewReader(conn)
	for {
		// Читаем строку от клиента
		text, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Клиент отключился: %s\n", clientAddr)
			return
		}
		
		text = strings.TrimSpace(text)
		fmt.Printf("Получено от %s: %s\n", clientAddr, text)
		
		// Отправляем ответ обратно
		response := "Сервер получил: " + text + "\n"
		conn.Write([]byte(response))
	}
}
