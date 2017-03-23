/*
Copyright 2016 Iguazio.io Systems Ltd.

Licensed under the Apache License, Version 2.0 (the "License") with
an addition restriction as set forth herein. You may not use this
file except in compliance with the License. You may obtain a copy of
the License at http://www.apache.org/licenses/LICENSE-2.0.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing
permissions and limitations under the License.

In addition, you may not use the software for any purposes that are
illegal under applicable law, and the grant of the foregoing license
under the Apache 2.0 license is conditioned upon your compliance with
such restriction.
*/
package httpblaster

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"net"
	"sync"
	"time"
	"crypto/tls"
)

const DialTimeout = 60 * time.Second

type worker_load struct {
	req       *fasthttp.Request
	req_count uint64
	duration  duration
	port      string
}

type worker_results struct {
	count uint64
	min   time.Duration
	max   time.Duration
	avg   time.Duration
	read  uint64
	write uint64
	codes map[int]uint64
}

func (self *worker_load) Prepare_request(content_type string,
	header_args map[string]string, method string, uri string, body string) {

	self.req = &fasthttp.Request{}
	header := fasthttp.RequestHeader{}
	header.SetContentType(content_type)

	header.SetMethod(method)
	header.SetRequestURI(uri)

	for k, v := range header_args {
		header.Set(k, v)
	}
	self.req.AppendBodyString(body)
	header.CopyTo(&self.req.Header)
}

type worker struct {
	host                string
	conn                net.Conn
	results             worker_results
	connection_restarts uint32
	error_count         uint32
	is_tls_client       bool
	base_uri            string
	client    *fasthttp.HostClient
}

func (w *worker) send_request(req *fasthttp.Request, response *fasthttp.Response) (error, time.Duration) {
	var (
		code int
	)
	err, duration := w.send(req, response)

	if err != nil || response.ConnectionClose() {
		w.restart_connection()
	}
	if err == nil {
		code = response.StatusCode()
		w.results.codes[code]++

		w.results.count++
		if duration < w.results.min {
			w.results.min = duration
		}
		if duration > w.results.max {
			w.results.max = duration
		}
		w.results.avg = w.results.avg + (duration-w.results.avg)/time.Duration(w.results.count)
	} else {
		w.error_count++
	}

	return err, duration
}

func (w *worker) open_connection() {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}
	w.client = &fasthttp.HostClient{Addr: w.host, IsTLS:w.is_tls_client, TLSConfig:conf}
}

func (w *worker) close_connection() {
	if w.conn != nil {
		w.conn.Close()
	}
}

func (w *worker) restart_connection() {
	w.close_connection()
	w.open_connection()
	w.connection_restarts++
}

func (w *worker) send(req *fasthttp.Request, resp *fasthttp.Response) (error, time.Duration) {
	start := time.Now()
	r := fasthttp.Request{}
	req.CopyTo(&r)
	w.client.DoTimeout(&r, resp, time.Duration(10 * time.Second))
	end := time.Now()
	duration := end.Sub(start)
	return nil, duration
}

func (w *worker) gen_files_uri(file_index int, count int) chan string {
	ch := make(chan string, 1000)
	go func() {
		file_pref := file_index
		for {

			if file_pref == file_index + count {
				file_pref = file_index
			}
			ch <- fmt.Sprintf("%s_%d", w.base_uri, file_pref)
			file_pref += 1

		}
	}()
	return ch
}

func(w *worker) single_file_submitter(done chan struct{}, load *worker_load){
	request := clone_request(load.req)
	response := fasthttp.Response{}
LOOP:
	for {
		select {
		case <-done:
			break LOOP
		default:
			if w.results.count < load.req_count{
				w.send_request(request, &response)
			}else{
				break LOOP
			}

		}
	}

}

func (w *worker) multi_file_submitter(done chan struct{}, load *worker_load, file_index int, count int)  {
	ch_uri := w.gen_files_uri(file_index, count)
	request := clone_request(load.req)
	response := fasthttp.Response{}
WLoop:
	for {
		select {
		case <-done:
			break WLoop
		case uri := <-ch_uri:
			if w.results.count < load.req_count {
				request.SetRequestURI(uri)
				w.send_request(request, &response)
			} else {
				break WLoop
			}

		}
	}

}

func (w *worker) run_worker(load *worker_load, wg *sync.WaitGroup, file_index int, count int) {
	defer wg.Done()
	w.results.min = time.Duration(10 * time.Second)
	done := make(chan struct{})

	go func() {
		select {
		case <-time.After(load.duration.Duration):
			close(done)
		}
	}()
	if file_index == 0 && count == 0 {
		w.single_file_submitter(done, load)
	}else{
		w.multi_file_submitter(done, load, file_index, count)
	}
}

func NewWorker(host string, tls_client bool, base_uri string) *worker {
	if host == "" {
		return nil
	}
	worker := worker{host: host, is_tls_client: tls_client, base_uri: base_uri}
	worker.results.codes = make(map[int]uint64)
	worker.open_connection()
	return &worker
}
