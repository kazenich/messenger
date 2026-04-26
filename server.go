package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

type Client struct {
	Conn          *websocket.Conn
	Name          string
	CurrentDialog string
	IsGroup       bool
}

var clients = make(map[*Client]bool)
var clientsMu = &sync.RWMutex{}
var db *sql.DB

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Message struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Text      string `json:"text"`
	Time      string `json:"time"`
	Type      string `json:"type"`
	IsGroup   bool   `json:"isGroup"`
	GroupName string `json:"groupName"`
	Members   string `json:"members"`
	Password  string `json:"password"`
	Success   bool   `json:"success"`
	Error     string `json:"error"`
	ImageData string `json:"imageData"`
	Sticker   string `json:"sticker"`
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

	db.Exec(`CREATE TABLE IF NOT EXISTS contacts (
		user_name TEXT,
		contact_name TEXT,
		PRIMARY KEY (user_name, contact_name)
	)`)

	fmt.Println("База данных SQLite готова")
}

func isValidUsername(username string) bool {
	if len(username) < 3 || len(username) > 20 {
		return false
	}
	for _, ch := range username {
		if ch == ' ' {
			continue
		}
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

func addContact(user, contact string) error {
	var exists int
	db.QueryRow("SELECT 1 FROM users WHERE username = ?", contact).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("user_not_found")
	}
	db.Exec("INSERT OR IGNORE INTO contacts (user_name, contact_name) VALUES (?, ?)", user, contact)
	return nil
}

func getContacts(user string) []string {
	rows, _ := db.Query("SELECT contact_name FROM contacts WHERE user_name = ?", user)
	defer rows.Close()
	var contacts []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		contacts = append(contacts, c)
	}
	return contacts
}

func getAllUsers(current string) []string {
	rows, _ := db.Query("SELECT username FROM users WHERE username != ?", current)
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		users = append(users, u)
	}
	return users
}

func searchUsers(query, current string) []string {
	rows, _ := db.Query("SELECT username FROM users WHERE username LIKE ? AND username != ? LIMIT 10", "%"+query+"%", current)
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		users = append(users, u)
	}
	return users
}

func getOnlineUsers() []string {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	var users []string
	for client := range clients {
		users = append(users, client.Name)
	}
	sort.Strings(users)
	return users
}

func broadcastOnlineList() {
	users := getOnlineUsers()
	msg := Message{Type: "online", Text: strings.Join(users, ",")}
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for client := range clients {
		client.Conn.WriteJSON(msg)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var currentUser string
	var client *Client

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			if currentUser != "" {
				clientsMu.Lock()
				delete(clients, client)
				clientsMu.Unlock()
				broadcastOnlineList()
			}
			return
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
					errMsg = "Имя может содержать только буквы, цифры, пробелы и подчёркивание"
				case "user_exists":
					errMsg = "Имя уже занято"
				}
				conn.WriteJSON(Message{Type: "register_result", Success: false, Error: errMsg})
			} else {
				conn.WriteJSON(Message{Type: "register_result", Success: true, Text: "Регистрация успешна"})
			}

		case "login":
			if loginUser(msg.From, msg.Password) {
				currentUser = msg.From
				client = &Client{Conn: conn, Name: msg.From}
				clientsMu.Lock()
				clients[client] = true
				clientsMu.Unlock()

				contacts := getContacts(msg.From)
				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})

				allUsers := getAllUsers(msg.From)
				conn.WriteJSON(Message{Type: "user_list", Text: strings.Join(allUsers, ",")})

				conn.WriteJSON(Message{Type: "login_result", Success: true})
				broadcastOnlineList()
			} else {
				conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверный логин или пароль"})
			}

		case "get_users":
			users := getAllUsers("")
			conn.WriteJSON(Message{Type: "user_list", Text: strings.Join(users, ",")})

		case "search_users":
			users := searchUsers(msg.Text, "")
			conn.WriteJSON(Message{Type: "search_results", Text: strings.Join(users, ",")})

		case "add_contact":
			err := addContact(msg.From, msg.Text)
			if err != nil {
				conn.WriteJSON(Message{Type: "add_contact_result", Success: false, Error: err.Error()})
			} else {
				contacts := getContacts(msg.From)
				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})
				conn.WriteJSON(Message{Type: "add_contact_result", Success: true, Text: "Контакт добавлен"})
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
