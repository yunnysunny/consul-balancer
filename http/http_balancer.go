// Package http provides the utility for balancer on http protocol with consul.
package http

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/valyala/fasthttp"
)

const (
	DEFAULT_TIMEOUT_SECONDS_4_READY   = 5 // Default value for TimeoutSeconds4Ready
	DEFAULT_TIMEOUT_SECONDS_4_REQUEST = 10 // Default value for timeoutSeconds4Request
)
// the config used for initial HttpBalancer
type ServiceConfig struct {
	ServiceName            string		// The name for the serivce
	Tags                   []string		// The tags used for registering the service
	TimeoutSeconds4Ready   uint32		// The max time wait for the service's ready in consul, default value is DEFAULT_TIMEOUT_SECONDS_4_READY
	timeoutSeconds4Request uint32		// The max time for http request, default value is DEFAULT_TIMEOUT_SECONDS_4_REQUEST
	ConsulConfig           *api.Config	// The consul's config, it can be nil
}
// the object for http balancer, it use fasthttp.LBClient to do the process of balance. 
type HttpBalancer struct {
	consulClient           *api.Client
	serviceName            string
	tags                   []string
	lastIndex              uint64
	conns                  []string
	rwLocker               *sync.RWMutex
	hasFirstSuccessQueried bool
	firstQuerySuccessWait  *sync.WaitGroup
	reqSeq                 uint
	connLen                uint
	timeoutSeconds4Ready   uint32
	timeoutSeconds4Request uint32
	lbClient               *fasthttp.LBClient
}
// NewHttpBalancer init an instance of HttpBalancer
func NewHttpBalancer(config *ServiceConfig) (*HttpBalancer, error) {
	configConsul := api.DefaultConfig()
	if config.ConsulConfig != nil {
		configConsul = config.ConsulConfig
	}
	client, err := api.NewClient(configConsul)
	if err != nil {
		return nil, err
	}
	balancer := &HttpBalancer{
		consulClient:           client,
		serviceName:            config.ServiceName,
		tags:                   config.Tags,
		timeoutSeconds4Ready:   config.TimeoutSeconds4Ready,
		timeoutSeconds4Request: config.timeoutSeconds4Request,
		rwLocker:               &sync.RWMutex{},
		firstQuerySuccessWait:  &sync.WaitGroup{},
		lbClient:               &fasthttp.LBClient{},
	}
	balancer.lbClient.HealthCheck = func(req *fasthttp.Request, resp *fasthttp.Response, err error) bool {
		if err != nil {
			fmt.Println("err info:", err.Error())
			return false
		}
		return true
	}
	if balancer.timeoutSeconds4Ready == 0 {
		balancer.timeoutSeconds4Ready = DEFAULT_TIMEOUT_SECONDS_4_READY
	}
	if balancer.timeoutSeconds4Request == 0 {
		balancer.timeoutSeconds4Request = DEFAULT_TIMEOUT_SECONDS_4_REQUEST
	}
	balancer.firstQuerySuccessWait.Add(1)
	go balancer.watch()
	return balancer, nil
}

func (balancer *HttpBalancer) update(newAddrs []string) {
	if !balancer.hasFirstSuccessQueried && len(newAddrs) > 0 {
		balancer.hasFirstSuccessQueried = true
		balancer.firstQuerySuccessWait.Done()
	}

	balancer.rwLocker.Lock()
	balancer.conns = newAddrs
	balancer.connLen = uint(len(newAddrs))
	if balancer.connLen > 0 {
		for _, addr := range newAddrs {
			client := &fasthttp.HostClient{
				Addr: addr,
			}

			balancer.lbClient.Clients = append(balancer.lbClient.Clients, client)
		}
	}

	balancer.rwLocker.Unlock()
}

func (balancer *HttpBalancer) watch() {
	for { //long polling
		services, metainfo, err := balancer.consulClient.Health().ServiceMultipleTags(
			balancer.serviceName, balancer.tags, true, &api.QueryOptions{WaitIndex: balancer.lastIndex},
		)
		if err != nil {
			fmt.Printf("error retrieving instances from Consul: %v", err)
			continue
		}

		balancer.lastIndex = metainfo.LastIndex
		var newAddrs []string
		for _, service := range services {
			addr := fmt.Sprintf("%v:%v", service.Service.Address, service.Service.Port)
			newAddrs = append(newAddrs, addr)
		}
		balancer.update(newAddrs)
	}
}
func (balancer *HttpBalancer) checkIfConnNoneEmpty() bool {
	balancer.rwLocker.RLock()
	defer balancer.rwLocker.RUnlock()
	return balancer.connLen > 0
}
// Do a http request with fasthttp's Request object . It will overwirtes the header of HOST to `balancer.serviceName + ".service.consul"`.
// It call `DoTimeout` with the value of `balancer.timeoutSeconds4Request`.
func (balancer *HttpBalancer) Do(req *fasthttp.Request, res *fasthttp.Response) error {
	sleepStepMs := 100
	sleepCount := int(balancer.timeoutSeconds4Ready*1000/uint32(sleepStepMs)) + 1
	sleepNow := 0
	for {
		if balancer.checkIfConnNoneEmpty() {
			break
		}
		time.Sleep(time.Duration(sleepStepMs) * time.Millisecond)
		sleepNow++
		if sleepNow >= sleepCount {
			return errors.New("timeout for service ready")
		}
	}

	balancer.reqSeq++
	req.SetHost(balancer.serviceName + ".service.consul")
	return balancer.lbClient.DoTimeout(
		req,
		res,
		time.Duration(balancer.timeoutSeconds4Request)*time.Second,
	)

}
