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

// Notification when attempting unauthorized commands
const OWNER_ONLY_MSG = "オーナーさんに　ちょうせん　なんて　10000こうねん　はやいんだよ！　"

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
	Difficulty  map[string]int  // Scramble game difficulty
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
	Settings.Difficulty = map[string]int{
		"easy":   5,
		"normal": 7,
		"hard":   9,
		"insane": 9999,
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

	// Only react on #bot* channels or private messages
	var retryErr error
	for i := 0; i < 3; i++ {
		var ch *discordgo.Channel
		ch, retryErr = s.State.Channel(m.ChannelID)
		if retryErr != nil {
			if strings.HasPrefix(retryErr.Error(), "HTTP 5") {
				// Wait and retry if Discord server related
				time.Sleep(250 * time.Millisecond)
				continue
			} else {
				break
			}
		} else if !strings.HasPrefix(ch.Name, "bot") && !ch.IsPrivate {
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
		case "list":
			showList(s, m)
		case "kanji", "k":
			if len(input) >= 2 {
				err := sendKanjiInfo(s, m.ChannelID, strings.TrimSpace(m.Content[len(input[0])+1:]))
				if err != nil {
					msgSend(s, m.ChannelID, "Error: "+err.Error())
				}
			} else {
				msgSend(s, m.ChannelID, "No kanji specified!")
			}
		case "uptime":
			if m.Author.ID == Settings.Owner.ID {
				t := time.Since(Settings.TimeStarted)
				t -= t % time.Second
				msgSend(s, m.ChannelID, fmt.Sprintf("Uptime: **%s** ", t))
			} else {
				msgSend(s, m.ChannelID, OWNER_ONLY_MSG+m.Author.Mention())
			}
		case "draw":
			if m.Author.ID == Settings.Owner.ID {
				if len(input) >= 2 {
					imgSend(s, m.ChannelID, strings.Replace(m.Content[len(input[0])+1:], "\\n", "\n", -1))
				}
			} else {
				msgSend(s, m.ChannelID, OWNER_ONLY_MSG+m.Author.Mention())
			}
		case "output":
			// Sets Gauntlet score output channel
			if m.Author.ID == Settings.Owner.ID {
				putStorage("output", m.ChannelID)
				msgSend(s, m.ChannelID, "Gauntlet Score output set to this channel.")
			} else {
				msgSend(s, m.ChannelID, OWNER_ONLY_MSG+m.Author.Mention())
			}
		case "ongoing":
			if m.Author.ID == Settings.Owner.ID {
				msgOngoing(s, m.ChannelID)
			} else {
				msgSend(s, m.ChannelID, OWNER_ONLY_MSG+m.Author.Mention())
			}
		case "ping":
			msgSend(s, m.ChannelID, fmt.Sprintf("Latency: %d", time.Now().UnixNano()))
		case "time":
			msgSend(s, m.ChannelID, fmt.Sprintf("Time is: **%s**", time.Now().In(time.UTC)))
		case "mad", "fast", "mild", "slow":
			fallthrough
		case "quiz":
			if len(input) == 2 {
				go runQuiz(s, m.ChannelID, input[1], "", Settings.Speed[command])
			} else if len(input) == 3 {
				go runQuiz(s, m.ChannelID, input[1], input[2], Settings.Speed[command])
			} else {
				// Show if no quiz specified
				showList(s, m)
			}
		case "scramble":
			if len(input) == 1 {
				go runScramble(s, m.ChannelID, "")
			} else if len(input) == 2 {
				go runScramble(s, m.ChannelID, input[1])
			} else {
				// Show if no quiz specified
				showList(s, m)
			}
		case "gauntlet":
			if len(input) == 2 {
				go runGauntlet(s, m, input[1])
			} else {
				// Show if no quiz specified
				showHelp(s, m)
			}
		}
	}

	// Mostly a test to see if it reacts on mentions
	for _, u := range m.Mentions {
		if u.ID == s.State.User.ID {
			msgSend(s, m.ChannelID, "何故にボク、"+m.Author.Mention()+"？！")
		}
	}

}

// Show quiz list message in channel
func showList(s *discordgo.Session, m *discordgo.MessageCreate) {
	quizlist := GetQuizlist()
	sort.Strings(quizlist)
	msgSend(s, m.ChannelID, fmt.Sprintf("Available quizzes: ```%s```\nUse `%squiz <deck> [optional max score]` to start or `%shelp` for more detailed information.", strings.Join(quizlist, ", "), CMD_PREFIX, CMD_PREFIX))
}

// Show bot help message in channel
func showHelp(s *discordgo.Session, m *discordgo.MessageCreate) {

	var fields []*discordgo.MessageEmbedField

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "How to run a quiz round",
		Value:  fmt.Sprintf("Type `%squiz <deck> [optional max score]` in a #bot channel or by PM.\nUse `%sstop` to cancel a running quiz.", CMD_PREFIX, CMD_PREFIX),
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Educational decks",
		Value:  "jouyou, n0, n1, n2, n3, n4, n5, n5_adv, kanken_1k, kanken_j1k, kanken_2k, kanken_j2k, kanken_3k, kanken_4k, kanken_5k, kanken_6-10k, jlpt_blob, kanken_blob",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Name decks",
		Value:  "namae, myouji, onago, prefectures, stations",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Difficult decks",
		Value:  "n0, kanken_1k, kanken_j1k, kanken_2k, quirky",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Goofy decks",
		Value:  "obscure, yojijukugo, jukujikun, places, tokyo, niconico, kirakira, radicals, r18",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Alternative game modes",
		Value:  fmt.Sprintf("`%smad/fast/quiz/mild/slow <deck>` for 0/1/2/3/5 second answer windows.\n`%sgauntlet <deck>` in PM for a kanji time trial.", CMD_PREFIX, CMD_PREFIX),
		Inline: false,
	})

	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       fmt.Sprintf(":crossed_flags: Kanji Quiz Bot"),
		Description: fmt.Sprintf("Compete with other users on kanji readings!"),
		Color:       0xFADE40,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Owner: %s#%s", Settings.Owner.Username, Settings.Owner.Discriminator)},
	}

	embedSend(s, m.ChannelID, embed)
}

// Stop ongoing quiz in given channel
func stopQuiz(s *discordgo.Session, quizChannel string) {
	count := 0

	Ongoing.Lock()
	delete(Ongoing.ChannelID, quizChannel)
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
func startQuiz(s *discordgo.Session, quizChannel string) (err error) {
	count := 0

	Ongoing.Lock()
	_, exists := Ongoing.ChannelID[quizChannel]
	if !exists {
		Ongoing.ChannelID[quizChannel] = true
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
func hasQuiz(quizChannel string) bool {
	Ongoing.RLock()
	_, exists := Ongoing.ChannelID[quizChannel]
	Ongoing.RUnlock()

	return exists
}

// Run kanji quiz loop in given channel
func runQuiz(s *discordgo.Session, quizChannel string, quizname string, winLimitGiven string, waitTimeGiven int) {

	// Mark the quiz as started
	if err := startQuiz(s, quizChannel); err != nil {
		// Quiz already running, nothing to do here
		return
	}

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
		msgSend(s, quizChannel, "Failed to find quiz: "+quizname)
		stopQuiz(s, quizChannel)
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

	msgSend(s, quizChannel, fmt.Sprintf("```Starting new %s quiz (%d words) in 5 seconds:\n\"%s\"\nFirst to %d points wins.```", quizname, len(quiz.Deck), quiz.Description, winLimit))

	var quizHistory string
	players := make(map[string]int)
	var timeoutCount int

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
		imgSend(s, quizChannel, current.Question)

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

				embed := &discordgo.MessageEmbed{
					Type:        "rich",
					Title:       fmt.Sprintf(":no_entry: Timed out! %s", current.Question),
					Description: fmt.Sprintf("**%s**", strings.Join(current.Answers, ", ")),
					Color:       0xAA2222,
				}

				embedSend(s, quizChannel, embed)

				timeoutCount++
				if timeoutCount >= timeoutLimit {
					msgSend(s, quizChannel, "```Too many timeouts in a row reached, aborting quiz.```")
					break outer
				}
				break inner
			case msg := <-c:
				user := msg.Author
				if hasString(current.Answers, msg.Content) {
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
			var scorers []string
			for player, position := range scoreKeeper {
				players[player]++
				if position == 1 {
					fastest = "<@" + player + ">"
				} else {
					scorers = append(scorers, "<@"+player+">")
				}
				if players[player] >= winLimit {
					winnerExists = true
				}
			}

			scorers = append([]string{fastest}, scorers...)

			embed := &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       fmt.Sprintf(":white_check_mark: Correct: %s", current.Question),
				Description: fmt.Sprintf("**%s**", strings.Join(current.Answers, ", ")),
				Color:       0x22AA22,
				Fields: []*discordgo.MessageEmbedField{
					&discordgo.MessageEmbedField{
						Name:   "Scorers",
						Value:  strings.Join(scorers, ", "),
						Inline: false,
					}},
			}

			embedSend(s, quizChannel, embed)

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

	embedSend(s, quizChannel, embed)

	stopQuiz(s, quizChannel)
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

// Run private gauntlet quiz
func runGauntlet(s *discordgo.Session, m *discordgo.MessageCreate, quizname string) {

	quizChannel := m.ChannelID

	// Only react in private messages
	var retryErr error
	for i := 0; i < 3; i++ {
		var ch *discordgo.Channel
		ch, retryErr = s.State.Channel(quizChannel)
		if retryErr != nil {
			if strings.HasPrefix(retryErr.Error(), "HTTP 5") {
				// Wait and retry if Discord server related
				time.Sleep(250 * time.Millisecond)
				continue
			} else {
				break
			}
		} else if !ch.IsPrivate {
			msgSend(s, quizChannel, fmt.Sprintf(":no_entry_sign: Game mode `%sgauntlet` is only for PM!", CMD_PREFIX))
			return
		}

		break
	}
	if retryErr != nil {
		log.Println("ERROR, With channel name check:", retryErr)
		return
	}

	// Mark the quiz as started
	if err := startQuiz(s, quizChannel); err != nil {
		// Quiz already running, nothing to do here
		return
	}

	timeout := 120 // seconds to run complete gauntlet

	quiz := LoadQuiz(quizname)
	if len(quiz.Deck) == 0 {
		msgSend(s, quizChannel, "Failed to find quiz: "+quizname)
		stopQuiz(s, quizChannel)
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

	msgSend(s, quizChannel, fmt.Sprintf("```Starting new %s quiz (%d words) in 5 seconds:\n\"%s\"\nAnswer as many as you can within %d seconds.```", quizname, len(quiz.Deck), quiz.Description, timeout))

	var correct, total int
	var quizHistory string

	// Breathing room to read start info
	time.Sleep(5 * time.Second)

	// Set timeout for no correct answers
	timeoutChan := time.NewTimer(time.Duration(timeout) * time.Second)

outer:
	for len(quiz.Deck) > 0 {

		// Grab new word from the quiz
		var current Card
		current, quiz.Deck = quiz.Deck[len(quiz.Deck)-1], quiz.Deck[:len(quiz.Deck)-1]

		// Replace readings with hiragana-only version
		for i, ans := range current.Answers {
			current.Answers[i] = strings.Map(k2h, ans)
		}

		// Send out quiz question
		imgSend(s, m.ChannelID, current.Question)

		select {
		case <-quitChan:
			break outer
		case <-timeoutChan.C:
			break outer
		case msg := <-c:
			// Increase total question count
			total++

			// Increase score if correct answer
			if hasString(current.Answers, msg.Content) {
				correct++
			} else {
				// Add wrong answer to history
				quizHistory += current.Question + "　" // Japanese space (wider)
			}
		}
	}

	// Clean up
	killHandler()

	// Sleep for a little breathing room
	time.Sleep(1 * time.Second)

	var score float64
	if total > 0 {
		score = float64(correct*correct) / float64(total)
	}

	// Produce scoreboard
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       "Final Gauntlet Score: " + quizname,
		Description: fmt.Sprintf("%.2f points", score),
		Color:       0x33FF33,
		Footer:      &discordgo.MessageEmbedFooter{Text: "Mistakes: " + quizHistory},
	}

	embedSend(s, quizChannel, embed)

	stopQuiz(s, quizChannel)

	// Produce public scoreboard
	if len(getStorage("output")) != 0 {

		embed := &discordgo.MessageEmbed{
			Type:        "rich",
			Title:       ":stopwatch: New Gauntlet Score: " + quizname,
			Description: fmt.Sprintf("%s: %.2f points in %d seconds", m.Author.Mention(), score, timeout),
			Color:       0xFFAAAA,
		}

		embedSend(s, getStorage("output"), embed)
	}
}

// Scramble quiz
func runScramble(s *discordgo.Session, quizChannel string, difficulty string) {

	// Mark the quiz as started
	if err := startQuiz(s, quizChannel); err != nil {
		// Quiz already running, nothing to do here
		return
	}

	quizname := "Scramble"
	winLimit := 10    // winner score
	timeout := 30     // seconds to wait per round
	timeoutLimit := 5 // count before aborting
	maxLength := 7    // default word length maximum

	// Set delay before closing round
	waitTime := time.Duration(Settings.Speed["quiz"]) * time.Millisecond

	// Parse provided winLimit with sane defaults
	if level, okay := Settings.Difficulty[difficulty]; okay {
		maxLength = level
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

	msgSend(s, quizChannel, fmt.Sprintf("```Starting new %s quiz (%d words) in 5 seconds:\n\"%s\"\nFirst to %d points wins.```", quizname, len(Dictionary), "Unscramble the English word", winLimit))

	players := make(map[string]int)
	var timeoutCount int

outer:
	for word := range Dictionary {

		// Skip words that are too short/long or names
		if len(word) < 3 || len(word) > maxLength || word != strings.ToLower(word) {
			continue outer
		}

		var question string

		// Attempt to shuffle thrice to get something random enough
		for i := 0; i < 3; i++ {
			shuffled := []rune(word)
			Shuffle(shuffled)
			if !Dictionary[string(shuffled)] {
				question = string(shuffled)
				break
			}
		}

		// If we're still left with a proper word, give up and pick a new one
		if len(question) == 0 {
			continue outer
		}

		// Generate sorted character set from correct answer for later comparison
		wordSortedSlice := []rune(word)
		sort.Slice(wordSortedSlice, func(i, j int) bool { return wordSortedSlice[i] < wordSortedSlice[j] })
		wordSorted := string(wordSortedSlice)

		answers := []string{word}

		time.Sleep(5 * time.Second)

		// Round's score keeper
		scoreKeeper := make(map[string]int)

		// Send out quiz question
		imgSend(s, quizChannel, question)

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

				embed := &discordgo.MessageEmbed{
					Type:        "rich",
					Title:       fmt.Sprintf(":no_entry: Timed out! %s", question),
					Description: fmt.Sprintf("**%s**", word),
					Color:       0xAA2222,
				}

				embedSend(s, quizChannel, embed)

				timeoutCount++
				if timeoutCount >= timeoutLimit {
					msgSend(s, quizChannel, "```Too many timeouts in a row reached, aborting quiz.```")
					break outer
				}
				break inner
			case msg := <-c:
				user := msg.Author

				if len(msg.Content) != len(word) {
					break
				}

				answer := strings.ToLower(msg.Content)

				if !Dictionary[answer] {
					break
				}

				// Check if the character sets match
				answerSortedSlice := []rune(answer)
				sort.Slice(answerSortedSlice, func(i, j int) bool { return answerSortedSlice[i] < answerSortedSlice[j] })
				answerSorted := string(answerSortedSlice)

				if answerSorted != wordSorted {
					break
				}

				if len(scoreKeeper) == 0 {
					timeoutChan.Reset(waitTime)
				}

				// Make sure we don't add the same user again
				if _, exists := scoreKeeper[user.ID]; !exists {
					scoreKeeper[user.ID] = len(scoreKeeper) + 1
				}

				if !hasString(answers, answer) {
					answers = append(answers, answer)
				}

				// Reset timeouts since we're active
				timeoutCount = 0
			}
		}

		if len(scoreKeeper) > 0 {

			winnerExists := false
			var fastest string
			var scorers []string
			for player, position := range scoreKeeper {
				players[player]++
				if position == 1 {
					fastest = "<@" + player + ">"
				} else {
					scorers = append(scorers, "<@"+player+">")
				}
				if players[player] >= winLimit {
					winnerExists = true
				}
			}

			scorers = append([]string{fastest}, scorers...)

			embed := &discordgo.MessageEmbed{
				Type:        "rich",
				Title:       fmt.Sprintf(":white_check_mark: Correct: %s", question),
				Description: fmt.Sprintf("**%s**", strings.Join(answers, ", ")),
				Color:       0x22AA22,
				Fields: []*discordgo.MessageEmbedField{
					&discordgo.MessageEmbedField{
						Name:   "Scorers",
						Value:  strings.Join(scorers, ", "),
						Inline: false,
					}},
			}

			embedSend(s, quizChannel, embed)

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
	}

	embedSend(s, quizChannel, embed)

	stopQuiz(s, quizChannel)
}
