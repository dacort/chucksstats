package app

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"
	_ "appengine/remote_api"

	"github.com/dacort/chucksstats/chucks"
	"github.com/dacort/chucksstats/helpers"
	"github.com/dacort/chucksstats/scraper"
)

func Run() {
	http.Handle("/public/", http.FileServer(http.Dir("./public/")))
	http.HandleFunc("/", indexPage)
	http.HandleFunc("/current", currentText)
	http.HandleFunc("/today", beersToday)
	http.HandleFunc("/update_beers", SaveTheBeer)
	http.HandleFunc("/thisweek_old", beersWeekly)
	http.HandleFunc("/thisweek", beersWeekly2)
}
func unescaped(x string) interface{} { return template.HTML(x) }

// Templates for each page
type PageVariables struct {
	Title string
	Tab   string
}

var weeklyBeerTemplate = template.Must(template.New("weeklybeer.html").Funcs(fns).Parse(`
	<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
	<!-- Generated with d3-generator.com -->
	<html>
	  <head>
	    <title>Chuck's Weekly</title>
	    <meta http-equiv="X-UA-Compatible" content="IE=9">
	    <script src="http://d3js.org/d3.v2.min.js"></script>
	    <style>
	      h4 {
	        text-align: center;
	        margin: 0;
	      }
	    </style>
	  </head>
	  <body>
	    <table>
	      <tr>
	        <td>
	          <div id="top_breweries"></div>
	        </td>
	        <td>
	          <div id="top_styles"></div>
	        </td>
	      </tr>
	      <tr>
	        <td colspan="2"></td>
	      </tr>
	      <tr>
	        <td>
	          <div id="stale_beer"></div>
	        </td>
	        <td>
	          <div id="fresh_kegs"></div>
	        </td>
	      </tr>
	    </table>
	    <script>
	function renderChart(div, name, data) {

	var valueLabelWidth = 40; // space reserved for value labels (right)
	var barHeight = 20; // height of one bar
	var barLabelWidth = 250; // space reserved for bar labels
	var barLabelPadding = 5; // padding between bar and bar labels (left)
	var gridLabelHeight = 5; // space reserved for gridline labels
	var gridChartOffset = 0; // space between start of grid and first bar
	var maxBarWidth = 420; // width of the bar with the max value

	// accessor functions
	var barLabel = function(d) { return d['Name']; };
	var barValue = function(d) { return parseFloat(d['Value']); };

	// scales
	var yScale = d3.scale.ordinal().domain(d3.range(0, data.length)).rangeBands([0, data.length * barHeight]);
	var y = function(d, i) { return yScale(i); };
	var yText = function(d, i) { return y(d, i) + yScale.rangeBand() / 2; };
	var x = d3.scale.linear().domain([0, d3.max(data, barValue)]).range([0, maxBarWidth]);
	// svg container element
	d3.select('#'+div).append("h4")
	  .text(name)
	var chart = d3.select('#'+div).append("svg")
	  .attr('width', maxBarWidth + barLabelWidth + valueLabelWidth)
	  .attr('height', gridLabelHeight + gridChartOffset + data.length * barHeight);

	// bar labels
	var labelsContainer = chart.append('g')
	  .attr('transform', 'translate(' + (barLabelWidth - barLabelPadding) + ',' + (gridLabelHeight + gridChartOffset) + ')');
	labelsContainer.selectAll('text').data(data).enter().append('text')
	  .attr('y', yText)
	  .attr('stroke', 'none')
	  .attr('fill', 'black')
	  .attr("dy", ".35em") // vertical-align: middle
	  .attr('text-anchor', 'end')
	  .text(barLabel);
	// bars
	var barsContainer = chart.append('g')
	  .attr('transform', 'translate(' + barLabelWidth + ',' + (gridLabelHeight + gridChartOffset) + ')');
	barsContainer.selectAll("rect").data(data).enter().append("rect")
	  .attr('y', y)
	  .attr('height', yScale.rangeBand())
	  .attr('width', function(d) { return x(barValue(d)); })
	  .attr('stroke', 'white')
	  .attr('fill', 'steelblue');
	// bar value labels
	barsContainer.selectAll("text").data(data).enter().append("text")
	  .attr("x", function(d) { return x(barValue(d)); })
	  .attr("y", yText)
	  .attr("dx", 3) // padding-left
	  .attr("dy", ".35em") // vertical-align: middle
	  .attr("text-anchor", "start") // text-align: right
	  .attr("fill", "black")
	  .attr("stroke", "none")
	  .text(function(d) { return d3.round(barValue(d), 2); });
	// start line
	barsContainer.append("line")
	  .attr("y1", -gridChartOffset)
	  .attr("y2", yScale.rangeExtent()[1] + gridChartOffset)
	  .style("stroke", "#000");

	}
	    </script>

	    <script>

	      var data = JSON.parse(unescape({{.TopBreweriesJson}}));
	      renderChart("top_breweries", "Top Breweries", data);

	      var data = JSON.parse(unescape({{.TopStylesJson}}));
	      renderChart("top_styles", "Top Styles", data);

	      var data = JSON.parse(unescape({{.MostStaleJson}}));
	      renderChart("stale_beer", "Stale Beer", data);

	      var data = JSON.parse(unescape({{.FastConsumedJson}}));
	      renderChart("fresh_kegs", "Quickest Consumption", data);

	    </script>
	  </body>
	</html>
`))

var weeklyTemplate = template.Must(template.New("weekly.html").Funcs(fns).Parse(`
	{{template "bootstrap_base_head" .}}
	<body>
	  <div class="container">
	    {{ template "navbar" .}}
	    <div class="row">
	      <h3 class="text-center">Stats for the past week at Chuck's!</h3>
	    </div>
	    <div class="row">
	      <div class="col-md-4 col-md-offset-1">
	        <h4 class="text-center">Top Breweries (by # on tap)</h4>
	        {{ range .TopBreweries.Data }}
	        {{.Name}}
	        <div class="progress" style="margin-bottom: 4px;">
	          <div class="progress-bar progress-bar-info" style="width: {{.Percentage}}%">{{ .Value }}</div>
	        </div>
	        {{ end }}
	      </div>

	      <div class="col-md-4 col-md-offset-2">
	        <h4 class="text-center">Top Styles (by # on tap)</h4>
	        {{ range .TopStyles.Data }}
	        {{.Name}}
	        <div class="progress" style="margin-bottom: 4px;">
	          <div class="progress-bar progress-bar-info" style="width: {{.Percentage}}%">{{ .Value }}</div>
	        </div>
	        {{ end }}
	      </div>

	    </div>
	  </div>

	{{template "bootstrap_base_foot" .}}

	{{define "bootstrap_base_head"}}
	<!DOCTYPE html>
	<html lang="en">
	  <head>
	    <meta charset="utf-8">
	    <meta http-equiv="X-UA-Compatible" content="IE=edge">
	    <meta name="viewport" content="width=device-width, initial-scale=1">
	    <meta name="description" content="">
	    <meta name="author" content="">
	    <link rel="shortcut icon" href="../../assets/ico/favicon.ico">

	    <title>🍻 Chuck's Stats</title>

	    <!-- Bootstrap core CSS -->
	    <link rel="stylesheet" type="text/css" href="//netdna.bootstrapcdn.com/bootstrap/3.1.0/css/bootstrap.min.css">

	    <!-- Custom styles for this template -->
	    <link href="/public/css/slate.css" rel="stylesheet">
	    <link href="/public/css/sticky-footer.css" rel="stylesheet">
	    <link href="/public/css/fancy-badge.css" rel="stylesheet">

	    <!-- HTML5 shim and Respond.js IE8 support of HTML5 elements and media queries -->
	    <!--[if lt IE 9]>
	      <script src="https://oss.maxcdn.com/libs/html5shiv/3.7.0/html5shiv.js"></script>
	      <script src="https://oss.maxcdn.com/libs/respond.js/1.4.2/respond.min.js"></script>
	    <![endif]-->
	  </head>
	{{end}}

	{{define "navbar"}}
	      <div class="navbar navbar-default">
	        <div class="navbar-header">
	          <button type="button" class="navbar-toggle" data-toggle="collapse" data-target=".navbar-responsive-collapse">
	            <span class="icon-bar"></span>
	            <span class="icon-bar"></span>
	            <span class="icon-bar"></span>
	          </button>
	          <a class="navbar-brand" href="/">Chuck's Stats</a>
	        </div>
	        <div class="navbar-collapse collapse navbar-responsive-collapse">
	          <ul class="nav navbar-nav navbar-right">
	            <li{{if eq .Page.Tab "today"}} class="active"{{end}}><a href="/today">Today</a></li>
	            <li{{if eq .Page.Tab "thisweek"}} class="active"{{end}}><a href="/thisweek">This Week</a></li>
	<!--             <li class="dropdown">
	              <a href="#" class="dropdown-toggle" data-toggle="dropdown">Dropdown <b class="caret"></b></a>
	              <ul class="dropdown-menu">
	                <li><a href="#">Chuck's 85th</a></li>
	                <li><a href="#">Chuck's CD</a></li>
	                <li class="divider"></li>
	                <li><a href="#">Separated link</a></li>
	              </ul>
	            </li> -->
	          </ul>
	        </div><!-- /.nav-collapse -->
	      </div>
	{{ end }}

	{{define "bootstrap_base_foot"}}

	    <div id="footer">
	      <div class="container">
	        <p class="text-muted">Built for a love of beer, by <a href="https://twitter.com/dacort">@dacort</a>.</p>
	      </div>
	    </div>

	    <!-- Bootstrap core JavaScript
	    ================================================== -->
	    <!-- Placed at the end of the document so the pages load faster -->
	    <script src="https://ajax.googleapis.com/ajax/libs/jquery/1.11.0/jquery.min.js"></script>
	    <script src="//netdna.bootstrapcdn.com/bootstrap/3.1.0/js/bootstrap.min.js"></script>
	  </body>
	</html>
	{{end}}
`))

type WeeklyBeerVariables struct {
	TopBreweriesJson string
	TopStylesJson    string

	MostStaleJson    string
	FastConsumedJson string

	Page PageVariables
}

type WeeklyBeer2Variables struct {
	TopBreweries BarChart
	TopStyles    BarChart

	Page PageVariables
}

var indexTemplate = template.Must(template.New("index.html").Funcs(fns).Parse(`
	{{template "bootstrap_base_head" .}}

	  <body>
	    <div class="container">
	      {{ template "navbar" .}}

	      <!-- High-level stats -->
	      <div class="row">
	        <div class="col-md-3 col-md-offset-2">
	          <div class="panel panel-default">
	            <!-- Default panel contents -->
	            <div class="panel-heading">
	              <h3 class="panel-title">Fresh Taps!</h3>
	            </div>
	            <div class="panel-body">
	              <p>Beers that have been tapped today.</p>
	            </div>

	            <!-- List group -->
	            <ul class="list-group">
	              {{ range .NewBeersToday }}
	              <li class="list-group-item">{{ .FullName }}</li>
	              {{ else }}
	              <li class="list-group-item">Nothing new. :(</li>
	              {{ end }}
	            </ul>
	          </div>
	        </div>
	        <div class="col-md-3 col-md-offset-2">
	          <div class="panel panel-default">
	            <!-- Default panel contents -->
	            <div class="panel-heading">
	              <h3 class="panel-title">Top Breweries This Week!</h3>
	            </div>

	            <!-- List group -->
	            <ul class="list-group">
	              {{ range .Top5Breweries }}
	              <li class="list-group-item">{{ range $k, $v := . }}{{ $k }}<span class="badge badge-info">{{ $v }}</span>{{ end }}</li>
	              {{ end }}
	            </ul>
	          </div>
	        </div>
	      </div>
	    </div> <!-- .container -->

	{{template "bootstrap_base_foot" .}}

	{{define "bootstrap_base_head"}}
	<!DOCTYPE html>
	<html lang="en">
	  <head>
	    <meta charset="utf-8">
	    <meta http-equiv="X-UA-Compatible" content="IE=edge">
	    <meta name="viewport" content="width=device-width, initial-scale=1">
	    <meta name="description" content="">
	    <meta name="author" content="">
	    <link rel="shortcut icon" href="../../assets/ico/favicon.ico">

	    <title>🍻 Chuck's Stats</title>

	    <!-- Bootstrap core CSS -->
	    <link rel="stylesheet" type="text/css" href="//netdna.bootstrapcdn.com/bootstrap/3.1.0/css/bootstrap.min.css">

	    <!-- Custom styles for this template -->
	    <link href="/public/css/slate.css" rel="stylesheet">
	    <link href="/public/css/sticky-footer.css" rel="stylesheet">
	    <link href="/public/css/fancy-badge.css" rel="stylesheet">

	    <!-- HTML5 shim and Respond.js IE8 support of HTML5 elements and media queries -->
	    <!--[if lt IE 9]>
	      <script src="https://oss.maxcdn.com/libs/html5shiv/3.7.0/html5shiv.js"></script>
	      <script src="https://oss.maxcdn.com/libs/respond.js/1.4.2/respond.min.js"></script>
	    <![endif]-->
	  </head>
	{{end}}

	{{define "navbar"}}
	      <div class="navbar navbar-default">
	        <div class="navbar-header">
	          <button type="button" class="navbar-toggle" data-toggle="collapse" data-target=".navbar-responsive-collapse">
	            <span class="icon-bar"></span>
	            <span class="icon-bar"></span>
	            <span class="icon-bar"></span>
	          </button>
	          <a class="navbar-brand" href="/">Chuck's Stats</a>
	        </div>
	        <div class="navbar-collapse collapse navbar-responsive-collapse">
	          <ul class="nav navbar-nav navbar-right">
	            <li{{if eq .Page.Tab "today"}} class="active"{{end}}><a href="/today">Today</a></li>
	            <li{{if eq .Page.Tab "thisweek"}} class="active"{{end}}><a href="/thisweek">This Week</a></li>
	<!--             <li class="dropdown">
	              <a href="#" class="dropdown-toggle" data-toggle="dropdown">Dropdown <b class="caret"></b></a>
	              <ul class="dropdown-menu">
	                <li><a href="#">Chuck's 85th</a></li>
	                <li><a href="#">Chuck's CD</a></li>
	                <li class="divider"></li>
	                <li><a href="#">Separated link</a></li>
	              </ul>
	            </li> -->
	          </ul>
	        </div><!-- /.nav-collapse -->
	      </div>
	{{ end }}

	{{define "bootstrap_base_foot"}}

	    <div id="footer">
	      <div class="container">
	        <p class="text-muted">Built for a love of beer, by <a href="https://twitter.com/dacort">@dacort</a>.</p>
	      </div>
	    </div>

	    <!-- Bootstrap core JavaScript
	    ================================================== -->
	    <!-- Placed at the end of the document so the pages load faster -->
	    <script src="https://ajax.googleapis.com/ajax/libs/jquery/1.11.0/jquery.min.js"></script>
	    <script src="//netdna.bootstrapcdn.com/bootstrap/3.1.0/js/bootstrap.min.js"></script>
	  </body>
	</html>
	{{end}}
`))

type IndexPageVariables struct {
	NewBeersToday []chucks.Beer
	Top5Breweries []map[string]int

	Page PageVariables
}

var fns = template.FuncMap{
	"join": strings.Join,
	"eq": func(a, b string) bool {
		return a == b
	},
	"gt": func(a, b int) bool {
		return a > b
	},
	"last": func(x int, a interface{}) bool {
		return x == reflect.ValueOf(a).Len()-1
	},
}

var todayTemplate = template.Must(template.New("today.html").Funcs(fns).Parse(`
	{{template "bootstrap_base_head" .}}
	<body>
	  <div class="container">
	    {{ template "navbar" .}}
	    <div class="row">
	      <div class="col-md-6 col-md-offset-3">

	        <ul class="list-group">
	          <li class="list-group-item" style="background-color:lightslategrey;color:black"><strong>Beers on tap today - <span class="text-success">green</span> ones indicate newly on tap.</strong></li>
	          {{ range $index, $beers := .TapList}}
	          <li class="list-group-item">Tap {{$index}}) {{ range $beer_index, $beer := $beers }}{{ if last $beer_index $beers }}{{ if gt $beer_index 0}}<span class="text-success">{{end}}{{end}}{{ $beer.FullName }}{{ if last $beer_index $beers }}</span>{{end}}{{ if last $beer_index $beers }}. {{ else }}, {{ end }}{{ end }}</li>
	          {{ end }}
	        </ul>
	      </div>
	    </div>
	  </div>

	{{template "bootstrap_base_foot" .}}

	{{define "bootstrap_base_head"}}
	<!DOCTYPE html>
	<html lang="en">
	  <head>
	    <meta charset="utf-8">
	    <meta http-equiv="X-UA-Compatible" content="IE=edge">
	    <meta name="viewport" content="width=device-width, initial-scale=1">
	    <meta name="description" content="">
	    <meta name="author" content="">
	    <link rel="shortcut icon" href="../../assets/ico/favicon.ico">

	    <title>🍻 Chuck's Stats</title>

	    <!-- Bootstrap core CSS -->
	    <link rel="stylesheet" type="text/css" href="//netdna.bootstrapcdn.com/bootstrap/3.1.0/css/bootstrap.min.css">

	    <!-- Custom styles for this template -->
	    <link href="/public/css/slate.css" rel="stylesheet">
	    <link href="/public/css/sticky-footer.css" rel="stylesheet">
	    <link href="/public/css/fancy-badge.css" rel="stylesheet">

	    <!-- HTML5 shim and Respond.js IE8 support of HTML5 elements and media queries -->
	    <!--[if lt IE 9]>
	      <script src="https://oss.maxcdn.com/libs/html5shiv/3.7.0/html5shiv.js"></script>
	      <script src="https://oss.maxcdn.com/libs/respond.js/1.4.2/respond.min.js"></script>
	    <![endif]-->
	  </head>
	{{end}}

	{{define "navbar"}}
	      <div class="navbar navbar-default">
	        <div class="navbar-header">
	          <button type="button" class="navbar-toggle" data-toggle="collapse" data-target=".navbar-responsive-collapse">
	            <span class="icon-bar"></span>
	            <span class="icon-bar"></span>
	            <span class="icon-bar"></span>
	          </button>
	          <a class="navbar-brand" href="/">Chuck's Stats</a>
	        </div>
	        <div class="navbar-collapse collapse navbar-responsive-collapse">
	          <ul class="nav navbar-nav navbar-right">
	            <li{{if eq .Page.Tab "today"}} class="active"{{end}}><a href="/today">Today</a></li>
	            <li{{if eq .Page.Tab "thisweek"}} class="active"{{end}}><a href="/thisweek">This Week</a></li>
	<!--             <li class="dropdown">
	              <a href="#" class="dropdown-toggle" data-toggle="dropdown">Dropdown <b class="caret"></b></a>
	              <ul class="dropdown-menu">
	                <li><a href="#">Chuck's 85th</a></li>
	                <li><a href="#">Chuck's CD</a></li>
	                <li class="divider"></li>
	                <li><a href="#">Separated link</a></li>
	              </ul>
	            </li> -->
	          </ul>
	        </div><!-- /.nav-collapse -->
	      </div>
	{{ end }}

	{{define "bootstrap_base_foot"}}

	    <div id="footer">
	      <div class="container">
	        <p class="text-muted">Built for a love of beer, by <a href="https://twitter.com/dacort">@dacort</a>.</p>
	      </div>
	    </div>

	    <!-- Bootstrap core JavaScript
	    ================================================== -->
	    <!-- Placed at the end of the document so the pages load faster -->
	    <script src="https://ajax.googleapis.com/ajax/libs/jquery/1.11.0/jquery.min.js"></script>
	    <script src="//netdna.bootstrapcdn.com/bootstrap/3.1.0/js/bootstrap.min.js"></script>
	  </body>
	</html>
	{{end}}
`))

type TodayPageVariables struct {
	TapList map[int][]chucks.Beer

	Page PageVariables
}

// END Templates for each page
// Maybe use https://wrapbootstrap.com/theme/light-blue-responsive-admin-template-WB0T41TX4 ?

type BarChart struct {
	Data []BarChartRow
}
type BarChartRow struct {
	Name       string
	Value      int
	Percentage float64
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

func GetStartAndEndOfToday() (time.Time, time.Time) {
	// Set up our time frames (Pacific yo)
	location, _ := time.LoadLocation("America/Los_Angeles")
	today := time.Now().In(location)
	tomorrow := time.Now().In(location).Add(time.Duration(24 * time.Hour))

	// They open at 10, so don't show anything until then. The downside of this is that
	// there will be no results on the /today page between midnight and 10am.
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

func beersWeekly2(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	weekAgo, today := GetStartAndEndOfWeek()

	topBreweries, topStyles := chucks.GetTopBreweriesAndStyles(c, weekAgo, today, 5)

	var topBreweriesFormatted BarChart
	var topStylesFormatted BarChart
	var max float64

	for i := range topBreweries {
		for key, value := range topBreweries[i] {
			if i == 0 {
				max = float64(value)
			}
			topBreweriesFormatted.Data = append(
				topBreweriesFormatted.Data, BarChartRow{
					key,
					value,
					(float64(value) / max) * 100,
				})
		}
	}

	for i := range topStyles {
		for key, value := range topStyles[i] {
			if i == 0 {
				max = float64(value)
			}
			topStylesFormatted.Data = append(
				topStylesFormatted.Data, BarChartRow{
					key,
					value,
					(float64(value) / max) * 100,
				})
		}
	}

	if err := weeklyTemplate.Execute(w, WeeklyBeer2Variables{topBreweriesFormatted, topStylesFormatted, PageVariables{"Weekly Beer", "thisweek"}}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	sorted := helpers.SortMapByValue(breweryList)
	for i := 0; i < 5; i++ {
		breweryData.Data = append(breweryData.Data, BarChartRow{sorted[i].Key, sorted[i].Value, 0})
	}
	b, _ := json.Marshal(breweryData.Data)
	topBrewJson := fmt.Sprintf("%s", b)

	// Calculate the top styles
	styleList := make(map[string]int)
	for style, beers := range beerAndStyleList {
		styleList[style] = len(beers)
	}

	styleData := BarChart{}
	sorted = helpers.SortMapByValue(styleList)
	for i := 0; i < 5; i++ {
		styleData.Data = append(styleData.Data, BarChartRow{sorted[i].Key, sorted[i].Value, 0})
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
	sorted = helpers.SortMapByValue(styleList)
	for i := 0; i < 5; i++ {
		staleBeerData.Data = append(staleBeerData.Data, BarChartRow{beerByTimeList[i].Name, int(beerByTimeList[i].TotalTime.Hours()), 0})
	}
	b, _ = json.Marshal(staleBeerData.Data)
	staleBeerJson := fmt.Sprintf("%s", b)

	sort.Sort(ByFastestConsumption{beerByTimeList})
	freshBeerData := BarChart{}
	sorted = helpers.SortMapByValue(styleList)
	for i := 0; i < 5; i++ {
		freshBeerData.Data = append(freshBeerData.Data, BarChartRow{beerByTimeList[i].Name, int(beerByTimeList[i].TotalTime.Hours()), 0})
	}
	b, _ = json.Marshal(freshBeerData.Data)
	freshBeerJson := fmt.Sprintf("%s", b)

	if err := weeklyBeerTemplate.Execute(w, WeeklyBeerVariables{topBrewJson, topStyleJson, staleBeerJson, freshBeerJson, PageVariables{"Weekly Beer", "weeklybeer"}}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	// fmt.Fprintln(w, beerByTimeList[0])
	// for brewery, count := range breweryList {
	// 	fmt.Fprintf(w, "%s: %d\n", brewery, count)
	// }

	// Print out the top 5 brewerys
	// sorted := helpers.SortMapByValue(breweryList)
	// for i := 0; i < 5; i++ {
	// 	fmt.Fprintf(w, "%s: %d\n", sorted[i].Key, sorted[i].Value)
	// }
}

func beersToday(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	todayStart, todayEnd := GetStartAndEndOfToday()
	c.Debugf("Getting beers between %s and %s", todayStart, todayEnd)
	tapList := chucks.GetTapToUniqueBeerList(c, todayStart, todayEnd)

	pageVars := TodayPageVariables{
		tapList,
		PageVariables{"Today", "today"},
	}
	if err := todayTemplate.Execute(w, pageVars); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	return

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

	var top5Breweries []map[string]int

	// Calculate the top breweries
	breweryList := make(map[string]int)
	for i := 1; i <= len(pastWeekBeers); i++ {
		for j := 0; j < len(pastWeekBeers[i]); j++ {
			beer := pastWeekBeers[i][j]
			if _, ok := breweryList[beer.BreweryName()]; ok {
				breweryList[beer.BreweryName()] += 1
			} else {
				breweryList[beer.BreweryName()] = 1
			}
		}
	}
	sorted := helpers.SortMapByValue(breweryList)
	for i := 0; i < 5; i++ {

		top5Breweries = append(top5Breweries, map[string]int{sorted[i].Key: sorted[i].Value})
	}
	// top5Breweries = sorted[0:4]
	// END calculate the top breweries

	pageVars := IndexPageVariables{
		newBeers,
		top5Breweries,
		PageVariables{"Home", "home"},
	}
	if err := indexTemplate.Execute(w, pageVars); err != nil {
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
