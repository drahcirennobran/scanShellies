package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

type shelly struct {
	shellyStatus   *shellyStatus
	shellySettings *shellySettings
}

type wifiStaStruct struct {
	Ip   string `json:"ip"`
	RSSI int    `json:"RSSI"`
}

type shellyStatus struct {
	HasUpdate bool          `json:"has_update"`
	WifiSta   wifiStaStruct `json:"wifi_sta"`
	Mac       string        `json:"mac`
}

type shellySettings struct {
	Name string `json:"name"`
}

func main() {
	tr := &http.Transport{
		MaxIdleConns:    256,
		IdleConnTimeout: 5 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   1 * time.Second,
			KeepAlive: 1 * time.Second,
			DualStack: true}).DialContext,
	}
	client := &http.Client{Transport: tr}
	shellyChan := make(chan shelly, 256)
	fmt.Println("Scanning network...")
	for i := 2; i < 255; i++ {
		go scanShelly(client, fmt.Sprintf("http://192.168.0.%v", i), shellyChan)
	}
	time.Sleep(5 * time.Second)
	close(shellyChan)
	showAndUpdateShellies(client, shellyChan)
}
func scanShelly(client *http.Client, url string, shellyChan chan shelly) {
	shellyStatus := requestShellyStatus(client, url)
	shellySettings := requestShellySettings(client, url)
	if shellyStatus != nil && shellySettings != nil {
		shellyChan <- shelly{shellyStatus, shellySettings}
	}
}
func showAndUpdateShellies(client *http.Client, c chan shelly) {
	for s := range c {
		hasUpdateText := "Up to date"
		if s.shellyStatus.HasUpdate {
			hasUpdateText = "running update"
			client.Get(fmt.Sprintf("http://%v/ota?update=1", s.shellyStatus.WifiSta.Ip))
		}
		fmt.Printf("%v, %v, %v , %v : %v\n",
			s.shellyStatus.WifiSta.Ip,
			s.shellyStatus.Mac,
			s.shellyStatus.WifiSta.RSSI,
			s.shellySettings.Name,
			hasUpdateText)
	}
}

func requestShellyStatus(client *http.Client, url string) (shellyStatus *shellyStatus) {
	resp, err := client.Get(url + "/status")

	if err != nil {
		return nil
	}
	if resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal([]byte(bodyBytes), &shellyStatus); err != nil {
		return nil
	}
	return shellyStatus
}

func requestShellySettings(client *http.Client, url string) (shellySettings *shellySettings) {
	resp, err := client.Get(url + "/settings")

	if err != nil {
		return nil
	}
	if resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal([]byte(bodyBytes), &shellySettings); err != nil {
		return nil
	}
	return shellySettings
}
