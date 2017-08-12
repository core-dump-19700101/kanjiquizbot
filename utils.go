package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

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

// Return Kanji info from jitenon

// Return Kanji info from jitenon
func sendKanjiInfo(s *discordgo.Session, m *discordgo.MessageCreate, query string) error {

	// Only grab first character, since it's a single kanji lookup
	query = string([]rune(query)[0])

	// Run a search on Jitenon
	searchUrl := "http://kanji.jitenon.jp/cat/search.php?search=match&search2=kanji&getdata="
	searchUrl += url.QueryEscape(query)

	searchResp, err := http.Get(searchUrl)
	if err != nil {
		return fmt.Errorf("Failed to reach web server")
	}
	defer searchResp.Body.Close()

	searchData, err := ioutil.ReadAll(searchResp.Body)
	if err != nil || searchResp.StatusCode != 200 {
		return fmt.Errorf("Failed to search information from web server")
	}

	// Check if there was a result
	re := regexp.MustCompile(`<td class="searchtbtd2"><a href="http://kanji.jitenon.jp/kanji(.?/[0-9]+).html">「(.+?)」について</a></td>`)
	searchResult := re.FindStringSubmatch(string(searchData))
	if len(searchResult) == 0 {
		return fmt.Errorf("Kanji '%s' not found", query)
	}

	// Get the individual kanji page from Jitenon
	kanjiUrl := "http://kanji.jitenon.jp/kanji" + searchResult[1] + ".html"

	kanjiResp, err := http.Get(kanjiUrl)
	if err != nil {
		return err
	}
	defer kanjiResp.Body.Close()

	kanjiData, err := ioutil.ReadAll(kanjiResp.Body)
	if err != nil || kanjiResp.StatusCode != 200 {
		return fmt.Errorf("Failed to get kanji information from web server")
	}

	// Strip the kanji info we need from the response
	kanjiInfo := string(kanjiData)

	// Jouyou-gai symbol
	symbol := "【△】"

	var kanji struct {
		On     []string
		Kun    []string
		Type   []string
		Grade  string
		Kanken string
	}

	// On-yomi
	re = regexp.MustCompile(`(?ms)<h3>音読み</h3>.*?<th `)
	res := re.FindStringSubmatch(kanjiInfo)
	if len(res) > 0 {
		re := regexp.MustCompile(`<td>(.+?)?\s?<a href=".*?">(.+?)</a>.*?</td>`)
		for _, str := range re.FindAllStringSubmatch(res[0], -1) {
			line := strings.Join(str[1:], "")
			if len(str[1]) == 0 {
				line = symbol + line
			}

			kanji.On = append(kanji.On, line)
		}
	}

	// Kun-yomi
	re = regexp.MustCompile(`(?ms)<h3>訓読み</h3>.*?<th `)
	res = re.FindStringSubmatch(kanjiInfo)
	if len(res) > 0 {
		re := regexp.MustCompile(`<td>(.+?)?\s?<a href=".*?">(.+?)</a>.*?</td>`)
		for _, str := range re.FindAllStringSubmatch(res[0], -1) {
			line := strings.Join(str[1:], "")
			if len(str[1]) == 0 {
				line = symbol + line
			}

			kanji.Kun = append(kanji.Kun, line)
		}
	}

	// Gakunen
	re = regexp.MustCompile(`(?ms)<h3>学年</h3>.*?<a href=".*?">(.+?)学校(.+?)年生</a>`)
	for _, str := range re.FindAllStringSubmatch(kanjiInfo, -1) {
		line := strings.Join(str[1:], "")
		kanji.Grade = line
	}

	// Kanken kyuu
	re = regexp.MustCompile(`(?ms)<h3>漢字検定</h3>.*?<a href=".*?">(.+?)</a>`)
	for _, str := range re.FindAllStringSubmatch(kanjiInfo, -1) {
		line := strings.Join(str[1:], "")
		kanji.Kanken = line
	}

	// Type
	re = regexp.MustCompile(`(?ms)<h3>種別</h3>.*?</tr>`)
	res = re.FindStringSubmatch(kanjiInfo)
	if len(res) > 0 {
		re := regexp.MustCompile(`<a href=".*?">(.+?)</a>`)
		for _, str := range re.FindAllStringSubmatch(res[0], -1) {
			line := strings.Join(str[1:], " ")
			kanji.Type = append(kanji.Type, line)
		}
	}

	// Build a Discord message with the result
	var fields []*discordgo.MessageEmbedField

	if len(kanji.On) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "On-yomi",
			Value:  strings.Join(kanji.On, "\n"),
			Inline: true,
		})
	}

	if len(kanji.Kun) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Kun-yomi",
			Value:  strings.Join(kanji.Kun, "\n"),
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
