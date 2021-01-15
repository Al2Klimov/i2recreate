package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type closableReader struct {
	r io.Reader
}

var _ io.ReadCloser = closableReader{}

func (cr closableReader) Read(p []byte) (int, error) {
	return cr.r.Read(p)
}

func (cr closableReader) Close() error {
	return nil
}

var _2Pow256 = big.NewInt(2)
var _64 = big.NewInt(64)
var client = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
var i2req = &http.Request{URL: &url.URL{Scheme: "https"}, Header: map[string][]string{"Accept": {"application/json"}}}
var i2cc = flag.String("i2-chkcmd", "", "CHECK_COMMAND")

func main() {
	_2Pow256.Exp(_2Pow256, big.NewInt(256), nil)

	i2host := flag.String("i2-host", "", "HOST")
	i2port := flag.Int("i2-port", 5665, "PORT")
	i2user := flag.String("i2-user", "root", "USER")
	i2pass := flag.String("i2-pass", "", "PASS")

	iw2url := flag.String("iw2-url", "", "URL")
	objects := flag.Int("objects", 0, "AMOUNT")

	flag.Parse()

	iw2hosts, errPU := url.Parse(*iw2url)
	assert(errPU)

	if !strings.HasSuffix(iw2hosts.Path, "/") {
		iw2hosts.Path += "/"
	}

	iw2hosts = iw2hosts.ResolveReference(&url.URL{Path: "monitoring/list/hosts"})

	i2req.URL.Host = net.JoinHostPort(*i2host, strconv.FormatInt(int64(*i2port), 10))
	i2req.SetBasicAuth(*i2user, *i2pass)

	iw2req := &http.Request{Method: "GET", URL: iw2hosts, Header: map[string][]string{"Accept": {"application/json"}}}
	iw2req.SetBasicAuth(*i2user, *i2pass)

	monObjs := make([]string, 0, *objects)
	for i := 0; i < *objects; i++ {
		monObjs = append(monObjs, rand64())
	}

	for _, name := range monObjs {
		create(name)
	}

	for {
		for _, name := range monObjs {
			log.Printf("Deleting host %#v...", name)

			i2req.Method = "DELETE"
			i2req.URL.Path = "/v1/objects/hosts/" + url.PathEscape(name)
			i2req.URL.RawQuery = "cascade=1"

			doReq(i2req).Close()
			i2req.URL.RawQuery = ""

			log.Print("Done.")

			create(name)

			log.Print("Waiting for it to be visible in the web...")

		Wait:
			for {
				time.Sleep(time.Second)
				os.Stderr.Write([]byte{'.'})

				body := doReq(iw2req)
				var hosts []struct {
					HostName string `json:"host_name"`
				}

				assert(json.NewDecoder(bufio.NewReader(body)).Decode(&hosts))
				body.Close()

				for _, host := range hosts {
					if host.HostName == name {
						os.Stderr.Write([]byte{'\n'})
						break Wait
					}
				}
			}

			log.Print("Done.")
		}
	}
}

func assert(err error) {
	if err != nil {
		log.Fatal(err.Error())
	}
}

func rnd(max *big.Int) *big.Int {
	i, errIn := rand.Int(rand.Reader, max)
	assert(errIn)
	return i
}

func rand64() string {
	return fmt.Sprintf("%x", rnd(_2Pow256))
}

func create(host string) {
	log.Printf("Creating host %#v...", host)

	i2req.Method = "PUT"
	i2req.URL.Path = "/v1/objects/hosts/" + url.PathEscape(host)

	{
		attrs := map[string]interface{}{"check_command": *i2cc, "enable_active_checks": false}
		for i := rnd(_64).Int64() + 1; i > 0; i-- {
			attrs["vars."+rand64()] = rand64()
		}

		buf := &bytes.Buffer{}
		assert(json.NewEncoder(buf).Encode(map[string]interface{}{"attrs": attrs}))
		i2req.Body = closableReader{buf}
	}

	doReq(i2req).Close()
	log.Print("Done.")
}

func doReq(req *http.Request) io.ReadCloser {
	resp, errRq := client.Do(req)
	assert(errRq)

	if resp.StatusCode >= 400 {
		log.Fatal(resp.StatusCode)
	}

	return resp.Body
}
