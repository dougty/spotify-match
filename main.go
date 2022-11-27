package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dougty/levdist"
)

var client = &http.Client{Timeout: 10 * time.Second}

type configStruct struct {
	ClientID     string
	ClientSecret string
	Token        string
	ExpireTime   time.Time
}

var config configStruct

func getAccessToken() {
	addr := "https://accounts.spotify.com/api/token"
	form := url.Values{}
	form.Add("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", addr, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatalln("error creating request:", err)
	}

	auth := config.ClientID + ":" + config.ClientSecret
	encoded := base64.RawStdEncoding.EncodeToString([]byte(auth))

	req.Header.Set("Authorization", "Basic "+encoded)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln("error doing request:", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("error reading body:", err)
	}

	resjson := struct {
		Access_token string
		Expires_in   int
	}{}
	err = json.Unmarshal(b, &resjson)
	if err != nil {
		log.Fatalln("error unmarshaling response:", err)
	}

	config.Token = resjson.Access_token
	config.ExpireTime = time.Now().Add(time.Duration(resjson.Expires_in * int(time.Second)))

	saveConfig()
}

func saveConfig() {
	configjson, err := json.MarshalIndent(config, "", "\t")
	if err != nil {
		log.Fatalln("error marshaling config json:", err)
	}

	err = os.WriteFile("config.json", []byte(configjson), 0644)
	if err != nil {
		log.Fatalln("error writing config.json:", err)
	}
}

type artist struct {
	Name string
}

type trackitem struct {
	Artists []artist
	Name    string
	Uri     string
}

type track struct {
	Items []trackitem
}

type response struct {
	Tracks track
}

func doReq(song string) response {
	song = url.QueryEscape(song)
	addr := "https://api.spotify.com/v1/search?q=" + song + "&type=track&market=US&limit=5"

	req, err := http.NewRequest("GET", addr, nil)
	if err != nil {
		log.Fatalln("error creating request:", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalln("error doing request:", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("error reading body:", err)
	}

	if resp.StatusCode != 200 {
		log.Fatalln("non-200 status code:", err, string(b))
	}

	var resjson response
	err = json.Unmarshal(b, &resjson)
	if err != nil {
		log.Fatalln("error unmarshaling response:", err)
	}

	return resjson
}

func main() {
	log.Println("reading playlist....")

	if len(os.Args) < 2 {
		log.Fatalln("specify a playlist file to match")
	}

	fn := os.Args[1]
	b, err := os.ReadFile(fn)
	if err != nil {
		log.Fatalln("error reading playlist file:", err)
	}
	playlist := strings.Split(string(b), "\n")

	log.Println("loading config...")

	b, err = os.ReadFile("config.json")
	if err != nil {
		saveConfig()
		log.Fatalln("couldn't read config file\nplease edit config.json and run again")
	}

	err = json.Unmarshal(b, &config)
	if err != nil {
		log.Fatalln("error unmarshaling config:", err)
	}

	// get token
	if config.Token == "" || config.ExpireTime.Before(time.Now()) {
		log.Println("getting new token...")
		getAccessToken()
	} else {
		log.Println("using config token")
	}

	// match songs
	partialLog := ""
	unmatchedLog := ""
	matches := ""

	log.Println("matching songs...")
	for _, song := range playlist {
		if song == "" {
			continue
		}

		song = strings.ReplaceAll(song, "’", "'")
		song = strings.ReplaceAll(song, "\r", "")
		song = strings.ReplaceAll(song, "\n", "")

		resp := doReq(song)
		matched := false

		for i := range resp.Tracks.Items {
			uri := resp.Tracks.Items[i].Uri

			resp_artist := resp.Tracks.Items[i].Artists[0].Name
			resp_title := strings.ReplaceAll(resp.Tracks.Items[i].Name, "’", "'")

			resp_song := resp_artist + " - " + resp_title
			dist := levdist.Measure(song, resp_song)

			if dist == 0 {
				log.Println("         ", song)
				matches += uri + "\n"
				matched = true
				break
			} else if dist <= 3 {
				str := fmt.Sprintf("`%s` *vs* `%s`", song, resp_song)
				log.Println("PARTIAL:", str)
				partialLog += str + "\n"
				matches += uri + "\n"
				matched = true
				break
			}
		}

		if !matched {
			log.Println("UNMATCHED:", song)
			unmatchedLog += song + "\n"
		}

		time.Sleep(500 * time.Millisecond)
	}

	// save file
	log.Println("saving output...")

	err = os.WriteFile("output_partial.txt", []byte(partialLog), 0644)
	if err != nil {
		log.Fatalln("error writing partial.txt:", err)
	}

	err = os.WriteFile("output_unmatched.txt", []byte(unmatchedLog), 0644)
	if err != nil {
		log.Fatalln("error writing unmatched.txt:", err)
	}

	err = os.WriteFile("output_matches.txt", []byte(matches), 0644)
	if err != nil {
		log.Fatalln("error writing matches.txt:", err)
	}
}
