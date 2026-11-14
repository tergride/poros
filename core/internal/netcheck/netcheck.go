// Package netcheck asks outside services who sees the user: the address with and without the VPN, and the DNS
// servers that resolve the user's names, to tell a DNS leak. The services are listed here and nowhere else, so
// one can be replaced without touching the rest. Every request reveals the user's address to the service; the
// engine sends none while the user has the check turned off.
package netcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Info is what a service knows about an address.
type Info struct {
	IP string
	// CountryCode is ISO 3166, upper case.
	CountryCode string
	City        string
	// ISP is the provider's name; ASN its autonomous system, 0 when unknown.
	ISP string
	ASN int
}

// Service is one source of Info for the address a request comes from.
type Service struct {
	URL   string
	Parse func(body []byte) (Info, error)
}

// Services are asked in turn until one answers: all over HTTPS, without keys.
var Services = []Service{
	{URL: "https://ipleak.net/json/", Parse: parseIPLeak},
	{URL: "https://ipinfo.io/json", Parse: parseIPInfo},
	{URL: "https://ipwho.is/", Parse: parseIPWho},
}

// LeakService is bash.ws: it hands out a test ID, sees which DNS servers ask for names under it, and lists them.
var LeakService = "https://bash.ws"

const (
	requestTimeout = 8 * time.Second
	maxBody        = 64 << 10
	// probes are the names resolved in a leak test; each may go to another server of the user's resolvers.
	probes       = 10
	probeTimeout = 3 * time.Second
)

// Lookup asks the services in turn who the client is: the client decides the way, through the VPN or around it.
func Lookup(ctx context.Context, client *http.Client, services []Service) (Info, error) {
	var errs []error
	for _, service := range services {
		body, err := get(ctx, client, service.URL)
		if err == nil {
			var info Info
			if info, err = service.Parse(body); err == nil {
				return info, nil
			}
		}
		errs = append(errs, fmt.Errorf("%s: %w", hostOf(service.URL), err))
	}
	return Info{}, errors.Join(errs...)
}

// Resolver resolves a name the way the user's programs do; its answer does not matter, only who is asked.
type Resolver func(ctx context.Context, host string) error

var testID = regexp.MustCompile(`^[0-9a-zA-Z]{1,64}$`)

// Leak runs a leak test: resolve names under a fresh test ID, then ask which DNS servers asked for them.
func Leak(ctx context.Context, client *http.Client, base string, resolve Resolver) ([]Info, error) {
	body, err := get(ctx, client, base+"/id")
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(string(body))
	if !testID.MatchString(id) {
		return nil, errors.New("the leak test service gave no test ID")
	}
	host := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	var wg sync.WaitGroup
	for i := 1; i <= probes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			_ = resolve(probeCtx, fmt.Sprintf("%d.%s.%s", i, id, host))
		}()
	}
	wg.Wait()
	if body, err = get(ctx, client, base+"/dnsleak/test/"+id+"?json=2"); err != nil {
		return nil, err
	}
	return parseLeak(body)
}

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxBody))
}

func hostOf(url string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	host, _, _ = strings.Cut(host, "/")
	return host
}

var errNoAddress = errors.New("the answer has no address")

func parseIPLeak(body []byte) (Info, error) {
	var answer struct {
		IP          string `json:"ip"`
		CountryCode string `json:"country_code"`
		City        string `json:"city_name"`
		ISP         string `json:"isp_name"`
		ASN         int    `json:"as_number"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return Info{}, err
	}
	if answer.IP == "" {
		return Info{}, errNoAddress
	}
	return Info{IP: answer.IP, CountryCode: strings.ToUpper(answer.CountryCode), City: answer.City, ISP: answer.ISP, ASN: answer.ASN}, nil
}

func parseIPInfo(body []byte) (Info, error) {
	var answer struct {
		IP      string `json:"ip"`
		Country string `json:"country"`
		City    string `json:"city"`
		Org     string `json:"org"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return Info{}, err
	}
	if answer.IP == "" {
		return Info{}, errNoAddress
	}
	asn, isp := splitAS(answer.Org)
	return Info{IP: answer.IP, CountryCode: strings.ToUpper(answer.Country), City: answer.City, ISP: isp, ASN: asn}, nil
}

func parseIPWho(body []byte) (Info, error) {
	var answer struct {
		IP          string `json:"ip"`
		Success     bool   `json:"success"`
		Message     string `json:"message"`
		CountryCode string `json:"country_code"`
		City        string `json:"city"`
		Connection  struct {
			ASN int    `json:"asn"`
			ISP string `json:"isp"`
		} `json:"connection"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return Info{}, err
	}
	if !answer.Success {
		return Info{}, fmt.Errorf("the service refused: %s", answer.Message)
	}
	if answer.IP == "" {
		return Info{}, errNoAddress
	}
	return Info{IP: answer.IP, CountryCode: strings.ToUpper(answer.CountryCode), City: answer.City, ISP: answer.Connection.ISP, ASN: answer.Connection.ASN}, nil
}

// splitAS reads "AS64500 Example Networks" into the number and the name.
func splitAS(text string) (int, string) {
	text = strings.TrimSpace(text)
	head, rest, _ := strings.Cut(text, " ")
	if len(head) > 2 && strings.EqualFold(head[:2], "AS") {
		if asn, err := strconv.Atoi(head[2:]); err == nil {
			return asn, strings.TrimSpace(rest)
		}
	}
	return 0, text
}

func parseLeak(body []byte) ([]Info, error) {
	var records []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		IP      string `json:"ip"`
		Country string `json:"country"`
		City    string `json:"city_name"`
		ASN     string `json:"asn"`
	}
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, err
	}
	servers := []Info{}
	for _, record := range records {
		switch record.Type {
		case "error":
			return nil, fmt.Errorf("the leak test service: %s", record.Text)
		case "dns":
			asn, isp := splitAS(record.ASN)
			servers = append(servers, Info{IP: record.IP, CountryCode: strings.ToUpper(record.Country), City: record.City, ISP: isp, ASN: asn})
		}
	}
	return servers, nil
}
