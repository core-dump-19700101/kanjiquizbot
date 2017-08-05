package main

import (
	"fmt"
	"strings"
	"time"
	"os"
	"os/signal"
	"syscall"
	"io/ioutil"
	"encoding/json"
	"math/rand"
	"reflect"
	"sort"
	"sync"

	"github.com/bwmarrin/discordgo"
)

var Token = "MzQyODExMTg5OTkyMjI2ODI2.DGVDXA.ODuuJ5rKG2SYGXYbc3FWp_Ubsxo"

var quizzes = map[string]string{
	"prefectures": "prefectures.json",
	"insane": "insane.json",
	"n3": "n3.json",
	"kanken_1k": "kanken_1k.json",
	"kanken_j1k": "kanken_j1k.json",
	"yojijukugo": "yojijukugo.json",
	"kanken_j2k": "kanken_j2k.json",
	"kanken_2k": "kanken_2k.json",
	"kanken_3k": "kanken_3k.json",
	"kanken_4k": "kanken_4k.json",
	"kanken_5k": "kanken_5k.json",
	"kanken_6-10k": "kanken_6-10k.json",
	"onago": "onago.json",
	"kirakira": "kirakira-name.json",
}

var Ongoing struct {
	sync.RWMutex
	ChannelID map[string]bool
}

type Question struct {
	Word string
	Reading string
}

func loadQuiz(name string) (questions []Question) {

	if filename, ok := quizzes[name]; ok {
		file, err := ioutil.ReadFile(filename)
		if err != nil {
			fmt.Println("ERROR reading json: ", err)
			return
		}

		err = json.Unmarshal(file, &questions)
		if err != nil {
			fmt.Println("ERROR unmarshalling json: ", err)
			return
		}
	}

	Shuffle(questions)

	return
}

// Supposedly shuffles any slice, don't forget the seed first
func Shuffle(slice interface{}) {
    rv := reflect.ValueOf(slice)
    swap := reflect.Swapper(slice)
    length := rv.Len()
    for i := length - 1; i > 0; i-- {
            j := rand.Intn(i + 1)
            swap(i, j)
    }
}

func main() {

	// New seed for random in order to shuffle properly
	rand.Seed(time.Now().UnixNano())
	Ongoing.ChannelID = make(map[string]bool)

	session, err := discordgo.New("Bot " + Token)

	if err != nil {
		fmt.Println("Something went wrong: ", err)
		return
	}


	// Register the messageCreate func as a callback for MessageCreate events.
	session.AddHandler(messageCreate)


	err = session.Open()
	if err != nil {
		fmt.Println("Something went wrong: ", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID || m.Author.Bot {
		return
	}

	// Only react on #bot* channels
	if ch, err := s.Channel(m.ChannelID); err != nil || !strings.HasPrefix(ch.Name, "bot") {
		if err != nil {
			fmt.Println("ERROR with bot channel stuff: ", err)
		}
		return
	}

	input := strings.Fields(strings.ToLower(strings.TrimSpace(m.Content)))
	var command string
	if len(input) >= 1 {
		command = input[0]
	}

	switch command {
	case "kq!help":
		var quizlist []string
		for k := range quizzes {
			quizlist = append(quizlist, k)
		}
		sort.Strings(quizlist)
		msgSend(s, m, "Available quizzes: ```" + strings.Join(quizlist, ", ") + "```\nUse `kq!quiz <name>` to start.")
	case "kq!time":
		_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Time is: **%s** ", time.Now()))
		if err != nil {
			fmt.Println("Something went wrong: ", err)
		}
	case "kq!hello":
		imgSend(s, m, "Hello!")
	case "kq!quiz":
		Ongoing.RLock()
		if _, ok := Ongoing.ChannelID[m.ChannelID]; !ok {
			quizname := "n3"
			if len(input) == 2 {
				quizname = input[1]
			}
			go runQuiz(s, m, quizname)
		}
		Ongoing.RUnlock()
	}

	for _, u := range m.Mentions {
		if u.ID == s.State.User.ID {
			msgSend(s, m, "何故にボク、" + m.Author.Mention() + "？！")
		}
	}

}

func msgSend(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {
	_, err := s.ChannelMessageSend(m.ChannelID, msg)
	if err != nil {
		fmt.Println("ERROR Message Send: ", err)
	}
}

func stopQuiz(m *discordgo.MessageCreate) {
	// set Quizzing to zero time
	Ongoing.Lock()
	delete(Ongoing.ChannelID, m.ChannelID)
	Ongoing.Unlock()
}

func runQuiz(s *discordgo.Session, m *discordgo.MessageCreate, quizname string) {

	// Already running?
	Ongoing.RLock()
	if _, ok := Ongoing.ChannelID[m.ChannelID]; ok {
		Ongoing.RUnlock()
		return
	}
	Ongoing.RUnlock()

	quizChannel := m.ChannelID
	winlimit := 10
	timeoutlimit := 5

	Ongoing.Lock()
	Ongoing.ChannelID[m.ChannelID] = true
	Ongoing.Unlock()

	quiz := loadQuiz(quizname)
	if len(quiz) == 0 {
		stopQuiz(m)
		msgSend(s, m, "Failed to find quiz: " + quizname)
		return
	}

	c := make(chan *discordgo.MessageCreate, 100)
	quitchan := make(chan struct{}, 100)

	killHandler := s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore all messages created by the bot itself
		// This isn't required in this specific example but it's a good practice.
		if m.Author.ID == s.State.User.ID || m.Author.Bot {
			return
		}

		// Only react on current quiz channel
		if m.ChannelID != quizChannel {
			return
		}

		// Handle quiz aborts
		if strings.ToLower(strings.TrimSpace(m.Content)) == "kq!quiz" {
			quitchan <- struct{}{}
			return
		}

		// Relay the message to the quiz loop
		c <- m
	})

	msgSend(s, m, fmt.Sprintf("```Starting new kanji quiz (%d words) in 5 seconds;\ngive your answer in hiragana!```", len(quiz)))

	players := make(map[string]int)
	var timeouts int

outer:
	for len(quiz) > 0 {
		time.Sleep(5*time.Second)

		// Grab new word from the quiz
		var current Question
		current, quiz = quiz[len(quiz)-1], quiz[:len(quiz)-1]

		// Send out quiz question
		imgSend(s, m, current.Word)

		// Set timeout for no correct answers
		timeout := time.After(20 * time.Second)

inner:
		for {

			select {
			case msg := <-c:
				if (msg.Content == current.Reading) {
					user := msg.Author
					msgSend(s, m, ":white_check_mark: " + user.Mention() + " is correct: **" + msg.Content + "** (" + current.Word + ")")
					players[user.ID]++

					if players[user.ID] >= winlimit {
						break outer
					}

					// Reset timeouts since we're active
					timeouts = 0
					break inner
				}
			case <-timeout:
				msgSend(s, m, ":no_entry: Timed out!\nCorrect answer: **" + current.Reading + "** (" + current.Word + ")")
				timeouts++
				if timeouts >= timeoutlimit {
					msgSend(s, m, "```Too many timeouts reached, aborting quiz.```")
					break outer
				}
				break inner
			case <-quitchan:
				break outer
			}
		}
	}

	// Clean up
	killHandler()
	close(c)
	close(quitchan)

	// Produce scoreboard
	fields := make([]*discordgo.MessageEmbedField, 0, len(players))
	var participants string

	for i, p := range ranking(players) {
		if i == 0 {
			fields = append(fields, &discordgo.MessageEmbedField{
				Name: "Winner",
				Value: fmt.Sprintf("<@%s>: %d points", p.Name, p.Score),
				Inline: false,
			})
		} else {
			participants += fmt.Sprintf("<@%s>: %d point(s)\n", p.Name, p.Score)
		}
	}

	if len(participants) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Participants",
			Value: participants,
			Inline: false,
		})
	}

	// Sleep for a little breathing room
	time.Sleep(1*time.Second)

	embed := &discordgo.MessageEmbed{
		Type: "rich",
		Title: "Final Quiz Scoreboard",
		Description: "-------------------------",
		Color: 0x33FF33,
		Fields: fields,
	}

	embedSend(s, m, embed)

	stopQuiz(m)
}

// Send an image message to Discord
func imgSend(s *discordgo.Session, m *discordgo.MessageCreate, word string) {

    image := generateImage(word)

    _, err := s.ChannelFileSend(m.ChannelID, "word.png", image)
    if err != nil {
        fmt.Println("ERROR Could not send image:", err)
        return
    }

}

// Player type for ranking list
type Player struct {
	Name string
	Score int
}

// Sort the player ranking list
func ranking(players map[string]int) (result []Player) {

	for k, v := range players {
		result = append(result, Player{k, v})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Score > result[j].Score })

	return
}

// Send embedded message type to Discord
func embedSend(s *discordgo.Session, m *discordgo.MessageCreate, embed *discordgo.MessageEmbed) {

    _, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
    if err != nil {
        fmt.Println("ERROR Could not send embed:", err)
        return
    }

}