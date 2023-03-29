package resolver

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type Client interface {
	Exchange(m *dns.Msg, address string) (r *dns.Msg, rtt time.Duration, err error)
}

type Resolver struct {
	servers []string
	client  Client
}

func New(servers []string) *Resolver {
	return &Resolver{
		servers: servers,
		client: &dns.Client{
			Net:          "tcp",
			ReadTimeout:  time.Second * 5,
			WriteTimeout: time.Second * 5,
		},
	}
}

func (r *Resolver) Lookup(req *dns.Msg) (*dns.Msg, error) {
	qName := req.Question[0].Name

	res := make(chan *dns.Msg, 1)
	defer close(res)
	var wg sync.WaitGroup
	L := func(nameserver string) {
		defer wg.Done()
		rrsp, _, err := r.client.Exchange(req, nameserver)
		if err != nil {
			log.Printf("%s socket error on %s", qName, nameserver)
			log.Printf("error:%s", err.Error())
			return
		}
		if rrsp != nil && rrsp.Rcode != dns.RcodeSuccess {
			if rrsp.Rcode == dns.RcodeServerFailure {
				return
			}
		}
		select {
		case res <- rrsp:
		default:
		}
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Start lookup on each nameserver top-down, in every second
	for _, nameserver := range r.servers {
		wg.Add(1)
		go L(nameserver)
		// but exit early, if we have an answer
		select {
		case r := <-res:
			return r, nil
		case <-ticker.C:
			continue
		}
	}

	// wait for all the namservers to finish
	wg.Wait()
	select {
	case r := <-res:
		return r, nil
	default:
		return nil, errors.New("can't resolve ip for" + qName)
	}
}
