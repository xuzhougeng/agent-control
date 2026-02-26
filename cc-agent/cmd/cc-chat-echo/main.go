package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type Message struct {
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	SessionID string `json:"session_id,omitempty"`
}

func main() {
	sessionID := os.Getenv("CC_CHAT_SESSION_ID")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		reply := Message{
			MessageID: msg.MessageID,
			Content:   "[echo] " + msg.Content,
			SessionID: sessionID,
		}
		data, _ := json.Marshal(reply)
		fmt.Println(string(data))
	}
}
