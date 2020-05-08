package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/chennqqi/goutils/yamlconfig"
	"github.com/immortal/logrotate"
	"github.com/sirupsen/logrus"
)

func main() {
	var conf string
	flag.StringVar(&conf, "conf", "litedns.yml", "set configure path")
	flag.Parse()

	var cfg Config
	err := yamlconfig.Load(&cfg, conf)
	if os.IsNotExist(err) {
		logrus.Errorf("[main.go::main] yamlconfig.Load (%v) not exist, make default", cfg)
		yamlconfig.Save(cfg, conf)
		return
	} else if err != nil {
		logrus.Errorf("[main.go::main] yamlconfig.Load error: %v", err)
		return
	}

	switch strings.ToUpper(cfg.LogLevel) {
	case "ERROR":
		logrus.SetLevel(logrus.ErrorLevel)
	case "WARN", "WARNING":
		logrus.SetLevel(logrus.WarnLevel)
	case "DEBUG", "DBG":
		logrus.SetLevel(logrus.DebugLevel)
	default:
		fallthrough
	case "INFO":
		logrus.SetLevel(logrus.InfoLevel)
	}
	logrus.Println("CFG:", cfg)

	logfile, err := logrotate.New(cfg.LogFile, 86400, 7, 0, false)
	if cfg.LogFile != strings.ToLower("console") && err == nil {
		logrus.SetOutput(logfile)
	}
	app, err := NewApp(&cfg)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		app.Run()
	}()

	var w *Web
	if cfg.Web.Enable {
		w = NewWeb(&cfg.Web, app)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if w != nil {
			w.Run()
		}
	}()

	quitCh := make(chan os.Signal)
	signal.Notify(quitCh, os.Interrupt, os.Kill)
	<-quitCh

	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
	app.Shutdown(ctx)
	if w != nil {
		w.Shutdown(ctx)
	}

	if logfile != nil {
		logfile.Close()
	}
}
