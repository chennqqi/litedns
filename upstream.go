package main

import (
	"errors"
	"net/url"
	"time"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

//update by main.conf, not support update
//a dns forward or proxy. simpledns will auto try every connection you set
type UpstreamServer struct {
	targets     []string
	index       int
	cacheExpire time.Duration
	app         *App
}

func NewUpstream(conf *UpstreamConf, app *App) (*UpstreamServer, error) {
	var up UpstreamServer

	for i := 0; i < len(conf.Upstreams); i++ {
		u, err := url.Parse(conf.Upstreams[i])
		if err != nil {
			logrus.Warnf("[server.go::UpstreamServer.NewUpstreamServer] parse upstream(%v), error: %v", conf.Upstreams[i], err)
			continue
		}
		switch u.Scheme {
		case "udp":
			up.targets = append(up.targets, u.Host)
		default:
			logrus.Warnf("[server.go::UpstreamServer.NewUpstreamServer] parse upstream(%v), not support ignore", conf.Upstreams[i])
		}
	}
	if len(conf.Upstreams) == 0 {
		return nil, errors.New("no valid upstream server")
	}
	up.cacheExpire = time.Duration(conf.Expire)
	up.app = app
	return &up, nil
}

func (s *UpstreamServer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	app := s.app
	if len(r.Question) >= 1 {
		//only response the first question
		for i := 0; i < 1; i++ {
			class := r.Question[i].Qclass
			name := r.Question[i].Name
			t := r.Question[i].Qtype
			if class == dns.ClassINET {
				switch t {
				case dns.TypeA:
					v, err := app.Get(name)
					if err == nil {
						m := new(dns.Msg)
						m.SetReply(r)
						m.Used(v.ToA())
						w.WriteMsg(m)
						return
					}
				case dns.TypeAAAA:
					v, err := app.Get(name)
					if err == nil {
						m := new(dns.Msg)
						m.SetReply(r)
						m.Used(v.ToAAAA())
						w.WriteMsg(m)
						return
					}
				}
			}
		}
	}

	//retry max
	index := s.index
	for retry := 0; retry < len(s.targets); retry++ {
		addr := s.targets[index]
		resp, err := dns.Exchange(r, addr)
		if err != nil {
			logrus.Errorf("[server.go:ForwardServer.handleRequest] ExchangeConn error: %v", err)
			s.index = (index + 1) % len(s.targets)
		} else {
			if len(r.Question) >= 1 {
				for i := 0; i < 1; i++ {
					q := &r.Question[i]
					//only cache class_INET and TypeA
					logrus.Println("QUERY:", i, q.String())
					if q.Qclass == dns.ClassINET && q.Qtype == dns.TypeA {
						logrus.Println("SET RR:", i, q.Name)
						app.SetRR(q.Name, resp.Answer, s.cacheExpire)
					}
				}
			}
			w.WriteMsg(resp)
			return
		}
	}

	dns.HandleFailed(w, r)
}
