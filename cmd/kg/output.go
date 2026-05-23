package main

import (
	"encoding/json"
	"io"
)

type envelope struct {
	OK    bool    `json:"ok"`
	Data  any     `json:"data,omitempty"`
	Error *envErr `json:"error,omitempty"`
}

type envErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func writeOK(w io.Writer, data any) error {
	return writeJSON(w, envelope{OK: true, Data: data})
}

func writeErr(w io.Writer, code, message, hint string) error {
	return writeJSON(w, envelope{OK: false, Error: &envErr{Code: code, Message: message, Hint: hint}})
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
