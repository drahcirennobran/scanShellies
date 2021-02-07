package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type mqttStruct struct {
	Enable bool   `enable`
	Id     string `json:"id"`
}

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
	Name string     `json:"name"`
	Mqtt mqttStruct `json:"mqtt"`
}

func main() {
	login := flag.String("login", "", "shelly login")
	password := flag.String("password", "", "shelly password")
	upgrade := flag.Bool("upgrade", false, "true to launch upgrade")
	network := flag.String("network", "192.168.0.*", "C class network to scan, 3 bytes only, * mandatory for last byte. example \"192.168.0.*\"")

	flag.Parse()
	networkIP := strings.Split(*network, ".")
	if len(networkIP) != 4 || networkIP[3] != "*" {
		log.Fatal("ip bad format, enter something like -network=\"192.168.0.*\"")
	}

	a, err := strconv.Atoi(networkIP[0])
	if err != nil {
		log.Fatal("ip bad format, enter something like -network=\"192.168.0.*\"")
	}
	b, err := strconv.Atoi(networkIP[1])
	if err != nil {
		log.Fatal("ip bad format, enter something like -network=\"192.168.0.*\"")
	}
	c, err := strconv.Atoi(networkIP[2])
	if err != nil {
		log.Fatal("ip bad format, enter something like -network=\"192.168.0.*\"")
	}

	tr := &http.Transport{
		MaxIdleConns:    256,
		IdleConnTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 1 * time.Second,
			DualStack: true}).DialContext,
	}
	client := &http.Client{Transport: tr}

	shellyChan := make(chan shelly, 256)
	fmt.Println("Scanning network...")
	for i := 2; i < 255; i++ {
		ip := net.IPv4(byte(a), byte(b), byte(c), byte(i))
		go scanShelly(ip, client, *login, *password, shellyChan)
	}
	time.Sleep(7 * time.Second)
	close(shellyChan)
	fmt.Printf("found %v shellies\n", len(shellyChan))

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
	fmt.Println("ip               mqtt.id                         mqtt enable  RSSI  status            HTTP  name")

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
		mqttEnable := "false        "
		if s.shellySettings.Mqtt.Enable == true {
			mqttEnable = "true         "
		}
		fmt.Printf("%v%v%v%v%v%v%v\n",
			rpad(fmt.Sprintf("%v", s.ip), " ", 17),
			rpad(s.shellySettings.Mqtt.Id, " ", 32),
			mqttEnable,
			rpad(fmt.Sprintf("%v", s.shellyStatus.WifiSta.RSSI), " ", 6),
			rpad(hasUpdateText, " ", 18),
			rpad(fmt.Sprintf("%v", s.shellyStatus.respHttpStatus), " ", 6),
			rpad(s.shellySettings.Name, " ", 30))
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
	myShellySettings = &shellySettings{"unknow", mqttStruct{false, "unknow"}}
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

func rpad(s string, pad string, plength int) string {
	for i := len(s); i < plength; i++ {
		s = s + pad
	}
	return s
}
