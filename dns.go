package waymark

import (
	"encoding/binary"
	"net"
	"strings"
)

const (
	typeA    uint16 = 1
	typeAAAA uint16 = 28
	classIN  uint16 = 1

	rcodeOK      uint16 = 0
	rcodeForm    uint16 = 1
	rcodeNX      uint16 = 3
	rcodeRefused uint16 = 5
)

// ServeDNS writes a response for one query datagram.
func ServeDNS(book *Book, query []byte) []byte {
	if len(query) < 12 || query[2]&0x80 != 0 {
		return nil
	}
	id := binary.BigEndian.Uint16(query[:2])
	flags := binary.BigEndian.Uint16(query[2:4])
	qd := binary.BigEndian.Uint16(query[4:6])
	if qd != 1 {
		return headerOnly(id, flags, rcodeForm)
	}
	name, qtype, qclass, end, ok := question(query, 12)
	if !ok {
		return headerOnly(id, flags, rcodeForm)
	}
	if qclass != classIN {
		return withQuestion(id, flags, rcodeRefused, query[12:end], 0)
	}
	known, answers := book.Lookup(name)
	if !known {
		return withQuestion(id, flags, rcodeNX, query[12:end], 0)
	}
	if qtype != typeA {
		code := rcodeOK
		if qtype != typeAAAA {
			code = rcodeOK
		}
		return withQuestion(id, flags, code, query[12:end], 0)
	}
	return withAnswers(id, flags, query[12:end], answers)
}

func headerOnly(id, reqFlags, rcode uint16) []byte {
	buf := make([]byte, 12)
	putHeader(buf, id, responseFlags(reqFlags, rcode), 0, 0)
	return buf
}

func withQuestion(id, reqFlags, rcode uint16, question []byte, answers uint16) []byte {
	buf := make([]byte, 12+len(question))
	putHeader(buf, id, responseFlags(reqFlags, rcode), 1, answers)
	copy(buf[12:], question)
	return buf
}

func withAnswers(id, reqFlags uint16, question []byte, answers []Answer) []byte {
	buf := withQuestion(id, reqFlags, rcodeOK, question, uint16(len(answers)))
	for _, answer := range answers {
		buf = append(buf, 0xC0, 0x0C)
		tmp := make([]byte, 10)
		binary.BigEndian.PutUint16(tmp[0:2], typeA)
		binary.BigEndian.PutUint16(tmp[2:4], classIN)
		binary.BigEndian.PutUint32(tmp[4:8], answer.TTL)
		binary.BigEndian.PutUint16(tmp[8:10], 4)
		buf = append(buf, tmp...)
		ip := answer.IP.To4()
		buf = append(buf, ip...)
	}
	if len(buf) > 512 {
		buf = buf[:512]
		binary.BigEndian.PutUint16(buf[2:4], binary.BigEndian.Uint16(buf[2:4])|0x0200)
	}
	return buf
}

func putHeader(buf []byte, id, flags, qd, an uint16) {
	binary.BigEndian.PutUint16(buf[0:2], id)
	binary.BigEndian.PutUint16(buf[2:4], flags)
	binary.BigEndian.PutUint16(buf[4:6], qd)
	binary.BigEndian.PutUint16(buf[6:8], an)
}

func responseFlags(req, rcode uint16) uint16 {
	flags := uint16(0x8480) | rcode
	if req&0x0100 != 0 {
		flags |= 0x0100
	}
	return flags
}

func question(msg []byte, off int) (name string, qtype, qclass uint16, end int, ok bool) {
	name, end, ok = readName(msg, off)
	if !ok || end+4 > len(msg) {
		return "", 0, 0, 0, false
	}
	qtype = binary.BigEndian.Uint16(msg[end : end+2])
	qclass = binary.BigEndian.Uint16(msg[end+2 : end+4])
	return name, qtype, qclass, end + 4, true
}

func readName(msg []byte, off int) (string, int, bool) {
	var labels []string
	pos := off
	moved := false
	for hops := 0; hops < 16; hops++ {
		if pos >= len(msg) {
			return "", 0, false
		}
		n := int(msg[pos])
		if n == 0 {
			if !moved {
				off = pos + 1
			}
			return strings.Join(labels, "."), off, true
		}
		if n&0xC0 == 0xC0 {
			if pos+1 >= len(msg) {
				return "", 0, false
			}
			if !moved {
				off = pos + 2
				moved = true
			}
			pos = int(msg[pos]&0x3F)<<8 | int(msg[pos+1])
			continue
		}
		if n&0xC0 != 0 || pos+1+n > len(msg) {
			return "", 0, false
		}
		labels = append(labels, strings.ToLower(string(msg[pos+1:pos+1+n])))
		pos += 1 + n
		if !moved {
			off = pos
		}
	}
	return "", 0, false
}

// Listen serves DNS on a loopback UDP address.
func Listen(addr string, book *Book) (*Server, error) {
	if err := loopback(addr); err != nil {
		return nil, err
	}
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{book: book, conn: conn}
	go s.serve()
	return s, nil
}

// Server is one UDP listener.
type Server struct {
	book *Book
	conn net.PacketConn
}

// Addr is the bound UDP address.
func (s *Server) Addr() string { return s.conn.LocalAddr().String() }

// Close stops the listener.
func (s *Server) Close() error { return s.conn.Close() }

func (s *Server) serve() {
	buf := make([]byte, 512)
	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		resp := ServeDNS(s.book, buf[:n])
		if resp == nil {
			continue
		}
		_, _ = s.conn.WriteTo(resp, addr)
	}
}

func loopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return ErrInvalid
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return ErrInvalid
	}
	return nil
}
