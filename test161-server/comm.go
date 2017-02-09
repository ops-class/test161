package main

import (
	"fmt"
	"net/http"
)

const JsonHeader = "application/json; charset=UTF-8"
const TextHeader = "text/plain; charset=UTF-8"

func sendErrorCode(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", TextHeader)
	w.WriteHeader(code)
	fmt.Fprintf(w, "%v", err)
}
