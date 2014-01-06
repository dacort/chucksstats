package chucks

import (
	"time"

	"appengine"
	"appengine/datastore"
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
