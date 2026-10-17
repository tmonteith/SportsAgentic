package resolver

import (
	"strings"
)

var teamAliases = map[string]string{
	// SEC
	"bama":          "Alabama",
	"elephants":     "Alabama",
	"crimson tide":  "Alabama",
	"ole miss":      "Mississippi",
	"mississippi":   "Mississippi",
	"rebels":        "Mississippi",
	"miss state":    "Mississippi State",
	"bulldogs":      "Georgia",
	"uga":           "Georgia",
	"lsue":          "LSU",
	"tigers":        "LSU",
	"vandy":         "Vanderbilt",
	"commodores":    "Vanderbilt",
	"vols":          "Tennessee",
	"tennessee":     "Tennessee",
	"gators":        "Florida",
	"uf":            "Florida",
	"aggies":        "Texas A&M",
	"a&m":           "Texas A&M",
	"tamu":          "Texas A&M",
	"longhorns":     "Texas",
	"ut":            "Texas",
	"sooners":       "Oklahoma",
	"ou":            "Oklahoma",

	// Big Ten
	"osu":           "Ohio State",
	"buckeyes":      "Ohio State",
	"ohio st":       "Ohio State",
	"um":            "Michigan",
	"wolverines":    "Michigan",
	"mich state":    "Michigan State",
	"msu":           "Michigan State",
	"spartans":      "Michigan State",
	"psu":           "Penn State",
	"nittany lions": "Penn State",
	"penn st":       "Penn State",
	"badgers":       "Wisconsin",
	"cornhuskers":   "Nebraska",
	"huskers":       "Nebraska",
	"hawkeyes":      "Iowa",

	// ACC / Big 12 / Pac-12 / Others
	"fsu":           "Florida State",
	"seminoles":     "Florida State",
	"clemson":       "Clemson",
	"canes":         "Miami",
	"hurricanes":    "Miami",
	"nd":            "Notre Dame",
	"fighting irish":"Notre Dame",
	"irish":         "Notre Dame",
	"usc":           "USC",
	"trojans":       "USC",
	"ducks":         "Oregon",
}

func ResolveTeamName(input string) string {
	clean := strings.ToLower(strings.TrimSpace(input))
	if canonical, ok := teamAliases[clean]; ok {
		return canonical
	}
	return strings.Title(clean)
}
