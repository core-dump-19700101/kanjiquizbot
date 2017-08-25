package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Internally loaded kanji info type
type Kanji struct {
	Character string   `json:"character,omitempty"`
	On        []string `json:"on,omitempty"`
	Kun       []string `json:"kun,omitempty"`
	Kanken    string   `json:"kanken,omitempty"`
	Grade     string   `json:"grade,omitempty"`
	Type      []string `json:"type,omitempty"`
}

// All kanji info map
var KanjiMap map[string]Kanji

// Storage container for saving things on disk
var Storage struct {
	sync.RWMutex
	Map map[string]string
}

func init() {

	// Read all Jitenon.jp kanji info data into memory
	file, err := ioutil.ReadFile("all-kanji.json")
	if err != nil {
		log.Println("ERROR, Reading kanji json: ", err)
		return
	}

	err = json.Unmarshal(file, &KanjiMap)
	if err != nil {
		log.Println("ERROR, Unmarshalling kanji json: ", err)
		return
	}

	// Initialize Storage map
	Storage.Map = make(map[string]string)
	loadStorage()
}

// Helper function to find string in set
func hasString(set []string, s string) bool {
	for _, str := range set {
		if s == str {
			return true
		}
	}

	return false
}

// Helper function to force katakana to hiragana conversion
func k2h(r rune) rune {
	switch {
	case r >= 'ァ' && r <= 'ヶ':
		return r - 0x60
	}
	return r
}

// Send a given message to channel
func msgSend(s *discordgo.Session, cid string, msg string) {

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelMessageSend(cid, msg)
		return err
	})
	if retryErr != nil {
		log.Println("ERROR, Could not send message: ", retryErr)
	}
}

// Send an image message to Discord
func imgSend(s *discordgo.Session, cid string, word string) {

	image := GenerateImage(word)

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelFileSend(cid, "word.png", image)
		return err
	})
	if retryErr != nil {
		log.Println("ERROR, Could not send image:", retryErr)
	}
}

// Send an embedded message type to Discord
func embedSend(s *discordgo.Session, cid string, embed *discordgo.MessageEmbed) {

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelMessageSendEmbed(cid, embed)
		return err
	})
	if retryErr != nil {
		log.Println("ERROR, Could not send embed:", retryErr)
	}
}

// Edit a given message on a channel
func msgEdit(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelMessageEdit(m.ChannelID, m.ID, msg)
		return err
	})
	if retryErr != nil {
		log.Println("ERROR, Could not edit message: ", retryErr)
	}
}

// Send ongoing quiz info to channel
func msgOngoing(s *discordgo.Session, cid string) {

	var sessions []string

	Ongoing.RLock()
	for channelID := range Ongoing.ChannelID {
		ch, _ := s.State.Channel(channelID)
		if ch.IsPrivate {
			sessions = append(sessions, ch.Recipient.Username+"#"+ch.Recipient.Discriminator)
		} else {
			sessions = append(sessions, "<#"+channelID+">")
		}
	}
	Ongoing.RUnlock()

	msgSend(s, cid, fmt.Sprintf("Ongoing quizzes: %s\n", strings.Join(sessions, ", ")))
}

// Try API thrice in case of timeouts
func retryOnServerError(f func() error) (err error) {

	for i := 0; i < 3; i++ {
		err = f()
		if err != nil {
			if strings.HasPrefix(err.Error(), "HTTP 5") {
				// Wait and retry if Discord server related
				time.Sleep(1 * time.Second)
				continue
			} else {
				break
			}
		} else {
			// In case of no error, return
			return
		}
	}

	return
}

// ---------

// Return Kanji info from jitenon loaded from local cache
func sendKanjiInfo(s *discordgo.Session, cid string, query string) error {

	// Only grab first character, since it's a single kanji lookup
	query = string([]rune(query)[0])

	var kanji Kanji
	var exists bool
	if kanji, exists = KanjiMap[query]; !exists {
		return fmt.Errorf("Kanji '%s' not found", query)
	}

	// Custom joiner to bold jouyou readings
	join := func(s []string, sep string) string {
		var result string

		for i, str := range s {
			if !strings.ContainsRune(str, '△') {
				str = "**" + str + "**"
			}

			if i == 0 {
				result = str
			} else {
				result += sep + str
			}
		}

		return result
	}

	// Build a Discord message with the result
	var fields []*discordgo.MessageEmbedField

	if len(kanji.On) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "On-yomi",
			Value:  join(kanji.On, "\n"),
			Inline: true,
		})
	}

	if len(kanji.Kun) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Kun-yomi",
			Value:  join(kanji.Kun, "\n"),
			Inline: true,
		})
	}

	if len(kanji.Kanken) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Kanken",
			Value:  kanji.Kanken,
			Inline: true,
		})
	}

	if len(kanji.Type) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Type",
			Value:  strings.Join(kanji.Type, "\n"),
			Inline: true,
		})
	}

	if len(kanji.Grade) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Grade",
			Value:  kanji.Grade,
			Inline: true,
		})
	}

	embed := &discordgo.MessageEmbed{
		Type:   "rich",
		Title:  "Kanji: " + query,
		Color:  0xFADE40,
		Fields: fields,
	}

	embedSend(s, cid, embed)

	// Got this far without errors
	return nil
}

// Reads key from Storage and returns its value
func getStorage(key string) string {
	Storage.RLock()
	result := Storage.Map[key]
	Storage.RUnlock()

	return result
}

// Puts key into Storage with given value
func putStorage(key, value string) {
	Storage.Lock()
	Storage.Map[key] = value
	Storage.Unlock()

	// Save it to disk as well
	writeStorage()
}

// Writes Storage map as JSON to disk
func writeStorage() {
	Storage.RLock()
	b, err := json.Marshal(Storage)
	Storage.RUnlock()
	if err != nil {
		log.Println("ERROR, Could not marshal Storage to json: ", err)
	} else if err = ioutil.WriteFile("storage.json", b, 0644); err != nil {
		log.Println("ERROR, Could not write Storage file to disk: ", err)
	}
}

// Load Storage map from JSON on disk
func loadStorage() {

	// Read storage data into memory
	file, err := ioutil.ReadFile("storage.json")
	if err != nil {
		log.Println("ERROR, Reading Storage json: ", err)

		// Never saved anything before, create map from scratch
		writeStorage()

		return
	}

	Storage.Lock()
	err = json.Unmarshal(file, &Storage)
	Storage.Unlock()
	if err != nil {
		log.Println("ERROR, Unmarshalling Storage json: ", err)
	}
}
