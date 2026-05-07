package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

func initDB() {
	var err error
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		panic("DATABASE_URL не установлен")
	}

	db, err = sql.Open("postgres", databaseURL)
	if err != nil {
		panic(err)
	}

	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		password_hash TEXT
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id SERIAL PRIMARY KEY,
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

	fmt.Println("PostgreSQL готова")
}

func registerUser(username, password string) error {
	var exists int
	db.QueryRow("SELECT 1 FROM users WHERE username = $1", username).Scan(&exists)
	if exists == 1 {
		return fmt.Errorf("user_exists")
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	_, err := db.Exec("INSERT INTO users (username, password_hash) VALUES ($1, $2)", username, string(hash))
	return err
}

func loginUser(username, password string) bool {
	var hash string
	err := db.QueryRow("SELECT password_hash FROM users WHERE username = $1", username).Scan(&hash)
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func saveMessage(from, to, text, imageData, sticker string, isGroup bool, groupID string) {
	db.Exec(`INSERT INTO messages (from_user, to_user, text, image_data, sticker, time, is_group, group_id) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		from, to, text, imageData, sticker, time.Now().Format("15:04"), isGroup, groupID)
}

func getPrivateHistory(u1, u2 string) []Message {
	rows, _ := db.Query(`SELECT from_user, text, image_data, sticker, time FROM messages 
		WHERE is_group = 0 AND ((from_user = $1 AND to_user = $2) OR (from_user = $3 AND to_user = $4)) 
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
		WHERE is_group = 1 AND group_id = $1 ORDER BY id ASC LIMIT 50`, groupID)
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
	db.Exec("INSERT INTO groups (id, name, creator) VALUES ($1, $2, $3)", groupID, name, creator)
	db.Exec("INSERT INTO group_members (group_id, user_name) VALUES ($1, $2)", groupID, creator)
	return groupID
}

func addMemberToGroup(groupID, userName, creator string) error {
	db.Exec("INSERT INTO group_members (group_id, user_name) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, userName)
	return nil
}

func removeMemberFromGroup(groupID, userName, creator string) error {
	if userName == creator {
		return fmt.Errorf("cannot_remove_creator")
	}
	db.Exec("DELETE FROM group_members WHERE group_id = $1 AND user_name = $2", groupID, userName)
	return nil
}

func getGroupMembers(groupID string) []string {
	rows, _ := db.Query("SELECT user_name FROM group_members WHERE group_id = $1", groupID)
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
	db.QueryRow("SELECT name FROM groups WHERE id = $1", groupID).Scan(&name)
	return name
}

func getGroupCreator(groupID string) string {
	var creator string
	db.QueryRow("SELECT creator FROM groups WHERE id = $1", groupID).Scan(&creator)
	return creator
}

func getUserGroups(username string) []string {
	rows, _ := db.Query("SELECT group_id FROM group_members WHERE user_name = $1", username)
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
	db.Exec("INSERT INTO contacts (user_name, contact_name) VALUES ($1, $2) ON CONFLICT DO NOTHING", user, contact)
	return nil
}

func getContacts(user string) []string {
	rows, _ := db.Query("SELECT contact_name FROM contacts WHERE user_name = $1", user)
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
	db.Exec("DELETE FROM contacts WHERE user_name = $1 AND contact_name = $2", user, contact)
}

func searchUsers(query, current string) []string {
	rows, _ := db.Query("SELECT username FROM users WHERE username ILIKE $1 AND username != $2 LIMIT 10", "%"+query+"%", current)
	defer rows.Close()
	var users []string
	for rows.Next() {
		var u string
		rows.Scan(&u)
		users = append(users, u)
	}
	return users
}
