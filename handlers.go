package main

import (
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
			broadcastGroupListToAll()
			client.Conn.WriteJSON(Message{Type: "group_created", Success: true})

		case "add_group_member":
			err := addMemberToGroup(msg.To, msg.Text, client.Name)
			if err != nil {
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: false, Error: err.Error()})
			} else {
				broadcastGroupListToAll()
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true, Text: "Участник добавлен"})
			}

		case "remove_group_member":
			err := removeMemberFromGroup(msg.To, msg.Text, client.Name)
			if err != nil {
				client.Conn.WriteJSON(Message{Type: "group_action_result", Success: false, Error: err.Error()})
			} else {
				broadcastGroupListToAll()
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
