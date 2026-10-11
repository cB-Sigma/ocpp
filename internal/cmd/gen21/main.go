// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Command gen21 writes v21/messages.go from the embedded OCPP 2.1 schemas.
// Run it with go generate ./v21 from the module root.
package main

import (
	"log"
	"os"

	"github.com/cB-Sigma/ocpp"
	"github.com/cB-Sigma/ocpp/internal/gen21"
)

func main() {
	files, err := ocpp.SchemaFiles("2.1")
	if err != nil {
		log.Fatal(err)
	}
	src, err := gen21.Generate(files, ocpp.SendActions21)
	if err != nil {
		log.Fatal(err)
	}
	out := "messages.go"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}
