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
	Password  string `json:"password"`
	Success   bool   `json:"success"`
	Error     string `json:"error"`
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "/tmp/chat.db")
	if err != nil { panic(err) }
	
	sql1 := `CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password_hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	
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
	
	sql5 := `CREATE TABLE IF NOT EXISTS user_dialogs (
		user_name TEXT,
		contact_name TEXT,
		last_message_time DATETIME,
		PRIMARY KEY (user_name, contact_name)
	);`
	
	sql6 := `CREATE TABLE IF NOT EXISTS user_groups (
		user_name TEXT,
		group_id TEXT,
		group_name TEXT,
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_name, group_id)
	);`
	
	db.Exec(sql1)
	db.Exec(sql2)
	db.Exec(sql3)
	db.Exec(sql4)
	db.Exec(sql5)
	db.Exec(sql6)
	fmt.Println("База данных готова")
}

func saveDialog(userName, contactName string) {
	db.Exec(`
		INSERT OR REPLACE INTO user_dialogs (user_name, contact_name, last_message_time) 
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, userName, contactName)
}

func getUserDialogs(userName string) []string {
	rows, err := db.Query(`
		SELECT contact_name FROM user_dialogs 
		WHERE user_name = ? 
		ORDER BY last_message_time DESC
	`, userName)
	if err != nil {
		return nil
	}
	defer rows.Close()
	
	var contacts []string
	for rows.Next() {
		var contact string
		rows.Scan(&contact)
		contacts = append(contacts, contact)
	}
	return contacts
}

func saveUserGroup(userName, groupID, groupName string) {
	db.Exec(`
		INSERT OR IGNORE INTO user_groups (user_name, group_id, group_name) 
		VALUES (?, ?, ?)
	`, userName, groupID, groupName)
}

func getUserGroupsFromDB(userName string) []struct{ID, Name string} {
	rows, err := db.Query(`
		SELECT group_id, group_name FROM user_groups 
		WHERE user_name = ?
	`, userName)
	if err != nil {
		return nil
	}
	defer rows.Close()
	
	var groups []struct{ID, Name string}
	for rows.Next() {
		var g struct{ID, Name string}
		rows.Scan(&g.ID, &g.Name)
		groups = append(groups, g)
	}
	return groups
}

func registerUser(username, password string) error {
	var exists int
	err := db.QueryRow("SELECT 1 FROM users WHERE username = ?", username).Scan(&exists)
	if err == nil {
		return fmt.Errorf("user_exists")
	}
	
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	
	_, err = db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, string(hash))
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
	return err == nil
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

func getGroupName(groupID string) string {
	var name string
	db.QueryRow("SELECT name FROM groups WHERE id = ?", groupID).Scan(&name)
	return name
}

func searchUsers(query, currentUser string) []string {
	rows, err := db.Query(`
		SELECT username FROM users 
		WHERE username LIKE ? AND username != ?
		LIMIT 10
	`, "%"+query+"%", currentUser)
	if err != nil {
		return nil
	}
	defer rows.Close()
	
	var users []string
	for rows.Next() {
		var user string
		rows.Scan(&user)
		users = append(users, user)
	}
	return users
}

func getAllUsers(currentUser string) []string {
	rows, err := db.Query("SELECT username FROM users WHERE username != ?", currentUser)
	if err != nil {
		return nil
	}
	defer rows.Close()
	
	var users []string
	for rows.Next() {
		var user string
		rows.Scan(&user)
		users = append(users, user)
	}
	return users
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

func sendGroupList(client *Client) {
	groups := getUserGroupsFromDB(client.Name)
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
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	fmt.Println("=== МЕССЕНДЖЕР С АККАУНТАМИ ===")
	fmt.Println("Сервер запущен на порту:", port)
	http.ListenAndServe(":"+port, nil)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	
	var username string
	
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil { return }
		
		msg.Time = time.Now().Format("15:04")
		
		switch msg.Type {
		case "register":
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
			if loginUser(msg.From, msg.Password) {
				username = msg.From
				client := &Client{Conn: conn, Name: username}
				
				mutex.Lock()
				clients[client] = true
				mutex.Unlock()
				
				fmt.Println(username, "вошел в систему")
				broadcastOnlineList()
				sendGroupList(client)
				
				contacts := getUserDialogs(username)
				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})
				
				allUsers := getAllUsers(username)
				conn.WriteJSON(Message{Type: "user_list", Text: strings.Join(allUsers, ",")})
				
				conn.WriteJSON(Message{Type: "login_result", Success: true, Text: "Вход выполнен"})
				
				handleChat(client)
				return
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
					m = strings.TrimSpace(m)
					if m != client.Name && m != "" {
						joinGroup(groupID, m)
						saveUserGroup(m, groupID, msg.GroupName)
					}
				}
				saveUserGroup(client.Name, groupID, msg.GroupName)
				client.Conn.WriteJSON(Message{Type: "group_created", To: groupID, GroupName: msg.GroupName})
				broadcastGroupListToAll()
			}
			
		case "search_users":
			query := strings.ToLower(msg.Text)
			users := searchUsers(query, client.Name)
			client.Conn.WriteJSON(Message{
				Type: "search_results",
				Text: strings.Join(users, ","),
			})
			
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
				
				saveDialog(client.Name, msg.To)
				saveDialog(msg.To, client.Name)
				
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

func broadcastGroupListToAll() {
	mutex.Lock()
	defer mutex.Unlock()
	for c := range clients {
		sendGroupList(c)
	}
}