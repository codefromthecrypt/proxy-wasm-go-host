/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"mosn.io/proxy-wasm-go-host/proxywasm/common"
	proxywasm "mosn.io/proxy-wasm-go-host/proxywasm/v2"
	"mosn.io/proxy-wasm-go-host/wazero"
)

var (
	contextIDGenerator int32
	rootContextID      int32
)

var (
	lock    sync.Mutex
	once    sync.Once
	wasmCtx *proxywasm.ABIContext
)

var _ proxywasm.ImportsHandler = &importHandler{}

// implement v2.ImportsHandler.
type importHandler struct {
	reqHeader common.HeaderMap
	proxywasm.DefaultImportsHandler
}

// override.
func (im *importHandler) GetHttpRequestHeader() common.HeaderMap {
	return im.reqHeader
}

// override.
func (im *importHandler) Log(level proxywasm.LogLevel, msg string) proxywasm.Result {
	fmt.Println(msg)
	return proxywasm.ResultOk
}

// serve HTTP req
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("receive request %s\n", r.URL)
	for k, v := range r.Header {
		fmt.Printf("print header from server host, %v -> %v\n", k, v)
	}

	// get wasm vm instance
	ctx := getWasmContext()

	// create context id for the http req
	contextID := atomic.AddInt32(&contextIDGenerator, 1)

	// do wasm

	// according to ABI, we should create a root context id before any operations
	once.Do(func() {
		if err := ctx.GetExports().ProxyOnContextCreate(rootContextID, 0, proxywasm.ContextTypeHttpContext); err != nil {
			log.Panicln(err)
		}
	})

	// lock wasm vm instance for exclusive ownership
	ctx.Instance.Lock(ctx)
	defer ctx.Instance.Unlock()

	// Set the import handler to the current request.
	ctx.SetImports(&importHandler{reqHeader: &myHeaderMap{r.Header}})

	// create wasm-side context id for current http req
	if err := ctx.GetExports().ProxyOnContextCreate(contextID, rootContextID, proxywasm.ContextTypeHttpContext); err != nil {
		log.Panicln(err)
	}

	// call wasm-side on_request_header
	if _, err := ctx.GetExports().ProxyOnRequestHeaders(contextID, int32(len(r.Header)), 1); err != nil {
		log.Panicln(err)
	}

	// delete wasm-side context id to prevent memory leak
	if _, err := ctx.GetExports().ProxyOnDone(contextID); err != nil {
		log.Panicln(err)
	}

	// reply with ok
	w.WriteHeader(http.StatusOK)
}

func main() {
	// create root context id
	rootContextID = atomic.AddInt32(&contextIDGenerator, 1)

	// serve http
	http.HandleFunc("/", ServeHTTP)
	if err := http.ListenAndServe("127.0.0.1:2045", nil); err != nil {
		log.Panicln(err)
	}
}

func getWasmContext() *proxywasm.ABIContext {
	lock.Lock()
	defer lock.Unlock()

	if wasmCtx == nil {
		wasmBytes, err := os.ReadFile("data/main.wasm")
		if err != nil {
			log.Panicln(err)
		}
		instance := wazero.NewInstanceFromBinary(wasmBytes)

		// create ABI context
		wasmCtx = &proxywasm.ABIContext{Imports: &proxywasm.DefaultImportsHandler{}, Instance: instance}

		// register ABI imports into the wasm vm instance
		wasmCtx.RegisterImports()

		// start the wasm vm instance
		if err = instance.Start(); err != nil {
			log.Panicln(err)
		}
	}

	return wasmCtx
}

// wrapper for http.Header, convert Header to api.HeaderMap.
type myHeaderMap struct {
	realMap http.Header
}

func (m *myHeaderMap) Get(key string) (string, bool) {
	return m.realMap.Get(key), true
}

func (m *myHeaderMap) Set(key, value string) { panic("implemented") }

func (m *myHeaderMap) Add(key, value string) { panic("implemented") }

func (m *myHeaderMap) Del(key string) { panic("implemented") }

func (m *myHeaderMap) Range(f func(key string, value string) bool) {
	for k := range m.realMap {
		v := m.realMap.Get(k)
		f(k, v)
	}
}

func (m *myHeaderMap) Clone() common.HeaderMap { panic("implemented") }

func (m *myHeaderMap) ByteSize() uint64 { panic("implemented") }
