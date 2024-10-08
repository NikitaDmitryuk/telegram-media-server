package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

var messages map[MessageID]map[string]string

func LoadMessagesFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, &messages)
}

func GetMessage(id MessageID, args ...interface{}) string {
	if m, ok := messages[id]; ok {
		if msg, ok := m[lang]; ok {
			return fmt.Sprintf(msg, args...)
		}
		if msg, ok := m["en"]; ok {
			return fmt.Sprintf(msg, args...)
		}
	}
	log.Printf("Message not found")
	return "Message not found"
}
