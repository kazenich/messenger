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

				log.Printf("Отправка contact_list для %s", msg.From)
				contacts := getContacts(msg.From)
				conn.WriteJSON(Message{Type: "contact_list", Text: strings.Join(contacts, ",")})

				log.Printf("Отправка group_list для %s", msg.From)
				groups := getUserGroups(msg.From)
				conn.WriteJSON(Message{Type: "group_list", Text: strings.Join(groups, ",")})

				log.Printf("Отправка login_result для %s", msg.From)
				conn.WriteJSON(Message{Type: "login_result", Success: true})
				
				log.Printf("Отправка online списка")
				broadcastOnlineList()

				log.Printf("Запуск handleChat для %s", msg.From)
				handleChat(client)  // НЕ go handleChat - запускаем в этом же потоке
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
	log.Printf("handleChat запущен для %s", client.Name)
	
	defer func() {
		log.Printf("handleChat завершен для %s", client.Name)
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
			log.Printf("Ошибка чтения в handleChat для %s: %v", client.Name, err)
			return
		}
		
		msg.From = client.Name
		msg.Time = time.Now().Format("15:04")
		if msg.Text != "" {
			msg.Text = sanitizeText(msg.Text)
		}

		log.Printf("handleChat [%s]: type=%s to=%s text=%s", client.Name, msg.Type, msg.To, msg.Text)

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
			err := addContact(client.Name, msg.Text)
			if err != nil {
				client.Conn.WriteJSON(Message{Type: "add_contact_result", Success: false, Error: "Пользователь не найден"})
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
			} else {
				members := getGroupMembers(client.CurrentDialog)
				clientsMu.RLock()
				for c := range clients {
					for _, m := range members {
						if c.Name == m && c.Name != client.Name {
							c.Conn.WriteJSON(Message{
								From: client.Name, Text: msg.Text, ImageData: msg.ImageData,
								Sticker: msg.Sticker, Time: msg.Time, Type: "message", IsGroup: true,
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
