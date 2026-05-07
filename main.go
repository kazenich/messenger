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
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	http.HandleFunc("/list-users", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT username FROM users")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h1>Пользователи</h1>")
		for rows.Next() {
			var u string
			rows.Scan(&u)
			fmt.Fprintf(w, "%s<br>", u)
		}
	})

	http.HandleFunc("/list-messages", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, from_user, to_user, text, time FROM messages ORDER BY id DESC LIMIT 100")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h1>Сообщения</h1>")
		for rows.Next() {
			var id int
			var from, to, text, time string
			rows.Scan(&id, &from, &to, &text, &time)
			fmt.Fprintf(w, "[%d] %s -> %s: %s (%s)<br>", id, from, to, text, time)
		}
	})

	http.HandleFunc("/test-search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			w.Write([]byte("?q=текст"))
			return
		}
		users := searchUsers(query, "")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<h1>Поиск: '%s'</h1>", query)
		fmt.Fprintf(w, "Найдено: %d<br>", len(users))
		for _, u := range users {
			fmt.Fprintf(w, "%s<br>", u)
		}
	})

	http.HandleFunc("/db-stats", func(w http.ResponseWriter, r *http.Request) {
		var users, msgs, groups int
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
		db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&msgs)
		db.QueryRow("SELECT COUNT(*) FROM groups").Scan(&groups)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "Пользователей: %d<br>Сообщений: %d<br>Групп: %d", users, msgs, groups)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Сервер запущен на порту %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
