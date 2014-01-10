package app

import (
	"fmt"
	"net/http"
	"sort"
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
	http.HandleFunc("/thisweek", beersWeekly)
}

type BeerByTime struct {
	Name       string
	LastTimeOn time.Time
	TotalTime  time.Duration
	StillOnTap bool
}

// A slice of beers by time
type BeerList []BeerByTime

func (b BeerList) Len() int      { return len(b) }
func (b BeerList) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

// ByStale implements sort.Interface and finds longest beers on tap
type ByStale struct{ BeerList }

func (b ByStale) Less(i, j int) bool { return b.BeerList[i].TotalTime > b.BeerList[j].TotalTime }

// ByFastestConsumption finds shortest beers not still on tap
type ByFastestConsumption struct{ BeerList }

func (b ByFastestConsumption) Less(i, j int) bool {
	return b.BeerList[i].StillOnTap == false && (b.BeerList[i].TotalTime < b.BeerList[j].TotalTime)
}

// Will be  used later to return an array of valid times that Chuck's is open
type TimeFrame struct {
	StartTime time.Time
	EndTime   time.Time
}

type Pair struct {
	Key   string
	Value int
}

// A slice of Pairs that implements sort.Interface to sort by Value.
type PairList []Pair

func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PairList) Len() int           { return len(p) }
func (p PairList) Less(i, j int) bool { return p[i].Value > p[j].Value }

// A function to turn a map into a PairList, then sort and return it.
func sortMapByValue(m map[string]int) PairList {
	p := make(PairList, len(m))
	i := 0
	for k, v := range m {
		p[i] = Pair{k, v}
		i += 1
	}
	sort.Sort(p)
	return p
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

// Chuck's is open 7 days a week, from 10am to 12am every day except Sunday (11pm)
func GetStartAndEndOfWeek() (time.Time, time.Time) { //[]TimeFrame) {
	location, _ := time.LoadLocation("America/Los_Angeles")
	today := time.Now().In(location)
	tomorrow := today.Add(time.Duration(24 * time.Hour))
	weekAgo := today.Add(time.Duration(-(24 * 6) * time.Hour))

	startOfDay := fmt.Sprintf("%s 10:00", weekAgo.Format("2006-01-02"))
	endOfDay := fmt.Sprintf("%s 00:00", tomorrow.Format("2006-01-02"))

	startTime, _ := time.ParseInLocation("2006-01-02 15:04", startOfDay, location)
	endTime, _ := time.ParseInLocation("2006-01-02 15:04", endOfDay, location)

	return startTime.UTC(), endTime.UTC()
	// We'll make this better later
	// for i := 0; i < 7; i++{
	// 	day := today.Add(time.Duration(-(24 * i) * time.Hour))
	// 	if day.Weekday() == time.Sunday {

	// 	}
	// }
}

func beersWeekly(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	weekAgo, today := GetStartAndEndOfWeek()
	beers := chucks.GetBeersBetween(c, weekAgo, today)

	// Build list of beers by brewery
	breweryList := make(map[string]int)
	for i := 0; i < len(beers); i++ {
		if _, ok := breweryList[beers[i].Brewery]; ok {
			breweryList[beers[i].Brewery] += 1
		} else {
			breweryList[beers[i].Brewery] = 0
		}
	}

	// Build time on Tap for each beer
	timeOnTapList := make(map[string]BeerByTime)
	for i := 0; i < len(beers); i++ {
		if _, ok := timeOnTapList[beers[i].FullName()]; ok {
			beer := beers[i]
			oldBeer := timeOnTapList[beer.FullName()]
			oldBeer.TotalTime += beer.RecordedAt.Truncate(time.Minute).Sub(oldBeer.LastTimeOn)
			oldBeer.LastTimeOn = beer.RecordedAt.Truncate(time.Minute)

			// There are 38 taps, so record these beers on tap
			if i >= len(beers)-38 {
				oldBeer.StillOnTap = true
			}

			// Save it back to the map (gotta be a better way?)
			timeOnTapList[beer.FullName()] = oldBeer
		} else {
			beer := beers[i]
			timeOnTapList[beer.FullName()] = BeerByTime{beer.FullName(), beer.RecordedAt.Truncate(time.Minute), 0, false}
		}
	}
	// fmt.Fprintln(w, timeOnTapList)

	var beerByTimeList BeerList
	for _, v := range timeOnTapList {
		beerByTimeList = append(beerByTimeList, v)
	}
	sort.Sort(ByStale{beerByTimeList})
	fmt.Fprintln(w, beerByTimeList[0])

	sort.Sort(ByFastestConsumption{beerByTimeList})
	fmt.Fprintln(w, beerByTimeList[0])

	// for brewery, count := range breweryList {
	// 	fmt.Fprintf(w, "%s: %d\n", brewery, count)
	// }

	// Print out the top 5 brewerys
	sorted := sortMapByValue(breweryList)
	for i := 0; i < 5; i++ {
		fmt.Fprintf(w, "%s: %d\n", sorted[i].Key, sorted[i].Value)
	}
}

func beersToday(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	todayStart, todayEnd := GetStartAndEndOfToday()
	beers := chucks.GetBeersBetween(c, todayStart, todayEnd)

	// Caress this into a map of Taps -> Beers
	tapList := make(map[int][]chucks.Beer)
	for i := 0; i < len(beers); i++ {
		c.Debugf("Beer %d is %s", beers[i].Tap, beers[i].Name)
		tapList[int(beers[i].Tap)] = append(tapList[int(beers[i].Tap)], beers[i])
	}

	c.Debugf("Taplist is %d beers long", len(tapList))

	for _, beers := range tapList {
		// for i := 0; i < len(tapList); i++ {
		// c.Debugf("Tap %d", tapList[i][0].Tap)
		for j := 0; j < len(beers); j++ {
			beer := beers[j]

			if j == 0 {
				fmt.Fprintf(w, "Tap %d): %s %s", beer.Tap, strings.Trim(beer.Brewery, " "), strings.Trim(beer.Name, " "))
			} else {
				oldBeer := beers[j-1]
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
