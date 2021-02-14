package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	AuthURL     = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:ratelimitpreview/test:pull"
	RegistryURL = "https://registry-1.docker.io/v2/ratelimitpreview/test/manifests/latest"
)

type RateLimit struct {
	Limit     int
	Remaining int
	window    time.Duration
}

type Response struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

type Options struct {
	username string
	password string
	token    string
}

type Option func(*Options)

func withAuth(username, password string) Option {
	return func(args *Options) {
		args.username = username
		args.password = password
	}
}

func withToken(token string) Option {
	return func(args *Options) {
		args.token = token
	}
}

func newRequestWithContext(ctx context.Context, method string, url string, options ...Option) (*http.Response, error) {
	opt := Options{}
	for _, o := range options {
		o(&opt)
	}

	c := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	if opt.username != "" && opt.password != "" {
		req.SetBasicAuth(opt.username, opt.password)
	}

	if opt.token != "" {
		req.Header.Add("Authorization", "Bearer "+opt.token)
	}

	return c.Do(req)
}

func getAuthToken(ctx context.Context, username, password string) (string, error) {
	resp, err := newRequestWithContext(ctx, http.MethodGet, AuthURL, withAuth(username, password))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed with status code %d", resp.StatusCode)
	}

	var res Response
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.AccessToken, nil
}

func checkLimit(ctx context.Context, username, password string) (*RateLimit, error) {
	token, err := getAuthToken(ctx, username, password)
	if err != nil {
		return nil, err
	}

	resp, err := newRequestWithContext(ctx, http.MethodHead, RegistryURL, withToken(token))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authenticate is failed. status code is %d", resp.StatusCode)
	}

	limit, _ := parseHeader(resp.Header.Get("RateLimit-Limit"))
	remaining, window := parseHeader(resp.Header.Get("RateLimit-Remaining"))

	res := &RateLimit{
		Limit:     limit,
		Remaining: remaining,
		window:    window,
	}
	return res, nil
}

func parseHeader(s string) (int, time.Duration) {
	parts := strings.SplitN(s, ";w=", 2)

	num, _ := strconv.Atoi(parts[0])
	dur := time.Duration(0)

	if len(parts) > 1 {
		i, _ := strconv.Atoi(parts[1])
		dur = time.Duration(i) * time.Second
	}

	return num, dur
}
