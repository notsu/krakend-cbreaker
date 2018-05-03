package proxy

import (
	"context"

	"github.com/devopsfaith/krakend/config"
	"github.com/devopsfaith/krakend/proxy"

	"github.com/afex/hystrix-go/hystrix"
	"github.schibsted.io/spt-infrastructure/apigw-krakend/cbreaker"
	"net/http"
)

// BackendFactory adds a cb middleware wrapping the internal factory
func BackendFactory(next proxy.BackendFactory) proxy.BackendFactory {
	return func(cfg *config.Backend) proxy.Proxy {
		return NewMiddleware(cfg)(next(cfg))
	}
}

var Backend500Error = hystrix.CircuitError{"Backend 500 error"}

// NewMiddleware builds a middleware based on the extra config params or fallbacks to the next proxy
func NewMiddleware(remote *config.Backend) proxy.Middleware {
	data := cbreaker.ConfigGetter(remote.ExtraConfig).(cbreaker.Config)
	if data == cbreaker.ZeroCfg {
		return proxy.EmptyMiddleware
	}

	cb := cbreaker.NewCommand(data)

	return func(next ...proxy.Proxy) proxy.Proxy {
		if len(next) > 1 {
			panic(proxy.ErrTooManyProxies)
		}
		return NewCbRequest(cb, next[0])
	}
}

func NewCbRequest(cb *cbreaker.HystrixCommand, next proxy.Proxy) proxy.Proxy {
	var response *proxy.Response
	return func(ctx context.Context, request *proxy.Request) (*proxy.Response, error) {
		err := cb.Execute(func() error {
			var err error
			response, err = next(ctx, request)
			if response != nil {
				if response.Metadata.StatusCode == http.StatusInternalServerError {
					return Backend500Error
				}
			}
			return err
		}, nil)
		if err != nil {
			if err == Backend500Error {
				return response, err
			}
			return response, nil
		}
		return response, err
	}
}