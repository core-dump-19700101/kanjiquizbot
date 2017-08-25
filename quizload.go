package main

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"reflect"
	"sync"
)

const QUIZ_FOLDER = "./quizzes/"

// Quiz filename container
var Quizzes struct {
	sync.RWMutex
	Map map[string]string
}

// Quiz struct to hold entire quiz data
type Quiz struct {
	Description string `json:"description"`
	Deck        []Card `json:"deck"`
}

// Card struct to hold question-answer set
type Card struct {
	Question string   `json:"question"`
	Answers  []string `json:"answers"`
	Comment  string   `json:"comment,omitempty"`
}

// English Dictionary map
var Dictionary map[string]bool

func init() {

	// TODO: Make it auto-search the quiz folder with filepath.Glob()

	Quizzes.Lock()
	Quizzes.Map = map[string]string{
		"prefectures":  "prefectures.json",
		"tokyo":        "tokyo.json",
		"stations":     "stations.json",
		"places":       "places.json",
		"quirky":       "quirky.json",
		"obscure":      "obscure.json",
		"yojijukugo":   "yojijukugo.json",
		"jukujikun":    "jukujikun.json",
		"kanken_1k":    "kanken_1k.json",
		"kanken_j1k":   "kanken_j1k.json",
		"kanken_j2k":   "kanken_j2k.json",
		"kanken_2k":    "kanken_2k.json",
		"kanken_3k":    "kanken_3k.json",
		"kanken_4k":    "kanken_4k.json",
		"kanken_5k":    "kanken_5k.json",
		"kanken_6-10k": "kanken_6-10k.json",
		"onago":        "onago.json",
		"kirakira":     "kirakira-name.json",
		"n0":           "n0.json",
		"n1":           "jlpt_n1.json",
		"n2":           "jlpt_n2.json",
		"n3":           "jlpt_n3.json",
		"n4":           "jlpt_n4.json",
		"n5":           "jlpt_n5.json",
		"n5_adv":       "jlpt_n5_adv.json",
		"jouyou":       "jouyou.json",
		"namae":        "namae.json",
		"myouji":       "myouji.json",
		"r18":          "r18.json",
		"niconico":     "niconico-170806.json",
		"kanken_blob":  "kanken_blob.json",
		"jlpt_blob":    "jlpt_blob.json",
		"radicals":     "radicals.json",
		"numbers":      "numbers.json",
		"abh":          "abh.json",
	}
	Quizzes.Unlock()

	// Load up English dictionary for Scramble quiz
	Dictionary = make(map[string]bool)

	dictFile, err := os.Open("dictionary.txt")
	if err != nil {
		log.Fatalln("ERROR, Could not open English dictionary file:", err)
	}
	defer dictFile.Close()

	scanner := bufio.NewScanner(dictFile)
	for scanner.Scan() {
		if len(scanner.Text()) > 0 {
			Dictionary[scanner.Text()] = true
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln("ERROR, Could not scan English dictionary file:", err)
	}

}

// Returns a slice of quiz names
func GetQuizlist() []string {
	var quizlist []string
	Quizzes.RLock()
	for k := range Quizzes.Map {
		quizlist = append(quizlist, k)
	}
	Quizzes.RUnlock()

	return quizlist
}

// Returns a slice of shuffled Questions from a given quiz
func LoadQuiz(name string) (quiz Quiz) {

	Quizzes.RLock()
	filename, ok := Quizzes.Map[name]
	Quizzes.RUnlock()

	if ok {
		file, err := ioutil.ReadFile(QUIZ_FOLDER + filename)
		if err != nil {
			log.Printf("ERROR, Reading json '%s': %s\n", name, err)
			return
		}

		err = json.Unmarshal(file, &quiz)
		if err != nil {
			log.Printf("ERROR, Unmarshalling json '%s': %s\n", name, err)
			return
		}
	}

	Shuffle(quiz.Deck)

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
