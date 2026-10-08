package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testGoals = `{"donate": "https://web.tribute.tg/d/REA", "items": [
	{"id": "panel-move", "title": {"ru": "Переезд панели на другой сервер", "en": "Moving the panel"}, "target": 100, "raised": 1, "currency": "USD", "status": "open"},
	{"id": "utm", "title": {"ru": "Метки рекламных кампаний"}, "target": 50, "raised": 0, "currency": "USD", "status": "open", "tribute": 77}]}`

// Donations land on their goals once each, by title or by Tribute id; what matches no goal
// is kept for a person to look at; the signed goals show the sum in whole units.
func TestDonationsLedger(t *testing.T) {
	f := newFixture(t, changelog)
	goals := f.write("goals.json", testGoals)
	ledger := filepath.Join(f.dir, "donations.json")
	key := func(c string) string { return strings.Repeat(c, 64) }
	give := func(k string, id int64, name string, cents int64, cur string) error {
		return makeDonation([]string{"-goals", goals, "-ledger", ledger, "-out", ledger, "-key", k, "-request-id", itoa(id), "-name", name, "-amount", itoa(cents), "-currency", cur})
	}
	for _, d := range []struct {
		k    string
		id   int64
		name string
		c    int64
		cur  string
	}{
		{key("a"), 5, "  переезд панели на  другой сервер ", 150, "USD"}, // the title, any case and spaces
		{key("a"), 5, "Переезд панели на другой сервер", 150, "USD"},     // a retry: counted once
		{key("b"), 5, "Moving the panel", 75, "USD"},
		{key("c"), 77, "a renamed goal", 100, "USD"}, // the Tribute id wins
		{key("d"), 9, "Something else", 500, "USD"},  // no goal
		{key("e"), 5, "Moving the panel", 9000, "RUB"},
	} {
		if err := give(d.k, d.id, d.name, d.c, d.cur); err != nil {
			t.Fatal(err)
		}
	}
	l, err := readLedger(ledger)
	if err != nil {
		t.Fatal(err)
	}
	if l.Raised["panel-move"]["USD"] != 225 || l.Raised["utm"]["USD"] != 100 || l.Raised["panel-move"]["RUB"] != 9000 || len(l.Unmatched) != 1 || len(l.Events) != 5 {
		t.Fatalf("ledger: %+v", l)
	}
	// Into the signed index: 1 by hand + 2.25 counted = 3 whole dollars; roubles stay out.
	first := filepath.Join(f.dir, "first")
	if err := makeIndex([]string{"-version", "0.5.0.1", "-from", "0.4.5", "-seed", "-out", first}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(f.dir, "out")
	if err := makeGoals([]string{"-file", goals, "-ledger", ledger, "-in", filepath.Join(first, "index.json"), "-out", out}); err != nil {
		t.Fatal(err)
	}
	ix := f.readIndex(out)
	if ix.Goals.Items[0].Raised != 3 || ix.Goals.Items[1].Raised != 1 {
		t.Fatalf("goals: %+v", ix.Goals.Items)
	}
	// What the webhook sends is checked before anything is counted.
	for name, args := range map[string][]string{
		"short key":      {"-key", "abc", "-amount", "100", "-currency", "USD"},
		"no amount":      {"-key", key("f"), "-amount", "0", "-currency", "USD"},
		"huge amount":    {"-key", key("f"), "-amount", "999999999999", "-currency", "USD"},
		"bad currency":   {"-key", key("f"), "-amount", "100", "-currency", "usd"},
		"negative id":    {"-key", key("f"), "-amount", "100", "-currency", "USD", "-request-id", "-1"},
		"no ledger path": {"-key", key("f"), "-amount", "100", "-currency", "USD", "-ledger", ""},
	} {
		full := append([]string{"-goals", goals, "-ledger", ledger, "-out", ledger}, args...)
		if err := makeDonation(full); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
