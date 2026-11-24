package waymark

import (
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

type target struct {
	ip      net.IP
	expires time.Time
}

// Book is the set of names this server will answer.
type Book struct {
	mu    sync.Mutex
	now   func() time.Time
	names map[string][]target
}

// New starts an empty book.
func New() *Book {
	return &Book{
		now:   time.Now,
		names: map[string][]target{},
	}
}

// Set replaces the addresses for name. ttl is how long they stay answerable.
func (b *Book) Set(name string, addresses []string, ttl time.Duration) error {
	key, err := cleanName(name)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return ErrInvalid
	}
	seen := map[string]struct{}{}
	var items []target
	deadline := b.clock().Add(ttl)
	for _, address := range addresses {
		ip := net.ParseIP(address).To4()
		if ip == nil {
			return ErrInvalid
		}
		text := ip.String()
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		items = append(items, target{ip: append(net.IP(nil), ip...), expires: deadline})
	}
	if len(items) == 0 {
		return ErrInvalid
	}
	b.mu.Lock()
	b.names[key] = items
	b.mu.Unlock()
	return nil
}

// Drop removes one address. The name is removed when its last address is gone.
func (b *Book) Drop(name, address string) error {
	key, err := cleanName(name)
	if err != nil {
		return err
	}
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return ErrInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var kept []target
	for _, item := range b.names[key] {
		if item.ip.Equal(ip) {
			continue
		}
		kept = append(kept, item)
	}
	if len(kept) == 0 {
		delete(b.names, key)
		return nil
	}
	b.names[key] = kept
	return nil
}

// Lookup returns the addresses still inside their TTL, and the remaining seconds.
func (b *Book) Lookup(name string) (known bool, answers []Answer) {
	key, err := cleanName(name)
	if err != nil {
		return false, nil
	}
	now := b.clock()
	b.mu.Lock()
	defer b.mu.Unlock()
	items := b.names[key]
	if items == nil {
		return false, nil
	}
	var left []target
	for _, item := range items {
		if !now.Before(item.expires) {
			continue
		}
		sec := int(item.expires.Sub(now).Seconds())
		if sec < 1 {
			sec = 1
		}
		left = append(left, item)
		answers = append(answers, Answer{IP: append(net.IP(nil), item.ip...), TTL: uint32(sec)})
	}
	if len(left) == 0 {
		delete(b.names, key)
		return false, nil
	}
	b.names[key] = left
	return true, answers
}

func (b *Book) clock() time.Time {
	if b.now == nil {
		return time.Now()
	}
	return b.now()
}

// NameView is one name and the addresses still inside their TTL.
type NameView struct {
	Name      string        `json:"name"`
	Addresses []AddressView `json:"addresses"`
}

// AddressView is one live address.
type AddressView struct {
	Address string `json:"address"`
	TTL     uint32 `json:"ttl"`
}

// List returns every name that still has a live address.
func (b *Book) List() []NameView {
	now := b.clock()
	b.mu.Lock()
	defer b.mu.Unlock()
	names := make([]string, 0, len(b.names))
	for name := range b.names {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []NameView{}
	for _, name := range names {
		var left []target
		var addresses []AddressView
		for _, item := range b.names[name] {
			if !now.Before(item.expires) {
				continue
			}
			sec := int(item.expires.Sub(now).Seconds())
			if sec < 1 {
				sec = 1
			}
			left = append(left, item)
			addresses = append(addresses, AddressView{Address: item.ip.String(), TTL: uint32(sec)})
		}
		if len(left) == 0 {
			delete(b.names, name)
			continue
		}
		b.names[name] = left
		out = append(out, NameView{Name: name, Addresses: addresses})
	}
	return out
}

// Answer is one IPv4 address and the TTL to send in the DNS reply.
type Answer struct {
	IP  net.IP
	TTL uint32
}

func cleanName(name string) (string, error) {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if name == "" || len(name) > 253 {
		return "", ErrInvalid
	}
	labels := strings.Split(name, ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return "", ErrInvalid
		}
		for i, r := range label {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			case r == '-':
				if i == 0 || i == len(label)-1 {
					return "", ErrInvalid
				}
			default:
				return "", ErrInvalid
			}
		}
	}
	return name, nil
}
