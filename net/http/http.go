package http

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/vlorc/lua-vm/base"
	vmnet "github.com/vlorc/lua-vm/net"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HTTPFactory struct {
	client *http.Client
}

type Request struct {
	Method string
	Url    string
	Type   string
	Header map[string]string
	Query  map[string]string
	Cookie map[string]string
	Body   io.Reader
}

func NewHTTPFactory(driver vmnet.NetDriver) *HTTPFactory {
	return &HTTPFactory{
		client: __client(driver, &tls.Config{InsecureSkipVerify: true}),
	}
}

func __client(driver vmnet.NetDriver, config *tls.Config) *http.Client {
	return &http.Client{
		Timeout: 45 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: config,
			Dial: func(network, addr string) (net.Conn, error) {
				return driver.Dial(context.Background(), network, addr)
			},
			DialContext:           driver.Dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

type Body struct {
	io.Reader
	closer []io.Closer
}

func (b *Body) Close() error {
	for i := len(b.closer) - 1; i >= 0; i-- {
		b.closer[i].Close()
	}
	return nil
}
func (b *Body) Append(c io.Closer) {
	b.closer = append(b.closer, c)
}
func __response(resp *http.Response, err error) (*http.Response, error) {
	if nil != err {
		return nil, err
	}
	reader := &Body{
		Reader: resp.Body,
		closer: []io.Closer{resp.Body},
	}
	if resp.Header.Get("Content-Encoding") == "gzip" {
		ungzip, err := gzip.NewReader(reader.Reader)
		if nil != err {
			return resp, err
		}
		defer ungzip.Close()
		reader.Reader = ungzip
		reader.Append(ungzip)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "charset=GBK") {
		reader.Reader = transform.NewReader(reader.Reader, simplifiedchinese.GBK.NewDecoder())
	}
	resp.Body = reader
	return resp, nil
}

func (f *HTTPFactory) __do(method, rawurl string, body io.Reader, header ...string) (*http.Response, error) {
	req, err := http.NewRequest(method, rawurl, body)
	if err != nil {
		return nil, err
	}
	for i, l := 0, len(header); i < l; i += 2 {
		req.Header.Set(header[i*2+0], header[i*2+1])
	}
	return __response(f.client.Do(req))
}

func (f *HTTPFactory) Do(r *Request) (*http.Response, error) {
	req, err := http.NewRequest(r.Method, r.Url, r.Body)
	if nil != err {
		return nil, err
	}
	if len(r.Query) > 0 {
		query := url.Values{}
		for k, v := range r.Query {
			query[k] = []string{v}
		}
		req.URL.RawQuery = query.Encode()
	}
	if len(r.Header) > 0 {
		for k, v := range r.Header {
			req.Header.Set(k, v)
		}
	}
	if len(r.Cookie) > 0 {
		cookie := &http.Cookie{}
		for k, v := range r.Cookie {
			cookie.Name = k
			cookie.Value = v
			req.AddCookie(cookie)
		}
	}
	return __response(f.client.Do(req))
}

func (f *HTTPFactory) Delete(rawurl string) (*http.Response, error) {
	return f.__do("DELETE", rawurl, nil)
}
func (f *HTTPFactory) Put(rawurl string) (*http.Response, error) {
	return f.__do("PUT", rawurl, nil)
}
func (f *HTTPFactory) Get(rawurl string) (*http.Response, error) {
	return f.__do("GET", rawurl, nil)
}
func (f *HTTPFactory) Post(rawurl, contentType string, body io.Reader) (*http.Response, error) {
	return f.__do("POST", rawurl, body, "Content-Type", contentType)
}
func (f *HTTPFactory) Head(rawurl string) (*http.Response, error) {
	return f.__do("HEAD", rawurl, nil)
}
func (f *HTTPFactory) PostJson(rawurl string, values interface{}, args ...string) (*http.Response, error) {
	contentType := "application/json"
	if len(args) > 0 {
		contentType = args[0]
	}
	r, w := io.Pipe()
	go func() {
		json.NewEncoder(w).Encode(values)
		w.Close()
	}()
	return f.Post(rawurl, contentType, r)
}
func (f *HTTPFactory) PostForm(rawurl string, values url.Values, args ...string) (*http.Response, error) {
	contentType := "application/x-www-form-urlencoded"
	if len(args) > 0 {
		contentType = args[0]
	}
	return f.Post(rawurl, contentType, strings.NewReader(values.Encode()))
}
func (f *HTTPFactory) PostString(rawurl string, values string, args ...string) (*http.Response, error) {
	contentType := "text/plain"
	if len(args) > 0 {
		contentType = args[0]
	}
	return f.Post(rawurl, contentType, strings.NewReader(values))
}

func (f *HTTPFactory) PostBuffer(rawurl string, values base.Buffer, args ...string) (*http.Response, error) {
	contentType := "application/octet-stream"
	if len(args) > 0 {
		contentType = args[0]
	}
	return f.Post(rawurl, contentType, bytes.NewReader(values))
}

func (f *HTTPFactory) GetString(rawurl string) (string, error) {
	buf, err := f.GetBuffer(rawurl)
	if nil != err {
		return "", err
	}
	return buf.ToString("raw"), nil
}

func (f *HTTPFactory) GetBuffer(rawurl string) (base.Buffer, error) {
	resp, err := f.Get(rawurl)
	if nil != err {
		return nil, err
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		return nil, err
	}
	return base.Buffer(buf), nil
}
