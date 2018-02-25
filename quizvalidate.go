package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// StringSet is a simple implementation of a set of strings
// I kind of wish golang has sets implemented, but then I guess they'd also need generics...
//
// *TODO* evaluate merits of breaking this out to its own file when validations list grows too long
type StringSet struct {
	content map[string]bool
}

// NewStringSet initializes set
func NewStringSet() *StringSet {
	return &StringSet{make(map[string]bool)}
}

// Add inserts string into set and returns boolean for change
func (set *StringSet) Add(s string) bool {
	exists := set.content[s]
	set.content[s] = true
	return !exists
}

// AddAll inserts multiple string elements into set
// Returns any strings that already exist in set (unconventional, but I want to print them)
func (set *StringSet) AddAll(strs ...string) []string {
	var dups []string
	for _, s := range strs {
		changed := set.Add(s)
		if !changed {
			dups = append(dups, s)
		}
	}
	return dups
}

// Remove removes given string from set
func (set *StringSet) Remove(s string) {
	delete(set.content, s)
}

// IsEmpty checks the set's emptiness by evaluating content's zero value
func (set StringSet) IsEmpty() bool {
	return set.content == nil || len(set.content) == 0
}

// Values returns a slice of set members
func (set StringSet) Values() []string {
	keys := make([]string, len(set.content))
	i := 0
	for key := range set.content {
		keys[i] = key
		i++
	}
	return keys
}

// ValidateQuizzes will run through the following checks:
//   checkDuplicates - validates duplicate questions and answers
//
// Parameter quizNames defines the quizzes to be checked
// Parameter generateFix is a boolean that controls the creation of fixed quiz copies
func ValidateQuizzes(quizNames []string, generateFix bool) {
	for _, quizName := range quizNames {
		quiz := LoadQuiz(quizName)
		fmt.Printf("[%s] running checks...\n", quizName)

		// Run checks
		//
		// *TODO* look into making a validator interface and move specific validation logic into structs when the list
		//        of validations grows too long, or if some have particularly complex logic
		fixed := checkDuplicates(quiz)

		if generateFix {

			// Create a copy of quiz file
			// Delete if exists
			fileName := QUIZ_FOLDER + Quizzes.Map[quizName] + ".fix"
			if _, err := os.Stat(fileName); !os.IsNotExist(err) {
				err := os.Remove(fileName)
				if err != nil {
					log.Fatal(err)
				}
			}
			f, err := os.Create(fileName)
			if err != nil {
				log.Fatal(err)
			}
			w := bufio.NewWriter(f)
			defer f.Close()

			// Write quiz JSON file
			b, err := json.MarshalIndent(fixed, "", "    ")
			if err != nil {
				log.Fatal(err)
			}
			w.Write(b)
			w.Flush()
			fmt.Printf("[%s] generated fixed file %s\n", quizName, fileName)
		}
	}
}

// Checks duplicate questions and answers in a given quiz
// Currently the strategy is to merge the answers and comments for cards with the same question
// Returns fixed quiz
func checkDuplicates(quiz Quiz) Quiz {
	fmt.Println("checking duplicates...")

	// Use a map to hold merged card data temporarily
	cardMap := make(map[string][]*StringSet)
	for _, card := range quiz.Deck {
		question := card.Question

		var cardDataSets []*StringSet
		if cardMap[question] != nil {
			cardDataSets = cardMap[question]
			fmt.Printf("\tFound duplicate question: %s", question)
		} else {
			cardDataSets = []*StringSet{NewStringSet(), NewStringSet()}
		}
		cardDataSets[1].Add(card.Comment)

		dups := cardDataSets[0].AddAll(card.Answers...)
		if dups != nil {
			fmt.Printf("\tFound duplicate answers: %s\n", strings.Join(dups, ", "))
		}
		cardMap[question] = cardDataSets
	}

	// Populate quiz deck with fixed cards
	fixedDeck := make([]Card, len(cardMap))
	i := 0
	for question, cardDataSets := range cardMap {
		fixedDeck[i] = Card{question, cardDataSets[0].Values(), strings.Join(cardDataSets[1].Values(), "\n")}
		i++
	}

	return Quiz{quiz.Description, quiz.Type, quiz.Timeout, fixedDeck}
}
