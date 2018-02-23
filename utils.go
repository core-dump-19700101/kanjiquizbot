package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	JLPT      string   `json:"jlpt,omitempty"`
}

// All kanji info map
var KanjiMap map[string]Kanji

// Internally loaded word frequency info type
type WordFrequency struct {
	Lexeme              string
	Orthography         string
	Ranking             string
	Frequency           string
	LiteratureFrequency string
	PartOfSpeech        string
	Reading             string
}

// All word frequency info map
var WordFrequencyMap map[string][]WordFrequency

// All pitch info map
var PitchMap map[string]string

// Storage container for saving things on disk
var Storage struct {
	sync.RWMutex
	Map map[string]string
}

// Preload all files at startup
func loadFiles() {

	// Initialize Storage map
	Storage.Map = make(map[string]string)
	loadStorage()

	// Initialize Kanji info map
	loadAllKanji()

	// Initialize Word Frequency map
	loadWordFrequency()

	// Initialize Pitch info map
	loadPitchInfo()

	// Load font file
	loadFont()

	// Load English dictionary for Scramble
	loadScrambleDictionary()

	// Load Quiz List map
	loadQuizList()
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

// Helper function to pick min value out of two ints
func minint(a, b int) int {
	if a < b {
		return a
	}

	return b
}

// Helper function to pick max value out of two ints
func maxint(a, b int) int {
	if a > b {
		return a
	}

	return b
}

// Helper function to truncate long strings (Discord field limit)
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) > n && len(s) >= 6 {
		s = string([]rune(s)[:n-6]) + " [...]"
	}

	return s
}

// Helper function to deep copy quiz structures
func copyQuiz(q Quiz) (cp Quiz) {

	// Go through JSON to make a deep copy with new allocations
	b, err := json.Marshal(q)
	if err != nil {
		log.Println("ERROR, Could not JSON marshal quiz copy: ", err)
		return
	}

	err = json.Unmarshal(b, &cp)
	if err != nil {
		log.Println("ERROR, Could not JSON unmarshal quiz copy: ", err)
		return
	}

	return cp
}

// Helper function to find string in set
func hasString(set []string, s string) bool {
	for _, str := range set {
		if len(s) == len(str) && (s == str || strings.ToLower(s) == str) {
			return true
		}
	}

	return false
}

// Helper function to force katakana to hiragana conversion
func k2h(s string) string {
	katakana2hiragana := func(r rune) rune {
		switch {
		case r >= 'ァ' && r <= 'ヶ':
			return r - 0x60
		}
		return r
	}

	return strings.Map(katakana2hiragana, s)
}

// Sort the unicode character set in a string
func sortedChars(s string) string {
	slice := []rune(s)
	sort.Slice(slice, func(i, j int) bool { return slice[i] < slice[j] })
	return string(slice)
}

// Supposedly shuffles any slice, don't forget the seed first
func shuffle(slice interface{}) {
	rv := reflect.ValueOf(slice)
	swap := reflect.Swapper(slice)
	length := rv.Len()
	for i := length - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		swap(i, j)
	}
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
		// Check if it's a private channel or not
		if ch.Type&discordgo.ChannelTypeDM != 0 {
			var recipients []string
			for _, user := range ch.Recipients {
				recipients = append(recipients, user.Username+"#"+user.Discriminator)
			}

			sessions = append(sessions, "("+strings.Join(recipients, " + ")+")")
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

// Determine if given line is a bot command
func isBotCommand(s string) bool {

	if len(s) < len(CMD_PREFIX) {
		return false
	}

	return s[:len(CMD_PREFIX)] == CMD_PREFIX || strings.ToLower(s[:len(CMD_PREFIX)]) == strings.ToLower(CMD_PREFIX)
}

// Determine if given channel is for bot spam
func isBotChannel(s *discordgo.Session, cid string) bool {

	// Only react on #bot* channels or private messages
	var retryErr error
	for i := 0; i < 3; i++ {
		var ch *discordgo.Channel
		ch, retryErr = s.State.Channel(cid)
		if retryErr != nil {
			if strings.HasPrefix(retryErr.Error(), "HTTP 5") {
				// Wait and retry if Discord server related
				time.Sleep(250 * time.Millisecond)
				continue
			} else {
				break
			}
		} else if !strings.HasPrefix(ch.Name, "bot") && ch.Type&discordgo.ChannelTypeDM == 0 {
			return false
		}

		break
	}
	if retryErr != nil {
		log.Println("ERROR, With channel name check:", retryErr)
		return false
	}

	return true
}

// Load all kanji info into memory
func loadAllKanji() {

	// Open Jitenon.jp kanji info data file
	file, err := os.Open(RESOURCES_FOLDER + "all-kanji.json")
	if err != nil {
		log.Fatalln("ERROR, Reading kanji json file: ", err)
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&KanjiMap)
	if err != nil {
		log.Fatalln("ERROR, Unmarshalling kanji json: ", err)
	}

}

// Return Kanji info from jitenon loaded from local cache
func sendKanjiInfo(s *discordgo.Session, cid string, query string) error {

	// Only grab first character, since it's a single kanji lookup
	if len(query) == 0 {
		return fmt.Errorf("No query provided")
	}
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

	if len(kanji.JLPT) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "JLPT",
			Value:  kanji.JLPT,
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

// Load Word Frequency Map from TSV on disk
func loadWordFrequency() {

	WordFrequencyMap = make(map[string][]WordFrequency, 70000)

	freqFile, err := os.Open(RESOURCES_FOLDER + "wordfrequency.tsv")
	if err != nil {
		log.Fatalln("ERROR, Could not open Word Frequency file:", err)
	}
	defer freqFile.Close()

	// Format:
	// Ranking Lexeme Orthography Reading PartOfSpeech Frequency ReadingAlt LiteratureFrequency
	parts := 8

	scanner := bufio.NewScanner(freqFile)
	for scanner.Scan() {
		if len(scanner.Text()) == 0 {
			continue
		}

		line := strings.SplitN(scanner.Text(), "\t", parts)

		// Prioritize regular reading field over alternate
		reading := line[3]
		if reading == "#N/A" || reading == "0" {
			reading = line[6]
		}

		wf := WordFrequency{
			Lexeme:              line[1],
			Orthography:         line[2],
			Ranking:             line[0],
			Frequency:           line[5],
			LiteratureFrequency: line[7],
			PartOfSpeech:        line[4],
			Reading:             k2h(reading),
		}

		WordFrequencyMap[wf.Lexeme] = append(WordFrequencyMap[wf.Lexeme], wf)

		// Add standard orthonography reading if needed
		if wf.Orthography != wf.Lexeme && wf.Orthography != "#N/A" && wf.Orthography != "＊" {
			WordFrequencyMap[wf.Orthography] = append(WordFrequencyMap[wf.Orthography], wf)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln("ERROR, Could not scan Word Frequency file:", err)
	}
}

// Return Word Frequency info loaded from local cache
func sendWordFrequencyInfo(s *discordgo.Session, cid string, query string) error {

	var wfs []WordFrequency
	var exists bool
	if wfs, exists = WordFrequencyMap[query]; !exists {
		return fmt.Errorf("Word '%s' not found", query)
	}

	// Build a Discord message with the result
	var fields []*discordgo.MessageEmbedField

	for _, wf := range wfs {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s （%s） %s", wf.Lexeme, wf.Orthography, wf.Reading),
			Value:  fmt.Sprintf("#%s [%s/mil] %s (lit.#%s)", wf.Ranking, wf.Frequency, wf.PartOfSpeech, wf.LiteratureFrequency),
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Type:   "rich",
		Title:  ":u5272: Word Frequency Information",
		Color:  0xFADE40,
		Fields: fields,
	}

	embedSend(s, cid, embed)

	// Got this far without errors
	return nil
}

// Load Pitch Info into memory
func loadPitchInfo() {

	// Open pitch info data file
	file, err := os.Open(RESOURCES_FOLDER + "pitch.json")
	if err != nil {
		log.Fatalln("ERROR, Reading pitch json file: ", err)
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&PitchMap)
	if err != nil {
		log.Fatalln("ERROR, Unmarshalling pitch json: ", err)
	}
}

// Return Pitch Info loaded from local cache
func sendPitchInfo(s *discordgo.Session, cid string, query string) error {

	var pitches string
	var exists bool
	if pitches, exists = PitchMap[strings.ToLower(k2h(query))]; !exists {
		return fmt.Errorf("Word '%s' not found", query)
	}

	// Build a Discord message with the result
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       ":musical_note: Pitch Information",
		Color:       0xFADE40,
		Description: truncate(pitches, 2000),
	}

	embedSend(s, cid, embed)

	// Got this far without errors
	return nil
}

// Aliases for mapping currency names
var currencies = map[string]string{
	"yen":      "JPY",
	"dollar":   "USD",
	"dollars":  "USD",
	"bucks":    "USD",
	"bux":      "USD",
	"euro":     "EUR",
	"euros":    "EUR",
	"crowns":   "SEK",
	"pound":    "GBP",
	"pounds":   "GBP",
	"quid":     "GBP",
	"bitcoin":  "BTC",
	"bitcoins": "BTC",
}

// Check if an alias is registered, else strip and return
func checkCurrency(name string) string {
	name = strings.ToLower(name)

	if acronym, ok := currencies[name]; ok {
		name = acronym
	}

	name = strings.ToUpper(name)

	return name
}

// Format float in a human way
func humanize(f float64) string {

	sign := ""
	if f < 0 {
		sign = "-"
		f = -f
	}

	n := uint64(f)

	// Grab two rounded decimals
	decimals := uint64((f+0.005)*100) % 100

	var buf []byte

	if n == 0 {
		buf = []byte{'0'}
	} else {
		buf = make([]byte, 0, 16)

		for n >= 1000 {
			for i := 0; i < 3; i++ {
				buf = append(buf, byte(n%10)+'0')
				n /= 10
			}

			buf = append(buf, ',')
		}

		for n > 0 {
			buf = append(buf, byte(n%10)+'0')
			n /= 10
		}
	}

	// Reverse the byte slice
	for l, r := 0, len(buf)-1; l < r; l, r = l+1, r-1 {
		buf[l], buf[r] = buf[r], buf[l]
	}

	return fmt.Sprintf("%s%s.%02d", sign, buf, decimals)
}

// Return Yahoo currency conversion
func Currency(query string) string {
	yahoo := "https://query1.finance.yahoo.com/v10/finance/quoteSummary/"
	yahooParams := "?formatted=true&modules=price&corsDomain=finance.yahoo.com"

	parts := strings.Split(strings.TrimSpace(query), " ")
	if len(parts) != 4 {
		return "Error - Malformed query (ex. 100 JPY in USD)"
	}

	r := strings.NewReplacer(",", "", "K", "e3", "M", "e6", "B", "e9")

	multiplier, err := strconv.ParseFloat(r.Replace(strings.ToUpper(strings.TrimSpace(parts[0]))), 64)
	if err != nil {
		return "Error - " + err.Error()
	}

	from := checkCurrency(parts[1])
	to := checkCurrency(parts[3])

	// Do query in both exchange directions in case it's too small for 4 decimals
	queryUrl := yahoo + from + to + "=X" + yahooParams

	resp, err := http.Get(queryUrl)
	if err != nil {
		return "Error - " + err.Error()
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "Error - " + err.Error()
	}

	if resp.StatusCode != 200 {
		if strings.Contains(string(data), "Quote not found for ticker symbol") {
			return "Error - Currency not found"
		}

		log.Println("Yahoo error dump: ", string(data))
		return "Error - Something went wrong"
	}

	re := regexp.MustCompile(`"regularMarketPrice":{"raw":(.+?),"fmt":`)
	matched := re.FindStringSubmatch(string(data))

	if len(matched) != 2 {
		return "Error - Unknown currency"
	}

	number, err := strconv.ParseFloat(matched[1], 64)
	if err != nil {
		return "Error - " + err.Error()
	}

	return fmt.Sprintf("Currency: %s %s is **%s** %s", parts[0], from, humanize(multiplier*number), to)
}

// Return frequency stats from corpus of novels
func corpusSearch(s *discordgo.Session, cid string, query string) error {

	target := []byte(query)
	sob := []byte("@@@[NOVEL_START=")
	eob := []byte("@@@[NOVEL_END]@@@")
	var countBook, countTotal, booksTotal, booksMatched int
	var titleBook string
	bookList := make([]int, 0, 1500)

	// Allow more examples in bot-spam channels and DM
	examplesLimit := 2
	if isBotChannel(s, cid) {
		examplesLimit = 6
	}

	// Prepare somewhere to save example sentences
	examples := make([][]byte, examplesLimit)
	sources := make([]string, len(examples))
	for i := 0; i < len(examples); i++ {
		examples[i] = make([]byte, 0, 900)
	}
	exampleCount := 0

	// Prepare a regexp to cut up individual sentences
	expr, err := regexp.Compile(`([^「」。！？!?]*?` + regexp.QuoteMeta(query) + `[^「」。！？!?]*[。！？!?]*)`)
	if err != nil {
		return fmt.Errorf("Could not compile regexp: " + err.Error())
	}

	// Generate a lockless random seed
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	corpusFile, err := os.Open(RESOURCES_FOLDER + "corpus.txt")
	if err != nil {
		return fmt.Errorf("Could not open Corpus file: " + err.Error())
	}
	defer corpusFile.Close()

	scanner := bufio.NewScanner(corpusFile)
	scanner.Buffer(make([]byte, 1*1024*1024), 1*1024*1024)
	for scanner.Scan() {

		// If we are at the start of the book, save title for example source
		if bytes.HasPrefix(scanner.Bytes(), sob) {
			start := len(sob)
			end := bytes.LastIndex(scanner.Bytes(), []byte("]@@@"))
			if end >= 0 {
				titleBook = string(scanner.Bytes()[start:end])
			} else {
				return fmt.Errorf("Could not parse Corpus book title")
			}
			continue
		}

		// If we hit the end of the book, compile the stats so far
		if bytes.Equal(scanner.Bytes(), eob) {
			booksTotal++
			if countBook > 0 {
				booksMatched += 1
				countTotal += countBook
				bookList = append(bookList, countBook)
				countBook = 0
			}
			continue
		}

		// Count the number of instances in this line
		count := bytes.Count(scanner.Bytes(), target)
		countBook += count

		if count > 0 {
			// Only save an example when we don't have any, or if shrinking probability tells us to replace
			if exampleCount < len(examples) || r.Intn(countTotal+countBook) < len(examples) {
				matches := expr.FindAll(scanner.Bytes(), -1)

				// Exclude sentences that are too long for Discord
				for i := 0; i < len(matches); i++ {
					if len(matches[i]) > cap(examples[0]) {
						matches[i] = matches[len(matches)-1]
						matches = matches[:len(matches)-1]
						i--
					}
				}

				// Store or replace the extracted sample sentence
				if len(matches) > 0 {
					match := matches[r.Intn(len(matches))]
					index := exampleCount % len(examples)
					examples[index] = examples[index][:len(match)]
					copy(examples[index], match)
					sources[index] = titleBook
					exampleCount++
				}
			}

		}

	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Could not scan Corpus file: " + err.Error())
	}

	// Calculate the minimum, median, and maximum occurrence
	spread := ""
	if len(bookList) > 1 {
		sort.Ints(bookList)
		median := 0

		if len(bookList)%2 == 0 {
			median = (bookList[len(bookList)/2-1] + bookList[len(bookList)/2]) / 2
		} else {
			median = bookList[len(bookList)/2]
		}

		spread = fmt.Sprintf(" [%d, %d, %d]", bookList[0], median, bookList[len(bookList)-1])
	}

	// Format the list of usage examples
	amazonURL := "https://www.amazon.co.jp/s/?url=search-alias%3Dstripbooks&field-keywords="
	escaper := strings.NewReplacer(
		"]", "",
		"[", "",
		"(", "",
		")", "",
		"<", "%3C",
		">", "%3E",
		"　", "+",
		" ", "+",
	)

	var exampleList string
	for i, example := range examples {
		if len(example) == 0 {
			continue
		}

		current := strings.TrimSpace(strings.Replace(string(example), string(query), "__"+string(query)+"__", -1)) + "\n"
		current += "　[" + sources[i] + "](" + amazonURL + escaper.Replace(sources[i]) + ")\n"

		// Get rid of weird double markup for repetitions
		current = strings.Replace(current, "____", "", -1)

		// Only include if Discord message can fit it
		if len(exampleList)+len(current) <= DISCORD_DESC_MAX {
			exampleList += current
		}
	}

	stats := fmt.Sprintf(
		"%d hits in %d (%.1f%%) books%s for '%s'",
		countTotal,
		booksMatched,
		100*float64(booksMatched)/float64(booksTotal),
		spread,
		query,
	)

	// Build a Discord message with the result
	embed := &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       stats,
		Color:       0xFADE40,
		Description: truncate(exampleList, DISCORD_DESC_MAX),
	}

	embedSend(s, cid, embed)

	return nil
}

// Figure out current time in given location
func getTime(place string) string {

	aliases := map[string]string{
		"stockholm":   "Europe/Stockholm",
		"helsinki":    "Europe/Helsinki",
		"oslo":        "Europe/Oslo",
		"berlin":      "Europe/Berlin",
		"london":      "Europe/London",
		"paris":       "Europe/Paris",
		"madrid":      "Europe/Madrid",
		"riga":        "Europe/Riga",
		"rome":        "Europe/Rome",
		"zurich":      "Europe/Zurich",
		"tokyo":       "Asia/Tokyo",
		"singapore":   "Asia/Singapore",
		"seoul":       "Asia/Seoul",
		"reykjavik":   "Atlantic/Reykjavik",
		"new york":    "America/New_York",
		"vancouver":   "America/Vancouver",
		"los angeles": "America/Los_Angeles",
	}

	place = strings.TrimSpace(place)

	if alias, okay := aliases[strings.ToLower(place)]; okay {
		place = alias
	}

	loc, err := time.LoadLocation(place)
	if err != nil {
		return "Error - Location not found!"
	}

	return time.Now().In(loc).Format(time.UnixDate)
}
