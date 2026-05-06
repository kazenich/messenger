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
		log.Printf("HTTP %s %s от %s", r.Method, r.URL.Path, r.RemoteAddr)
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "chat.html")
			return
		}
		http.NotFound(w, r)
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("WebSocket подключение от %s", r.RemoteAddr)
		handleWebSocket(w, r)
	})

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
		
		var users []string
		for rows.Next() {
			var u string
			rows.Scan(&u)
			users = append(users, u)
		}
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<h1>Пользователи (%d)</h1>", len(users))
		for _, u := range users {
			fmt.Fprintf(w, "%s<br>", u)
		}
	})

	http.HandleFunc("/test-search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			w.Write([]byte("Укажите параметр ?q=текст"))
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

	http.HandleFunc("/list-messages", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, from_user, to_user, text, time FROM messages ORDER BY id DESC LIMIT 100")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h1>Последние сообщения</h1>")
		for rows.Next() {
			var id int
			var from, to, text, time string
			rows.Scan(&id, &from, &to, &text, &time)
			fmt.Fprintf(w, "[%d] %s -> %s: %s (%s)<br>", id, from, to, text, time)
		}
	})

	http.HandleFunc("/db-stats", func(w http.ResponseWriter, r *http.Request) {
		var userCount, msgCount, groupCount int
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
		db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&msgCount)
		db.QueryRow("SELECT COUNT(*) FROM groups").Scan(&groupCount)
		
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<h1>Статистика БД</h1>")
		fmt.Fprintf(w, "Пользователей: %d<br>", userCount)
		fmt.Fprintf(w, "Сообщений: %d<br>", msgCount)
		fmt.Fprintf(w, "Групп: %d<br>", groupCount)
	})

	http.HandleFunc("/reset-all", func(w http.ResponseWriter, r *http.Request) {
		db.Close()
		os.Remove("./chat.db")
		initDB()
		w.Write([]byte("База данных пересоздана"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	log.Printf("Сервер запущен на порту %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Ошибка запуска сервера:", err)
	}
}
