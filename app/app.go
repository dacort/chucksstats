package app

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"

	"github.com/dacort/chucksstats/chucks"
	"github.com/dacort/chucksstats/scraper"
)

func init() {
	http.HandleFunc("/", handler)
	http.HandleFunc("/today", beersToday)
	http.HandleFunc("/update_beers", SaveTheBeer)
}

func GetStartAndEndOfToday() (time.Time, time.Time) {
	// Set up our time frames (Pacific yo)
	location, _ := time.LoadLocation("America/Los_Angeles")
	today := time.Now().In(location)
	tomorrow := time.Now().In(location).Add(time.Duration(24 * time.Hour))
	startOfDay := fmt.Sprintf("%s 10:00", today.Format("2006-01-02"))
	endOfDay := fmt.Sprintf("%s 00:00", tomorrow.Format("2006-01-02"))

	fmt.Println("Start:", startOfDay)
	fmt.Println("End: ", endOfDay)

	startTime, _ := time.ParseInLocation("2006-01-02 15:04", startOfDay, location)
	endTime, _ := time.ParseInLocation("2006-01-02 15:04", endOfDay, location)

	return startTime.UTC(), endTime.UTC()
}

func beersToday(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	todayStart, todayEnd := GetStartAndEndOfToday()
	beers := chucks.GetBeersBetween(c, todayStart, todayEnd)

	// Caress this into a map of Taps -> Beers
	tapList := make(map[int][]chucks.Beer)
	for i := 0; i < len(beers); i++ {
		tapList[int(beers[i].Tap)] = append(tapList[int(beers[i].Tap)], beers[i])
	}

	for i := 0; i < len(tapList); i++ {
		for j := 0; j < len(tapList[i]); j++ {
			beer := tapList[i][j]

			if j == 0 {
				fmt.Fprintf(w, "Tap %d): %s %s", beer.Tap, strings.Trim(beer.Brewery, " "), strings.Trim(beer.Name, " "))
			} else {
				oldBeer := tapList[i][j-1]
				if strings.Trim(oldBeer.Name, " ") != strings.Trim(beer.Name, " ") {
					fmt.Fprintf(w, ", %s %s", strings.Trim(beer.Brewery, " "), strings.Trim(beer.Name, " "))
				}
			}
		}
		fmt.Fprint(w, "\n")
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	scrape := scraper.New()
	scrape.FetchData(c)

	beers := scrape.BeerList()

	for i := 0; i < len(beers); i++ {
		fmt.Fprintf(w, "Beer %d: %s\n", beers[i].Tap, beers[i].Brewery)
	}
}

func SaveTheBeer(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	scrape := scraper.New()
	scrape.FetchData(c)
	beers := scrape.BeerList()

	// SAVETHEBEERS TOTHEDATABASE
	// Assumption: We record this every hour on the hour
	var keys []*datastore.Key

	// Create a set of keys for our beer
	for i := 0; i < len(beers); i++ {
		key := datastore.NewIncompleteKey(c, "Beer", nil)
		keys = append(keys, key)
	}

	// MultiPut in one shot - no party fouls here
	_, err := datastore.PutMulti(c, keys, beers)

	if err != nil {
		c.Errorf("Error saving beer: %s", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
