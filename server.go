package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

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
		password_hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_user TEXT,
		to_user TEXT,
		text TEXT,
		image_data TEXT,
		sticker TEXT,
		time TEXT,
		is_group INTEGER DEFAULT 0,
		group_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS groups (
		id TEXT PRIMARY KEY,
		name TEXT,
		creator TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS group_members (
		group_id TEXT,
		user_name TEXT,
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (group_id, user_name)
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS contacts (
		user_name TEXT,
		contact_name TEXT,
		PRIMARY KEY (user_name, contact_name)
	)`)

	fmt.Println("База данных SQLite готова")
}

func registerUser(username, password string) error {
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

func saveMessage(from, to, text, imageData, sticker string, isGroup bool, groupID string) {
	db.Exec(`INSERT INTO messages (from_user, to_user, text, image_data, sticker, time, is_group, group_id) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		from, to, text, imageData, sticker, time.Now().Format("15:04"), isGroup, groupID)
}

func getPrivateHistory(u1, u2 string) []Message {
	rows, _ := db.Query(`SELECT from_user, text, image_data, sticker, time FROM messages 
		WHERE is_group = 0 AND ((from_user = ? AND to_user = ?) OR (from_user = ? AND to_user = ?)) 
		ORDER BY id ASC LIMIT 50`, u1, u2, u2, u1)
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.From, &m.Text, &m.ImageData, &m.Sticker, &m.Time)
		m.Type = "history"
		msgs = append(msgs, m)
	}
	return msgs
}

func getGroupHistory(groupID string) []Message {
	rows, _ := db.Query(`SELECT from_user, text, image_data, sticker, time FROM messages 
		WHERE is_group = 1 AND group_id = ? ORDER BY id ASC LIMIT 50`, groupID)
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		rows.Scan(&m.From, &m.Text, &m.ImageData, &m.Sticker, &m.Time)
		m.Type = "history"
		msgs = append(msgs, m)
	}
	return msgs
}

func createGroup(name, creator string) string {
	groupID := "g" + fmt.Sprintf("%d", time.Now().UnixNano())
	db.Exec("INSERT INTO groups (id, name, creator) VALUES (?, ?, ?)", groupID, name, creator)
	db.Exec("INSERT INTO group_members (group_id, user_name) VALUES (?, ?)", groupID, creator)
	return groupID
}

func addMemberToGroup(groupID, userName, creator string) error {
	var exists int
	db.QueryRow("SELECT 1 FROM groups WHERE id = ? AND creator = ?", groupID, creator).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("not_creator")
	}
	db.Exec("INSERT OR IGNORE INTO group_members (group_id, user_name) VALUES (?, ?)", groupID, userName)
	return nil
}

func removeMemberFromGroup(groupID, userName, creator string) error {
	var exists int
	db.QueryRow("SELECT 1 FROM groups WHERE id = ? AND creator = ?", groupID, creator).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("not_creator")
	}
	if userName == creator {
		return fmt.Errorf("cannot_remove_creator")
	}
	db.Exec("DELETE FROM group_members WHERE group_id = ? AND user_name = ?", groupID, userName)
	return nil
}

func getGroupMembers(groupID string) []string {
	rows, _ := db.Query("SELECT user_name FROM group_members WHERE group_id = ?", groupID)
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

func getGroupCreator(groupID string) string {
	var creator string
	db.QueryRow("SELECT creator FROM groups WHERE id = ?", groupID).Scan(&creator)
	return creator
}

func getUserGroups(username string) []string {
	rows, _ := db.Query("SELECT group_id FROM group_members WHERE user_name = ?", username)
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var g string
		rows.Scan(&g)
		groups = append(groups, g+"|"+getGroupName(g))
	}
	return groups
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

func deleteContact(user, contact string) {
	db.Exec("DELETE FROM contacts WHERE user_name = ? AND contact_name = ?", user, contact)
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

func getOnlineUsers() []string {
	mutex.Lock()
	defer mutex.Unlock()
	var users []string
	for c := range clients {
		users = append(users, c.Name)
	}
	sort.Strings(users)
	return users
}

func broadcastOnlineList() {
	mutex.Lock()
	defer mutex.Unlock()
	users := getOnlineUsers()
	msg := Message{Type: "online", Text: strings.Join(users, ",")}
	for c := range clients {
		c.Conn.WriteJSON(msg)
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

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			return
		}

		switch msg.Type {
		case "register":
			err := registerUser(msg.From, msg.Password)
			if err != nil {
				conn.WriteJSON(Message{Type: "register_result", Success: false, Error: "Имя занято"})
			} else {
				conn.WriteJSON(Message{Type: "register_result", Success: true, Text: "Регистрация успешна"})
			}

		case "login":
			if loginUser(msg.From, msg.Password) {
				client := &Client{Conn: conn, Name: msg.From}
				mutex.Lock()
				clients[client] = true
				mutex.Unlock()

				contacts := getContacts(msg.From)
				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})

				groups := getUserGroups(msg.From)
				conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})

				conn.WriteJSON(Message{Type: "login_result", Success: true})
				broadcastOnlineList()

				handleChat(client)
				return
			} else {
				conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверный логин или пароль"})
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
		if err != nil {
			return
		}
		msg.From = client.Name
		msg.Time = time.Now().Format("15:04")

		switch msg.Type {
		case "select_dialog":
			client.CurrentDialog = msg.To
			client.IsGroup = false
			history := getPrivateHistory(client.Name, msg.To)
			for _, h := range history {
				client.Conn.WriteJSON(h)
			}

		case "select_group":
			client.CurrentDialog = msg.To
			client.IsGroup = true
			history := getGroupHistory(msg.To)
			for _, h := range history {
				client.Conn.WriteJSON(h)
			}
			members := getGroupMembers(msg.To)
			creator := getGroupCreator(msg.To)
			client.Conn.WriteJSON(Message{
				Type:      "member_list",
				Text:      strings.Join(members, ","),
				GroupName: creator,
			})

		case "create_group":
			groupID := createGroup(msg.GroupName, client.Name)
			members := strings.Split(msg.Members, ",")
			for _, m := range members {
				m = strings.TrimSpace(m)
				if m != "" && m != client.Name {
					addMemberToGroup(groupID, m, client.Name)
				}
			}
			mutex.Lock()
			for c := range clients {
				groups := getUserGroups(c.Name)
				c.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})
			}
			mutex.Unlock()
			client.Conn.WriteJSON(Message{Type: "group_created", Success: true})

		case "add_group_member":
			err := addMemberToGroup(msg.To, msg.Text, client.Name)
			if err != nil {
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: false, Error: err.Error()})
			} else {
				members := getGroupMembers(msg.To)
				creator := getGroupCreator(msg.To)
				mutex.Lock()
				for c := range clients {
					for _, m := range members {
						if c.Name == m {
							c.Conn.WriteJSON(Message{
								Type:      "member_list",
								Text:      strings.Join(members, ","),
								GroupName: creator,
							})
							groups := getUserGroups(c.Name)
							c.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})
							break
						}
					}
				}
				mutex.Unlock()
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true, Text: "Участник добавлен"})
			}

		case "remove_group_member":
			err := removeMemberFromGroup(msg.To, msg.Text, client.Name)
			if err != nil {
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: false, Error: err.Error()})
			} else {
				members := getGroupMembers(msg.To)
				creator := getGroupCreator(msg.To)
				mutex.Lock()
				for c := range clients {
					for _, m := range members {
						if c.Name == m {
							c.Conn.WriteJSON(Message{
								Type:      "member_list",
								Text:      strings.Join(members, ","),
								GroupName: creator,
							})
							groups := getUserGroups(c.Name)
							c.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})
							break
						}
					}
					if c.Name == msg.Text {
						groups := getUserGroups(c.Name)
						c.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})
					}
				}
				mutex.Unlock()
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true, Text: "Участник удалён"})
			}

		case "search_users":
			users := searchUsers(msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "search_results", Text: strings.Join(users, ",")})

		case "get_users":
			users := getAllUsers(client.Name)
			client.Conn.WriteJSON(Message{Type: "user_list", Text: strings.Join(users, ",")})

		case "add_contact":
			err := addContact(client.Name, msg.Text)
			if err != nil {
				client.Conn.WriteJSON(Message{Type: "add_contact_result", Success: false, Error: err.Error()})
			} else {
				contacts := getContacts(client.Name)
				client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})
				client.Conn.WriteJSON(Message{Type: "add_contact_result", Success: true, Text: "Контакт добавлен"})
			}

		case "delete_contact":
			deleteContact(client.Name, msg.Text)
			contacts := getContacts(client.Name)
			client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})
			client.Conn.WriteJSON(Message{Type: "delete_contact_result", Success: true, Text: "Контакт удалён"})

		case "message":
			if client.IsGroup {
				groupID := client.CurrentDialog
				members := getGroupMembers(groupID)
				saveMessage(client.Name, "", msg.Text, msg.ImageData, msg.Sticker, true, groupID)
				mutex.Lock()
				for c := range clients {
					for _, m := range members {
						if c.Name == m && c.CurrentDialog == groupID && c.IsGroup {
							c.Conn.WriteJSON(Message{
								From:      client.Name,
								Text:      msg.Text,
								ImageData: msg.ImageData,
								Sticker:   msg.Sticker,
								Time:      msg.Time,
								Type:      "message",
								IsGroup:   true,
							})
							break
						}
					}
				}
				mutex.Unlock()
			} else {
				saveMessage(client.Name, msg.To, msg.Text, msg.ImageData, msg.Sticker, false, "")
				addContact(client.Name, msg.To)
				addContact(msg.To, client.Name)
				client.Conn.WriteJSON(Message{
					From:      client.Name,
					Text:      msg.Text,
					ImageData: msg.ImageData,
					Sticker:   msg.Sticker,
					Time:      msg.Time,
					Type:      "message",
					IsGroup:   false,
				})
				mutex.Lock()
				for c := range clients {
					if c.Name == msg.To {
						c.Conn.WriteJSON(Message{
							From:      client.Name,
							Text:      msg.Text,
							ImageData: msg.ImageData,
							Sticker:   msg.Sticker,
							Time:      msg.Time,
							Type:      "message",
							IsGroup:   false,
						})
						break
					}
				}
				mutex.Unlock()
			}
		}
	}
} 
