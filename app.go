package main

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/chennqqi/goutils/utime"
	"github.com/go-redis/redis"
	"github.com/miekg/dns"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
)

const (
	LITEDNS_PREFIX = "litedns."
)

var ErrNotExist = redis.Nil

type Resolve struct {
	Name   string         `json:"name"`
	A      []string       `json:"A"`
	AAAA   []string       `json:"AAAA"`
	TTL    uint32         `json:"ttl"`
	Expire utime.Duration `json:"expire"`
}

func fixResolveName(s string) string {
	if strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}

func (t Resolve) ToA() []dns.RR {
	var rr []dns.RR
	for i := 0; i < len(t.A); i++ {
		a := new(dns.A)
		a.A = net.ParseIP(t.A[i])
		a.Hdr.Class = dns.ClassINET
		a.Hdr.Rrtype = dns.TypeA
		a.Hdr.Ttl = t.TTL
		a.Hdr.Name = t.Name
		rr = append(rr, a)
	}
	return rr
}

func (t Resolve) ToAAAA() []dns.RR {
	var rr []dns.RR
	for i := 0; i < len(t.AAAA); i++ {
		a := new(dns.AAAA)
		a.AAAA = net.ParseIP(t.AAAA[i])
		a.Hdr.Class = dns.ClassINET
		a.Hdr.Rrtype = dns.TypeAAAA
		a.Hdr.Ttl = t.TTL
		a.Hdr.Name = t.Name
		rr = append(rr, a)
	}
	return rr
}

func (t Resolve) ToAny() []dns.RR {
	var rr []dns.RR
	rr = append(rr, t.ToA()...)
	rr = append(rr, t.ToAAAA()...)
	return rr
}

func (t *Resolve) Fix() {
	t.Name = fixResolveName(t.Name)
}

func (t *Resolve) From(name string, rr []dns.RR, expire time.Duration) {
	t.A = make([]string, 0)
	t.AAAA = make([]string, 0)
	t.Name = name
	var first bool
	for i := 0; i < len(rr); i++ {
		r := rr[i]
		switch r.(type) {
		case *dns.A:
			pa := r.(*dns.A)
			t.A = append(t.A, pa.A.String())
			if !first {
				t.TTL = pa.Header().Ttl
				first = true
			}
		case *dns.AAAA:
			pa := r.(*dns.AAAA)
			t.AAAA = append(t.AAAA, pa.AAAA.String())
			if !first {
				t.TTL = pa.Header().Ttl
				first = true
			}
			// case *dns.CNAME:
			// 	pa := r.(*dns.CNAME)
			// 	t.AAAA = append(t.AAAA, pa.AAAA.String())
			// 	t.Name = pa.Header().Name
			// 	t.TTL = pa.Header().Ttl
			// 	first = true
		}
	}
	t.Expire = utime.Duration(expire)
	//default TTL
	t.TTL = 3600
	t.Fix()
}

func (t *Resolve) Update(name string, rr []dns.RR) bool {
	var v4Changed, V6Changed bool
	for i := 0; i < len(rr); i++ {
		r := rr[i]
		switch r.(type) {
		case *dns.A:
			pa := r.(*dns.A)
			var found bool
			for j := 0; j < len(t.A); j++ {
				if t.A[j] == pa.A.String() {
					found = true
					break
				}
			}
			if !found {
				v4Changed = true
			}
		case *dns.AAAA:
			pa := r.(*dns.AAAA)
			var found bool
			for j := 0; j < len(t.A); j++ {
				if t.AAAA[j] == pa.AAAA.String() {
					found = true
					break
				}
			}
			if !found {
				V6Changed = true
			}
		}
	}
	if v4Changed {
		t.A = make([]string, 0)
		for i := 0; i < len(rr); i++ {
			r := rr[i]
			switch r.(type) {
			case *dns.A:
				pa := r.(*dns.A)
				t.A = append(t.A, pa.A.String())
			}
		}
	}
	if V6Changed {
		t.AAAA = make([]string, 0)
		for i := 0; i < len(rr); i++ {
			r := rr[i]
			switch r.(type) {
			case *dns.AAAA:
				pa := r.(*dns.AAAA)
				t.AAAA = append(t.AAAA, pa.AAAA.String())
			}
		}
	}

	return v4Changed || V6Changed
}

type App struct {
	cache  *cache.Cache
	rds    redis.UniversalClient
	server *dns.Server
	mux    *dns.ServeMux
	//serversMap map[string]DomainNameServer
}

func NewApp(conf *Config) (*App, error) {
	var app App
	if conf.Redis.Enable {
		cfg := &conf.Redis
		rds := redis.NewUniversalClient(&redis.UniversalOptions{
			Addrs:      cfg.Addrs,
			MasterName: cfg.Master,
			Password:   cfg.Password,
		})
		_, err := rds.Ping().Result()
		if err != nil {
			logrus.Errorf("[app.go::NewApp] redis ping error: %v", err)
			return nil, err
		}
		app.rds = rds
	}
	expire := time.Duration(conf.Expire)
	cache := cache.New(expire, 2*expire)
	app.cache = cache
	app.server = &dns.Server{
		Addr:    conf.Addr,
		Handler: &app,
		Net:     "udp",
		// The net.Conn.SetReadTimeout value for new connections, defaults to 2 * time.Second.
		ReadTimeout: 5 * time.Second,
		// The net.Conn.SetWriteTimeout value for new connections, defaults to 2 * time.Second.
		WriteTimeout: 5 * time.Second,
	}

	ms := make(map[string]dns.Handler)
	mux := dns.NewServeMux()
	for i := 0; i < len(conf.Forwards); i++ {
		sv := &conf.Forwards[i]
		up, err := NewUpstream(sv, &app)
		//not dynamic update
		if err != nil {
			logrus.Errorf("[server.go::Server.Init] NewUpstream error: %v", err)
		} else {
			_, exist := ms[sv.Name]
			if exist {
				logrus.Errorf("[server.go::Server.Init] ignore duplicate forward name: %v", sv.Name)
			} else {
				ms[sv.Name] = up
				mux.Handle(sv.Name, up)
			}
		}
	}
	app.mux = mux
	return &app, nil
}

func (self *App) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)

	if len(r.Question) >= 1 {
		//only response the first question
		for i := 0; i < 1; i++ {
			class := r.Question[i].Qclass
			name := r.Question[i].Name
			t := r.Question[i].Qtype

			qr, err := self.Get(name)
			if err == ErrNotExist {
				break
			} else if err != nil {
				logrus.Fatalf("[app.go::ServeDNS] Get ERROR: ", err)
				return
			}

			if class == dns.ClassINET {
				switch t {
				case dns.TypeA:
					m := new(dns.Msg)
					m.SetReply(r)
					m.Used(qr.ToA())
					w.WriteMsg(m)
					return
				case dns.TypeAAAA:
					m := new(dns.Msg)
					m.SetReply(r)
					m.Used(qr.ToAAAA())
					w.WriteMsg(m)
					return
				case dns.TypeANY:
					m := new(dns.Msg)
					m.SetReply(r)
					m.Used(qr.ToAny())
					w.WriteMsg(m)
					return
				}
			}
		}
	}

	mux := self.mux
	mux.ServeDNS(w, r)
}

func (self *App) Get(name string) (Resolve, error) {
	name = fixResolveName(name)
	c := self.cache
	rds := self.rds
	var r Resolve

	v, expireAt, exist := c.GetWithExpiration(name)
	if exist {
		r = v.(Resolve)
		r.Expire = utime.Duration(expireAt.Sub(time.Now()))
		return r, nil
	}

	if rds == nil {
		return r, ErrNotExist
	}

	//redis中不存在
	v, exist = c.Get(name + ".exist")
	logrus.Println("try get redis exist", v, exist)
	if exist {
		return r, ErrNotExist
	}

	d, err := rds.Get(LITEDNS_PREFIX + name).Result()
	logrus.Println("try get redis", d, err)
	if err == redis.Nil {
		//防止每次都走redis
		//理论上这里有一致性的问题，如果redis set和这里同时发生，有可能会出现
		//redis中存在但是1分钟之内无法获取到redis里的结果，实际中只要
		//cache的expire超时大于这里的超时就没有关系，可以走内存中的缓存
		c.Set(name+".exist", true, 60*time.Second)
		return r, ErrNotExist
	} else if err != nil {
		return r, err
	}

	if err = json.Unmarshal([]byte(d), &r); err != nil {
		return r, err
	}
	du, _ := rds.TTL(LITEDNS_PREFIX + name).Result()
	r.Expire = utime.Duration(du)
	c.Set(name, r, r.Expire.Duration())
	c.Delete(name + ".exist")
	return r, err
}

func (self *App) Set(name string, r Resolve) error {
	c := self.cache
	rds := self.rds
	c.Set(name, r, r.Expire.Duration())

	if rds == nil {
		return nil
	}

	txt, _ := json.Marshal(r)
	_, err := rds.Set(LITEDNS_PREFIX+name, string(txt), r.Expire.Duration()).Result()
	return err
}

func (self *App) SetRR(name string, rr []dns.RR, expire time.Duration) error {
	c := self.cache
	rds := self.rds
	var r Resolve
	r.From(name, rr, expire)
	c.Set(name, r, expire)
	if rds == nil {
		return nil
	}

	c.Delete(name + ".exist")
	txt, _ := json.Marshal(r)
	_, err := rds.Set(LITEDNS_PREFIX+name, string(txt), r.Expire.Duration()).Result()
	return err
}

func (self *App) Run() error {
	return self.server.ListenAndServe()
}

func (self *App) Shutdown(ctx context.Context) error {
	return self.server.ShutdownContext(ctx)
}
