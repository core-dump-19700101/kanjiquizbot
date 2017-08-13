package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
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
}

// Send a given message to channel
func msgSend(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelMessageSend(m.ChannelID, msg)
		return err
	})
	if retryErr != nil {
		log.Println("ERROR, Could not send message: ", retryErr)
	}
}

// Send an image message to Discord
func imgSend(s *discordgo.Session, m *discordgo.MessageCreate, word string) {

	image := GenerateImage(word)

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelFileSend(m.ChannelID, "word.png", image)
		return err
	})
	if retryErr != nil {
		log.Println("ERROR, Could not send image:", retryErr)
	}
}

// Send an embedded message type to Discord
func embedSend(s *discordgo.Session, m *discordgo.MessageCreate, embed *discordgo.MessageEmbed) {

	// Try thrice in case of timeouts
	retryErr := retryOnServerError(func() error {
		_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
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
func sendKanjiInfo(s *discordgo.Session, m *discordgo.MessageCreate, query string) error {

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

	embedSend(s, m, embed)

	// Got this far without errors
	return nil
}
