// Package google implements a Service which adds !commands for Google custom search engine.
// Initially this package just supports image search but could be expanded to provide other functionality provided by the Google custom search engine API - https://developers.google.com/custom-search/json-api/v1/overview
package google

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/go-neb/types"
	"github.com/matrix-org/gomatrix"
)

// ServiceType of the Google service
const ServiceType = "google"

var httpClient = &http.Client{}

// Unsused -- leaving this in place for the time being to show structure of the request
// type googleQuery struct {
// 	// Query search text
// 	Query string `json:"q"`
// 	// Number of search results
// 	Num int `json:"num"`
// 	// Search result offset
// 	Start int `json:"start"`
// 	// Size of images to serch for (usually set to "medium")
// 	ImgSize string `json:"imgSize"`
// 	// Type of search - Currently always set to "image"
// 	SearchType string `json:"searchType"`
// 	// Type of image file to retur64 `json:"totalResults"`
// 	FileType string `json:"fileType"`
// 	// API key
// 	Key string `json:"key"`
// 	// Custom serch engine ID
// 	Cx string `json:"cx"`
// }

type googleSearchResults struct {
	SearchInformation struct {
		TotalResults int64 `json:"totalResults,string"`
	} `json:"searchInformation"`
	Items []googleSearchResult `json:"items"`
}

type googleSearchResult struct {
	Title       string      `json:"title"`
	HTMLTitle   string      `json:"htmlTitle"`
	Link        string      `json:"link"`
	DisplayLink string      `json:"displayLink"`
	Snippet     string      `json:"snippet"`
	HTMLSnippet string      `json:"htmlSnippet"`
	Mime        string      `json:"mime"`
	FileFormat  string      `json:"fileFormat"`
	Image       googleImage `json:"image"`
}

type googleImage struct {
	ContextLink     string  `json:"contextLink"`
	Height          float64 `json:"height"`
	Width           float64 `json:"width"`
	ByteSize        int64   `json:"byteSize"`
	ThumbnailLink   string  `json:"thumbnailLink"`
	ThumbnailHeight float64 `json:"thumbnailHeight"`
	ThumbnailWidth  float64 `json:"thumbnailWidth"`
}

// Service contains the Config fields for the Google service.
// TODO - move the google custom search engine ID in here!
//
// Example request:
//   {
//       "api_key": "AIzaSyA4FD39m9pN-hiYf2NRU9x9cOv5tekRDvM"
//   }
type Service struct {
	types.DefaultService
	// The Google API key to use when making HTTP requests to Google.
	APIKey string `json:"api_key"`
}

// Commands supported:
//    !google some search query without quotes
// Responds with a suitable image into the same room as the command.
func (s *Service) Commands(client *gomatrix.Client) []types.Command {
	return []types.Command{
		types.Command{
			Path: []string{"google"},
			Command: func(roomID, userID string, args []string) (interface{}, error) {
				return s.cmdGoogle(client, roomID, userID, args)
			},
		},
	}
}

// usageMessage returns a matrix TextMessage representation of the service usage
func usageMessage() *gomatrix.TextMessage {
	return &gomatrix.TextMessage{"m.notice",
		`Usage: !google image image_search_text`}
}

func (s *Service) cmdGoogle(client *gomatrix.Client, roomID, userID string, args []string) (interface{}, error) {

	if len(args) < 2 || args[0] != "image" {
		return usageMessage(), nil
	}
	// Drop the search type (should currently always be "image")
	args = args[1:]

	// only 1 arg which is the text to search for.
	querySentence := strings.Join(args, " ")

	searchResult, err := s.text2imgGoogle(querySentence)

	if err != nil {
		return nil, err
	}

	var imgURL = searchResult.Link
	if imgURL == "" {
		return gomatrix.TextMessage{
			MsgType: "m.text.notice",
			Body:    "No image found!",
		}, nil
	}

	// FIXME -- Sometimes upload fails with a cryptic error - "msg=Upload request failed code=400 "
	resUpload, err := client.UploadLink(imgURL)
	if err != nil {
		return nil, fmt.Errorf("Failed to upload Google image to matrix: %s", err.Error())
	}

	img := searchResult.Image
	return gomatrix.ImageMessage{
		MsgType: "m.image",
		Body:    querySentence,
		URL:     resUpload.ContentURI,
		Info: gomatrix.ImageInfo{
			Height:   uint(math.Floor(img.Height)),
			Width:    uint(math.Floor(img.Width)),
			Mimetype: searchResult.Mime,
		},
	}, nil
}

// text2imgGoogle returns info about an image
func (s *Service) text2imgGoogle(query string) (*googleSearchResult, error) {
	log.Info("Searching Google for an image of a ", query)

	u, err := url.Parse("https://www.googleapis.com/customsearch/v1")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("q", query)            // String to search for
	q.Set("num", "1")            // Just return 1 image result
	q.Set("start", "1")          // No search result offset
	q.Set("imgSize", "medium")   // Just search for medium size images
	q.Set("searchType", "image") // Search for images
	// q.set("fileType, "")                             // Any file format

	var key = s.APIKey
	if key == "" {
		key = "AIzaSyA4FD39m9pN-hiYf2NRU9x9cOv5tekRDvM" // FIXME -- Should be instantiated from service config
	}
	q.Set("key", key)                                // Set the API key for the request
	q.Set("cx", "003141582324323361145:f5zyrk9_8_m") // Set the custom search engine ID

	u.RawQuery = q.Encode()
	// log.Info("Request URL: ", u)

	res, err := http.Get(u.String())
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 200 {
		return nil, fmt.Errorf("Request error: %d, %s", res.StatusCode, response2String(res))
	}
	var searchResults googleSearchResults

	// log.Info(response2String(res))
	if err := json.NewDecoder(res.Body).Decode(&searchResults); err != nil || len(searchResults.Items) < 1 {
		// Google return a JSON object which has { items: [] } if there are 0 results.
		// This fails to be deserialised by Go.

		// TODO -- Find out how to just return an error string (with no formatting)
		// return nil, errors.New("No images found")
		// return nil, fmt.Errorf("No results - %s", err)
		return nil, fmt.Errorf("No images found%s", "")
	}

	// Return only the first search result
	return &searchResults.Items[0], nil
}

// response2String returns a string representation of an HTTP response body
func response2String(res *http.Response) (responseText string) {
	bs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "Failed to decode response body"
	}
	str := string(bs)
	return str
}

// Initialise the service
func init() {
	types.RegisterService(func(serviceID, serviceUserID, webhookEndpointURL string) types.Service {
		return &Service{
			DefaultService: types.NewDefaultService(serviceID, serviceUserID, ServiceType),
		}
	})
}
