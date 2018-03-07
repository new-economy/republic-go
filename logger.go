package node

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type Logger struct {
	Plugins []Plugin
}

// NewLogger returns a new Logger that will start and stop a set of plugins.
func NewLogger(plugins ...Plugin) *Logger {
	return &Logger{
		Plugins: plugins,
	}
}

// Start starts all the plugins of the logger
func (logger *Logger) Start() error {
	for _, plugin := range logger.Plugins {
		err := plugin.Start()
		if err != nil {
			return err
		}
	}
	return nil
}

// Stop stops all the plugins of the logger
func (logger Logger) Stop() error {
	for _, plugin := range logger.Plugins {
		plugin.Stop()
	}
	return nil
}

// Error outputs the error though each plugin
func (logger Logger) Error(err error) {
	for _, plugin := range logger.Plugins {
		plugin.Error(err)
	}
}

// Info outputs the info though each plugin
func (logger Logger) Info(info string) {
	for _, plugin := range logger.Plugins {
		plugin.Info(info)
	}
}

// Warning outputs the warning though each plugin
func (logger Logger) Warning(warning string) {
	for _, plugin := range logger.Plugins {
		plugin.Warning(warning)
	}
}

type Request struct {
	Type string      `json:"type"`
	Data RequestData `json:"data"`
}

type RequestData struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Interval int       `json:"interval"`
}

type Usage struct {
	Type string    `json:"type"`
	Time time.Time `json:"timestamp"`
	Data UsageData `json:"data"`
}

type UsageData struct {
	Cpu     float32 `json:"cpu"`
	Memory  int     `json:"memory"`
	network int     `json:"network"`
}

type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"timestamp"`
	Data EventData `json:"data"`
}

type EventData struct {
	Tag     string `json:"tag"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Plugin
type Plugin interface {
	Start() error
	Stop() error

	Info(info string)
	Warning(warning string)
	Error(err error)
}

// A FilePlugin implements the Plugin interface by logging all events to an
// output file.
type FilePlugin struct {
	Path string
	File *os.File
}

func NewFilePlugin(path string) Plugin {
	return &FilePlugin{
		Path: path,
	}
}

func (plugin *FilePlugin) Start() error {
	var err error
	plugin.File, err = os.OpenFile(plugin.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	 _, err = plugin.File.WriteString(time.Now().Format("2006/01/02 15:04:05"))
	if err != nil {
		log.Println(123)
		panic(err)
	}
	return err
}

func (plugin *FilePlugin) Stop() error {
	return plugin.File.Close()
}

func (plugin *FilePlugin) Info(info string) {
	if plugin.File == nil {
		log.Println("file is nil:", info)
		return
	}
	log.Println("file is not nil")
	_, err := plugin.File.WriteString(time.Now().Format("2006/01/02 15:04:05 "))
	if err != nil {
	}
	_, err = plugin.File.WriteString("INFO : ")
	if err != nil {
		panic(err)
	}
	_, err = plugin.File.WriteString(info + "\n")
	if err != nil {
		panic(err)
	}
}

func (plugin *FilePlugin) Warning(warning string) {
	plugin.File.Write([]byte(time.Now().Format("2006/01/02 15:04:05 ")))
	plugin.File.Write([]byte("WARNING : "))
	plugin.File.Write([]byte(warning + "\n"))
}

func (plugin *FilePlugin) Error(err error) {
	plugin.File.Write([]byte(time.Now().Format("2006/01/02 15:04:05 ")))
	plugin.File.Write([]byte("ERROR : "))
	plugin.File.Write([]byte(err.Error() + "\n"))
}

type WebSocketPlugin struct {
	Srv        *http.Server
	Connection *websocket.Conn
	Host       string
	Port       string
	Username   string
	Password   string
}

func NewWebSocketPlugin(host, port, username, password string) Plugin {
	plugin := WebSocketPlugin{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	}
	return plugin
}

func (plugin WebSocketPlugin) logHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	upgrader := websocket.Upgrader{}
	plugin.Connection, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		plugin.Error(err)
		return
	}

	defer plugin.Connection.Close()
	for {
		request := new(Request)
		err := plugin.Connection.ReadJSON(request)
		if err != nil {
			plugin.Error(err)
			return
		}

		switch request.Type{
		case  "usage":
			err = plugin.Connection.WriteJSON(request) // todo
		case "event":
			err = plugin.Connection.WriteJSON(request) // todo
		}

		if err != nil {
			plugin.Error(err)
			return
		}
	}
}

func (plugin WebSocketPlugin) Start() error {
	plugin.Srv = &http.Server{
		Addr: ":8080",
	}
	http.HandleFunc("/logs", plugin.logHandler)
	go func() {
		plugin.Info(fmt.Sprintf("WebSocket logger listening on %s:%s", plugin.Host, plugin.Port))
		plugin.Srv.ListenAndServe()
	}()

	return nil
}

func (plugin WebSocketPlugin) Stop() error {
	return plugin.Srv.Shutdown(nil)
}

type Message struct {
	Time    string
	Type    string
	Message string
}

func (plugin WebSocketPlugin) Info(info string) {
	if plugin.Connection == nil {
		log.Println("nil websocket infor ")
		return
	}

	err := plugin.Connection.WriteJSON(Message{
		Time:    time.Now().Format("2006/01/02 15:04:05"),
		Type:    "INFO",
		Message: info,
	})
	if err != nil{
		log.Print("websocket  error: ",err)
	}
}

func (plugin WebSocketPlugin) Error(err error) {
	if plugin.Connection == nil {
		return
	}
	e:= plugin.Connection.WriteJSON(Message{
		Time:    time.Now().Format("2006/01/02 15:04:05"),
		Type:    "ERROR",
		Message: err.Error(),
	})
	if e != nil{
		log.Print("websocket  error: ",err)
	}
}

func (plugin WebSocketPlugin) Warning(warning string) {
	if plugin.Connection == nil {
		return
	}

	err := plugin.Connection.WriteJSON(Message{
		Time:    time.Now().Format("2006/01/02 15:04:05"),
		Type:    "warning",
		Message: warning,
	})
	if err != nil{
		log.Print("websocket  error: ",err)
	}
}
