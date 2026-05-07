package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	log.Printf("WebSocket соединение установлено")

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Ошибка чтения: %v", err)
			conn.Close()
			return
		}

		log.Printf("type=%s from=%s password=%s text=%s", msg.Type, msg.From, msg.Password, msg.Text)

		switch msg.Type {
		case "register":
			log.Printf("Регистрация: user=%s pass=%s", msg.From, msg.Password)
			err := registerUser(msg.From, msg.Password)
			if err != nil {
				log.Printf("Ошибка: %v", err)
				conn.WriteJSON(Message{Type: "register_result", Success: false, Error: "Имя занято"})
			} else {
				conn.WriteJSON(Message{Type: "register_result", Success: true})
			}

		case "login":
			log.Printf("Вход: user=%s pass=%s", msg.From, msg.Password)
			if loginUser(msg.From, msg.Password) {
				clientsMu.Lock()
				for c := range clients {
					if c.Name == msg.From {
						c.Conn.Close()
						delete(clients, c)
						break
					}
				}
				clientsMu.Unlock()

				client := &Client{Conn: conn, Name: msg.From}
				clientsMu.Lock()
				clients[client] = true
				clientsMu.Unlock()

				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(getContacts(msg.From), ",")})
				conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(getUserGroups(msg.From), ",")})
				conn.WriteJSON(Message{Type: "login_result", Success: true})
				broadcastOnlineList()
				handleChat(client)
				return
			}
			conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверный логин или пароль"})

		case "search_users":
			users := searchUsers(msg.Text, "")
			conn.WriteJSON(Message{Type: "search_results", Text: strings.Join(users, ",")})
		}
	}
}

func handleChat(client *Client) {
	defer func() {
		clientsMu.Lock()
		delete(clients, client)
		clientsMu.Unlock()
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
		msg.Text = sanitizeText(msg.Text)

		switch msg.Type {
		case "select_dialog":
			client.CurrentDialog = msg.To
			client.IsGroup = false
			for _, h := range getPrivateHistory(client.Name, msg.To) {
				client.Conn.WriteJSON(h)
			}

		case "select_group":
			client.CurrentDialog = msg.To
			client.IsGroup = true
			for _, h := range getGroupHistory(msg.To) {
				client.Conn.WriteJSON(h)
			}
			client.Conn.WriteJSON(Message{
				Type: "member_list", Text: strings.Join(getGroupMembers(msg.To), ","),
				GroupName: getGroupCreator(msg.To),
			})

		case "search_users":
			users := searchUsers(msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "search_results", Text: strings.Join(users, ",")})

		case "get_contacts":
			client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(getContacts(client.Name), ",")})

		case "add_contact":
			addContact(client.Name, msg.Text)
			client.Conn.WriteJSON(Message{Type: "add_contact_result", Success: true})
			client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(getContacts(client.Name), ",")})

		case "message":
			saveMessage(client.Name, msg.To, msg.Text, msg.ImageData, msg.Sticker, client.IsGroup, client.CurrentDialog)
			client.Conn.WriteJSON(Message{
				From: client.Name, Text: msg.Text, Sticker: msg.Sticker,
				Time: msg.Time, Type: "message", IsGroup: client.IsGroup,
			})
			if !client.IsGroup {
				clientsMu.RLock()
				for c := range clients {
					if c.Name == msg.To {
						c.Conn.WriteJSON(Message{
							From: client.Name, Text: msg.Text, Sticker: msg.Sticker,
							Time: msg.Time, Type: "message",
						})
						break
					}
				}
				clientsMu.RUnlock()
			}

		case "get_groups":
			client.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(getUserGroups(client.Name), ",")})

		case "create_group":
			groupID := createGroup(msg.GroupName, client.Name)
			for _, m := range strings.Split(msg.Members, ",") {
				m = strings.TrimSpace(m)
				if m != "" && m != client.Name {
					addMemberToGroup(groupID, m, client.Name)
				}
			}
			broadcastGroupListToAll()
			client.Conn.WriteJSON(Message{Type: "group_created", Success: true})

		case "get_group_members":
			client.Conn.WriteJSON(Message{
				Type: "member_list", Text: strings.Join(getGroupMembers(msg.To), ","),
				GroupName: getGroupCreator(msg.To),
			})

		case "add_group_member":
			addMemberToGroup(msg.To, msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true})

		case "remove_group_member":
			removeMemberFromGroup(msg.To, msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true})
		}
	}
}
