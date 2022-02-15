package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	dac "github.com/xinsnake/go-http-digest-auth-client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var f string

type Config struct {
	Game  GameInfo  `toml:"game"`
	Altas AltasInfo `toml:"altas"`
}

type GameInfo struct {
	Name string `toml:"name"`
	Dba  string `toml:"dba"`
}

type AltasInfo struct {
	Projectid string `toml:"projectid"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
}

type OpenFireInfo struct {
	Links      interface{}      `json:"links"`
	Results    []OpenFireResult `json:"results"`
	TotalCount int              `json:"totalCount"`
}

type CurrentValue struct {
	Number float64 `json:"number"`
	Units  string  `json:"utits"`
}

type OpenFireResult struct {
	Id              string       `json:"id"`
	GroupId         string       `json:"groupId"`
	AlertConfigId   string       `json:"alertConfigId"`
	HostId          string       `json:"hostId"`
	HostnameAndPort string       `json:"hostnameAndPort"`
	EventTypeName   string       `json:"eventTypeName"`
	Status          string       `json:"status"`
	Created         time.Time    `json:"created"`
	Updated         time.Time    `json:"updated"`
	LastNotified    time.Time    `json:"lastNotified"`
	MetricName      string       `json:"metricName"`
	Cv              CurrentValue `json:"currentValue"`
}

type Message struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

var ZapLog_V1 *zap.Logger

const wecomURL = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=4bfa0ecd-e354-4636-98d0-6301991ec641"

func init() {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "time",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "caller",
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		//EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
	}

	atom := zap.NewAtomicLevelAt(zap.InfoLevel)
	config := zap.Config{
		Level:       atom,
		Development: true,
		//Encoding:         "json",
		Encoding:         "console",
		EncoderConfig:    encoderConfig,
		InitialFields:    map[string]interface{}{"serviceName": "wisdom_park"},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}
	config.EncoderConfig.EncodeLevel = zapcore.LowercaseColorLevelEncoder

	var err error
	ZapLog_V1, err = config.Build()
	if err != nil {
		panic(fmt.Sprintf("log 初始化失败: %v", err))
	}

}

func main() {
	flag.StringVar(&f, "f", "/", "config file path")
	flag.Parse()
	t := ParseToml(f)
	username := t.Altas.Username
	password := t.Altas.Password
	projectid := t.Altas.Projectid

	altasURL := fmt.Sprintf("https://cloud.mongodb.com/api/atlas/v1.0/groups/%v/alerts?status=OPEN", projectid)

	for {
		alarms := GetAlarms(username, password, altasURL)
		if alarms != nil {
			for _, v := range alarms {
				go func(i string) {
					SendMessage(i)
				}(v)
			}
		}
		time.Sleep(300 * time.Second)
	}

	// quit := make(chan os.Signal)
	// signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	// <-quit
}

func GetAlarms(user string, password string, url string) (msg []string) {
	t := dac.NewTransport(user, password)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		ZapLog_V1.Error("fial to parse url", zap.Error(err))
	}
	resp, err := t.RoundTrip(req)
	if err != nil {
		ZapLog_V1.Error("invalid response information", zap.Error(err))
	}

	defer resp.Body.Close()

	body, reader := ioutil.ReadAll(resp.Body)
	if reader != nil {
		ZapLog_V1.Error("parser http body  error ", zap.Error(err))
	}
	response := OpenFireInfo{}

	if err := json.Unmarshal(body, &response); err != nil {
		ZapLog_V1.Error("Unmarshal http body  error ", zap.Error(err))
	}

	if response.TotalCount == 0 {
		ZapLog_V1.Info("There is no alerts")
		return nil
	}
	l := response.Results

	for _, key := range l {

		infos := `
Region: GCP / Montreal (northamerica-northeast1)
Cloud : Altas 
GameName : Streetfighter
MetricName: %v
EventTypeName :%v
AlterStatus: %v
Value : %v
DBA ： leoyhou@tencent.com		`

		info := fmt.Sprintf(infos, key.MetricName, key.Status, key.EventTypeName, key.Cv.Number)
		msg = append(msg, info)
	}
	return msg
}

func SendMessage(msg string) {
	var m Message
	m.MsgType = "text"
	m.Text.Content = msg
	jsons, err := json.Marshal(m)
	if err != nil {
		ZapLog_V1.Error("SendMessage Marshal failed", zap.Error(err))
		return
	}
	resp := string(jsons)
	client := &http.Client{}
	req, err := http.NewRequest("POST", wecomURL, strings.NewReader(resp))
	if err != nil {
		ZapLog_V1.Error("SendMessage http NewRequest failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := client.Do(req)
	if err != nil {
		ZapLog_V1.Error("SendMessage client Do failed", zap.Error(err))
		return
	}
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		ZapLog_V1.Error("SendMessage ReadAll Body failed", zap.Error(err))
		return
	}
	ZapLog_V1.Info("SendMessage success", zap.String("body", string(body)))
}

func ParseToml(filePath string) *Config {
	var t Config

	if _, err := toml.DecodeFile(filePath, &t); err != nil {
		panic(err)
	}

	return &t
}
