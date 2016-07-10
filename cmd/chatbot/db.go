package main

import (
	"bufio"
	"log"
	"os"
	"regexp"
	"strings"
)

// prime database with log file
func prime() {
	s := bufio.NewScanner(os.Stdin)
	i := 0
	for s.Scan() {
		if i%10 == 0 {
			log.Printf("\033[J%d\033[0G\033[1A", i)
		}
		if err := s.Err(); err != nil {
			log.Fatal(err)
		}
		store(stripPrefix(s.Text()))
		i++
	}
}

func store(s string) {
	s = strings.TrimSpace(s)

	if stringMatchList(s, config.Ignore.linePatterns) {
		log.Printf("ignoring %q due to Ignore.Line", s)
		return
	}

	rawWords := strings.Fields(s)
	if len(rawWords) == 0 {
		return
	}
	words := make([]string, 0, len(rawWords))

	for _, word := range rawWords {
		if stringMatchList(word, config.Ignore.wordPatterns) {
			continue
		}

		// trim surrounding punctuation?
		//word = strings.TrimFunc(word, unicode.IsPunct)
		// strip color codes and other non-printers
		/*
			buf := make([]byte, 0, len(word))
			for _, c := range []byte(word) {
				if c > ' ' {
					buf = append(buf, c)
				}
			}
			words = append(words, string(buf))
		*/
		words = append(words, word)
	}
	if len(words) == 0 {
		return
	}

	log.Print("storing ", words)

	insertPair("", words[0])
	insertPair(words[len(words)-1], "")

	for i := 0; i < len(words)-1; i++ {
		insertPair(words[i], words[i+1])
	}
}

func insertPair(a, b string) {
	_, err := upsertStmt.Exec(a, b)

	if err != nil {
		log.Print(err)
	}
}

var (
	timestampPat = regexp.MustCompile(`^\[\d\d:\d\d(:\d\d)?\]\s*`)
	nickPat      = regexp.MustCompile(`^<([^>]+)>\s*`)
	quitJoinPat  = regexp.MustCompile(`^\s*\*+\s*`)
)

// remove leading timestamp, nick, and other noise
func stripPrefix(s string) string {
	s = timestampPat.ReplaceAllLiteralString(s, "")
	if quitJoinPat.MatchString(s) {
		return ""
	}

	for _, nick := range config.Ignore.nickPatterns {
		m := nickPat.FindStringSubmatch(s)
		if len(m) > 1 && nick.MatchString(m[1]) {
			return ""
		}
	}
	return nickPat.ReplaceAllLiteralString(s, "")
}

func buildMessage() (string, error) {
	s := ""
	err := buildStmt.QueryRow().Scan(&s)
	if err != nil {
		return "", err
	}

	return s, nil
}
