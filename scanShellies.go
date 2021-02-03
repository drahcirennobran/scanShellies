package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type shelly struct {
	ip             net.IP
	shellyStatus   *shellyStatus
	shellySettings *shellySettings
}

type wifiStaStruct struct {
	Ip   string `json:"ip"`
	RSSI int    `json:"RSSI"`
}

type shellyStatus struct {
	respHttpStatus int
	HasUpdate      bool          `json:"has_update"`
	WifiSta        wifiStaStruct `json:"wifi_sta"`
	Mac            string        `json:"mac`
}

type shellySettings struct {
	Name string `json:"name"`
}

func main() {
	login := flag.String("login", "", "login")
	password := flag.String("password", "", "password")
	upgrade := flag.Bool("upgrade", false, "launch upgrade")
	flag.Parse()
	fmt.Println(*login)
	fmt.Println(*password)
	fmt.Println(*upgrade)

	tr := &http.Transport{
		MaxIdleConns:    256,
		IdleConnTimeout: 10 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
			DualStack: true}).DialContext,
	}
	client := &http.Client{Transport: tr}

	shellyChan := make(chan shelly, 256)
	fmt.Println("Scanning network...")
	for i := 2; i < 255; i++ {
		go scanShelly(net.IPv4(192, 168, 0, byte(i)), client, *login, *password, shellyChan)
	}
	time.Sleep(5 * time.Second)
	close(shellyChan)
	showAndUpdateShellies(client, getSortedChan(shellyChan), *upgrade)
}

func scanShelly(ip net.IP, client *http.Client, login string, password string, shellyChan chan shelly) {
	shellyStatus, err := requestShellyStatus(client, ip, login, password)
	if err != nil {
		return
	}
	shellySettings := requestShellySettings(client, ip, login, password)
	if shellyStatus != nil {
		shellyChan <- shelly{ip, shellyStatus, shellySettings}
	}
}

func showAndUpdateShellies(client *http.Client, c chan shelly, update bool) {
	fmt.Println("ip        \tMac            \tRSSI\tstatus   \tHTTP\tname")

	for s := range c {
		hasUpdateText := "Up to date"
		if s.shellyStatus.HasUpdate {
			if update == true {
				hasUpdateText = "running update"
				client.Get(fmt.Sprintf("http://%v/ota?update=1", s.shellyStatus.WifiSta.Ip))
			} else {
				hasUpdateText = "update available"
			}
		}
		fmt.Printf("%v\t%v\t%v\t%v\t%v\t%v\n",
			s.ip,
			s.shellyStatus.Mac,
			s.shellyStatus.WifiSta.RSSI,
			hasUpdateText,
			s.shellyStatus.respHttpStatus,
			s.shellySettings.Name)
	}
}

func requestShellyStatus(client *http.Client, ip net.IP, login string, password string) (myShellyStatus *shellyStatus, err error) {
	myShellyStatus = &shellyStatus{0, false, wifiStaStruct{"", 0}, "unknow      "}
	req, err := http.NewRequest("GET", "http://"+ip.String()+"/status", nil)
	if err != nil {
		return
	}

	req.SetBasicAuth(login, password)
	resp, err := client.Do(req)

	if err != nil {
		return
	}

	myShellyStatus.respHttpStatus = resp.StatusCode
	fmt.Printf("%v : %v\n", ip, resp.StatusCode)
	if resp.StatusCode == 401 {
		return
	}
	if resp.StatusCode != 200 {
		err = errors.New(resp.Status)
		return
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)

	err = json.Unmarshal([]byte(bodyBytes), &myShellyStatus)
	return
}

func requestShellySettings(client *http.Client, ip net.IP, login string, password string) (myShellySettings *shellySettings) {
	myShellySettings = &shellySettings{"unknow"}
	req, err := http.NewRequest("GET", "http://"+ip.String()+"/settings", nil)
	if err != nil {
		return
	}
	req.SetBasicAuth(login, password)
	resp, err := client.Do(req)

	if err != nil {
		return
	}

	if resp.StatusCode == 401 {
		return
	}
	if resp.StatusCode != 200 {
		err = errors.New(resp.Status)
		return
	}
	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)

	if err := json.Unmarshal([]byte(bodyBytes), &myShellySettings); err != nil {
		return
	}
	return
}

func getSortedChan(c chan shelly) chan shelly {
	m := make(map[int]shelly)
	result := make(chan shelly, len(c))

	keys := make([]int, 0, len(c))
	for item := range c {
		d, _ := strconv.Atoi(strings.Split(item.ip.String(), ".")[3])

		keys = append(keys, d)
		m[d] = item
	}
	sort.Ints(keys)
	for _, key := range keys {
		result <- m[key]
	}
	close(result)

	return result
}
