package app

import (
	"encoding/json"
	"fmt"
	"html/template"
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
	http.HandleFunc("/", indexPage)
	http.HandleFunc("/current", currentText)
	http.HandleFunc("/today", beersToday)
	http.HandleFunc("/update_beers", SaveTheBeer)
	http.HandleFunc("/thisweek", beersWeekly)
}
func unescaped(x string) interface{} { return template.HTML(x) }

// Templates for each page
var weeklyBeerTemplate = template.Must(template.ParseFiles(
	"app/weeklybeer.html",
))

type WeeklyBeerVariables struct {
	TopBreweriesJson string
	TopStylesJson    string

	MostStaleJson    string
	FastConsumedJson string
}

var indexTemplate = template.Must(template.ParseFiles(
	"templates/index.html",
	"templates/bootstrap_base_head.html",
))

type IndexPageVariables struct {
	NewBeersToday []chucks.Beer
	Top5Breweries []string
}

// END Templates for each page

type BarChart struct {
	Data []BarChartRow
}
type BarChartRow struct {
	Name  string
	Value int
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
	// This is tough because we only want to count a brewery once
	// per beer, not per *instance* of beer
	brewAndBeerList := make(map[string][]string)
	beerAndStyleList := make(map[string][]string)

	for i := 0; i < len(beers); i++ {
		beer := beers[i]

		brewery_name := strings.Trim(beer.Brewery, " ")
		beer_name := strings.Trim(beer.Name, " ")
		if !stringInSlice(beer_name, brewAndBeerList[brewery_name]) {
			brewAndBeerList[brewery_name] = append(brewAndBeerList[brewery_name], beer_name)
		}

		beer_type := beer.Type
		beer_full_name := strings.Trim(beer.FullName(), " ")
		if beer_type == "" {
			beer_type = "Unspecified"
		}
		if !stringInSlice(beer_full_name, beerAndStyleList[beer_type]) {
			beerAndStyleList[beer_type] = append(beerAndStyleList[beer_type], beer_full_name)
		}
	}
	// Calculate the top breweries
	breweryList := make(map[string]int)
	for brewery, beers := range brewAndBeerList {
		breweryList[brewery] = len(beers)
	}
	breweryData := BarChart{}
	sorted := sortMapByValue(breweryList)
	for i := 0; i < 5; i++ {
		breweryData.Data = append(breweryData.Data, BarChartRow{sorted[i].Key, sorted[i].Value})
	}
	b, _ := json.Marshal(breweryData.Data)
	topBrewJson := fmt.Sprintf("%s", b)

	// Calculate the top styles
	styleList := make(map[string]int)
	for style, beers := range beerAndStyleList {
		styleList[style] = len(beers)
	}

	styleData := BarChart{}
	sorted = sortMapByValue(styleList)
	for i := 0; i < 5; i++ {
		styleData.Data = append(styleData.Data, BarChartRow{sorted[i].Key, sorted[i].Value})
	}
	b, _ = json.Marshal(styleData.Data)
	topStyleJson := fmt.Sprintf("%s", b)

	// Build time on Tap for each beer
	timeOnTapList := make(map[string]BeerByTime)
	for i := 0; i < len(beers); i++ {
		beer_full_name := fmt.Sprintf("%s %s", strings.Trim(beers[i].Brewery, " "), strings.Trim(beers[i].Name, " "))
		if _, ok := timeOnTapList[beer_full_name]; ok {
			beer := beers[i]
			oldBeer := timeOnTapList[beer_full_name]
			oldBeer.TotalTime += beer.RecordedAt.Truncate(time.Minute).Sub(oldBeer.LastTimeOn)
			oldBeer.LastTimeOn = beer.RecordedAt.Truncate(time.Minute)

			// There are 38 taps, so record these beers on tap
			if i >= len(beers)-38 {
				oldBeer.StillOnTap = true
			}

			// Save it back to the map (gotta be a better way?)
			timeOnTapList[beer_full_name] = oldBeer
		} else {
			beer := beers[i]
			timeOnTapList[beer_full_name] = BeerByTime{beer_full_name, beer.RecordedAt.Truncate(time.Minute), time.Duration(60 * time.Minute), false}
		}
	}
	// fmt.Fprintln(w, timeOnTapList)

	var beerByTimeList BeerList
	for _, v := range timeOnTapList {
		beerByTimeList = append(beerByTimeList, v)
	}

	sort.Sort(ByStale{beerByTimeList})
	staleBeerData := BarChart{}
	sorted = sortMapByValue(styleList)
	for i := 0; i < 5; i++ {
		staleBeerData.Data = append(staleBeerData.Data, BarChartRow{beerByTimeList[i].Name, int(beerByTimeList[i].TotalTime.Hours())})
	}
	b, _ = json.Marshal(staleBeerData.Data)
	staleBeerJson := fmt.Sprintf("%s", b)

	sort.Sort(ByFastestConsumption{beerByTimeList})
	freshBeerData := BarChart{}
	sorted = sortMapByValue(styleList)
	for i := 0; i < 5; i++ {
		freshBeerData.Data = append(freshBeerData.Data, BarChartRow{beerByTimeList[i].Name, int(beerByTimeList[i].TotalTime.Hours())})
	}
	b, _ = json.Marshal(freshBeerData.Data)
	freshBeerJson := fmt.Sprintf("%s", b)

	if err := weeklyBeerTemplate.Execute(w, WeeklyBeerVariables{topBrewJson, topStyleJson, staleBeerJson, freshBeerJson}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	// fmt.Fprintln(w, beerByTimeList[0])
	// for brewery, count := range breweryList {
	// 	fmt.Fprintf(w, "%s: %d\n", brewery, count)
	// }

	// Print out the top 5 brewerys
	// sorted := sortMapByValue(breweryList)
	// for i := 0; i < 5; i++ {
	// 	fmt.Fprintf(w, "%s: %d\n", sorted[i].Key, sorted[i].Value)
	// }
}

func beersToday(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	todayStart, todayEnd := GetStartAndEndOfToday()
	tapList := chucks.GetTapToUniqueBeerList(c, todayStart, todayEnd)

	c.Debugf("Taplist is %d beers long", len(tapList))

	fmt.Fprint(w, "<html><body>")
	for i := 1; i <= len(tapList); i++ {
		beers := tapList[i]
		beer := beers[0]
		fmt.Fprintf(w, "Tap %d): %s %s", beer.Tap, strings.Trim(beer.Brewery, " "), strings.Trim(beer.Name, " "))

		for j := 1; j < len(beers); j++ {
			beer = beers[j]
			fmt.Fprintf(w, ", %s %s", strings.Trim(beer.Brewery, " "), strings.Trim(beer.Name, " "))
		}
		fmt.Fprint(w, "<br />\n")
	}
	fmt.Fprint(w, "</body></html>")
}

func indexPage(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	// New beers for today
	todayStart, todayEnd := GetStartAndEndOfToday()
	tapList := chucks.GetTapToUniqueBeerList(c, todayStart, todayEnd)

	var newBeers []chucks.Beer

	for i := 1; i <= len(tapList); i++ {
		c.Debugf("%v", tapList[i])
		if len(tapList[i]) > 1 {
			newBeers = append(newBeers, tapList[i][len(tapList[i])-1])
		}
	}
	// END new beers for today

	// Top breweries for this week
	weekAgo, today := GetStartAndEndOfWeek()
	pastWeekBeers := chucks.GetTapToUniqueBeerList(c, weekAgo, today)

	var top5Breweries []string

	// Calculate the top breweries
	breweryList := make(map[string]int)
	for i := 1; i <= len(pastWeekBeers); i++ {
		for j := 0; j < len(pastWeekBeers[i]); j++ {
			beer := pastWeekBeers[i][j]
			if _, ok := breweryList[beer.Brewery]; ok {
				breweryList[beer.Brewery] += 1
			} else {
				breweryList[beer.Brewery] = 1
			}
		}
	}
	sorted := sortMapByValue(breweryList)
	for i := 0; i < 5; i++ {
		top5Breweries = append(top5Breweries, sorted[i].Key)
	}
	// END calculate the top breweries

	if err := indexTemplate.Execute(w, IndexPageVariables{newBeers, top5Breweries}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func currentText(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	scrape := scraper.New()
	scrape.FetchData(c)

	beers := scrape.BeerList()

	for i := 0; i < len(beers); i++ {
		fmt.Fprintf(w, "Beer %d: %s %s\n", beers[i].Tap, beers[i].Brewery, beers[i].Name)
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

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
