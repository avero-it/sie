// Package ipc is the control API of sie: every instance listens on a local
// socket and speaks newline-delimited JSON-RPC 2.0 on it, and `sie mcp`
// (the bridge, see mcp.go) turns that into MCP tools over stdio for an
// agent. Both ends live here so they share the wire types.
//
// The server never touches the editor directly: each request becomes a job
// for the editor's loop goroutine (the Submit function) and waits for the
// answer there.
//
// The socket code is built on Unix only (Linux, macOS); on Windows this
// package is empty until named pipes are implemented.
package ipc
