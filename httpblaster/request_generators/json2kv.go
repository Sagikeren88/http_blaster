package request_generators

import (
	"bufio"
	"fmt"
	"github.com/v3io/http_blaster/httpblaster/config"
	"github.com/v3io/http_blaster/httpblaster/igz_data"
	"github.com/valyala/fasthttp"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
)

type Json2KV struct {
	workload config.Workload
	RequestCommon
}

func (self *Json2KV) UseCommon(c RequestCommon) {

}

func (self *Json2KV) generate_request(ch_records chan []byte, ch_req chan *fasthttp.Request, host string,
	wg *sync.WaitGroup) {
	defer wg.Done()
	parser := igz_data.EmdSchemaParser{}
	var contentType string = "text/html"
	e := parser.LoadSchema(self.workload.Schema)
	if e != nil {
		panic(e)
	}
	for r := range ch_records {
		json_payload, err := parser.EmdFromJsonRecord(r)
		if err != nil {
			panic(err)
		}
		req := self.PrepareRequest(contentType, self.workload.Header, "PUT",
			self.base_uri, json_payload, host)
		ch_req <- req
	}
}

func (self *Json2KV) generate(ch_req chan *fasthttp.Request, payload string, host string) {
	defer close(ch_req)
	var ch_records chan []byte = make(chan []byte, 10000)

	wg := sync.WaitGroup{}
	wg.Add(runtime.NumCPU())
	for c := 0; c < runtime.NumCPU(); c++ {
		go self.generate_request(ch_records, ch_req, host, &wg)
	}
	ch_files := self.FilesScan(self.workload.Payload)

	for f := range ch_files {
		if file, err := os.Open(f); err == nil {
			reader := bufio.NewReader(file)
			var line_count int=0
			for {
				line, err := reader.ReadBytes('\n')
				if err == nil {
					ch_records <- line
					line_count++
					if line_count % 1024 == 0{
						log.Printf("line: %d from file %s was submitted", line_count, f)
					}
				} else if err == io.EOF {
					break
				} else {
					log.Fatal(err)
				}
			}

			log.Println(fmt.Sprintf("Finish file scaning, generated %d records", line_count))
		} else {
			panic(err)
		}
	}
	close(ch_records)
	wg.Wait()
}

func (self *Json2KV) GenerateRequests(global config.Global, wl config.Workload, tls_mode bool, host string) chan *fasthttp.Request {
	self.workload = wl
	//panic(fmt.Sprintf("workload key [%s] workload key sep [%s]", wl.KeyFormat, string(wl.KeyFormatSep.Rune)))
	if self.workload.Header == nil {
		self.workload.Header = make(map[string]string)
	}
	self.workload.Header["X-v3io-function"] = "PutItem"

	self.SetBaseUri(tls_mode, host, self.workload.Container, self.workload.Target)

	ch_req := make(chan *fasthttp.Request, 1000)

	go self.generate(ch_req, self.workload.Payload, host)

	return ch_req
}
