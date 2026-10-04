package wire

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
)

var errHeaderTooLarge = errors.New("proxy: request header too large")

// waitHead makes sure a complete request head (ending in a blank line) is
// buffered in br within MaxHeader bytes, without reading past it.
func waitHead(br *bufio.Reader) error {
	for {
		n := br.Buffered()
		if n > MaxHeader {
			n = MaxHeader
		}
		b, _ := br.Peek(n)
		if bytes.Contains(b, []byte("\r\n\r\n")) || bytes.Contains(b, []byte("\n\n")) {
			return nil
		}
		if len(b) >= MaxHeader {
			return errHeaderTooLarge
		}
		// Block until at least one more byte arrives.
		if _, err := br.Peek(len(b) + 1); err != nil {
			return err
		}
	}
}

var allowedForward = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodDelete: true, http.MethodPatch: true, http.MethodOptions: true,
}

func readHTTP(br *bufio.Reader, w io.Writer) (Request, error) {
	if err := waitHead(br); err != nil {
		if errors.Is(err, errHeaderTooLarge) {
			return Request{}, refuse(w, ProtoHTTPForward, ReplyBadRequest, "header too large")
		}
		return Request{}, err
	}
	req, err := http.ReadRequest(br)
	if err != nil {
		return Request{}, refuse(w, ProtoHTTPForward, ReplyBadRequest, "bad http request: %v", err)
	}
	if req.Method == http.MethodConnect {
		t, err := parseTarget(req.Host, 443)
		if err != nil {
			return Request{}, refuse(w, ProtoHTTPConnect, ReplyBadAddress, "%v", err)
		}
		return Request{Proto: ProtoHTTPConnect, Target: t}, nil
	}
	t, err := prepareForward(req)
	if err != nil {
		code := ReplyBadRequest
		if !allowedForward[req.Method] {
			code = ReplyMethodNotAllowed
		}
		return Request{}, refuse(w, ProtoHTTPForward, code, "%v", err)
	}
	return Request{Proto: ProtoHTTPForward, Target: t, HTTP: req}, nil
}

// ReadForward reads the next request on an HTTP forward (keep-alive)
// connection.
func ReadForward(br *bufio.Reader) (*http.Request, Target, error) {
	if err := waitHead(br); err != nil {
		return nil, Target{}, err
	}
	req, err := http.ReadRequest(br)
	if err != nil {
		return nil, Target{}, err
	}
	t, err := prepareForward(req)
	if err != nil {
		return nil, Target{}, err
	}
	return req, t, nil
}

// hopHeaders must not be forwarded (RFC 9110 §7.6.1 plus proxy headers).
var hopHeaders = []string{"Proxy-Connection", "Proxy-Authorization", "Proxy-Authenticate", "Keep-Alive", "TE", "Trailer", "Upgrade"}

// prepareForward validates an absolute-form http:// request, strips
// hop-by-hop headers and returns its target. req.Write then sends it in
// origin form.
func prepareForward(req *http.Request) (Target, error) {
	if !allowedForward[req.Method] {
		return Target{}, errors.New("method not allowed")
	}
	if !req.URL.IsAbs() || req.URL.Scheme != "http" || req.URL.Host == "" {
		return Target{}, errors.New("forward requests need an absolute http:// URL")
	}
	t, err := parseTarget(req.URL.Host, 80)
	if err != nil {
		return Target{}, err
	}
	for _, f := range req.Header.Values("Connection") {
		for _, h := range strings.Split(f, ",") {
			req.Header.Del(strings.TrimSpace(h))
		}
	}
	req.Header.Del("Connection")
	for _, h := range hopHeaders {
		req.Header.Del(h)
	}
	req.RequestURI = ""
	return t, nil
}
