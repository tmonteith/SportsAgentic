package cfbd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.collegefootballdata.com"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type Game struct {
	ID            int    `json:"id"`
	Season        int    `json:"season"`
	Week          int    `json:"week"`
	SeasonType    string `json:"season_type"`
	StartDate     string `json:"start_date"`
	Completed     bool   `json:"completed"`
	HomeTeam      string `json:"home_team"`
	HomeConference string `json:"home_conference"`
	HomePoints    *int   `json:"home_points"`
	AwayTeam      string `json:"away_team"`
	AwayConference string `json:"away_conference"`
	AwayPoints    *int   `json:"away_points"`
	Venue         string `json:"venue"`
}

type Team struct {
	ID           int      `json:"id"`
	School       string   `json:"school"`
	Mascot       string   `json:"mascot"`
	Abbreviation string   `json:"abbreviation"`
	Conference   string   `json:"conference"`
	Division     string   `json:"division"`
	Classification string `json:"classification"`
}

type PlayerStat struct {
	Season     int    `json:"season"`
	PlayerID   string `json:"playerId"`
	Player     string `json:"player"`
	Team       string `json:"team"`
	Conference string `json:"conference"`
	Category   string `json:"category"`
	StatType   string `json:"statType"`
	Stat       string `json:"stat"`
}

type RankingWeek struct {
	Season     int    `json:"season"`
	SeasonType string `json:"seasonType"`
	Week       int    `json:"week"`
	Polls      []Poll `json:"polls"`
}

type Poll struct {
	Poll    string `json:"poll"`
	Ranks   []Rank `json:"ranks"`
}

type Rank struct {
	Rank       int    `json:"rank"`
	School     string `json:"school"`
	Conference string `json:"conference"`
	FirstPlaceVotes int `json:"firstPlaceVotes"`
	Points     int    `json:"points"`
}

func (c *Client) get(endpoint string, query url.Values, v interface{}) error {
	reqURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	if len(query) > 0 {
		reqURL = fmt.Sprintf("%s?%s", reqURL, query.Encode())
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CFBD API returned status: %s", resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return nil
}

func (c *Client) GetGames(year int, week int, team string) ([]Game, error) {
	q := url.Values{}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	if week > 0 {
		q.Set("week", strconv.Itoa(week))
	}
	if team != "" {
		q.Set("team", team)
	}

	var games []Game
	err := c.get("/games", q, &games)
	return games, err
}

func (c *Client) GetTeams() ([]Team, error) {
	q := url.Values{}
	q.Set("conference", "SEC") // can expand or omit filters
	var teams []Team
	err := c.get("/teams/fbs", q, &teams)
	return teams, err
}

func (c *Client) GetPlayerStats(year int, team string, category string) ([]PlayerStat, error) {
	q := url.Values{}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	if team != "" {
		q.Set("team", team)
	}
	if category != "" {
		q.Set("category", category)
	}

	var stats []PlayerStat
	err := c.get("/stats/player/season", q, &stats)
	return stats, err
}

func (c *Client) GetRankings(year int, week int) ([]RankingWeek, error) {
	q := url.Values{}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	if week > 0 {
		q.Set("week", strconv.Itoa(week))
	}

	var rankings []RankingWeek
	err := c.get("/rankings", q, &rankings)
	return rankings, err
}
