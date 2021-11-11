package http

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	// "sync"
	"github.com/hashicorp/consul/api"
	"github.com/valyala/fasthttp"
)

const (
	httpPort     = 9000
	waitForReady = 2
	serverCount  = 3
	serviceName  = "balancer-server"
	tag          = "test"
	checkTTL     = 3
)

var (
	tags = []string{tag}
	meta = map[string]string {
		"ServiceType": "test",
	}
	currentIp string
)

func httpHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/test":
			ctx.Response.SetBody([]byte("OK"))
		case "/get":
			ctx.Response.SetBody([]byte("get-res"))
		default:
			ctx.Error("not found", fasthttp.StatusNotFound)
		}
	}
}

func registerServer(consulClient *api.Client, port int) error {
	registration := new(api.AgentServiceRegistration)
	registration.ID = serviceName + "-" + strconv.Itoa(port) 
	registration.Name = serviceName
	registration.Port = port
	registration.Tags = tags
	registration.Address = "localhost"
	registration.Meta = meta

	// 增加consul健康检查回调函数	
	check := new(api.AgentServiceCheck)
	check.HTTP = "http://" + currentIp + ":" + strconv.Itoa(port) + "/test"
	check.Interval = strconv.Itoa(int(checkTTL)) + "s"
	check.Timeout = check.Interval
	check.DeregisterCriticalServiceAfter = "6s"

	registration.Check = check
	err := consulClient.Agent().ServiceRegister(registration)
	return err
}

func getIp() string {
	addrs, err := net.InterfaceAddrs()

	if err != nil {
		fmt.Println("read network interface failed", err)
		return ""
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ip := ipnet.IP.String()
				if strings.HasPrefix(ip, "169.254.") {//not private network
					continue
				}
				fmt.Println("get local ip", ip)
				return ip
			}

		}
	}

	return ""
}

func TestMain(m *testing.M) {
	fmt.Println("begin")
	server := &fasthttp.Server{
		Handler: httpHandler(),
	}
	isFailed := false
	var serverError error
	for i := 0; i < serverCount; i++ {
		go func(index int) {
			address := ":" + strconv.Itoa(httpPort+index)
			err := server.ListenAndServe(address)
			if err != nil {
				isFailed = true
				serverError = err
				fmt.Printf("监听失败:%s\n", err)
				panic(err)
			}
		}(i)
	}

	time.Sleep(time.Duration(waitForReady) * time.Second)
	fmt.Println("sleep ", waitForReady, "s")
	if isFailed {
		fmt.Println("start server error:", serverError)
		return
	}
	config := api.DefaultConfig()
	addrFromEnv := os.Getenv("CONSUL_ADDR")
	if addrFromEnv != "" {
		config.Address = addrFromEnv
	}
	clientConsul, err := api.NewClient(config)
	if err != nil {
		fmt.Printf("error create consul client: %v\n", err)
		return
	}
	currentIp = getIp()
	for i:=0;i<serverCount;i++ {
		err = registerServer(clientConsul, httpPort+i)
		if err != nil {
			fmt.Println("register index ", i, " error ", err)
			return
		}
		fmt.Println("register index", i, "success")
	}
	fmt.Println("sleep ", waitForReady, "s")
	time.Sleep(time.Duration(waitForReady) * time.Second)
	m.Run()
	fmt.Println("end")
}

func TestServiceList(t *testing.T) {
	serviceConfig := &ServiceConfig {
		ServiceName: serviceName,
		Tags: tags,	
		TimeoutSeconds4Ready: 10,	
	}
	balancer, err := NewHttpBalancer(serviceConfig)
	if err != nil {
		t.Errorf("init balancer failed: %v", err)
		return
	}
	services, _, err := balancer.consulClient.Health().ServiceMultipleTags(
		balancer.serviceName, balancer.tags, true, &api.QueryOptions{WaitIndex: balancer.lastIndex},
	)
	if err != nil {
		t.Fatalf("error retrieving instances from Consul: %v", err)
		return
	}
	if len(services) == 0 {
		t.Fatal("service is empty")
		return
	}
}

func TestReq(t *testing.T) {
	serviceConfig := &ServiceConfig {
		ServiceName: serviceName,
		Tags: tags,	
		TimeoutSeconds4Ready: 10,	
	}
	balancer, err := NewHttpBalancer(serviceConfig)
	if err != nil {
		t.Errorf("init balancer failed: %v", err)
		return
	}
	// uri:= &fasthttp.URI{}
	// uri.Parse(nil, []byte("/get"))

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI("/get")
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err = balancer.Do(req, resp)
	if err != nil {
		t.Errorf("send request failed: %v", err)
		return
	}

	result := string(resp.Body())
	t.Logf("response result: %s", result)
}
