package tlsfrag

// Method is how a ClientHello is split.
type Method string

const (
	// MethodTCP sends the bytes before the SNI, the SNI in chunks pieces and
	// the rest as separate TCP segments.
	MethodTCP Method = "tcp"
	// MethodRecord re-frames the ClientHello as chunks TLS records, the
	// first boundary falling inside the SNI, sent in one write.
	MethodRecord Method = "record"
	// MethodBoth re-frames like MethodRecord and sends each record as its
	// own TCP segment.
	MethodBoth Method = "both"
)

// Split returns the segments to write one after another. Anything that is
// not a parseable ClientHello with an SNI comes back whole.
func Split(rec []byte, m Method, chunks int) [][]byte {
	whole := [][]byte{rec}
	start, end, ok := sniRange(rec)
	if !ok || chunks < 1 {
		return whole
	}
	switch m {
	case MethodTCP:
		return splitTCP(rec, start, end, chunks)
	case MethodRecord, MethodBoth:
		recs := splitRecords(rec, start, end, chunks)
		if recs == nil {
			return whole
		}
		if m == MethodBoth {
			return recs
		}
		var one []byte
		for _, r := range recs {
			one = append(one, r...)
		}
		return [][]byte{one}
	}
	return whole
}

func splitTCP(rec []byte, start, end, chunks int) [][]byte {
	out := [][]byte{rec[:start]}
	host := rec[start:end]
	size := len(host) / chunks
	for i := 0; i < chunks; i++ {
		lo := i * size
		hi := lo + size
		if i == chunks-1 {
			hi = len(host)
		}
		out = append(out, host[lo:hi])
	}
	return append(out, rec[end:])
}

// splitRecords re-frames the handshake payload of rec into chunks records.
// It returns nil when rec is not exactly one record or chunks < 2.
func splitRecords(rec []byte, start, end, chunks int) [][]byte {
	n, ok := RecordLen(rec)
	if !ok || n != len(rec) || chunks < 2 {
		return nil
	}
	payload := rec[5:]
	cuts := []int{(start+end)/2 - 5}
	rest := len(payload) - cuts[0]
	parts := chunks - 1
	if parts > rest {
		parts = rest
	}
	for i := 1; i < parts; i++ {
		cuts = append(cuts, cuts[0]+i*rest/parts)
	}
	cuts = append(cuts, len(payload))
	out := make([][]byte, 0, len(cuts))
	lo := 0
	for _, hi := range cuts {
		r := make([]byte, 0, 5+hi-lo)
		r = append(r, rec[0], rec[1], rec[2], byte((hi-lo)>>8), byte(hi-lo))
		out = append(out, append(r, payload[lo:hi]...))
		lo = hi
	}
	return out
}
