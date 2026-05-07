package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

func initDB() {
	var err error
	
	os.MkdirAll("/app/data", 0755)
	
	db, err = sql.Open("sqlite", "/app/data/chat.db")
	if err != nil {
		panic(err)
	}

	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password_hash TEXT
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
		group_id TEXT
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS groups (
		id TEXT PRIMARY KEY,
		name TEXT,
		creator TEXT
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS group_members (
		group_id TEXT,
		user_name TEXT,
		PRIMARY KEY (group_id, user_name)
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS contacts (
		user_name TEXT,
		contact_name TEXT,
		PRIMARY KEY (user_name, contact_name)
	)`)

	fmt.Println("База данных SQLite готова")
	
	var count int
	db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	fmt.Printf("[DEBUG] В БД %d сообщений\n", count)
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
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func saveMessage(from, to, text, imageData, sticker string, isGroup bool, groupID string) {
	db.Exec(`INSERT INTO messages (from_user, to_user, text, image_data, sticker, time, is_group, group_id) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, from, to, text, imageData, sticker, time.Now().Format("15:04"), isGroup, groupID)
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
	log.Printf("[ПОИСК] Запрос: '%s', текущий: '%s'", query, current)
	rows, _ := db.Query("SELECT username FROM users WHERE username LIKE ? AND username != ? LIMIT 10", "%"+query+"%", current)
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		users = append(users, u)
	}
	log.Printf("[ПОИСК] Найдено: %d -> %v", len(users), users)
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
