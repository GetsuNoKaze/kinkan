package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"mikan/internal/release"
)

// The donations ledger (donations.json on the "updates" release, next to index.json): what
// Tribute's donation webhooks brought, counted by the donations workflow. The goals in the
// repository keep their texts and sums; the ledger only adds to their "raised", so nothing
// has to be committed to main for a donation.
//
//	{"events": ["<event key>", …], "raised": {"<goal id>": {"USD": 150}},
//	 "unmatched": [{"at": "…", "request_id": 7, "name": "…", "amount": 100, "currency": "USD"}]}
//
// Amounts are in Tribute's minimal units (cents); a goal shows whole units.
type ledger struct {
	Events    []string                    `json:"events"`
	Raised    map[string]map[string]int64 `json:"raised"`
	Unmatched []unmatched                 `json:"unmatched"`
}

type unmatched struct {
	At        string `json:"at"`
	RequestID int64  `json:"request_id"`
	Name      string `json:"name"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
}

// How much of the ledger is kept: an event key is only needed while Tribute may retry it
// (a day), the unmatched list is for a person to read.
const (
	keepEvents    = 5000
	keepUnmatched = 100
)

var (
	eventKey = regexp.MustCompile(`^[0-9a-f]{32,128}$`)
	currency = regexp.MustCompile(`^[A-Z]{3}$`)
)

// readLedger reads a ledger; a missing file is an empty one (the first donation).
func readLedger(path string) (ledger, error) {
	l := ledger{Raised: map[string]map[string]int64{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	if err := json.Unmarshal(data, &l); err != nil {
		return l, fmt.Errorf("%s: %w", path, err)
	}
	if l.Raised == nil {
		l.Raised = map[string]map[string]int64{}
	}
	return l, nil
}

// addTo adds what the ledger counted to the goals' raised, in each goal's currency (whole
// units); money in another currency is not converted.
func (l ledger) addTo(g *release.Goals) {
	for i := range g.Items {
		g.Items[i].Raised += int(l.Raised[g.Items[i].ID][g.Items[i].Currency] / 100)
	}
}

// match finds the goal of a donation: by its Tribute id when the goal knows it, else by the
// title the goal has in Tribute (the same as ours, in either language).
func match(g release.Goals, requestID int64, name string) (string, bool) {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	for _, it := range g.Items {
		if it.Tribute != 0 && it.Tribute == requestID {
			return it.ID, true
		}
	}
	n := norm(name)
	for _, it := range g.Items {
		if it.Tribute == 0 && n != "" && (norm(it.Title["ru"]) == n || norm(it.Title["en"]) == n) {
			return it.ID, true
		}
	}
	return "", false
}

// makeDonation counts one donation into the ledger, once per event key: Tribute retries a
// webhook it did not get an answer to, and the Worker passes the same key on.
func makeDonation(args []string) error {
	fs := flag.NewFlagSet("donation", flag.ContinueOnError)
	goalsFile := fs.String("goals", ".github/goals.json", "the goals")
	in := fs.String("ledger", "", "the current donations.json (missing: the first donation)")
	out := fs.String("out", "", "where to write the ledger")
	key := fs.String("key", "", "the event's key (hex), the same for every retry of it")
	requestID := fs.Int64("request-id", 0, "Tribute's donation_request_id")
	name := fs.String("name", "", "Tribute's donation_name")
	amount := fs.Int64("amount", 0, "the amount in minimal units (cents)")
	cur := fs.String("currency", "", "the currency, like USD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch {
	case *in == "" || *out == "":
		return errors.New("-ledger and -out")
	case !eventKey.MatchString(*key):
		return errors.New("-key: 32 to 128 hex digits")
	case *amount <= 0 || *amount > 100_000_000:
		return fmt.Errorf("-amount %d", *amount)
	case !currency.MatchString(*cur):
		return fmt.Errorf("-currency %q", *cur)
	case *requestID < 0 || !utf8.ValidString(*name) || utf8.RuneCountInString(*name) > 200:
		return errors.New("-request-id or -name")
	}
	raw, err := os.ReadFile(*goalsFile)
	if err != nil {
		return err
	}
	goals, err := release.ParseGoals(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", *goalsFile, err)
	}
	l, err := readLedger(*in)
	if err != nil {
		return err
	}
	if slices.Contains(l.Events, *key) {
		fmt.Println("donation: already counted")
		return writeLedger(*out, l)
	}
	l.Events = append(l.Events, *key)
	if len(l.Events) > keepEvents {
		l.Events = l.Events[len(l.Events)-keepEvents:]
	}
	if id, ok := match(goals, *requestID, *name); ok {
		if l.Raised[id] == nil {
			l.Raised[id] = map[string]int64{}
		}
		l.Raised[id][*cur] += *amount
		fmt.Printf("donation: %d %s to %s\n", *amount, *cur, id)
	} else {
		l.Unmatched = append(l.Unmatched, unmatched{At: time.Now().UTC().Format(time.RFC3339), RequestID: *requestID, Name: *name, Amount: *amount, Currency: *cur})
		if len(l.Unmatched) > keepUnmatched {
			l.Unmatched = l.Unmatched[len(l.Unmatched)-keepUnmatched:]
		}
		fmt.Printf("donation: no goal for request %d %q; kept as unmatched\n", *requestID, *name)
	}
	return writeLedger(*out, l)
}

func writeLedger(path string, l ledger) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
