//go:build wasmer

/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the //License//); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an //AS IS// BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package proxy_wasm_go_host

import (
	_ "embed"
	"log"
	"os"
	"testing"

	"mosn.io/proxy-wasm-go-host/proxywasm/common"
	"mosn.io/proxy-wasm-go-host/wasmer"
	"mosn.io/proxy-wasm-go-host/wazero"

	proxywasm "mosn.io/proxy-wasm-go-host/proxywasm/v2"
)

var exampleWasm = func() []byte {
	wasmBytes, err := os.ReadFile("example/data/main.wasm")
	if err != nil {
		log.Panicln(err)
	}
	return wasmBytes
}()

func BenchmarkStartABIContext_wasmer(b *testing.B) {
	benchmarkStartABIContext(b, wasmer.NewInstanceFromBinary)
}

func BenchmarkStartABIContext_wazero(b *testing.B) {
	benchmarkStartABIContext(b, wazero.NewInstanceFromBinary)
}

func benchmarkStartABIContext(b *testing.B, newInstance func([]byte) common.WasmInstance) {
	for i := 0; i < b.N; i++ {
		if wasmCtx, err := startABIContext(newInstance); err != nil {
			b.Fatal(err)
		} else {
			wasmCtx.Instance.Stop()
		}
	}
}

func startABIContext(newInstance func([]byte) common.WasmInstance) (wasmCtx *proxywasm.ABIContext, err error) {
	instance := newInstance(exampleWasm)

	// create ABI context
	wasmCtx = &proxywasm.ABIContext{Imports: &proxywasm.DefaultImportsHandler{}, Instance: instance}

	// register ABI imports into the wasm vm instance
	wasmCtx.RegisterImports()

	// start the wasm vm instance
	if err = instance.Start(); err != nil {
		instance.Stop()
	}
	return
}

func BenchmarkContextLifecycle_wasmer(b *testing.B) {
	benchmarkContextLifecycle(b, wasmer.NewInstanceFromBinary)
}

func BenchmarkContextLifecycle_wazero(b *testing.B) {
	benchmarkContextLifecycle(b, wazero.NewInstanceFromBinary)
}

func benchmarkContextLifecycle(b *testing.B, newInstance func([]byte) common.WasmInstance) {
	wasmCtx, err := startABIContext(newInstance)
	if err != nil {
		b.Fatal(err)
	}
	defer wasmCtx.Instance.Stop()

	// make the root context
	if err := wasmCtx.GetExports().ProxyOnContextCreate(int32(0), int32(0), proxywasm.ContextTypeHttpContext); err != nil {
		b.Fatal(err)
	}

	// Time the guest call for context create and delete, which happens per-request.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err = wasmCtx.GetExports().ProxyOnContextCreate(int32(1), int32(0), proxywasm.ContextTypeHttpContext); err != nil {
			b.Fatal(err)
		}
		if _, err = wasmCtx.GetExports().ProxyOnDone(int32(1)); err != nil {
			b.Fatal(err)
		}
	}
}
