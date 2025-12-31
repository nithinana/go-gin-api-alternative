package main

import (
	"fmt"
	"net/http"
	"os" // Added for Render environment variables
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

const mainUrl = "https://einthusan.tv"

// Data structures matching your provided JSON formats
type MovieEntry struct {
	ImgUrl  string `json:"img_url"`
	PageUrl string `json:"page_url"`
	Title   string `json:"title"`
}

type SearchResponse struct {
	Language string       `json:"language"`
	Movies   []MovieEntry `json:"movies"`
	Query    string       `json:"q"`
}

type BrowseResponse struct {
	Category string       `json:"category"`
	HasMore  bool         `json:"has_more"`
	Language string       `json:"language"`
	Movies   []MovieEntry `json:"movies"`
	NextPage int          `json:"next_page"`
	Page     int          `json:"page"`
}

type ActorResponse struct {
	ActorID   string       `json:"actor_id"`
	ActorName string       `json:"actor_name"`
	HasMore   bool         `json:"has_more"`
	Language  string       `json:"language"`
	Movies    []MovieEntry `json:"movies"`
	NextPage  int          `json:"next_page"`
	Page     int          `json:"page"`
}

func main() {
	r := gin.Default()

	// 0. MAIN PAGE: List all available endpoints
	r.GET("/", func(c *gin.Context) {
		endpoints := gin.H{
			"message": "Welcome to the Einthusan API Wrapper",
			"endpoints": []gin.H{
				{
					"path":        "/search/:language?q=:query",
					"description": "Search for movies in a specific language",
					"example":     "/search/tamil?q=theri",
				},
				{
					"path":        "/language/:language?category=popular|recent&page=1",
					"description": "Browse movies by category and language",
					"example":     "/language/tamil?category=popular&page=1",
				},
				{
					"path":        "/actors/:language/:actorcode?page=1",
					"description": "Get movies by a specific actor code",
					"example":     "/actors/tamil/1Uv6Det8Ej?page=1",
				},
			},
		}
		c.JSON(http.StatusOK, endpoints)
	})

	// 1. SEARCH: /search/(LANGUAGE)?q=(SEARCH)
	r.GET("/search/:language", func(c *gin.Context) {
		language := c.Param("language")
		query := c.Query("q")

		if query == "" {
			c.JSON(http.StatusOK, SearchResponse{Language: language, Movies: []MovieEntry{}, Query: query})
			return
		}

		fixedQuery := strings.ReplaceAll(query, " ", "+")
		targetUrl := fmt.Sprintf("%s/movie/results/?lang=%s&query=%s", mainUrl, language, fixedQuery)
		
		movies, err := scrapeEinthusan(targetUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Replicates FuzzySearch logic from EinthusanProvider.kt
		sort.Slice(movies, func(i, j int) bool {
			scoreI := fuzzy.RankMatch(strings.ToLower(query), strings.ToLower(movies[i].Title))
			scoreJ := fuzzy.RankMatch(strings.ToLower(query), strings.ToLower(movies[j].Title))
			return scoreI > scoreJ
		})

		c.JSON(http.StatusOK, SearchResponse{Language: language, Movies: movies, Query: query})
	})

	// 2. BROWSE: /language/(LANGUAGE)?category=popular|recent&page=1
	r.GET("/language/:language", func(c *gin.Context) {
		language := c.Param("language")
		category := strings.ToLower(c.DefaultQuery("category", "recent"))
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)

		var targetUrl string
		if category == "popular" {
			// Targeted Popularity URL as requested
			targetUrl = fmt.Sprintf("%s/movie/results/?find=Popularity&lang=%s&ptype=view&tp=alltime", mainUrl, language)
		} else {
			targetUrl = fmt.Sprintf("%s/movie/results/?find=Recent&lang=%s", mainUrl, language)
		}

		if page > 1 {
			targetUrl = fmt.Sprintf("%s&page=%d", targetUrl, page)
		}

		movies, err := scrapeEinthusan(targetUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, BrowseResponse{
			Category: category,
			HasMore:  len(movies) > 0,
			Language: language,
			Movies:   movies,
			NextPage: page + 1,
			Page:     page,
		})
	})

	// 3. ACTORS: /actors/(LANGUAGE)/(ACTORCODE)?page=1
	r.GET("/actors/:language/:actorcode", func(c *gin.Context) {
		language := c.Param("language")
		actorCode := c.Param("actorcode")
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)

		targetUrl := fmt.Sprintf("%s/movie/results/?find=Cast&id=%s&lang=%s&role=", mainUrl, actorCode, language)
		if page > 1 {
			targetUrl = fmt.Sprintf("%s&page=%d", targetUrl, page)
		}

		movies, err := scrapeEinthusan(targetUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, ActorResponse{
			ActorID:   actorCode,
			ActorName: "Unknown Actor",
			HasMore:   len(movies) > 0,
			Language:  language,
			Movies:    movies,
			NextPage:  page + 1,
			Page:      page,
		})
	})

	// Updated port logic for Render deployment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}

// Scrape helper using selectors from EinthusanProvider.kt
func scrapeEinthusan(url string) ([]MovieEntry, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	var movies []MovieEntry
	doc.Find("#UIMovieSummary > ul > li").Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Find("div.block2 > a.title > h3").Text())
		href, _ := s.Find("div.block2 > a.title").Attr("href")
		imgSrc, _ := s.Find("div.block1 > a > img").Attr("src")

		if title != "" {
			fullImg := imgSrc
			if strings.HasPrefix(imgSrc, "//") {
				fullImg = "https:" + imgSrc
			} else if !strings.HasPrefix(imgSrc, "http") {
				fullImg = "https:" + imgSrc
			}

			movies = append(movies, MovieEntry{
				ImgUrl:  fullImg,
				PageUrl: mainUrl + href,
				Title:   title,
			})
		}
	})

	return movies, nil
}
