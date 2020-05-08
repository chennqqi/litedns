package main

import (
	"github.com/chennqqi/goutils/utime"
)

type UpstreamConf struct {
	Name      string         `json:"name" yaml:"name"`
	Expire    utime.Duration `json:"expire" yaml:"expire"`
	Upstreams []string       `json:"upstreams" yaml:"upstreams"`
}

type WebConf struct {
	Enable   bool   `json:"enable" yaml:"enable"`
	Addr     string `json:"addr" yaml:"addr"`
	Token    string `json:"token" yaml:"token"`
	Readonly bool   `json:"readonly" yaml:"readonly"`
}

type RedisConf struct {
	Enable   bool     `json:"enable" yaml:"enable"`
	Addrs    []string `json:"addrs" json:"addrs"`
	Master   string   `json:"master" yaml:"master"`
	Password string   `json:"password" yaml:"password"`
}

type Config struct {
	LogFile  string `json:"log_file" yaml:"log_file"`
	LogLevel string `json:"log_level" yaml:"log_level"`

	Forwards []UpstreamConf `json:"forwards" yaml:"forwards"`
	Addr     string         `json:"addr" yaml:"addr" default:""`
	Expire   utime.Duration `json:"expire" json:"expire"`

	//option
	Redis RedisConf `json:"redis" yaml:"redis"`
	Web   WebConf   `json:"web" yaml:"web"`
}

func ReadTxt(file string) ([]byte, error) {
	return nil, nil
}
