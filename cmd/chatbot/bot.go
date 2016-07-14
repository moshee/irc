// TODO: periodically remove pairs with very low occurrence to avoid gathering flukes as cruft
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	"ktkr.us/pkg/gas/db"
	"ktkr.us/pkg/irc"
)

// Config is the struct which should mirror the config json file
type Config struct {
	Network       string
	Secure        bool
	Nick          string
	User          string
	Realname      string
	RespondChance float64
	JoinChannels  []string
	NickservPass  string
	LogVerbose    bool
	WPM           float64
	Ignore        struct {
		Nick []string
		Line []string
		Word []string

		nickPatterns []*regexp.Regexp
		linePatterns []*regexp.Regexp
		wordPatterns []*regexp.Regexp
	}
}

var (
	config         Config
	loggedIn       = false
	upsertStmt     *sql.Stmt
	buildStmt      *sql.Stmt
	popularityStmt *sql.Stmt
	nickInLinePat  *regexp.Regexp

	upsertQuery = `
INSERT INTO markovbot.graph VALUES ( $1, $2, 1 )
ON CONFLICT ON CONSTRAINT graph_a_b_key
	DO UPDATE SET n = graph.n + 1
	WHERE graph.a = $1
	AND   graph.b = $2
`

	buildQuery = `
WITH RECURSIVE t(word, cnt) AS (
    VALUES ( '', 0 )
  UNION ALL
    SELECT * FROM (
        WITH tmp AS (
            SELECT
                g.a,
                g.b,
                g.n,
                t.word,
                t.cnt
            FROM
                markovbot.graph g,
                t
            WHERE a = t.word 
        ), z AS (
            SELECT
                a,
                b,
                n / (SELECT sum(n)::real FROM tmp) AS p,
                word,
                cnt
            FROM tmp
        ), thresh AS (
            SELECT (
				random()
					* (SELECT max(p) FROM z)
					^ (1 + random())
					+ (SELECT min(p) FROM z)
			) AS x
        )
        SELECT   z.b, z.cnt + 1
        FROM     z, thresh
        WHERE    z.p < thresh.x
        AND      (z.cnt = 0 OR z.a != '')
        ORDER BY z.p DESC
        LIMIT    1
    ) w
), nothing AS (
    INSERT INTO markovbot.first_words ( word )
        SELECT word FROM t WHERE word != '' ORDER BY t.cnt LIMIT 1
    ON CONFLICT ON CONSTRAINT first_words_word_key
        DO UPDATE SET n = first_words.n + 1
            WHERE first_words.word = EXCLUDED.word
)
SELECT string_agg(word, ' ') FROM t WHERE word != '';
`

	popularityQuery = `
SELECT sum(n) FROM markovbot.graph
	WHERE a = $1
	OR    (a = '' AND b = $1)
`
)

type Record struct {
	Channel string
	A       string
	B       string
	N       int
}

func main() {
	var (
		flagPrime  = flag.Bool("prime", false, "prime database with logs from STDIN")
		flagInsert = flag.String("insert", "", "just insert a pair and exit (comma separated)")
		flagC      = flag.String("c", "", "path to config file")
	)
	flag.Parse()

	err := loadConfig(*flagC)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	nickInLinePat = regexp.MustCompile(`\b` + strings.ToLower(config.Nick) + `\b`)

	upsertStmt, err = db.DB.Prepare(upsertQuery)
	if err != nil {
		log.Fatal(err)
	}
	buildStmt, err = db.DB.Prepare(buildQuery)
	if err != nil {
		log.Fatal(err)
	}
	popularityStmt, err = db.DB.Prepare(popularityQuery)
	if err != nil {
		log.Fatal(err)
	}

	if *flagPrime {
		prime()
		return
	}
	if *flagInsert != "" {
		parts := strings.Split(*flagInsert, ",")
		if len(parts) < 2 {
			log.Fatal("-insert: need comma separated word pair")
		}
		insertPair(parts[0], parts[1])
		return
	}

	go func() {
		log.Fatal(http.ListenAndServe(":6060", nil))
	}()

	c := &irc.Client{
		Addr:     config.Network,
		Nick:     config.Nick,
		User:     config.User,
		Realname: config.Realname,
		Verbose:  config.LogVerbose,
		Secure:   config.Secure,
	}

	c.HandleFunc("PRIVMSG", handlePRIVMSG)
	c.HandleFunc("MODE", handleMODE)

	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			c.SendRaw(s.Text())
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)

	go func() {
		<-sig
		os.Exit(1)
	}()

	for {
		if err := c.Connect(); err == nil {
			err = c.Run()
			if err == nil {
				return
			}
		} else {
			log.Print(err)
			time.Sleep(5 * time.Second)
		}
	}
}

func loadConfig(path string) error {
	if path == "" {
		return errors.New("-c flag required")
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}

	err = json.NewDecoder(f).Decode(&config)
	if err != nil {
		return err
	}

	config.Ignore.nickPatterns = compileRegexps(config.Ignore.Nick)
	config.Ignore.linePatterns = compileRegexps(config.Ignore.Line)
	config.Ignore.wordPatterns = compileRegexps(config.Ignore.Word)

	return nil
}

func compileRegexps(in []string) []*regexp.Regexp {
	if len(in) == 0 {
		return []*regexp.Regexp{}
	}
	r := make([]*regexp.Regexp, len(in))
	for i, s := range in {
		r[i] = regexp.MustCompile(s)
	}
	return r
}

func handlePRIVMSG(c *irc.Client, m *irc.Message) {
	switch targ := m.Target(); targ.(type) {
	case irc.ChannelTarget:
		channelmsg(c, m)
	case irc.UserTarget:
		privatemsg(c, m)
	}
}

func channelmsg(c *irc.Client, m *irc.Message) {
	if handleTrigger(c, m) {
		return
	}
	if stringMatchList(m.From.Nick, config.Ignore.nickPatterns) {
		log.Printf("ignoring %q due to Ignore.Nick", m.Trailing)
		return
	}
	if stringMatchList(m.Trailing, config.Ignore.linePatterns) {
		log.Printf("ignoring %q due to Ignore.Line", m.Trailing)
		return
	}
	if nickInLinePat.MatchString(strings.ToLower(m.Trailing)) || randomChance() {
		log.Print("building sentence...")
		t := time.Now()
		s, err := buildMessage()
		if err != nil {
			log.Print(err)
			return
		}
		log.Printf("query took %v", time.Since(t))

		if config.WPM > 0 {
			sz := float64(len(strings.Fields(s)))
			delay1 := (rand.Float64() * rand.Float64() * rand.Float64()) * 3000 // noticing delay
			delay2 := (sz / config.WPM) * 60 * 1000                             // typing delay
			log.Printf("delaying %d + %d ms", int64(delay1), int64(delay2))
			time.Sleep(time.Duration(delay1+delay2) * time.Millisecond)
		}

		c.PRIVMSG(m.Target().Name(), s)
	} else {
		store(m.Trailing)
	}
}

func privatemsg(c *irc.Client, m *irc.Message) {

}

func handleMODE(c *irc.Client, m *irc.Message) {
	if !loggedIn && m.From.Nick == c.Nick && len(m.Params) > 0 && m.Params[0] == c.Nick {
		loggedIn = true
		if config.NickservPass != "" {
			c.PRIVMSG("NickServ", "IDENTIFY "+config.NickservPass)
		}
		for _, ch := range config.JoinChannels {
			parts := strings.SplitN(ch, ":", 2)
			switch len(parts) {
			case 1:
				c.JOIN(parts[0])
			case 2:
				c.JOIN(parts[0], parts[1])
			}
		}

	}
}

func randomChance() bool {
	rand.Seed(time.Now().UnixNano())
	return rand.Float64() < config.RespondChance
}

func stringInList(s string, ss []string) bool {
	for _, t := range ss {
		if s == t {
			return true
		}
	}
	return false
}

func stringMatchList(s string, pats []*regexp.Regexp) bool {
	for _, pat := range pats {
		if pat.MatchString(s) {
			return true
		}
	}
	return false
}
