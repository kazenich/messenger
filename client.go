package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Сервер не запущен! Запустите сначала: go run server.go")
		return
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	
	// Читаем "Введите ваше имя: "
	welcome, _ := reader.ReadString('\n')
	fmt.Print(welcome)
	
	// Вводим имя и отправляем с \n
	console := bufio.NewReader(os.Stdin)
	name, _ := console.ReadString('\n')
	conn.Write([]byte(name))
	
	// Читаем приветствие от сервера ("Привет, Имя!")
	response, _ := reader.ReadString('\n')
	fmt.Println(response)
	fmt.Println("Начинайте писать сообщения (exit - выход):")

	// Горутина: читаем сообщения от сервера
	go func() {
		for {
			msg, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println("\n[Отключено от сервера]")
				os.Exit(0)
			}
			fmt.Print(msg)
		}
	}()

	// Пишем сообщения
	for {
		text, _ := console.ReadString('\n')
		text = strings.TrimSpace(text)
		
		if text == "exit" {
			return
		}
		
		conn.Write([]byte(text + "\n"))
	}
}
