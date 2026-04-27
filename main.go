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

	// Маршрут для скачивания базы данных
	http.HandleFunc("/download-db", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=chat.db")
		http.ServeFile(w, r, "chat.db")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
