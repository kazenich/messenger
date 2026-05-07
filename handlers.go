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
		log.Printf("Ошибка апгрейда: %v", err)
		return
	}

	log.Printf("WebSocket соединение установлено с %s", r.RemoteAddr)

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Ошибка чтения JSON: %v", err)
			conn.Close()
			return
		}

		log.Printf("Получено: type=%s from=%s text=%s", msg.Type, msg.From, msg.Text)

		switch msg.Type {
		case "register":
			log.Printf("Регистрация: %s", msg.From)
			err := registerUser(msg.From, msg.Password)
			if err != nil {
				errMsg := "Ошибка регистрации"
				if err.Error() == "user_exists" {
					errMsg = "Имя уже занято"
				}
				log.Printf("Ошибка регистрации: %s", errMsg)
				conn.WriteJSON(Message{Type: "register_result", Success: false, Error: errMsg})
			} else {
				log.Printf("Регистрация успешна: %s", msg.From)
				conn.WriteJSON(Message{Type: "register_result", Success: true, Text: "Регистрация успешна"})
			}

		case "login":
			log.Printf("Вход: %s", msg.From)
			if loginUser(msg.From, msg.Password) {
				log.Printf("Вход успешен: %s", msg.From)

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
			} else {
				conn.WriteJSON(Message{Type: "login_result", Success: false, Error: "Неверный логин или пароль"})
			}

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
				Type: "member_list", Text: strings.Join(members, ","), GroupName: creator,
			})

		case "search_users":
			users := searchUsers(msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "search_results", Text: strings.Join(users, ",")})

		case "get_contacts":
			contacts := getContacts(client.Name)
			client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})

		case "add_contact":
			addContact(client.Name, msg.Text)
			contacts := getContacts(client.Name)
			client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})
			client.Conn.WriteJSON(Message{Type: "add_contact_result", Success: true, Text: "Контакт добавлен"})

		case "delete_contact":
			deleteContact(client.Name, msg.Text)
			contacts := getContacts(client.Name)
			client.Conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})

		case "message":
			saveMessage(client.Name, msg.To, msg.Text, msg.ImageData, msg.Sticker, client.IsGroup, client.CurrentDialog)

			client.Conn.WriteJSON(Message{
				From: client.Name, Text: msg.Text, ImageData: msg.ImageData,
				Sticker: msg.Sticker, Time: msg.Time, Type: "message", IsGroup: client.IsGroup,
			})

			if !client.IsGroup {
				clientsMu.RLock()
				for c := range clients {
					if c.Name == msg.To {
						c.Conn.WriteJSON(Message{
							From: client.Name, Text: msg.Text, ImageData: msg.ImageData,
							Sticker: msg.Sticker, Time: msg.Time, Type: "message", IsGroup: false,
						})
						break
					}
				}
				clientsMu.RUnlock()
				addContact(client.Name, msg.To)
				addContact(msg.To, client.Name)
			}

		case "get_groups":
			groups := getUserGroups(client.Name)
			client.Conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})

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

		case "get_group_members":
			members := getGroupMembers(msg.To)
			creator := getGroupCreator(msg.To)
			client.Conn.WriteJSON(Message{
				Type: "member_list", Text: strings.Join(members, ","), GroupName: creator,
			})

		case "add_group_member":
			addMemberToGroup(msg.To, msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true, Text: "Участник добавлен"})

		case "remove_group_member":
			removeMemberFromGroup(msg.To, msg.Text, client.Name)
			client.Conn.WriteJSON(Message{Type: "group_action_result", Success: true, Text: "Участник удалён"})
		}
	}
}
