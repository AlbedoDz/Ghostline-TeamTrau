// Package tlsfrag recognises a TLS ClientHello and splits it so DPI that
// reads the SNI from a single packet or record cannot see it.
package tlsfrag

// RecordLen returns the full length (header included) of the TLS record
// whose 5-byte header is hdr.
func RecordLen(hdr []byte) (int, bool) {
	if len(hdr) < 5 {
		return 0, false
	}
	return 5 + (int(hdr[3])<<8 | int(hdr[4])), true
}

// IsClientHello reports whether rec starts a handshake record carrying a
// ClientHello.
func IsClientHello(rec []byte) bool {
	return len(rec) >= 6 && rec[0] == 0x16 && rec[5] == 0x01
}

// SNI returns the server name of the ClientHello in rec.
func SNI(rec []byte) (string, bool) {
	s, e, ok := sniRange(rec)
	if !ok {
		return "", false
	}
	return string(rec[s:e]), true
}

// sniRange returns the byte range of the SNI host name inside b.
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
