// Package fragdoh is a DNS-over-HTTPS upstream that splits the TLS
// ClientHello into several TCP segments, so DPI that inspects the SNI in a
// single packet cannot read it.
package fragdoh

// SplitClientHello splits a TLS record carrying a ClientHello into:
// the bytes before the SNI hostname, the hostname cut into chunks pieces,
// and the rest. Joining the result always gives back record. Anything that
// is not a parseable ClientHello with an SNI is returned whole.
func SplitClientHello(record []byte, chunks int) [][]byte {
	start, end, ok := sniRange(record)
	if !ok || chunks < 1 {
		return [][]byte{record}
	}
	out := [][]byte{record[:start]}
	host := record[start:end]
	size := len(host) / chunks
	for i := 0; i < chunks; i++ {
		lo := i * size
		hi := lo + size
		if i == chunks-1 {
			hi = len(host)
		}
		out = append(out, host[lo:hi])
	}
	return append(out, record[end:])
}

// sniRange returns the byte range of the SNI host name inside record.
func sniRange(b []byte) (start, end int, ok bool) {
	// TLS record header: type(1)=0x16 handshake, version(2), length(2).
	if len(b) < 5 || b[0] != 0x16 {
		return 0, 0, false
	}
	p := 5
	// Handshake header: type(1)=0x01 ClientHello, length(3).
	if len(b) < p+4 || b[p] != 0x01 {
		return 0, 0, false
	}
	p += 4
	p += 2 + 32 // client version + random
	skip := func(lenBytes int) bool {
		if len(b) < p+lenBytes {
			return false
		}
		n := 0
		for i := 0; i < lenBytes; i++ {
			n = n<<8 | int(b[p+i])
		}
		p += lenBytes + n
		return p <= len(b)
	}
	if !skip(1) || !skip(2) || !skip(1) { // session id, cipher suites, compression
		return 0, 0, false
	}
	if len(b) < p+2 {
		return 0, 0, false
	}
	extEnd := p + 2 + (int(b[p])<<8 | int(b[p+1]))
	p += 2
	if extEnd > len(b) {
		return 0, 0, false
	}
	for p+4 <= extEnd {
		typ := int(b[p])<<8 | int(b[p+1])
		n := int(b[p+2])<<8 | int(b[p+3])
		p += 4
		if p+n > extEnd {
			return 0, 0, false
		}
		if typ == 0x0000 { // server_name
			// list length(2), name type(1)=0 host_name, name length(2), name
			q := p + 2
			if q+3 > p+n || b[q] != 0 {
				return 0, 0, false
			}
			l := int(b[q+1])<<8 | int(b[q+2])
			s := q + 3
			if s+l > p+n || l == 0 {
				return 0, 0, false
			}
			return s, s + l, true
		}
		p += n
	}
	return 0, 0, false
}
