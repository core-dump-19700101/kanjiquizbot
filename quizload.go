package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"reflect"
	"sync"
)

const QUIZ_FOLDER = "./quizzes/"

// Quiz filename container
var Quizzes struct {
	sync.RWMutex
	Map map[string]string
}

// Question item
type Question struct {
	Word    string
	Reading string
}

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
		"n1":           "jlpt_n1.json",
		"n2":           "jlpt_n2.json",
		"n3":           "jlpt_n3.json",
		"n4":           "jlpt_n4.json",
		"n5":           "jlpt_n5.json",
		"namae":        "namae.json",
		"myouji":       "myouji.json",
		"r18":          "r18.json",
		"niconico":     "niconico-170806.json",
	}
	Quizzes.Unlock()
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
func LoadQuiz(name string) (questions []Question) {

	Quizzes.RLock()
	filename, ok := Quizzes.Map[name]
	Quizzes.RUnlock()

	if ok {
		file, err := ioutil.ReadFile(QUIZ_FOLDER + filename)
		if err != nil {
			log.Println("ERROR, Reading json: ", err)
			return
		}

		err = json.Unmarshal(file, &questions)
		if err != nil {
			log.Println("ERROR, Unmarshalling json: ", err)
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
