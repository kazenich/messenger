
package main

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"sync"
	"fmt"
	"time"
	"os"
	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

type Client struct {
	Conn *websocket.Conn
	Name string
	CurrentDialog string
	IsGroup bool
}

var clients = make(map[*Client]bool)
var mutex = &sync.Mutex{}
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
	Password  string `json:"password"` // Для регистрации/входа
	Success   bool   `json:"success"`  // Ответ от сервера
	Error     string `json:"error"`    // Ошибка
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "/tmp/chat.db")
	if err != nil { panic(err) }
	
	// Таблица пользователей (логин + пароль)
	sql1 := `CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password_hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	
	// Таблица сообщений (остается как была)
	sql2 := `CREATE TABLE IF NOT EXISTS private_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		dialog_key TEXT,
		from_user TEXT,
		to_user TEXT,
		text TEXT,
		time TEXT,
		is_group INTEGER DEFAULT 0,
		group_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	
	// Таблица групп
	sql3 := `CREATE TABLE IF NOT EXISTS groups (
		id TEXT PRIMARY KEY,
		name TEXT,
		creator TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	
	sql4 := `CREATE TABLE IF NOT EXISTS group_members (
		group_id TEXT,
		user_name TEXT,
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (group_id, user_name)
	);`
	
	db.Exec(sql1)
	db.Exec(sql2)
	db.Exec(sql3)
	db.Exec(sql4)
	fmt.Println("База данных с пользователями готова")
}

// Регистрация нового пользователя
func registerUser(username, password string) error {
	// Проверяем, есть ли уже такой пользователь
	var exists int
	err := db.QueryRow("SELECT 1 FROM users WHERE username = ?", username).Scan(&exists)
	if err == nil {
		return fmt.Errorf("user_exists")
	}
	
	// Хешируем пароль (bcrypt)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	
	// Сохраняем
	_, err = db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, string(hash))
	return err
}

// Проверка логина и пароля
func loginUser(username, password string) bool {
	var hash string
	err := db.QueryRow("SELECT password_hash FROM users WHERE username = ?", username).Scan(&hash)
	if err != nil {
		return false // Пользователь не найден
	}
	
	// Проверяем пароль
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Остальные функции (getDialogKey, saveMessage, getPrivateHistory и т.д.) остаются как в прошлой версии
func getDialogKey(u1, u2 string) string {
	if u1 > u2 {
		return u2 + "_" + u1
	}
	return u1 + "_" + u2
}

func saveMessage(from, to, text string, isGroup bool, groupID string) {
	if isGroup {
		_, err := db.Exec("INSERT INTO private_messages (from_user, to_user, text, time, is_group, group_id) VALUES (?, ?, ?, ?, ?, ?)", 
			from, to, text, time.Now().Format("15:04"), 1, groupID)
		if err != nil { fmt.Println("Ошибка сохранения:", err) }
	} else {
		key := getDialogKey(from, to)
		_, err := db.Exec("INSERT INTO private_messages (dialog_key, from_user, to_user, text, time, is_group) VALUES (?, ?, ?, ?, ?, ?)", 
			key, from, to, text, time.Now().Format("15:04"), 0)
		if err != nil { fmt.Println("Ошибка сохранения:", err) }
	}
}

func getPrivateHistory(u1, u2 string) []Message {
	key := getDialogKey(u1, u2)
	rows, err := db.Query("SELECT from_user, text, time FROM private_messages WHERE dialog_key = ? AND is_group = 0 ORDER BY id ASC LIMIT 50", key)
	if err != nil { return nil }
	defer rows.Close()
	
	var messages []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.From, &m.Text, &m.Time)
		m.Type = "history"
		messages = append(messages, m)
	}
	return messages
}

func getGroupHistory(groupID string) []Message {
	rows, err := db.Query("SELECT from_user, text, time FROM private_messages WHERE group_id = ? ORDER BY id ASC LIMIT 50", groupID)
	if err != nil { return nil }
	defer rows.Close()
	
	var messages []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.From, &m.Text, &m.Time)
		m.Type = "history"
		messages = append(messages, m)
	}
	return messages
}

func createGroup(name, creator string) string {
	groupID := "group_" + fmt.Sprintf("%d", time.Now().Unix())
	_, err := db.Exec("INSERT INTO groups (id, name, creator) VALUES (?, ?, ?)", groupID, name, creator)
	if err != nil {
		fmt.Println("Ошибка создания группы:", err)
		return ""
	}
	db.Exec("INSERT INTO group_members (group_id, user_name) VALUES (?, ?)", groupID, creator)
	return groupID
}

func joinGroup(groupID, userName string) bool {
	_, err := db.Exec("INSERT OR IGNORE INTO group_members (group_id, user_name) VALUES (?, ?)", groupID, userName)
	if err != nil {
		fmt.Println("Ошибка входа в группу:", err)
		return false
	}
	return true
}

func getUserGroups(userName string) []struct{ID, Name string} {
	rows, err := db.Query("SELECT g.id, g.name FROM groups g JOIN group_members gm ON g.id = gm.group_id WHERE gm.user_name = ?", userName)
	if err != nil { return nil }
	defer rows.Close()
	
	var groups []struct{ID, Name string}
	for rows.Next() {
		var g struct{ID, Name string}
		rows.Scan(&g.ID, &g.Name)
		groups = append(groups, g)
	}
	return groups
}

func getGroupMembers(groupID string) []string {
	rows, err := db.Query("SELECT user_name FROM group_members WHERE group_id = ?", groupID)
	if err != nil { return nil }
	defer rows.Close()
	
	var members []string
	for rows.Next() {
		var m string
		rows.Scan(&m)
		members = append(members, m)
	}
	return members
}

func getOnlineUsers(except string) []string {
	mutex.Lock()
	defer mutex.Unlock()
	
	var users []string
	for client := range clients {
		if client.Name != except {
			users = append(users, client.Name)
		}
	}
	sort.Strings(users)
	return users
}

func broadcastOnlineList() {
	mutex.Lock()
	defer mutex.Unlock()
	
	users := []string{}
	for c := range clients {
		users = append(users, c.Name)
	}
	sort.Strings(users)
	
	msg := Message{Type: "online", Text: strings.Join(users, ",")}
	for c := range clients {
		c.Conn.WriteJSON(msg)
	}
}

func broadcastGroupList() {
	mutex.Lock()
	defer mutex.Unlock()
	for c := range clients {
		sendGroupList(c)
	}
}

func sendGroupList(client *Client) {
	groups := getUserGroups(client.Name)
	var list []string
	for _, g := range groups {
		list = append(list, g.ID+"|"+g.Name)
	}
	client.Conn.WriteJSON(Message{
		Type: "group_list",
		Text: strings.Join(list, ","),
	})
}

func main() {
	initDB()
	defer db.Close()
	
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "chat.html")
	})
	http.HandleFunc("/ws", handleWebSocket)
	
	fmt.Println("=== МЕССЕНДЖЕР С АККАУНТАМИ ===")
	fmt.Println("Откройте: http://localhost:8080")
	port := os.Getenv("PORT")
if port == "" {
    port = "8080"
}
fmt.Println("Сервер запущен на порту:", port)
http.ListenAndServe(":"+port, nil)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	
	var username string // Запоминаем имя после авторизации
	
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil { return }
		
		msg.Time = time.Now().Format("15:04")
		
		switch msg.Type {
		case "register":
			// Регистрация нового пользователя
			err := registerUser(msg.From, msg.Password)
			if err != nil {
				if err.Error() == "user_exists" {
					conn.WriteJSON(Message{Type: "register_result", Success: false, Error: "Имя пользователя занято"})
				} else {
					conn.WriteJSON(Message{Type: "register_result", Success: false, Error: "Ошибка регистрации"})
				}
			} else {
				conn.WriteJSON(Message{Type: "register_result", Success: true, Text: "Регистрация успешна! Войдите в систему"})
			}
			
		case "login":
			// Вход в систему
			if loginUser(msg.From, msg.Password) {
				username = msg.From
				client := &Client{Conn: conn, Name: username}
				
				mutex.Lock()
				clients[client] = true
				mutex.Unlock()
				
				fmt.Println(username, "вошел в систему")
				broadcastOnlineList()
				sendGroupList(client)
				
				conn.WriteJSON(Message{Type: "login_result", Success: true, Text: "Вход выполнен"})
				
				// Теперь ждем обычные сообщения (чат)
				handleChat(client)
				return // Выходим из цикла, handleChat берет управление
			} else {
				conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверное имя или пароль"})
			}
		}
	}
}

func handleChat(client *Client) {
	defer func() {
		mutex.Lock()
		delete(clients, client)
		mutex.Unlock()
		broadcastOnlineList()
		client.Conn.Close()
	}()
	
	for {
		var msg Message
		err := client.Conn.ReadJSON(&msg)
		if err != nil { return }
		
		msg.From = client.Name
		msg.Time = time.Now().Format("15:04")
		
		switch msg.Type {
		case "select_dialog":
			client.IsGroup = false
			client.CurrentDialog = msg.To
			history := getPrivateHistory(client.Name, msg.To)
			for _, m := range history {
				client.Conn.WriteJSON(m)
			}
			
		case "select_group":
			client.IsGroup = true
			client.CurrentDialog = msg.To
			history := getGroupHistory(msg.To)
			for _, m := range history {
				client.Conn.WriteJSON(m)
			}
			members := getGroupMembers(msg.To)
			client.Conn.WriteJSON(Message{Type: "member_list", Text: strings.Join(members, ",")})
			
		case "create_group":
			groupID := createGroup(msg.GroupName, client.Name)
			if groupID != "" {
				members := strings.Split(msg.Members, ",")
				for _, m := range members {
					if m != client.Name {
						joinGroup(groupID, m)
					}
				}
				client.Conn.WriteJSON(Message{Type: "group_created", To: groupID, GroupName: msg.GroupName})
				broadcastGroupList()
			}
			
		case "message":
			if client.IsGroup {
				groupID := client.CurrentDialog
				members := getGroupMembers(groupID)
				saveMessage(client.Name, "", msg.Text, true, groupID)
				
				mutex.Lock()
				for c := range clients {
					for _, member := range members {
						if c.Name == member && c.CurrentDialog == groupID && c.IsGroup {
							c.Conn.WriteJSON(Message{
								From: client.Name,
								Text: msg.Text,
								Time: msg.Time,
								Type: "message",
								IsGroup: true,
							})
							break
						}
					}
				}
				mutex.Unlock()
			} else {
				if msg.To == "" { continue }
				saveMessage(client.Name, msg.To, msg.Text, false, "")
				
				client.Conn.WriteJSON(Message{
					From: client.Name,
					To: msg.To,
					Text: msg.Text,
					Time: msg.Time,
					Type: "message",
					IsGroup: false,
				})
				
				mutex.Lock()
				for c := range clients {
					if c.Name == msg.To && c.CurrentDialog == client.Name && !c.IsGroup {
						c.Conn.WriteJSON(Message{
							From: client.Name,
							To: msg.To,
							Text: msg.Text,
							Time: msg.Time,
							Type: "message",
							IsGroup: false,
						})
						break
					}
				}
				mutex.Unlock()
			}
		}
	}
}
