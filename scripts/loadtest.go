package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const baseURL = "http://localhost:8080"

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string `json:"token"`
}

type createURLRequest struct {
	URL string `json:"url"`
}

type urlResponse struct {
	ShortCode string `json:"short_code"`
}

func main() {
	durationFlag := flag.Duration("duration", 30*time.Second, "")
	concurrencyFlag := flag.Int("concurrency", 50, "")
	flag.Parse()

	transport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 500,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}

	noRedirectClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 5 * time.Second,
	}

	standardClient := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("sustained_%d@example.com", timestamp)
	password := "SecretPass123"

	regPayload, _ := json.Marshal(registerRequest{
		Name:     "Sustained Tester",
		Email:    email,
		Password: password,
	})
	resp, err := standardClient.Post(baseURL+"/api/v1/auth/register", "application/json", bytes.NewBuffer(regPayload))
	if err != nil {
		fmt.Printf("Registration failed: %v\n", err)
		return
	}
	resp.Body.Close()

	loginPayload, _ := json.Marshal(loginRequest{
		Email:    email,
		Password: password,
	})
	resp, err = standardClient.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(loginPayload))
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var authResp authResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		fmt.Printf("Failed to parse login response: %v\n", err)
		return
	}

	urlPayload, _ := json.Marshal(createURLRequest{
		URL: "https://example.com/sustained-load-target",
	})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/urls", bytes.NewBuffer(urlPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authResp.Token)

	resp, err = standardClient.Do(req)
	if err != nil {
		fmt.Printf("Failed to create short URL: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var urlResp urlResponse
	if err := json.NewDecoder(resp.Body).Decode(&urlResp); err != nil || urlResp.ShortCode == "" {
		fmt.Printf("Invalid URL creation response\n")
		return
	}

	fmt.Printf("Setup complete. Short code: %s\n", urlResp.ShortCode)
	fmt.Printf("Starting sustained load test for %v with %d concurrent workers...\n", *durationFlag, *concurrencyFlag)
	fmt.Println("----------------------------------------------------------------------")

	ctx, cancel := context.WithTimeout(context.Background(), *durationFlag)
	defer cancel()

	var total302 int64
	var otherErrors int64
	var lastReportedCount int64

	targetURL := baseURL + "/" + urlResp.ShortCode
	startTime := time.Now()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		lastTime := startTime
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				currentTotal := atomic.LoadInt64(&total302)
				deltaReq := currentTotal - lastReportedCount
				deltaSec := t.Sub(lastTime).Seconds()
				rate := float64(deltaReq) / deltaSec
				fmt.Printf("[%v] Completed: %d reqs | Current Rate: %.2f req/s\n",
					t.Sub(startTime).Round(time.Second), currentTotal, rate)
				lastReportedCount = currentTotal
				lastTime = t
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < *concurrencyFlag; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					r, err := noRedirectClient.Get(targetURL)
					if err != nil {
						atomic.AddInt64(&otherErrors, 1)
						continue
					}
					io.Copy(io.Discard, r.Body)
					r.Body.Close()

					if r.StatusCode == http.StatusFound {
						atomic.AddInt64(&total302, 1)
					} else {
						atomic.AddInt64(&otherErrors, 1)
					}
				}
			}
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	grandTotal := total302 + otherErrors
	avgReqPerSec := float64(grandTotal) / totalDuration.Seconds()

	fmt.Println("----------------------------------------------------------------------")
	fmt.Println("Sustained Load Test Summary")
	fmt.Println("----------------------------------------------------------------------")
	fmt.Printf("Total Duration     : %v\n", totalDuration)
	fmt.Printf("Concurrency Workers: %d\n", *concurrencyFlag)
	fmt.Printf("Total Requests Sent: %d\n", grandTotal)
	fmt.Printf("Successful (302)   : %d\n", total302)
	fmt.Printf("Failed / Unexpected: %d\n", otherErrors)
	fmt.Printf("Average Throughput : %.2f req/sec\n", avgReqPerSec)
	fmt.Println("----------------------------------------------------------------------")
}
