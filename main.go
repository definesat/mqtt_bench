package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gosuri/uilive"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/paulbellamy/ratecounter"
)

type deviceInfo struct {
	Oui   string `json:"oui,omitempty"`
	Model string `json:"model,omitempty"`
	Brand string `json:"brand,omitempty"`
	Image string `json:"image,omitempty"`
}

type deviceStatus struct {
	Volume             int    `json:"volume,omitempty"`
	Mute               bool   `json:"mute,omitempty"`
	RunTime            string `json:"run_time,omitempty"`
	Language           string `json:"language,omitempty"`
	CecAutoPowerOff    bool   `json:"cec_auto_power_off,omitempty"`
	DeviceName         string `json:"device_name,omitempty"`
	PowerKeyDefinition string `json:"powerkey_definition,omitempty"`
}

type deviceAppControl struct {
	App     string `json:"package"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Enable  bool   `json:"enable"`
}

type MqttThing struct {
	Version  string             `json:"version"`
	Info     deviceInfo         `json:"info"`
	Property deviceStatus       `json:"property"`
	Apps     []deviceAppControl `json:"apps"`
}

type MqttStatics struct {
	start          time.Time
	clients        int32
	connected      int32
	lost_con       int32
	msg_up         int32
	msg_down       int32
	msg_success    int32
	msg_total      int32
	lag            int32
	client_rater   *ratecounter.AvgRateCounter
	msg_down_rater *ratecounter.AvgRateCounter
	msg_up_rater   *ratecounter.AvgRateCounter
}

type MqttConfig struct {
	host     string
	port     int
	total    int
	qps      int
	interval int
	mode     string
}

type MqttClient struct {
	config *MqttConfig
	sn     string
	chipid string
	oui    string
	model  string
	brand  string
	thing  MqttThing
}

var m_statics MqttStatics

func MessageHandler(client mqtt.Client, msg mqtt.Message) {
	atomic.AddInt32(&m_statics.msg_down, 1)
	atomic.AddInt32(&m_statics.msg_total, 1)
	m_statics.msg_down_rater.Incr(1)
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	atomic.AddInt32(&m_statics.connected, 1)
	m_statics.client_rater.Incr(1)

	SubscribedTopics(client)
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	atomic.AddInt32(&m_statics.connected, -1)
	atomic.AddInt32(&m_statics.lost_con, 1)
	reader := client.OptionsReader()
	fmt.Printf("%s lost conection : %s\n", reader.ClientID(), err.Error())
}

func SubscribedTopics(client mqtt.Client) {
	reader := client.OptionsReader()

	topic := "thing/" + reader.ClientID() + "/desired"
	token := client.Subscribe(topic, 0, MessageHandler)
	token.Wait()

	topic = "thing/" + reader.ClientID() + "/control"
	token = client.Subscribe(topic, 0, MessageHandler)
	token.Wait()
}

func (c *MqttClient) Ticker(config *MqttConfig, client mqtt.Client, wg *sync.WaitGroup) {
	start := time.Now()

	for {
		select {
		case <-time.After(60 * time.Second):
			c.UpdateData(config, client, start)
		}
	}
}

func (c *MqttClient) UpdateData(config *MqttConfig, client mqtt.Client, start time.Time) {
	atomic.AddInt32(&m_statics.msg_total, 1)
	atomic.AddInt32(&m_statics.msg_up, 1)

	text := c.thingString(client, start)
	topic := "thing/" + c.sn + "/data"
	token := client.Publish(topic, 0, false, text)
	go func() {
		token.Wait()

		if token.Error() != nil {
			fmt.Println(token.Error())
		} else {
			atomic.AddInt32(&m_statics.msg_success, 1)
			m_statics.msg_up_rater.Incr(1)
		}
	}()
}

func (c *MqttClient) thingString(client mqtt.Client, start time.Time) string {
	thing := &c.thing

	thing.Version = "1.0"
	thing.Info.Image = "v20220325.r2"

	thing.Property.Volume = 60
	thing.Property.Mute = false
	thing.Property.DeviceName = c.sn
	thing.Property.Language = "en-us"
	thing.Property.CecAutoPowerOff = false
	thing.Property.PowerKeyDefinition = "powerkey_suspend"

	thing.Property.RunTime = time.Since(start).Round(time.Second).String()
	thing.Apps = make([]deviceAppControl, 0)

	var appControl deviceAppControl
	appControl.App = "org.google.youtube"
	appControl.Name = "Youtube"
	appControl.Version = "2546.486"
	appControl.Enable = true
	thing.Apps = append(thing.Apps, appControl)

	appControl.App = "org.google.chrome"
	appControl.Name = "Chrome"
	appControl.Version = "77.458.455.002"
	appControl.Enable = false
	thing.Apps = append(thing.Apps, appControl)

	j, err := json.Marshal(thing)
	if err != nil {
		return err.Error()
	}

	return string(j)
}

func client(config *MqttConfig, sn string, chipid string, wg *sync.WaitGroup) {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("recover: %v", e)
		}
	}()

	atomic.AddInt32(&m_statics.clients, 1)
	defer atomic.AddInt32(&m_statics.clients, -1)
	defer wg.Done()

	mqttC := MqttClient{
		config: config,
		sn:     sn,
		chipid: chipid,
		brand:  "define",
		model:  "sf8008",
		oui:    "aoui",
	}

	opts := mqtt.NewClientOptions().AddBroker(config.host + ":" + fmt.Sprint(config.port))
	opts.SetClientID(sn)
	opts.SetUsername(chipid)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	defer client.Disconnect(250)
	mqttC.Ticker(config, client, wg)
}

func static() {
	writer := uilive.New()      // writer for the first line
	writer2 := writer.Newline() // writer for the second line
	writer3 := writer.Newline() // writer for the second line
	writer4 := writer.Newline() // writer for the second line
	writer5 := writer.Newline() // writer for the second line
	writer6 := writer.Newline() // writer for the second line

	// start listening for updates and render
	writer.Start()
	defer writer.Stop() // flush and stop rendering

	for {
		fmt.Fprintf(writer, "running : %s\n", time.Since(m_statics.start).Truncate(time.Millisecond).String())
		fmt.Fprintf(writer2, "\t total(%d)/connected(%d) , rate %d/s\n", m_statics.clients, m_statics.connected, m_statics.client_rater.Hits())
		fmt.Fprintf(writer3, "\t message total(%d) /success (%d) \n", m_statics.msg_total, m_statics.msg_success)
		fmt.Fprintf(writer4, "\t message up(%d) /rate %d \n", m_statics.msg_up, m_statics.msg_up_rater.Hits())
		fmt.Fprintf(writer5, "\t message down(%d) /rate %d \n", m_statics.msg_down, m_statics.msg_down_rater.Hits())
		fmt.Fprintf(writer6, "\t latency %dms \n", m_statics.lag)

		time.Sleep(time.Millisecond * 500)
	}
}

func bench(config *MqttConfig) {
	go static()

	wg := new(sync.WaitGroup)
	m_statics.start = time.Now()
	m_statics.client_rater = ratecounter.NewAvgRateCounter(1 * time.Second)
	m_statics.msg_up_rater = ratecounter.NewAvgRateCounter(1 * time.Second)
	m_statics.msg_down_rater = ratecounter.NewAvgRateCounter(1 * time.Second)

	for i := 0; i < config.total; i++ {
		var clientId = uuid.New().String()
		var chipid = uuid.New().String()

		wg.Add(1)
		go client(config, clientId, chipid, wg)
		time.Sleep(time.Millisecond * 5)
	}

	fmt.Printf("bench started of client: %d\n", config.total)
	wg.Wait()
}

func main() {
	var config MqttConfig

	var help = flag.Bool("help", false, "Show help")
	flag.StringVar(&config.host, "host", "localhost", "host address")
	flag.IntVar(&config.port, "port", 1883, "host port")
	flag.IntVar(&config.total, "t", 100, "total")
	flag.IntVar(&config.qps, "c", 1000, "connect speed per second")
	flag.IntVar(&config.interval, "i", 30, "interval in second")
	flag.StringVar(&config.mode, "mode", "normal", "host address")

	// Parse the flag
	flag.Parse()

	// Usage Demo
	if *help {
		flag.Usage()
		os.Exit(0)
	}

	bench(&config)
}
