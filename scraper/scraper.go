package scraper

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"appengine"
	"appengine/urlfetch"

	"github.com/PuerkitoBio/goquery"
)

var url = "http://chucks85th.com/"

const (
	IPA   = "IPA"
	Sour  = "Sour"
	Stout = "Stout"
	Cider = "Cider"
)

type Chucks struct {
	resp *http.Response
}

type Beer struct {
	Tap     int64
	Brewery string
	Name    string

	ABV        float64
	PintUSD    float64
	GrowlerUSD float64

	Type string

	RecordedAtHour string
	RecordedAt     time.Time
}

func New() (c *Chucks) {
	return &Chucks{}
}

func (c *Chucks) FetchData(context appengine.Context) (err error) {
	client := urlfetch.Client(context)
	context.Debugf("Fetching data from %s", url)

	// TODO: Don't assume everything works
	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	c.resp = resp

	return nil
}

func (c *Chucks) BeerList() (beers []Beer) {
	// Record the current hour at which we recorded this beer
	recorded := time.Now().UTC()
	recorded_hour := recorded.Format("2006-01-02T03:04Z")
	css_selections := []string{"ul#draft_left li", "ul#draft_right li"}

	// Parse the beers
	doc, _ := goquery.NewDocumentFromResponse(c.resp)

	for i := 0; i < len(css_selections); i++ {
		doc.Find(css_selections[i]).Each(func(i int, s *goquery.Selection) {
			if s.HasClass("header") {
				return
			}
			beer := NewBeer(s)
			beer.RecordedAt = recorded
			beer.RecordedAtHour = recorded_hour

			beers = append(beers, beer)
		})
	}

	return beers
}

func NewBeer(s *goquery.Selection) (b Beer) {
	b.Type = FindType(s)

	parts := strings.Split(s.Text(), "\n")
	b.Brewery = parts[2]
	b.Name = parts[3]

	b.Tap = ExtractInt(parts[1])

	b.GrowlerUSD = ExtractFloat(parts[4])
	b.PintUSD = ExtractFloat(parts[5])
	b.ABV = ExtractFloat(parts[6])

	return b
}

func FindType(s *goquery.Selection) string {
	if s.HasClass("ipa") {
		return IPA
	} else if s.HasClass("stout") {
		return Stout
	} else if s.HasClass("cider") {
		return Cider
	} else if s.HasClass("sour") {
		return Sour
	} else {
		return ""
	}
}

func ExtractInt(s string) int64 {
	num_string := ExtractNumbers(s)
	value, _ := strconv.ParseInt(num_string, 10, 64)

	return value
}

func ExtractFloat(s string) float64 {
	num_string := ExtractNumbers(s)
	value, _ := strconv.ParseFloat(num_string, 10)

	return value
}

func ExtractNumbers(s string) string {
	reg, _ := regexp.Compile("[^0-9.]+")

	safe := reg.ReplaceAllString(s, "")
	return safe
}
