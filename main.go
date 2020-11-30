package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	"mvdan.cc/xurls"
)

var apikey string
var mqtthost string
var brokername string

var lastTweetId int64

func main() {
	lastTweetId = 0

	// Init godotenv
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// init params
	apikey = os.Getenv("TWEET2MQTT_APIKEY")
	mqtthost = os.Getenv("TWEET2MQTT_MQTT_HOST")
	brokername = os.Getenv("TWEET2MQTT_BROKER_NAME")

	// Init MQTT Connection
	mqtt.ERROR = log.New(os.Stdout, "[ERROR] ", 0)
	mqtt.CRITICAL = log.New(os.Stdout, "[CRIT] ", 0)
	mqtt.WARN = log.New(os.Stdout, "[WARN]  ", 0)
	mqtt.DEBUG = log.New(os.Stdout, "[DEBUG] ", 0)

	//opts := mqtt.NewClientOptions().AddBroker("tcp://test.mosquitto.org:1883") // Seems this is down at the moment
	opts := mqtt.NewClientOptions().AddBroker(mqtthost)
	opts.SetClientID("golang-tweet2mqtt") // Random client id
	opts.SetPingTimeout(10 * time.Second)
	opts.SetKeepAlive(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(10 * time.Second)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		fmt.Printf("!!!!!! mqtt connection lost error: %s\n" + err.Error())
	})

	mqttclient := mqtt.NewClient(opts)
	if token := mqttclient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error().Error())
	}

	for {
		// Forever loop
		fmt.Println("Last Tweet ID = " + strconv.FormatInt(lastTweetId, 10))
		client := &http.Client{}
		req, _ := http.NewRequest("GET", "https://api.twitter.com/2/tweets/search/recent", nil)

		req.Header.Add("Authorization", "Bearer "+apikey)

		q := req.URL.Query()
		q.Add("query", "from:infoBMKG peringatan jabodetabek")
		if lastTweetId > 0 {
			q.Add("since_id", strconv.FormatInt(lastTweetId, 10))
		}
		req.URL.RawQuery = q.Encode()

		resp, err := client.Do(req)

		if err == nil {

			defer resp.Body.Close()

			resp_body, _ := ioutil.ReadAll(resp.Body)

			if lastTweetId > 0 {
				fmt.Println(string(resp_body))
			}

			var result map[string]interface{}
			json.Unmarshal(resp_body, &result)

			resultCount := int(result["meta"].(map[string]interface{})["result_count"].(float64))
			fmt.Println("result count = " + string(resultCount))
			if resultCount > 0 {
				sinceId, _ := strconv.ParseInt(result["meta"].(map[string]interface{})["newest_id"].(string), 10, 64)
				// sinceId, _ := strconv.Atoi(result["meta"].(map[string]interface{})["newest_id"].(string))
				fmt.Println("Raw Since ID = " + result["meta"].(map[string]interface{})["newest_id"].(string))
				// fmt.Println("Since ID = " + strconv.Itoa(sinceId))
				lastTweetId = int64(sinceId)

				data1 := result["data"].([]interface{})[0]
				datatext := data1.(map[string]interface{})["text"].(string)
				fmt.Println("DATA text = " + datatext)

				// Parse URL
				rx := xurls.Relaxed
				urlBmkgAlert := rx.FindString(datatext)
				fmt.Println("GET URL = " + urlBmkgAlert)
				parseBmkgAlert(urlBmkgAlert, mqttclient)

				// Get URL
			} else {
				fmt.Println("No new entry")
			}

			// fmt.Println("ID : " + )
		} else {
			fmt.Println(err)
		}

		fmt.Println("Sleeping ...")
		time.Sleep(5 * time.Second)

	}

}

func parseBmkgAlert(url string, mqttclient mqtt.Client) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)

	if err == nil {

		defer resp.Body.Close()
		// resp_body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("Http status = " + resp.Status)
		// fmt.Println(string(resp_body))

		// Load the HTML document
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err == nil {
			doc.Find("meta").Each(func(i int, s *goquery.Selection) {
				if name, _ := s.Attr("name"); name == "description" {
					description, _ := s.Attr("content")
					fmt.Printf("Description field: %s\n", description)

					mqttclient.Publish(brokername, 0, false, description)
				}
			})

		}

	}
}
