package main

import (
	"sort"
	"strings"
	"sync"
)

var clients = make(map[*Client]bool)
var mutex = &sync.Mutex{}
var db *sql.DB

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

func sendGroupList(client *Client) {
	groups := getUserGroups(client.Name)
	var list []string
	for _, g := range groups {
		list = append(list, g+"|"+getGroupName(g))
	}
	client.Conn.WriteJSON(Message{
		Type: "group_list",
		Text: strings.Join(list, ","),
	})
}

func broadcastGroupListToAll() {
	mutex.Lock()
	defer mutex.Unlock()
	for c := range clients {
		sendGroupList(c)
	}
}