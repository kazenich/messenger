package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	initDB()
	defer db.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "chat.html")
	})
	http.HandleFunc("/ws", handleWebSocket)

	// ВРЕМЕННЫЙ МАРШРУТ ДЛЯ СБРОСА БД - УДАЛИТЬ ПОСЛЕ ИСПОЛЬЗОВАНИЯ!
	http.HandleFunc("/reset-db", func(w http.ResponseWriter, r *http.Request) {
		db.Close()
		os.Remove("chat.db")
		initDB()
		w.Write([]byte("База данных сброшена! Все пользователи и сообщения удалены."))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
