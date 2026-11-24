package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"waymark"
)

func main() {
	dnsAddr := flag.String("dns", "127.0.0.1:5300", "dns listen address")
	httpAddr := flag.String("http", "127.0.0.1:5301", "http listen address")
	flag.Parse()

	book := waymark.New()
	server, err := waymark.Listen(*dnsAddr, book)
	if err != nil {
		fail(err)
	}
	defer server.Close()
	httpServer, err := waymark.ServeHTTP(*httpAddr, book)
	if err != nil {
		fail(err)
	}
	log.Printf("waymark dns %s http %s", server.Addr(), *httpAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shut)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
