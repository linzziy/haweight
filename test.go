package main

import (
	"os"
	"text/template"
)

const haproxyTemplate = `
global
    log /dev/log local0
    maxconn {{.MaxConn}}

defaults
    mode http
    timeout connect {{.TimeoutConnect}}ms
    timeout client {{.TimeoutClient}}ms
    timeout server {{.TimeoutServer}}ms

frontend http-in
    bind *:{{.FrontendPort}}
    default_backend servers

backend servers
{{range .Servers}}
    server {{.Name}} {{.Address}} check
{{end}}
`

type Server struct {
	Name    string
	Address string
}

type Config struct {
	MaxConn        int
	TimeoutConnect int
	TimeoutClient  int
	TimeoutServer  int
	FrontendPort   int
	Servers        []Server
}

func main() {
	cfg := Config{
		MaxConn:        4096,
		TimeoutConnect: 5000,
		TimeoutClient:  50000,
		TimeoutServer:  50000,
		FrontendPort:   80,
		Servers: []Server{
			{"server1", "192.168.1.1:80"},
			{"server2", "192.168.1.2:80"},
		},
	}

	tmpl := template.Must(template.New("haproxy").Parse(haproxyTemplate))
	tmpl.Execute(os.Stdout, cfg)
}
