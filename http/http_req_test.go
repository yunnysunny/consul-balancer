package http

import (
	"fmt"
	"os"
	"strconv"
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
	check.HTTP = "http://localhost:" + strconv.Itoa(port) + "/test"
	check.Interval = strconv.Itoa(int(checkTTL)) + "s"
	check.Timeout = check.Interval
	check.DeregisterCriticalServiceAfter = "3s"

	registration.Check = check
	err := consulClient.Agent().ServiceRegister(registration)
	return err
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
	for i:=0;i<serverCount;i++ {
		registerServer(clientConsul, httpPort+i)
	}
	m.Run()
	fmt.Println("end")
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
	req.SetRequestURI("/get")
	resp := fasthttp.AcquireResponse()


	err = balancer.Do(req, resp)
	if err != nil {
		t.Errorf("send request failed: %v", err)
		return
	}

	result := string(resp.Body())
	t.Logf("response result: %s", result)
}
