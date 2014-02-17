package chucks

import (
	"strings"
	"time"

	"appengine"
	"appengine/datastore"

	"github.com/dacort/chucksstats/helpers"
)

const (
	IPA   = "IPA"
	Sour  = "Sour"
	Stout = "Stout"
	Cider = "Cider"

	RecordedAtHourFormat = "2006-01-02T15:04Z"
)

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

func (b *Beer) FullName() string {
	// strings.Trim(beer.Brewery, " "), strings.Trim(beer.Name, " ")
	return b.Brewery + " " + b.Name
}

func (b *Beer) BreweryName() string {
	return strings.Trim(b.Brewery, " ")
}

func (b *Beer) StyleName() string {
	if b.Type == "" {
		return "Unspecified"
	} else {
		return b.Type
	}
}

func GetBeersBetween(context appengine.Context, startTime time.Time, endTime time.Time) (beers []Beer) {
	// Convert start/end times to strings for query
	startHour := startTime.Format(RecordedAtHourFormat)
	endHour := endTime.Format(RecordedAtHourFormat)

	context.Debugf("Querying for beers between %s and %s", startHour, endHour)

	q := datastore.NewQuery("Beer").
		Filter("RecordedAtHour >=", startHour).
		Filter("RecordedAtHour <=", endHour).
		Order("RecordedAtHour").Order("Tap")

	// To retrieve the results,
	// you must execute the Query using its GetAll or Run methods.
	_, err := q.GetAll(context, &beers)
	if err != nil {
		context.Errorf("Uh oh: %v", err)
	}

	return
}

// A list of taps and the beers they've contained between a time range
// Return value is of the form:
// 1 => [Bud Light, Miller Lite, Piss Water]
// and guaranteed to be from 1 -> n (taps are 1-indexed)
func GetTapToUniqueBeerList(context appengine.Context, startTime time.Time, endTime time.Time) (tapMap map[int][]Beer) {
	beers := GetBeersBetween(context, startTime, endTime)
	tapMap = make(map[int][]Beer)
	everythingOnTap := make(map[int][]Beer)

	// Get a list of everything that's been on tap every hour
	for i := 0; i < len(beers); i++ {
		everythingOnTap[int(beers[i].Tap)] = append(everythingOnTap[int(beers[i].Tap)], beers[i])
	}

	// Condense the above list to unique things on tap
	for i := 1; i <= len(everythingOnTap); i++ {
		beers := everythingOnTap[i]
		for j := 0; j < len(beers); j++ {
			beer := beers[j]

			if j == 0 {
				tapMap[i] = append(tapMap[i], beer)
			} else {
				oldBeer := beers[j-1]
				if strings.Trim(oldBeer.Name, " ") != strings.Trim(beer.Name, " ") {
					tapMap[i] = append(tapMap[i], beer)
				}
			}
		}
	}

	return tapMap
}

// Retrieve the Top N breweries and styles
// This method is combined so as only to query the database once
func GetTopBreweriesAndStyles(context appengine.Context, startTime time.Time, endTime time.Time, topN int) (BreweryList []map[string]int, StyleList []map[string]int) {
	beerList := GetTapToUniqueBeerList(context, startTime, endTime)

	breweryList := make(map[string]int)
	styleList := make(map[string]int)

	for i := 1; i <= len(beerList); i++ {
		for _, beer := range beerList[i] {

			// Set or increment brewery count
			if _, ok := breweryList[beer.BreweryName()]; ok {
				breweryList[beer.BreweryName()] += 1
			} else {
				breweryList[beer.BreweryName()] = 1
			}

			// Set or increment style count
			if _, ok := styleList[beer.StyleName()]; ok {
				styleList[beer.StyleName()] += 1
			} else {
				styleList[beer.StyleName()] = 1
			}
		}
	}

	// Sort and create the return map
	sortedBrewery := helpers.SortMapByValue(breweryList)
	sortedStyle := helpers.SortMapByValue(styleList)

	for i := 0; i < topN; i++ {
		if i < len(sortedBrewery) {
			BreweryList = append(BreweryList, map[string]int{sortedBrewery[i].Key: sortedBrewery[i].Value})
		}

		if i < len(sortedStyle) {
			StyleList = append(StyleList, map[string]int{sortedStyle[i].Key: sortedStyle[i].Value})
		}
	}

	return
}
