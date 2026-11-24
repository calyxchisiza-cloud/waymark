package waymark

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLookupAndExpiry(t *testing.T) {
	book := New()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	book.now = func() time.Time { return now }
	if err := book.Set("payments.prod", []string{"127.0.0.1", "127.0.0.2"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := book.Drop("payments.prod", "127.0.0.2"); err != nil {
		t.Fatal(err)
	}
	known, answers := book.Lookup("Payments.Prod")
	if !known || len(answers) != 1 || answers[0].IP.String() != "127.0.0.1" || answers[0].TTL != 60 {
		t.Fatalf("%v %v", known, answers)
	}
	now = now.Add(time.Minute)
	known, _ = book.Lookup("payments.prod")
	if known {
		t.Fatal("expired name still answered")
	}
}

func TestDNSRoundTrip(t *testing.T) {
	book := New()
	if err := book.Set("payments.prod", []string{"127.0.0.1"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	query := encodeQuery(t, "payments.prod", typeA)
	resp := ServeDNS(book, query)
	ips := decodeA(t, resp)
	if len(ips) != 1 || ips[0] != "127.0.0.1" {
		t.Fatalf("%v", ips)
	}
	missing := ServeDNS(book, encodeQuery(t, "missing.prod", typeA))
	if binary.BigEndian.Uint16(missing[2:4])&0x000F != rcodeNX {
		t.Fatalf("flags %04x", binary.BigEndian.Uint16(missing[2:4]))
	}
	aaaa := ServeDNS(book, encodeQuery(t, "payments.prod", typeAAAA))
	if binary.BigEndian.Uint16(aaaa[6:8]) != 0 {
		t.Fatal("AAAA should be empty")
	}
	if binary.BigEndian.Uint16(aaaa[2:4])&0x000F != rcodeOK {
		t.Fatal("AAAA should not be NXDOMAIN")
	}
}

func TestUDPResolver(t *testing.T) {
	book := New()
	if err := book.Set("payments.prod", []string{"127.0.0.1"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	server, err := Listen("127.0.0.1:0", book)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
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
		t.Fatal(err)
	}
	if len(ips) != 1 || ips[0].String() != "127.0.0.1" {
		t.Fatalf("%v", ips)
	}
}

func TestHTTPRegister(t *testing.T) {
	book := New()
	req := httptest.NewRequest(http.MethodPost, "/v1/names/payments.prod", strings.NewReader(`{"addresses":["127.0.0.1"],"ttl_seconds":30}`))
	rec := httptest.NewRecorder()
	Handler(book).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	known, answers := book.Lookup("payments.prod")
	if !known || answers[0].IP.String() != "127.0.0.1" {
		t.Fatalf("%v %v", known, answers)
	}
}

func TestRejectsBadName(t *testing.T) {
	if err := New().Set("-bad.prod", []string{"127.0.0.1"}, time.Second); err != ErrInvalid {
		t.Fatalf("%v", err)
	}
}

func TestPageAndList(t *testing.T) {
	book := New()
	if err := book.Set("payments.prod", []string{"127.0.0.1"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	h := Handler(book)
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Hello") {
		t.Fatalf("page %d", page.Code)
	}
	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/v1/names", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "payments.prod") {
		t.Fatalf("%d %s", list.Code, list.Body.String())
	}
}

func encodeQuery(t *testing.T, name string, qtype uint16) []byte {
	t.Helper()
	buf := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for _, label := range strings.Split(name, ".") {
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0, 0, 0, 0, 1)
	binary.BigEndian.PutUint16(buf[len(buf)-4:], qtype)
	binary.BigEndian.PutUint16(buf[len(buf)-2:], classIN)
	return buf
}

func decodeA(t *testing.T, msg []byte) []string {
	t.Helper()
	if len(msg) < 12 {
		t.Fatalf("short %d", len(msg))
	}
	count := int(binary.BigEndian.Uint16(msg[6:8]))
	_, end, ok := readName(msg, 12)
	if !ok {
		t.Fatal("question")
	}
	off := end + 4
	var ips []string
	for i := 0; i < count; i++ {
		if off+12 > len(msg) {
			t.Fatalf("answer %d at %d", i, off)
		}
		if msg[off]&0xC0 == 0xC0 {
			off += 2
		} else {
			_, next, ok := readName(msg, off)
			if !ok {
				t.Fatal("answer name")
			}
			off = next
		}
		rdlen := int(binary.BigEndian.Uint16(msg[off+8 : off+10]))
		off += 10
		ips = append(ips, net.IP(msg[off:off+rdlen]).String())
		off += rdlen
	}
	return ips
}
