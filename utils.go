package main

import (
	"sort"
	"strings"
	"sync"
)

var clients = make(map[*Client]bool)
var clientsMu = &sync.RWMutex{}

func sanitizeText(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, `"`, "&quot;")
	text = strings.ReplaceAll(text, "'", "&#39;")
	return text
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

func broadcastGroupListToAll() {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for client := range clients {
		groups := getUserGroups(client.Name)
		client.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})
	}
}
