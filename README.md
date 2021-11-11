# consul-balancer

[![Go Reference](https://pkg.go.dev/badge/github.com/yunnysunny/consul-balancer.svg)](https://pkg.go.dev/github.com/yunnysunny/consul-balancer)
[![codecov](https://codecov.io/gh/yunnysunny/consul-balancer/branch/main/graph/badge.svg?token=36hjGphJnz)](https://codecov.io/gh/yunnysunny/consul-balancer)

The client balancer of consul. It only supports http protocol now.

## License

[MIT](LICENSE)

## Usage

Install via `go get`.
```shell
go get github.com/yunnysunny/consul-balancer
```

Send request to specified service.

```go
import (
    "github.com/yunnysunny/consul-balancer/http"
    "github.com/valyala/fasthttp"
)
func doGet(serviceName string, tags []string) {
    serviceConfig := &http.ServiceConfig {
        ServiceName: serviceName,
        Tags: tags,	
        TimeoutSeconds4Ready: 10,	
    }
    balancer, err := http.NewHttpBalancer(serviceConfig)
    if err != nil {
        fmt.Errorf("init balancer failed: %v", err)
        return
    }

    req := fasthttp.AcquireRequest()
    defer fasthttp.ReleaseRequest(req)
    req.SetRequestURI("/get")
    resp := fasthttp.AcquireResponse()
    defer fasthttp.ReleaseResponse(resp)

    err = balancer.Do(req, resp)
    if err != nil {
        fmt.Errorf("send request failed: %v", err)
        return
    }

    result := string(resp.Body())
    fmt.Logf("response result: %s", result)
}

```

consul-balancer use [fasthttp](https://github.com/valyala/fasthttp) to process http request. Be careful to use the reference of fasthttp.Response's internal buffer. In the code for example, resp.Body() return a slice that points to Response's internal buffer, after call `fasthttp.ReleaseResponse`, fasthttp will reuse the internal buffer, and the content of buffer may be changed. So the safety usage is copying the bytes gotten from `Response` before release it. See the issue of [#1013](https://github.com/valyala/fasthttp/issues/1013) [#1109](https://github.com/valyala/fasthttp/issues/1109) of fasthttp.

