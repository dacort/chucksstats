package scraper

import (
	"testing"

	"appengine/aetest"
)

func TestMyFunction(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Run code and tests requiring the appengine.Context using c.
	scrape := New()
	if scrape == nil {
		t.Error("Error: could not create new scraper instance")
	}

	err = scrape.FetchData(c)
	if err != nil {
		t.Error("Error: %v; FetchData error", err)
	}

	beers := scrape.BeerList()
	if len(beers) == 1 {
		t.Error("Error: received no beers. :(")
	}

	t.Errorf("%v", beers)
}
