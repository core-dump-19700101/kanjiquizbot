package main

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
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

// English Dictionary slice
var Dictionary [][]string

// Load Quiz List map from disk
func loadQuizList() error {

	file, err := ioutil.ReadFile(RESOURCES_FOLDER + "quizlist.json")
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

	dict := make(map[string][]string, 12000)

	dictFile, err := os.Open(RESOURCES_FOLDER + "dictionary.txt")
	if err != nil {
		log.Fatalln("ERROR, Could not open English dictionary file:", err)
	}
	defer dictFile.Close()

	// Collect scramble groups based on sorted character set
	scanner := bufio.NewScanner(dictFile)
	for scanner.Scan() {
		word := scanner.Text()
		if len(word) > 0 {
			sorted := sortedChars(word)
			dict[sorted] = append(dict[sorted], word)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln("ERROR, Could not scan English dictionary file:", err)
	}

	// Populate Scramble dictionary with word groups
	Dictionary = make([][]string, 0, len(dict))
	for _, group := range dict {
		Dictionary = append(Dictionary, group)
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

	shuffle(quiz.Deck)

	return
}
