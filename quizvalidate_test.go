package main

import (
	"encoding/json"
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const TestJSONFolder = "./quizzes_internal_test"
const TestQuiz = "quizvalidate_test"

func TestStringSet(t *testing.T) {
	set := NewStringSet()

	// Test Add and AddAll
	set.AddAll("a", "a", "b")
	set.Add("c")
	set.Add("c")
	add := NewStringSet()
	add.AddAll("a", "b", "c")
	if !reflect.DeepEqual(set, add) {
		t.Errorf("error:%+v != %+v\n", set, add)
	}

	// Test Remove
	set.Remove("c")
	remove := NewStringSet()
	remove.AddAll("a", "b")
	if !reflect.DeepEqual(set, remove) {
		t.Errorf("error:%+v != %+v\n", set, remove)
	}

	// Test IsEmpty
	set.Remove("a")
	set.Remove("b")
	empty := NewStringSet()
	if !set.IsEmpty() || !empty.IsEmpty() {
		t.Error("IsEmpty() should be true")
	}
}

func TestValidations(t *testing.T) {

	// Weird to initialize a global in a test like this
	// Would be better to eventually have another less specific test covering the bot's initialization
	Quizzes.Map = map[string]string{
		TestQuiz: "_" + TestQuiz + ".json",
	}
	fixedQuizPath := QUIZ_FOLDER + Quizzes.Map[TestQuiz] + ".fix"

	// Raw strings and indentation don't go together
	correctQuizRaw := `{
	"description": "Test quiz with duplicates",
	"type": "text",
	"deck": [
		{ "question": "q1", "answers": [ "aaa", "bbb" ], "comment": "c1\nc2" },
		{ "question": "q2", "answers": [ "ccc" ], "comment": "c3" }
	]
}`
	correctQuiz := createTestQuiz(correctQuizRaw)

	// Actual validation logic is tested below
	// This test covers the genereation of fixed quiz copies
	ValidateQuizzes([]string{TestQuiz}, true)
	fixedQuiz := new(Quiz)
	f, err := os.Open(fixedQuizPath)
	if err != nil {
		log.Fatal(err)
	}
	json.NewDecoder(f).Decode(&fixedQuiz)
	f.Close()

	// Ignoring comment field because supporting field-specific comparison behavior is too annoying
	if !quizEqual(correctQuiz, *fixedQuiz, cmpopts.IgnoreFields(Card{}, "Comment")) {
		t.Errorf("Check duplicates failed! %+v != %+v", correctQuiz, *fixedQuiz)
	}

	// Clean up
	os.Remove(fixedQuizPath)
}

func TestDuplicateValidation(t *testing.T) {

	// Raw strings and indentation don't go together
	dedupQuizRaw := `{
	"description": "Test quiz with duplicates",
	"type": "text",
	"deck": [
		{ "question": "q1", "answers": [ "aaa", "bbb" ], "comment": "c1\nc2" },
		{ "question": "q2", "answers": [ "ccc" ], "comment": "c3" }
	]
}`

	dupQuiz := new(Quiz)
	f, err := os.Open(QUIZ_FOLDER + "_" + TestQuiz + ".json")
	if err != nil {
		log.Fatal(err)
	}
	json.NewDecoder(f).Decode(&dupQuiz)

	dedupQuiz := createTestQuiz(dedupQuizRaw)
	fixedQuiz := checkDuplicates(*dupQuiz)

	// Ignoring comment field because supporting field-specific comparison behavior is too annoying
	if !quizEqual(dedupQuiz, fixedQuiz, cmpopts.IgnoreFields(Card{}, "Comment")) {
		t.Errorf("Check duplicates failed! %+v != %+v", dedupQuiz, fixedQuiz)
	}
}

func createTestQuiz(raw string) (quiz Quiz) {
	err := json.Unmarshal([]byte(raw), &quiz)
	if err != nil {
		log.Fatal(err)
	}
	return
}

// Helper function for comparing quizzes
// Using custom compare because reflect.DeepEqual() cannot correctly equate slices with different order
func quizEqual(qx, qy Quiz, additionalOpts ...cmp.Option) bool {
	opts := []cmp.Option{
		cmpopts.SortSlices(func(x, y string) bool { return x < y }),
		cmpopts.SortSlices(func(x, y Card) bool { return x.Question < y.Question }),
	}
	opts = append(opts, additionalOpts...)
	return cmp.Equal(qx, qy, opts...)
}
