package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/log"
)

func getEarliestCertExpiry(state *tls.ConnectionState) time.Time {
	earliest := time.Time{}
	for _, cert := range state.PeerCertificates {
		if (earliest.IsZero() || cert.NotAfter.Before(earliest)) && !cert.NotAfter.IsZero() {
			earliest = cert.NotAfter
		}
	}
	return earliest
}

func probeHTTP(target string, w http.ResponseWriter, module Module, payload string) (success bool) {
	var isSSL, redirects int
	config := module.HTTP

	client := &http.Client{
		Timeout: module.Timeout,
	}

	client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		redirects = len(via)
		if redirects > 10 || config.NoFollowRedirects {
			return errors.New("Don't follow redirects")
		} else {
			return nil
		}
	}

	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	if config.Method == "" {
		config.Method = "GET"
	}

	fmt.Printf("\\ -- // \\ // ---", payload, "\n")

	request, err := http.NewRequest(config.Method, target, strings.NewReader(payload))
	if err != nil {
		log.Errorf("Error creating request for target %s: %s", target, err)
	}

	resp, err := client.Do(request)
	// Err won't be nil if redirects were turned off. See https://github.com/golang/go/issues/3795
	if err != nil && resp == nil {
		log.Warnf("Error for HTTP request to %s: %s", target, err)
	} else {
		defer resp.Body.Close()
		if len(config.ValidStatusCodes) != 0 {
			for _, code := range config.ValidStatusCodes {
				if resp.StatusCode == code {
					success = true
					break
				}
			}
		} else if 200 <= resp.StatusCode && resp.StatusCode < 300 {
			success = true
		}
	}

	if resp == nil {
		resp = &http.Response{}
	}

	if resp.TLS != nil {
		isSSL = 1
		fmt.Fprintf(w, "probe_ssl_earliest_cert_expiry %f\n",
			float64(getEarliestCertExpiry(resp.TLS).UnixNano())/1e9)
		if config.FailIfSSL {
			success = false
		}
	} else if config.FailIfNotSSL {
		success = false
	}
	fmt.Fprintf(w, "probe_http_status_code %d\n", resp.StatusCode)
	fmt.Fprintf(w, "probe_http_content_length %d\n", resp.ContentLength)
	fmt.Fprintf(w, "probe_http_redirects %d\n", redirects)
	fmt.Fprintf(w, "probe_http_ssl %d\n", isSSL)
	return
}
