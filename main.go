package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

// This bot's unique command prefix for message parsing
const CMD_PREFIX = "kq!"

// Discord Bot token
var Token string

// Ongoing keeps track of active quizzes and the channels they belong to
var Ongoing struct {
	sync.RWMutex
	ChannelID map[string]bool
}

// General bot settings (READ ONLY)
var Settings struct {
	Owner       *discordgo.User // Bot owner account
	TimeStarted time.Time       // Bot startup time
	Speed       map[string]int  // Quiz game speed in ms
}

func init() {

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()

	// New seed for random in order to shuffle properly
	rand.Seed(time.Now().UnixNano())

	// Initialize settings
	Settings.TimeStarted = time.Now()
	Settings.Speed = map[string]int{
		"mad":  0,
		"fast": 1000,
		"quiz": 2000,
		"mild": 3000,
		"slow": 5000,
	}
	Ongoing.ChannelID = make(map[string]bool)

}

func main() {

	// Make sure we start with a token supplied
	if len(Token) == 0 {
		flag.Usage()
		return
	}

	// Initiate a new session using Bot Token for authentication
	session, err := discordgo.New("Bot " + Token)

	if err != nil {
		log.Fatalln("ERROR, Failed to create Discord session:", err)
	}

	// Open a websocket connection to Discord and begin listening
	err = session.Open()
	if err != nil {
		log.Fatalln("ERROR, Couldn't open websocket connection:", err)
	}

	// Figure out the owner of the bot for admin commands
	app, err := session.Application("@me")
	if err != nil {
		log.Fatalln("ERROR, Couldn't get app:", err)
	}
	Settings.Owner = app.Owner

	// Register the messageCreate func as a callback for MessageCreate events
	session.AddHandler(messageCreate)

	// Wait here until CTRL-C or other term signal is received
	log.Println("NOTICE, Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Handle bot's own ping-pong messages
	if m.Author.ID == s.State.User.ID && strings.HasPrefix(m.Content, "Latency:") {
		parts := strings.Fields(m.Content)
		if len(parts) == 2 {
			oldtime, err := strconv.Atoi(parts[1])
			if err != nil {
				log.Println("ERROR, With bot ping:", err)
			}

			t := time.Since(time.Unix(0, int64(oldtime)))
			t -= t % time.Millisecond
			msgEdit(s, m, fmt.Sprintf("Latency: **%s** ", t))
		}
	}

	// Ignore all messages created bots to avoid loops
	if m.Author.Bot {
		return
	}

	// Only react on #bot* channels
	var retryErr error
	for i := 0; i < 3; i++ {
		var ch *discordgo.Channel
		ch, retryErr = s.Channel(m.ChannelID)
		if retryErr != nil {
			if strings.HasPrefix(retryErr.Error(), "HTTP 5") {
				// Wait and retry if Discord server related
				time.Sleep(250 * time.Millisecond)
				continue
			} else {
				break
			}
		} else if !strings.HasPrefix(ch.Name, "bot") {
			return
		}

		break
	}
	if retryErr != nil {
		log.Println("ERROR, With channel name check:", retryErr)
		return
	}

	// Handle bot commmands
	if strings.HasPrefix(m.Content, CMD_PREFIX) {

		// Split up the message to parse the input string
		input := strings.Fields(strings.ToLower(strings.TrimSpace(m.Content)))
		var command string
		if len(input) >= 1 {
			command = input[0][len(CMD_PREFIX):]
		}

		switch command {
		case "help":
			showHelp(s, m)
		case "uptime":
			if m.Author.ID == Settings.Owner.ID {
				t := time.Since(Settings.TimeStarted)
				t -= t % time.Second
				msgSend(s, m, fmt.Sprintf("Uptime: **%s** ", t))
			} else {
				msgSend(s, m, "オーナーさんに　ちょうせん　なんて　10000こうねん　はやいんだよ！　"+m.Author.Mention())
			}
		case "draw":
			if m.Author.ID == Settings.Owner.ID {
				if len(input) >= 2 {
					imgSend(s, m, m.Content[len(input[0])+1:])
				}
			} else {
				msgSend(s, m, "オーナーさんに　ちょうせん　なんて　10000こうねん　はやいんだよ！　"+m.Author.Mention())
			}
		case "ping":
			msgSend(s, m, fmt.Sprintf("Latency: %d", time.Now().UnixNano()))
		case "time":
			msgSend(s, m, fmt.Sprintf("Time is: **%s**", time.Now().In(time.UTC)))
		case "hello":
			imgSend(s, m, "Hello!")
		case "mad", "fast", "mild", "slow":
			fallthrough
		case "quiz":
			if len(input) == 2 {
				go runQuiz(s, m, input[1], "", Settings.Speed[command])
			} else if len(input) == 3 {
				go runQuiz(s, m, input[1], input[2], Settings.Speed[command])
			} else {
				// Show if no quiz specified
				showHelp(s, m)
			}
		}
	}

	// Mostly a test to see if it reacts on mentions
	for _, u := range m.Mentions {
		if u.ID == s.State.User.ID {
			msgSend(s, m, "何故にボク、"+m.Author.Mention()+"？！")
		}
	}

}

// Show bot help message in channel
func showHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	quizlist := GetQuizlist()
	sort.Strings(quizlist)
	msgSend(s, m, fmt.Sprintf("Available quizzes: ```%s```\nUse `%squiz <name> [max score]` to start.", strings.Join(quizlist, ", "), CMD_PREFIX))
}

// Stop ongoing quiz in given channel
func stopQuiz(s *discordgo.Session, m *discordgo.MessageCreate) {
	count := 0

	Ongoing.Lock()
	delete(Ongoing.ChannelID, m.ChannelID)
	count = len(Ongoing.ChannelID)
	Ongoing.Unlock()

	// Update bot's user status to reflect running quizzes
	var status string
	if count == 1 {
		status = "1 quiz"
	} else if count >= 2 {
		status = fmt.Sprintf("%d quizzes", count)
	}

	err := s.UpdateStatus(0, status)
	if err != nil {
		log.Println("ERROR, Could not update status:", err)
	}
}

// Start ongoing quiz in given channel
func startQuiz(s *discordgo.Session, m *discordgo.MessageCreate) (err error) {
	count := 0

	Ongoing.Lock()
	_, exists := Ongoing.ChannelID[m.ChannelID]
	if !exists {
		Ongoing.ChannelID[m.ChannelID] = true
	} else {
		err = fmt.Errorf("Channel quiz already ongoing")
	}
	count = len(Ongoing.ChannelID)
	Ongoing.Unlock()

	// Update bot's user status to reflect running quizzes
	var status string
	if count == 1 {
		status = "1 quiz"
	} else if count >= 2 {
		status = fmt.Sprintf("%d quizzes", count)
	}

	err2 := s.UpdateStatus(0, status)
	if err2 != nil {
		log.Println("ERROR, Could not update status:", err2)
	}

	return
}

// Checks if given channel has ongoing quiz
func hasQuiz(m *discordgo.MessageCreate) bool {
	Ongoing.RLock()
	_, exists := Ongoing.ChannelID[m.ChannelID]
	Ongoing.RUnlock()

	return exists
}

// Run slow given quiz loop in given channel
func runQuiz(s *discordgo.Session, m *discordgo.MessageCreate, quizname string, winLimitGiven string, waitTimeGiven int) {

	// Mark the quiz as started
	if err := startQuiz(s, m); err != nil {
		// Quiz already running, nothing to do here
		return
	}

	quizChannel := m.ChannelID
	winLimit := 15    // winner score
	timeout := 20     // seconds to wait per round
	timeoutLimit := 5 // count before aborting

	// Set delay before closing round
	waitTime := time.Duration(waitTimeGiven) * time.Millisecond

	// Parse provided winLimit with sane defaults
	if i, err := strconv.Atoi(winLimitGiven); err == nil {
		if i > 100 {
			winLimit = 100
		} else if i < 1 {
			winLimit = 1
		} else {
			winLimit = i
		}
	}

	quiz := LoadQuiz(quizname)
	if len(quiz.Deck) == 0 {
		msgSend(s, m, "Failed to find quiz: "+quizname)
		stopQuiz(s, m)
		return
	}

	c := make(chan *discordgo.MessageCreate, 100)
	quitChan := make(chan struct{}, 100)

	killHandler := s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore all messages created by self and bots
		if m.Author.ID == s.State.User.ID || m.Author.Bot {
			return
		}

		// Only react on current quiz channel
		if m.ChannelID != quizChannel {
			return
		}

		// Handle quiz aborts
		if strings.ToLower(strings.TrimSpace(m.Content)) == CMD_PREFIX+"stop" {
			quitChan <- struct{}{}
			return
		}

		// Relay the message to the quiz loop
		c <- m
	})

	msgSend(s, m, fmt.Sprintf("```Starting new %s quiz (%d words) in 5 seconds:\n\"%s\"\nFirst to %d points wins.```", quizname, len(quiz.Deck), quiz.Description, winLimit))

	var quizHistory string
	players := make(map[string]int)
	var timeoutCount int

	// Helper function to force katakana to hiragana conversion
	k2h := func(r rune) rune {
		switch {
		case r >= 'ァ' && r <= 'ヶ':
			return r - 0x60
		}
		return r
	}

	// Helper function to find string in set
	has := func(set []string, s string) bool {
		for _, str := range set {
			if s == str {
				return true
			}
		}

		return false
	}

outer:
	for len(quiz.Deck) > 0 {
		time.Sleep(5 * time.Second)

		// Grab new word from the quiz
		var current Card
		current, quiz.Deck = quiz.Deck[len(quiz.Deck)-1], quiz.Deck[:len(quiz.Deck)-1]

		// Replace readings with hiragana-only version
		for i, ans := range current.Answers {
			current.Answers[i] = strings.Map(k2h, ans)
		}

		// Add word to quiz history
		quizHistory += current.Question + "　" // Japanese space (wider)

		// Round's score keeper
		scoreKeeper := make(map[string]int)

		// Send out quiz question
		imgSend(s, m, current.Question)

		// Set timeout for no correct answers
		timeoutChan := time.NewTimer(time.Duration(timeout) * time.Second)

	inner:
		for {

			select {
			case <-quitChan:
				break outer
			case <-timeoutChan.C:
				if len(scoreKeeper) > 0 {
					break inner
				}

				msgSend(s, m, fmt.Sprintf(":no_entry: Timed out!\nCorrect answer: **%s** (%s)", strings.Join(current.Answers, ", "), current.Question))
				timeoutCount++
				if timeoutCount >= timeoutLimit {
					msgSend(s, m, "```Too many timeouts in a row reached, aborting quiz.```")
					break outer
				}
				break inner
			case msg := <-c:
				user := msg.Author
				if has(current.Answers, msg.Content) {
					if len(scoreKeeper) == 0 {
						timeoutChan.Reset(waitTime)
					}

					// Make sure we don't add the same user again
					if _, exists := scoreKeeper[user.ID]; !exists {
						scoreKeeper[user.ID] = len(scoreKeeper) + 1
					}

					// Reset timeouts since we're active
					timeoutCount = 0
				}
			}
		}

		if len(scoreKeeper) > 0 {

			winnerExists := false
			var fastest string
			for player, position := range scoreKeeper {
				players[player]++
				if position == 1 {
					fastest = player
				}
				if players[player] >= winLimit {
					winnerExists = true
				}
			}

			var extras string
			if len(scoreKeeper) > 1 {
				extras = fmt.Sprintf("（+%d人）", len(scoreKeeper)-1)
			}

			msgSend(s, m, fmt.Sprintf(":white_check_mark: <@%s>%s got it right: **%s** (%s)", fastest, extras, strings.Join(current.Answers, ", "), current.Question))

			if winnerExists {
				break outer
			}
		}

	}

	// Clean up
	killHandler()

	// Produce scoreboard
	fields := make([]*discordgo.MessageEmbedField, 0, 2)
	var winners string
	var participants string

	for _, p := range ranking(players) {
		if p.Score >= winLimit {
			winners += fmt.Sprintf("<@%s>: %d points\n", p.Name, p.Score)
		} else {
			participants += fmt.Sprintf("<@%s>: %d point(s)\n", p.Name, p.Score)
		}
	}

	if len(winners) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Winner",
			Value:  winners,
			Inline: false,
		})
	}

	if len(participants) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Participants",
			Value:  participants,
			Inline: false,
		})
	}

	// Sleep for a little breathing room
	time.Sleep(1 * time.Second)

	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       "Final Quiz Scoreboard: " + quizname,
		Description: "-------------------------------",
		Color:       0x33FF33,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: quizHistory},
	}

	embedSend(s, m, embed)

	stopQuiz(s, m)
}

// Player type for ranking list
type Player struct {
	Name  string
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
