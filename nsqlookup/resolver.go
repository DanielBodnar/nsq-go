package nsqlookup

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/services"
)

type Registry = services.Registry

// LocalRegistry is an implementation of a immutable set of services. This type
// is mostly useful for testing purposes.
type LocalRegistry map[string][]string

func (r LocalRegistry) Lookup(ctx context.Context, service string, tags ...string) (addrs []string, ttl time.Duration, err error) {
	addrs, ttl, err = r[service], time.Second, ctx.Err()
	return
}

// ConsulRegistry implements a service registry which discovers services from a
// consul catalog.
type ConsulRegistry struct {
	Address   string
	TTL       time.Duration
	Transport http.RoundTripper
}

func (r *ConsulRegistry) Lookup(ctx context.Context, service string, tags ...string) (addrs []string, ttl time.Duration, err error) {
	var serviceResults []struct {
		Node struct {
			Node    string
			Address string
		}
		Service struct {
			Address string
			Port    int
		}
	}

	if err = r.get(ctx, "v1/health/service/"+service+"?passing&stale", &serviceResults); err != nil {
		return
	}

	addrs = make([]string, 0, len(serviceResults))

	for _, r := range serviceResults {
		host := r.Service.Address
		port := r.Service.Port

		if len(host) == 0 {
			host = r.Node.Address
		}

		addrs = append(addrs, net.JoinHostPort(host, strconv.Itoa(port)))
	}

	ttl = r.TTL
	return
}

func (r *ConsulRegistry) get(ctx context.Context, endpoint string, result interface{}) error {
	var address = r.Address
	var req *http.Request
	var res *http.Response
	var t http.RoundTripper
	var err error

	if t = r.Transport; t == nil {
		t = http.DefaultTransport
	}

	if len(address) == 0 {
		address = "http://localhost:8500"
	}

	if strings.Index(address, "://") < 0 {
		address = "http://" + address
	}

	if req, err = http.NewRequest("GET", address+"/"+endpoint, nil); err != nil {
		return err
	}
	req.Header.Set("User-Agent", "nsqlookup consul resolver")
	req.Header.Set("Accept", "application/json")

	if ctx != nil {
		req = req.WithContext(ctx)
	}

	if res, err = t.RoundTrip(req); err != nil {
		return err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return err
	default:
		err = fmt.Errorf("error looking up %s on consul agent at %s: %d %s", endpoint, address, res.StatusCode, res.Status)
		return err
	}

	if err = json.NewDecoder(res.Body).Decode(result); err != nil {
		return err
	}

	return nil
}

var (
	_ services.Registry = (LocalRegistry)(nil)
	_ services.Registry = (*ConsulRegistry)(nil)
)
