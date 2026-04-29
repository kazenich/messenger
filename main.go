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

	// ВРЕМЕННЫЙ МАРШРУТ ДЛЯ УДАЛЕНИЯ ПОЛЬЗОВАТЕЛЯ
	http.HandleFunc("/deluser", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("name")
		if username == "" {
			w.Write([]byte("Используйте: /deluser?name=имя"))
			return
		}
		result, err := db.Exec("DELETE FROM users WHERE username = ?", username)
		if err != nil {
			w.Write([]byte("Ошибка: " + err.Error()))
			return
		}
		affected, _ := result.RowsAffected()
		w.Write([]byte(fmt.Sprintf("Удалено пользователей: %d", affected)))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
