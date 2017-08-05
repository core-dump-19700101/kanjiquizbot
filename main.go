package main

import (
	"fmt"
	"strings"
	"time"
	"os"
	"os/signal"
	"syscall"
	"math/rand"
	"sort"
	"sync"
	"flag"

	"github.com/bwmarrin/discordgo"
)

// This bot's unique command prefix for message parsing
const CMD_PREFIX = "bug!"

// Discord Bot token
var Token string

// Ongoing keeps track of active quizzes and the channels they belong to
var Ongoing struct {
	sync.RWMutex
	ChannelID map[string]bool
}

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()

	// New seed for random in order to shuffle properly
	rand.Seed(time.Now().UnixNano())
	Ongoing.ChannelID = make(map[string]bool)

}


func main() {

	// Initiate a new session using Bot Token for authentication
	session, err := discordgo.New("Bot " + Token)

	if err != nil {
		fmt.Println("ERROR, Failed to create Discord session:", err)
		return
	}


	// Register the messageCreate func as a callback for MessageCreate events
	session.AddHandler(messageCreate)

	// Open a websocket connection to Discord and begin listening
	err = session.Open()
	if err != nil {
		fmt.Println("ERROR, Couldn't open websocket connection:", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received
	fmt.Println("NOTICE, Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself (or any bot)
	// This isn't required in this specific example but it's a good practice
	if m.Author.ID == s.State.User.ID || m.Author.Bot {
		return
	}

	// Only react on #bot* channels
	if ch, err := s.Channel(m.ChannelID); err != nil || !strings.HasPrefix(ch.Name, "bot") {
		if err != nil {
			fmt.Println("ERROR, With bot channel stuff:", err)
		}
		return
	}

	// Split up the message to parse the input string
	input := strings.Fields(strings.ToLower(strings.TrimSpace(m.Content)))
	var command string
	if len(input) >= 1 {
		command = input[0]
	}

	switch command {
	case CMD_PREFIX + "help":
		showHelp(s, m)
	case CMD_PREFIX + "time":
		msgSend(s, m, fmt.Sprintf("Time is: **%s** ", time.Now().In(time.UTC)))
	case CMD_PREFIX + "hello":
		imgSend(s, m, "Hello!")
	case CMD_PREFIX + "quiz":
		if len(input) == 2 {
			go runQuiz(s, m, input[1])
		} else if !hasQuiz(m) {
			// Show help unless already running, since that's handled elsewhere
			showHelp(s, m)
		}
	}

	// Mostly a test to see if it reacts on mentions
	for _, u := range m.Mentions {
		if u.ID == s.State.User.ID {
			msgSend(s, m, "何故にボク、" + m.Author.Mention() + "？！")
		}
	}

}

// Show bot help message in channel
func showHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	quizlist := GetQuizlist()
	sort.Strings(quizlist)
	msgSend(s, m, fmt.Sprintf("Available quizzes: ```%s```\nUse `%squiz <name>` to start.", strings.Join(quizlist, ", "), CMD_PREFIX))
}

// Stop ongoing quiz in given channel
func stopQuiz(m *discordgo.MessageCreate) {
	Ongoing.Lock()
	delete(Ongoing.ChannelID, m.ChannelID)
	Ongoing.Unlock()
}

// Start ongoing quiz in given channel
func startQuiz(m *discordgo.MessageCreate) (err error) {
	Ongoing.Lock()
	_, exists := Ongoing.ChannelID[m.ChannelID]
	if !exists {
		Ongoing.ChannelID[m.ChannelID] = true
	} else {
		err = fmt.Errorf("Channel quiz already ongoing")
	}
	Ongoing.Unlock()

	return
}

// Checks if given channel has ongoing quiz
func hasQuiz(m *discordgo.MessageCreate) bool {
	Ongoing.RLock()
	_, exists := Ongoing.ChannelID[m.ChannelID]
	Ongoing.RUnlock()

	return exists
}

// Run given quiz loop in given channel
func runQuiz(s *discordgo.Session, m *discordgo.MessageCreate, quizname string) {

	// Mark the quiz as started
	if err := startQuiz(m); err != nil {
		// Quiz already running, nothing to do here
		return
	}

	quizChannel := m.ChannelID
	winlimit := 10
	timeoutlimit := 5

	quiz := LoadQuiz(quizname)
	if len(quiz) == 0 {
		msgSend(s, m, "Failed to find quiz: " + quizname)
		stopQuiz(m)
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
		if strings.ToLower(strings.TrimSpace(m.Content)) == CMD_PREFIX + "quiz" {
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
					msgSend(s, m, "```Too many timeouts in a row reached, aborting quiz.```")
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

// Send a given message to channel
func msgSend(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {
	_, err := s.ChannelMessageSend(m.ChannelID, msg)
	if err != nil {
		fmt.Println("ERROR, Could not send message: ", err)
	}
}

// Send an image message to Discord
func imgSend(s *discordgo.Session, m *discordgo.MessageCreate, word string) {

    image := GenerateImage(word)

    _, err := s.ChannelFileSend(m.ChannelID, "word.png", image)
    if err != nil {
        fmt.Println("ERROR, Could not send image:", err)
        return
    }

}

// Send an embedded message type to Discord
func embedSend(s *discordgo.Session, m *discordgo.MessageCreate, embed *discordgo.MessageEmbed) {

    _, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
    if err != nil {
        fmt.Println("ERROR, Could not send embed:", err)
        return
    }

}
