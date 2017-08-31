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

// Quiz List filename container
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

// Load Quiz List map from disk
func loadQuizList() error {

	file, err := ioutil.ReadFile("quizlist.json")
	if err != nil {
		log.Println("ERROR, Reading Quiz List json: ", err)
		return err
	}

	Quizzes.Lock()
	// Clear old map first
	Quizzes.Map = make(map[string]string)
	err = json.Unmarshal(file, &Quizzes.Map)
	Quizzes.Unlock()
	if err != nil {
		log.Println("ERROR, Unmarshalling Quiz List json: ", err)
		return err
	}

	return nil
}

// Load up English dictionary for Scramble quiz
func loadScrambleDictionary() {

	Dictionary = make(map[string]bool, 12000)

	dictFile, err := os.Open("dictionary.txt")
	if err != nil {
		log.Fatalln("ERROR, Could not open English dictionary file:", err)
	}
	defer dictFile.Close()

	scanner := bufio.NewScanner(dictFile)
	for scanner.Scan() {
		word := scanner.Text()
		if len(word) > 0 {
			Dictionary[word] = true
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
