package logger

import (
	"fmt"

	"io"

	"log"

	"net/http"

	"os"

	"time"

	"github.com/gorilla/websocket"
)

// Plugin
type Plugin interface {
	Start() error
	Stop() error

	Info(string)
	Warning(string)
	Error(string)
}

// A FilePlugin implements the Plugin interface by logging all events to an
// output file.
type FilePlugin struct {
	Path string
	File *os.File
}

func NewFilePlugin(path string) (Plugin, error) {
	return &Plugin {
		Path: path
	}
}

func (plugin *FilePlugin) Start() error {
	plugin.File, err := os.OpenFile(plugin.Path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	return nil
}

func (plugin *FilePlugin) Stop() error {
	return plugin.File.Close()
}

type WebSocketPlugin struct {
	Host string
	Port string
	Username string
	Password string
	Handler func(w http.ResponseWriter, r *http.Request)
}

func NewWebSocketPlugin(host, port, username, password string) (Plugin) {
	return &WebSocketPlugin {
		Host: host,
		Port: port,
		Username: username,
		Password: password,
		Handler: func(w http.ResponseWriter, r *http.Request) {

		}
	}
}

func (plugin *WebSocketPlugin) Start() error {
	http.HandleFunc("/logs", plugin.Handler)
	go func() {
		plugin.Info(fmt.Sprintf("WebSocket logger listening on %s:%s", address, port))
		http.ListenAndServe(fmt.Sprintf("%s:%s", plugin.Host, plugin.Port), nil)
	}()
	return nil
}

func (plugin *WebSocketPlugin) Stop() error {
	return nil
}

type Logger struct {
	Plugins []*Plugin
}

// NewLogger returns a new Logger that will start and stop a set of plugins.
func NewLogger(plugins ...*Plugin) Logger {
	return Logger{
		Plugins: plugins,
	}
}

func (logger Logger) Start() error {
	for _, plugin := range logger.Plugins {
		if err := plugin.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (logger Logger) Stop() error {
	panic("unimplemented")
}

func (logger Logger) logHandler(w http.ResponseWriter, r *http.Request) {

	// Parse query parameters

	//requestType := r.URL.Query()["type"]

	//if len(requestType) == 1 {

	//

	//}

	upgrader := websocket.Upgrader{}

	c, err := upgrader.Upgrade(w, r, nil)

	if err != nil {

		logger.Error(err)

		return

	}

	defer c.Close()

	for {

		// todo : handle request

		request := new(Request)

		err := c.ReadJSON(request)

		if err != nil {

			logger.Error(err)

			return

		}

		log.Println(request)

		err = c.WriteJSON(request)

		if err != nil {

			logger.Error(err)

			return

		}

	}

}

func (logger Logger) Error(err error) {

	for _, plugin := range logger.Plugins {

		plugin.Write([]byte(time.Now().Format("2006/01/02 15:04:05 ")))

		plugin.Write([]byte("ERROR : "))

		plugin.Write([]byte(err.Error() + "\n"))

	}

}

func (logger Logger) Info(info string) {

	for _, plugin := range logger.Plugins {

		plugin.Write([]byte(time.Now().Format("2006/01/02 15:04:05 ")))

		plugin.Write([]byte("INFO : "))

		plugin.Write([]byte(info + "\n"))

	}

}

func (logger Logger) Debug(debug string) {

	for _, plugin := range logger.Plugins {

		plugin.Write([]byte(time.Now().Format("2006/01/02 15:04:05 ")))

		plugin.Write([]byte("DEBUG : "))

		plugin.Write([]byte(debug + "\n"))

	}

}

type Request struct {
	Type string `json:"type"`

	Data RequestData `json:"data"`
}

type RequestData struct {
	Start time.Time `json:"start"`

	End time.Time `json:"end"`

	Interval int `json:"interval"`
}

type Usage struct {
	Type string `json:"type"`

	Time time.Time `json:"timestamp"`

	Data UsageData `json:"data"`
}

type UsageData struct {
	Cpu float32 `json:"cpu"`

	Memory int `json:"memory"`

	network int `json:"network"`
}

type Event struct {
	Type string `json:"type"`

	Time time.Time `json:"timestamp"`

	Data EventData `json:"data"`
}

type EventData struct {
	Tag string `json:"tag"`

	Level string `json:"level"`

	Message string `json:"message"`
}
