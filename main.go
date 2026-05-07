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

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	http.HandleFunc("/list-users", func(w http.ResponseWriter, r *http.Request) {
		rows, _ := db.Query("SELECT username FROM users")
		defer rows.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		for rows.Next() {
			var u string
			rows.Scan(&u)
			fmt.Fprintf(w, "%s<br>", u)
		}
	})

	http.HandleFunc("/test-search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		users := searchUsers(query, "")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "Поиск: '%s'<br>Найдено: %d<br>", query, len(users))
		for _, u := range users {
			fmt.Fprintf(w, "%s<br>", u)
		}
	})

	http.HandleFunc("/reset-all", func(w http.ResponseWriter, r *http.Request) {
		db.Close()
		os.Remove("/app/data/chat.db")
		initDB()
		w.Write([]byte("База данных пересоздана"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Сервер запущен на порту %s", port)
	http.ListenAndServe(":"+port, nil)
}
