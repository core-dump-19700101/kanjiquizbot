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

	"net/http"
)

import _ "net/http/pprof"

// This bot's unique command prefix for message parsing
const CMD_PREFIX = "kq!"

// Path to folder containing resources
const RESOURCES_FOLDER = "./resources/"

// Notification when attempting unauthorized commands
const OWNER_ONLY_MSG = "オーナーさんに　ちょうせん　なんて　10000こうねん　はやいんだよ！　"

// Discord Bot token
var Token string

// Ongoing keeps track of active quizzes and the channels they belong to
var Ongoing struct {
	sync.RWMutex
	ChannelID map[string]bool
}

// Review keeps track of review quizzes and the channels they belong to
var Review struct {
	sync.RWMutex
	ChannelID map[string]Quiz
}

// General bot settings (READ ONLY)
var Settings struct {
	Owner       *discordgo.User   // Bot owner account
	TimeStarted time.Time         // Bot startup time
	Speed       map[string][2]int // Quiz game speed in ms, window/pause
	Difficulty  map[string][2]int // Scramble game difficulty low/high
}

func init() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.Parse()

	// New seed for random in order to shuffle properly
	rand.Seed(time.Now().UnixNano())

	// Initialize settings
	Settings.TimeStarted = time.Now()
	Settings.Speed = map[string][2]int{
		"flash": [2]int{250, 500},
		"mad":   [2]int{0, 5000},
		"fast":  [2]int{1000, 5000},
		"quiz":  [2]int{2000, 5000},
		"mild":  [2]int{3000, 5000},
		"slow":  [2]int{5000, 5000},
		"multi": [2]int{1500, 5000},
	}
	Settings.Difficulty = map[string][2]int{
		"easy":   [2]int{3, 5},
		"normal": [2]int{3, 7},
		"hard":   [2]int{4, 9},
		"insane": [2]int{5, 9999},
	}

	Ongoing.ChannelID = make(map[string]bool)
	Review.ChannelID = make(map[string]Quiz)
}

func main() {

	// Make sure we start with a token supplied
	if len(Token) == 0 {
		flag.Usage()
		return
	}

	// Initialize necessary files loaded from disk
	loadFiles()

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

	// Handle bot commmands
	if isBotCommand(m.Content) {

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
				err := sendKanjiInfo(s, m.ChannelID, input[1])
				if err != nil {
					msgSend(s, m.ChannelID, "Error: "+err.Error())
				}
			} else {
				msgSend(s, m.ChannelID, "No kanji specified!")
			}
		case "frequency", "f":
			if len(input) >= 2 {
				err := sendWordFrequencyInfo(s, m.ChannelID, strings.TrimSpace(m.Content[len(input[0])+1:]))
				if err != nil {
					msgSend(s, m.ChannelID, "Error: "+err.Error())
				}
			} else {
				msgSend(s, m.ChannelID, "No query word specified!")
			}
		case "s":
			if len(input) >= 2 {
				msgSend(s, m.ChannelID, corpusSearch(m.Content[len(input[0])+1:]))
			} else {
				msgSend(s, m.ChannelID, "No query term specified!")
			}
		case "pitch", "p":
			if len(input) >= 2 {
				err := sendPitchInfo(s, m.ChannelID, strings.TrimSpace(m.Content[len(input[0])+1:]))
				if err != nil {
					msgSend(s, m.ChannelID, "Error: "+err.Error())
				}
			} else {
				msgSend(s, m.ChannelID, "https://imgur.com/ThGj3XP") // Send pitch info graphic
			}
		case "currency", "c":
			if len(input) >= 2 {
				msgSend(s, m.ChannelID, Currency(m.Content[len(input[0])+1:]))
			}
		case "uptime":
			if m.Author.ID == Settings.Owner.ID {
				t := time.Since(Settings.TimeStarted)
				t -= t % time.Second
				msgSend(s, m.ChannelID, fmt.Sprintf("Uptime: **%s** ", t))
			} else {
				msgSend(s, m.ChannelID, OWNER_ONLY_MSG+m.Author.Mention())
			}
		case "reload":
			if m.Author.ID == Settings.Owner.ID {
				if err := loadQuizList(); err == nil {
					showList(s, m)
				} else {
					msgSend(s, m.ChannelID, "Error: Failed to load quiz list!")
				}
			} else {
				msgSend(s, m.ChannelID, OWNER_ONLY_MSG+m.Author.Mention())
			}
		case "draw":
			if len(input) >= 2 {
				imgSend(s, m.ChannelID, strings.Replace(m.Content[len(input[0])+1:], "\\n", "\n", -1))
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
		case "flash", "mad", "fast", "mild", "slow":
			fallthrough
		case "quiz":
			if !isBotChannel(s, m.ChannelID) {
				break
			}
			if len(input) == 2 {
				go runQuiz(s, m.ChannelID, input[1], "", Settings.Speed[command][0], Settings.Speed[command][1])
			} else if len(input) == 3 {
				go runQuiz(s, m.ChannelID, input[1], input[2], Settings.Speed[command][0], Settings.Speed[command][1])
			} else {
				// Show if no quiz specified
				showList(s, m)
			}
		case "multi":
			if !isBotChannel(s, m.ChannelID) {
				break
			}
			if len(input) == 2 {
				go runMultiQuiz(s, m.ChannelID, input[1], "", Settings.Speed[command][0], Settings.Speed[command][1])
			} else if len(input) == 3 {
				go runMultiQuiz(s, m.ChannelID, input[1], input[2], Settings.Speed[command][0], Settings.Speed[command][1])
			} else {
				// Show if no quiz specified
				showList(s, m)
			}
		case "scramble":
			if !isBotChannel(s, m.ChannelID) {
				break
			}
			if len(input) == 1 {
				go runScramble(s, m.ChannelID, "")
			} else if len(input) == 2 {
				go runScramble(s, m.ChannelID, input[1])
			} else {
				// Show if no quiz specified
				showList(s, m)
			}
		case "gauntlet":
			if !isBotChannel(s, m.ChannelID) {
				break
			}
			if len(input) == 2 {
				go runGauntlet(s, m, input[1])
			} else {
				// Show if no quiz specified
				showHelp(s, m)
			}
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
		Value:  "n0, n1, n2, n3, n4, n5, n5_adv, jlpt_blob, kanken_1k, kanken_j1k, kanken_2k, kanken_j2k, kanken_3k, kanken_4k, kanken_5k, kanken_6-10k, kanken_blob, common, jouyou, kklc, eee, jjj",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Name decks",
		Value:  "namae, myouji, onago, prefectures, stations",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Difficult decks",
		Value:  "n0, kanken_1k, kanken_j1k, kanken_2k, quirky, kklc, tough, jp_syn, ee, jj",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Goofy decks",
		Value:  "obscure, yojijukugo, jukujikun, places, tokyo, niconico, kirakira, radicals, esyn, numbers, honyaku, r18",
		Inline: false,
	})

	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Alternative game modes",
		Value:  fmt.Sprintf("`%smad/fast/quiz/mild/slow <deck>` for 0/1/2/3/5 second answer windows.\n`%smulti <deck>` for scoring on multiple answers to the same question.\n`%sgauntlet <deck>` in PM for a kanji time trial.\n`%sscramble [easy/normal/hard/insane]` for an English Word Scramble quiz.", CMD_PREFIX, CMD_PREFIX, CMD_PREFIX, CMD_PREFIX),
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

// Get review quiz for given channel
func getReview(quizChannel string) Quiz {

	var result Quiz

	Review.Lock()
	result = Review.ChannelID[quizChannel]
	delete(Review.ChannelID, quizChannel)
	Review.Unlock()

	shuffle(result.Deck)

	return result
}

// Insert quiz into Review for given channel
func putReview(quizChannel string, quiz Quiz) {
	Review.Lock()
	Review.ChannelID[quizChannel] = quiz
	Review.Unlock()
}
