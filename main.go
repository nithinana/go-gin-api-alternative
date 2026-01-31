package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

const mainUrl = "https://einthusan.tv"

// Data structures
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
	Page      int          `json:"page"`
}

type WatchResponse struct {
	Title    string `json:"title"`
	VideoUrl string `json:"video_url"`
	ImgUrl   string `json:"img_url"` 
}

func main() {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://thirai.me", "http://thirai.me", "https://www.thirai.me"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS", "PUT"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "thirai api",
			"endpoints": gin.H{
				"search": "/search/:language?q=movie_title",
				"browse": "/language/:language?category=recent|popular&page=1",
				"actors": "/actors/:language/:actorcode?page=1",
				"genre":  "/genre/:language?action=0-4&comedy=0-4&romance=0-4&storyline=0-4&performance=0-4&ratecount=1&page=1",
				"decade": "/decade/:language/:decade?page=1",
				"year":   "/year/:language/:year?page=1",
				"watch":  "/watch?url=einthusan_page_url",
			},
			"example_usage": "Try /year/tamil/2025 or /genre/hindi?action=4&ratecount=5",
		})
	})

	// 1. SEARCH
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
		sort.Slice(movies, func(i, j int) bool {
			scoreI := fuzzy.RankMatch(strings.ToLower(query), strings.ToLower(movies[i].Title))
			scoreJ := fuzzy.RankMatch(strings.ToLower(query), strings.ToLower(movies[j].Title))
			return scoreI > scoreJ
		})
		c.JSON(http.StatusOK, SearchResponse{Language: language, Movies: movies, Query: query})
	})

	// 2. BROWSE
	r.GET("/language/:language", func(c *gin.Context) {
		language := c.Param("language")
		category := strings.ToLower(c.DefaultQuery("category", "recent"))
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)
		var targetUrl string
		if category == "popular" {
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
		c.JSON(http.StatusOK, BrowseResponse{Category: category, HasMore: len(movies) > 0, Language: language, Movies: movies, NextPage: page + 1, Page: page})
	})

	// 3. ACTORS
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
		c.JSON(http.StatusOK, ActorResponse{ActorID: actorCode, ActorName: "Unknown Actor", HasMore: len(movies) > 0, Language: language, Movies: movies, NextPage: page + 1, Page: page})
	})

	// 4. GENRE
	r.GET("/genre/:language", func(c *gin.Context) {
		language := c.Param("language")
		action := c.DefaultQuery("action", "0")
		comedy := c.DefaultQuery("comedy", "0")
		romance := c.DefaultQuery("romance", "0")
		storyline := c.DefaultQuery("storyline", "0")
		performance := c.DefaultQuery("performance", "0")
		ratecount := c.DefaultQuery("ratecount", "1") // Dynamic ratecount
		
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)

		targetUrl := fmt.Sprintf(
			"%s/movie/results/?lang=%s&find=Rating&action=%s&comedy=%s&romance=%s&storyline=%s&performance=%s&ratecount=%s",
			mainUrl, language, action, comedy, romance, storyline, performance, ratecount,
		)
		if page > 1 {
			targetUrl = fmt.Sprintf("%s&page=%d", targetUrl, page)
		}
		movies, err := scrapeEinthusan(targetUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, BrowseResponse{Category: "Genre", HasMore: len(movies) > 0, Language: language, Movies: movies, NextPage: page + 1, Page: page})
	})

	// 5. DECADE
	r.GET("/decade/:language/:decade", func(c *gin.Context) {
		language := c.Param("language")
		decade := c.Param("decade")
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)

		targetUrl := fmt.Sprintf("%s/movie/results/?decade=%s&find=Decade&lang=%s", mainUrl, decade, language)
		if page > 1 {
			targetUrl = fmt.Sprintf("%s&page=%d", targetUrl, page)
		}

		movies, err := scrapeEinthusan(targetUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, BrowseResponse{Category: "Decade: " + decade, HasMore: len(movies) > 0, Language: language, Movies: movies, NextPage: page + 1, Page: page})
	})

	// 6. YEAR
	r.GET("/year/:language/:year", func(c *gin.Context) {
		language := c.Param("language")
		year := c.Param("year")
		pageStr := c.DefaultQuery("page", "1")
		page, _ := strconv.Atoi(pageStr)

		targetUrl := fmt.Sprintf("%s/movie/results/?find=Year&lang=%s&year=%s", mainUrl, language, year)
		if page > 1 {
			targetUrl = fmt.Sprintf("%s&page=%d", targetUrl, page)
		}

		movies, err := scrapeEinthusan(targetUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, BrowseResponse{Category: "Year: " + year, HasMore: len(movies) > 0, Language: language, Movies: movies, NextPage: page + 1, Page: page})
	})

	// 7. WATCH
	r.GET("/watch", func(c *gin.Context) {
		pageUrl := c.Query("url")
		if pageUrl == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "URL parameter is required"})
			return
		}
		watchData, err := scrapeWatchDetails(pageUrl)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json; charset=utf-8")
		encoder := json.NewEncoder(c.Writer)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(watchData); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}

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
			}
			movies = append(movies, MovieEntry{ImgUrl: fullImg, PageUrl: mainUrl + href, Title: title})
		}
	})
	return movies, nil
}

func scrapeWatchDetails(url string) (*WatchResponse, error) {
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(doc.Find("#UIMovieSummary div.block2 a.title h3").First().Text())

	imgSrc, _ := doc.Find("#UIMovieSummary div.block1 img").Attr("src")
	if strings.HasPrefix(imgSrc, "//") {
		imgSrc = "https:" + imgSrc
	}

	videoPlayer := doc.Find("#UIVideoPlayer")
	mp4Link, _ := videoPlayer.Attr("data-mp4-link")
	if mp4Link == "" {
		mp4Link, _ = videoPlayer.Attr("data-hls-link")
	}

	finalUrl := mp4Link
	if finalUrl != "" {
		if strings.HasPrefix(finalUrl, "//") {
			finalUrl = "https:" + finalUrl
		}
		re := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
		finalUrl = re.ReplaceAllString(finalUrl, "cdn1.einthusan.io")
	}

	return &WatchResponse{
		Title:    title,
		VideoUrl: finalUrl,
		ImgUrl:   imgSrc,
	}, nil
}
