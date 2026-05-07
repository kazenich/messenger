package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	initDB()
	defer db.Close()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Сервер запускается...")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "chat.html")
			return
		}
		http.NotFound(w, r)
	})

	http.HandleFunc("/ws", handleWebSocket)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Сервер запущен на порту %s", port)
	http.ListenAndServe(":"+port, nil)
}
