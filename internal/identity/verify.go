package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ExitIdentity struct {
	IP      string
	ASN     string
	Country string
}

type ExitProbe interface {
	Fetch(ctx context.Context) (ExitIdentity, error)
}

type MixedPortProbe struct {
	MixedPort string
	URL       string
	Timeout   time.Duration
	Client    *http.Client
}

func (p MixedPortProbe) Fetch(ctx context.Context) (ExitIdentity, error) {
	client := p.Client
	if client == nil {
		mp := p.MixedPort
		if mp == "" {
			mp = "127.0.0.1:7889"
		}
		proxyURL, err := url.Parse("http://" + mp)
		if err != nil {
			return ExitIdentity{}, err
		}
		to := p.Timeout
		if to == 0 {
			to = 20 * time.Second
		}
		client = &http.Client{
			Timeout:   to,
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		}
	}
	u := p.URL
	if u == "" {
		u = "https://ipinfo.io/json"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ExitIdentity{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return ExitIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ExitIdentity{}, fmt.Errorf("exit probe returned status %d", resp.StatusCode)
	}
	var payload struct {
		IP      string `json:"ip"`
		Org     string `json:"org"`
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ExitIdentity{}, err
	}
	if payload.IP == "" {
		return ExitIdentity{}, fmt.Errorf("exit probe returned no IP (status %d)", resp.StatusCode)
	}
	return ExitIdentity{IP: payload.IP, ASN: payload.Org, Country: payload.Country}, nil
}

type ExitVerdict int

const (
	ExitUnverifiable ExitVerdict = iota
	ExitMatched
	ExitMismatched
)

func ClassifyExit(got ExitIdentity, exp ExpectedIdentity) (ExitVerdict, string) {
	if exp.ExitIP == "" && exp.ASN == "" && exp.Country == "" {
		return ExitUnverifiable, "no expected identity configured"
	}
	if exp.ExitIP != "" && got.IP != exp.ExitIP {
		return ExitMismatched, fmt.Sprintf("exit IP mismatch: got %s want %s", got.IP, exp.ExitIP)
	}
	if exp.ASN != "" && !strings.Contains(strings.ToLower(got.ASN), strings.ToLower(exp.ASN)) {
		return ExitMismatched, fmt.Sprintf("ASN mismatch: got %s want %s", got.ASN, exp.ASN)
	}
	if exp.Country != "" && !strings.EqualFold(got.Country, exp.Country) {
		return ExitMismatched, fmt.Sprintf("country mismatch: got %s want %s", got.Country, exp.Country)
	}
	return ExitMatched, ""
}
