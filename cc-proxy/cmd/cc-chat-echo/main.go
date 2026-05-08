package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type Message struct {
	MessageID    string        `json:"message_id"`
	Content      string        `json:"content"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ContentPart struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
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
		content := msg.Content
		if content == "" && len(msg.ContentParts) > 0 {
			content = "[parts]"
		}
		echoContent := "[echo]"
		if content != "" {
			// Keep markdown block boundaries intact (e.g. fenced code) by
			// separating the echo prefix from the original content.
			echoContent = "[echo]\n\n" + content
		}
		reply := Message{
			MessageID: msg.MessageID,
			Content:   echoContent,
			SessionID: sessionID,
		}
		data, _ := json.Marshal(reply)
		fmt.Println(string(data))
	}
}
