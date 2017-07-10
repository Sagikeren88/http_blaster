package request_generators

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"log"
	"os"
	"path/filepath"
)

const (
	PERFORMANCE = "performance"
	LINE2STREAM = "line2stream"
	CSV2KV      = "csv2kv"
	JSON2KV     = "json2kv"
	STREAM_GET  = "stream_get"
)

type RequestCommon struct {
	ch_files chan string
	base_uri string
}

func (self *RequestCommon) PrepareRequest(content_type string,
	header_args map[string]string,
	method string, uri string,
	body string, host string,req *fasthttp.Request){
	//req := fasthttp.AcquireRequest()

	header := fasthttp.RequestHeader{}
	header.SetContentType(content_type)

	header.SetMethod(method)
	header.SetRequestURI(uri)
	header.SetHost(host)

	for k, v := range header_args {
		header.Set(k, v)
	}
	req.AppendBodyString(body)
	header.CopyTo(&req.Header)
	//return req
}

func (self *RequestCommon) SubmitFiles(path string, info os.FileInfo, err error) error {
	log.Print(path)
	if err != nil {
		log.Print(err)
		return nil
	}
	if !info.IsDir() {
		self.ch_files <- path
	}
	fmt.Println(path)
	return nil
}

func (self *RequestCommon) FilesScan(path string) chan string {
	self.ch_files = make(chan string)
	go func() {
		err := filepath.Walk(path, self.SubmitFiles)
		if err != nil {
			log.Fatal(err)
		}
		close(self.ch_files)
	}()
	return self.ch_files
}

func (self *RequestCommon) SetBaseUri(tls_mode bool, host string, container string, target string) {
	http := "http"
	if tls_mode {
		http += "s"
	}
	self.base_uri = fmt.Sprintf("%s://%s/%s/%s", http, host, container, target)
}
