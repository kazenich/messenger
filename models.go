package main

import "github.com/gorilla/websocket"

type Client struct {
	Conn          *websocket.Conn
	Name          string
	CurrentDialog string
	IsGroup       bool
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
	FileData  string `json:"fileData"`
	FileName  string `json:"fileName"`
	FileType  string `json:"fileType"`
}
