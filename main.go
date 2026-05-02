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

	// Диагностические маршруты
	http.HandleFunc("/list-users", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT username FROM users")
		if err != nil {
			w.Write([]byte("Ошибка: " + err.Error()))
			return
		}
		defer rows.Close()
		w.Write([]byte("Пользователи:\n"))
		for rows.Next() {
			var u string
			rows.Scan(&u)
			w.Write([]byte(u + "\n"))
		}
	})

	http.HandleFunc("/delete-user", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("name")
		if username == "" {
			w.Write([]byte("Используйте: /delete-user?name=имя"))
			return
		}
		db.Exec("DELETE FROM users WHERE username = ?", username)
		w.Write([]byte("Пользователь " + username + " удалён"))
	})

	http.HandleFunc("/reset-all", func(w http.ResponseWriter, r *http.Request) {
		db.Close()
		os.Remove("chat.db")
		initDB()
		w.Write([]byte("База данных пересоздана"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
