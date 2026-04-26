package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"unicode"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var db *sql.DB

type Message struct {
	Type     string `json:"type"`
	From     string `json:"from"`
	To       string `json:"to"`
	Text     string `json:"text"`
	Password string `json:"password"`
	Success  bool   `json:"success"`
	Error    string `json:"error"`
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./chat.db")
	if err != nil {
		panic(err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password_hash TEXT
	)`)
	fmt.Println("DB ready")
}

func isValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 20 {
		return false
	}
	for _, ch := range username {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || 
		     (ch >= '0' && ch <= '9') || ch == '_' ||
		     (ch >= 'а' && ch <= 'я') || (ch >= 'А' && ch <= 'Я') || ch == 'ё' || ch == 'Ё') {
			return false
		}
	}
	return true
}

func registerUser(username, password string) error {
	if !isValidUsername(username) {
		if len(username) < 3 || len(username) > 20 {
			return fmt.Errorf("username_length")
		}
		return fmt.Errorf("invalid_chars")
	}
	var exists int
	db.QueryRow("SELECT 1 FROM users WHERE username = ?", username).Scan(&exists)
	if exists == 1 {
		return fmt.Errorf("user_exists")
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	_, err := db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, string(hash))
	return err
}

func loginUser(username, password string) bool {
	var hash string
	err := db.QueryRow("SELECT password_hash FROM users WHERE username = ?", username).Scan(&hash)
	if err != nil {
		return false
	}
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		switch msg.Type {
		case "register":
			err := registerUser(msg.From, msg.Password)
			if err != nil {
				errMsg := "Ошибка регистрации"
				switch err.Error() {
				case "username_length":
					errMsg = "Имя должно быть от 3 до 20 символов"
				case "invalid_chars":
					errMsg = "Имя может содержать только буквы, цифры и подчёркивание"
				case "user_exists":
					errMsg = "Имя уже занято"
				}
				conn.WriteJSON(Message{Type: "register_result", Success: false, Error: errMsg})
			} else {
				conn.WriteJSON(Message{Type: "register_result", Success: true, Text: "Регистрация успешна"})
			}

		case "login":
			if loginUser(msg.From, msg.Password) {
				conn.WriteJSON(Message{Type: "login_result", Success: true, Text: "Вход выполнен"})
			} else {
				conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверный логин или пароль"})
			}
		}
	}
}

func main() {
	initDB()
	defer db.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "chat.html")
	})
	http.HandleFunc("/ws", handleWebSocket)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}
