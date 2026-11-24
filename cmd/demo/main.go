package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"waymark"
)

func main() {
	book := waymark.New()
	if err := book.Set("payments.prod", []string{"127.0.0.1"}, time.Minute); err != nil {
		fail(err)
	}
	server, err := waymark.Listen("127.0.0.1:0", book)
	if err != nil {
		fail(err)
	}
	defer server.Close()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "udp", server.Addr())
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := resolver.LookupNetIP(ctx, "ip4", "payments.prod.")
	if err != nil {
		fail(err)
	}
	fmt.Printf("payments.prod %s\n", ips[0].String())
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
