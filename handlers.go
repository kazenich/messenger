package main

import (
	"fmt"
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
				client := &Client{Conn: conn, Name: msg.From}
				clientsMu.Lock()
				clients[client] = true
				clientsMu.Unlock()

				contacts := getContacts(msg.From)
				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})

				groups := getUserGroups(msg.From)
				conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})

				allUsers := getAllUsers(msg.From)
				conn.WriteJSON(Message{Type: "user_list", Text: strings.Join(allUsers, ",")})

				conn.WriteJSON(Message{Type: "login_result", Success: true})
				broadcastOnlineList()

				go handleChat(client)
				return
			} else {
				conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверный логин или пароль"})
			}

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
			originalText := msg.Text
			saveMessage(client.Name, msg.To, originalText, msg.ImageData, msg.Sticker, client.IsGroup, client.CurrentDialog)

			client.Conn.WriteJSON(Message{
				From:      client.Name,
				Text:      originalText,
				ImageData: msg.ImageData,
				Sticker:   msg.Sticker,
				Time:      msg.Time,
				Type:      "message",
				IsGroup:   client.IsGroup,
			})

			if !client.IsGroup {
				clientsMu.RLock()
				for c := range clients {
					if c.Name == msg.To {
						c.Conn.WriteJSON(Message{
							From:      client.Name,
							Text:      originalText,
							ImageData: msg.ImageData,
							Sticker:   msg.Sticker,
							Time:      msg.Time,
							Type:      "message",
							IsGroup:   false,
						})
						break
					}
				}
				clientsMu.RUnlock()
				addContact(client.Name, msg.To)
				addContact(msg.To, client.Name)
			} else {
				members := getGroupMembers(client.CurrentDialog)
				clientsMu.RLock()
				for c := range clients {
					for _, m := range members {
						if c.Name == m && c.CurrentDialog == client.CurrentDialog && c.IsGroup {
							c.Conn.WriteJSON(Message{
								From:      client.Name,
								Text:      originalText,
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
				clientsMu.RUnlock()
			}
		}
	}
}
