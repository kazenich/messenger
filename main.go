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

	// ВРЕМЕННЫЙ МАРШРУТ ДЛЯ ОЧИСТКИ БД
	http.HandleFunc("/clear", func(w http.ResponseWriter, r *http.Request) {
		ClearAllUsers()
		w.Write([]byte("База данных очищена! Все пользователи и сообщения удалены."))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
