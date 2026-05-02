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

	// ВРЕМЕННЫЙ МАРШРУТ ДЛЯ ОЧИСТКИ ВСЕХ СЕССИЙ
	http.HandleFunc("/logout-all", func(w http.ResponseWriter, r *http.Request) {
		clientsMu.Lock()
		for c := range clients {
			c.Conn.Close()
			delete(clients, c)
		}
		clientsMu.Unlock()
		w.Write([]byte("Все сессии закрыты"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
